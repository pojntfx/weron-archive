package networking

import (
	"errors"
	"sync"

	"nhooyr.io/websocket"

	api "github.com/pojntfx/weron/pkg/api/websockets/v1"
)

type CommunitiesManager struct {
	communities map[string]map[string]*websocket.Conn

	lock sync.Mutex

	onIntroduction func(mac string, conn *websocket.Conn) error
	onExchange     func(exchange api.Exchange, conn *websocket.Conn) error
	onResignation  func(mac string, conn *websocket.Conn) error
}

func NewCommunitiesManager(
	onIntroduction func(mac string, conn *websocket.Conn) error,
	onExchange func(exchange api.Exchange, conn *websocket.Conn) error,
	onResignation func(mac string, conn *websocket.Conn) error,
) *CommunitiesManager {
	return &CommunitiesManager{
		communities: map[string]map[string]*websocket.Conn{},

		onIntroduction: onIntroduction,
		onExchange:     onExchange,
		onResignation:  onResignation,
	}
}

func (m *CommunitiesManager) HandleApplication(community string, mac string, conn *websocket.Conn) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	// Create or copy community
	newCommunity := make(map[string]*websocket.Conn)
	if candidate, ok := m.communities[community]; ok {
		newCommunity = candidate
	}

	// Abort if MAC address is already in community
	if _, ok := newCommunity[mac]; ok {
		return errors.New("could not add MAC address to community: MAC address is already in community")
	}
	newCommunity[mac] = conn

	// Apply changes
	m.communities[community] = newCommunity

	return nil
}

func (m *CommunitiesManager) HandleReady(community string, mac string) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	// Get matching community for ready
	comm, err := m.getCommunity(community, mac)
	if err != nil {
		return err
	}

	for candidate, conn := range comm {
		// Ignore the node which sent the ready message
		if candidate == mac {
			continue
		}

		// Send introduction
		if err := m.onIntroduction(mac, conn); err != nil {
			return err
		}
	}

	return nil
}

func (m *CommunitiesManager) HandleExchange(community string, mac string, exchange api.Exchange) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	// Get matching community for exchange
	comm, err := m.getCommunity(community, mac)
	if err != nil {
		return err
	}

	// Get the connection for the destination
	destination, ok := comm[exchange.Mac]
	if !ok {
		return errors.New("could not use MAC address: connection with dst MAC address doesn't exist")
	}

	// Swap source and destination MACs in exchange
	exchange.Mac = mac

	// Send exchange
	if err := m.onExchange(exchange, destination); err != nil {
		return err
	}

	return nil
}

func (m *CommunitiesManager) HandleExited(community string, mac string, err error) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	// Get matching community for exited node; use communityErr so that err doesn't get overwritten
	comm, communityErr := m.getCommunity(community, mac)
	if communityErr != nil {
		return communityErr
	}

	for candidate, conn := range comm {
		// Ignore the node which sent the exited message
		if candidate == mac {
			continue
		}

		// Send resignation
		if err := m.onResignation(mac, conn); err != nil {
			return err
		}
	}

	// Get a copy of the connection
	conn := comm[mac]

	// Delete the connection from the community
	delete(comm, mac)

	// Delete the community if it is now empty
	if len(comm) == 0 {
		delete(m.communities, community)
	}

	// Close the connection (irregular)
	if err != nil {
		msg := err.Error()
		if len(msg) >= 123 {
			msg = msg[:122] // string max is 123 in WebSockets
		}

		return conn.Close(websocket.StatusProtocolError, msg)
	}

	// Close the connection (regular)
	return conn.Close(websocket.StatusNormalClosure, "resignation")
}

func (m *CommunitiesManager) getCommunity(community string, mac string) (map[string]*websocket.Conn, error) {
	// Check if community exists
	comm, ok := m.communities[community]
	if !ok {
		return nil, errors.New("could not access community: community doesn't exist")
	}

	// Check if src mac exists
	if _, ok := comm[mac]; !ok {
		return nil, errors.New("could not use MAC address: connection with MAC address doesn't exist")
	}

	return comm, nil
}
