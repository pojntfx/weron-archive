package core

import (
	"context"
	"errors"
	"sync"

	api "github.com/pojntfx/weron/pkg/api/websockets/v1"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

type Signaler struct {
	lock sync.Mutex

	communities map[string]connections
}

type connections map[string]*websocket.Conn

func NewSignaler() *Signaler {
	return &Signaler{
		communities: map[string]connections{},
	}
}

func (s *Signaler) HandleApplication(community string, mac string, conn *websocket.Conn) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	// Initialize or copy community
	comm := make(connections)
	if candidate, ok := s.communities[community]; ok {
		comm = candidate
	}

	// Prevent duplicate MAC addresses
	if _, ok := comm[mac]; ok {
		return errors.New("could not add MAC address to community: MAC address is already in community")
	}
	comm[mac] = conn

	// Apply changes
	s.communities[community] = comm

	return nil
}

func (s *Signaler) HandleReady(community string, srcMAC string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	// Check if community and MAC exist
	comm, err := s.ensureCommunityAndMAC(community, srcMAC)
	if err != nil {
		return err
	}

	for candidate, conn := range comm {
		// Ignore the node which sent the ready message
		if candidate == srcMAC {
			continue
		}

		// Send introduction
		if err := wsjson.Write(context.Background(), conn, api.NewIntroduction(srcMAC)); err != nil {
			return err
		}
	}

	return nil
}

func (s *Signaler) HandleExchange(community string, srcMAC string, exchange api.Exchange) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	// Check if community and MAC exist
	comm, err := s.ensureCommunityAndMAC(community, srcMAC)
	if err != nil {
		return err
	}

	// Get dst connection
	dst, ok := comm[exchange.Mac]
	if !ok {
		return errors.New("could not use MAC address: connection with dst MAC address doesn't exist")
	}

	// Swap src and dst MACs in exchange
	exchange.Mac = srcMAC

	// Send exchange
	return wsjson.Write(context.Background(), dst, exchange)
}

func (s *Signaler) HandleExited(community string, srcMAC string, msg string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	// Check if community and MAC exist
	comm, err := s.ensureCommunityAndMAC(community, srcMAC)
	if err != nil {
		return err
	}

	for candidate, conn := range comm {
		// Ignore the node which sent the exited message
		if candidate == srcMAC {
			continue
		}

		// Send resignation
		if err := wsjson.Write(context.Background(), conn, api.NewResignation(srcMAC)); err != nil {
			return err
		}
	}

	// Get a copy of the connection
	conn := comm[srcMAC]

	// Delete the connection from the community
	delete(comm, srcMAC)

	// Delete the community if it is now empty
	if len(comm) == 0 {
		delete(s.communities, community)
	}

	// Close the connection (regular)
	if msg == "" {
		msg = "resignation"

		if err := conn.Close(websocket.StatusNormalClosure, msg); err != nil {
			return err
		}
	}

	// Close the connection (non-regular)
	if len(msg) >= 123 {
		msg = msg[:122] // string max is 123
	}
	if err := conn.Close(websocket.StatusProtocolError, msg); err != nil {
		return err
	}

	return nil
}

func (s *Signaler) ensureCommunityAndMAC(community, mac string) (connections, error) {
	// Check if community exists
	comm, ok := s.communities[community]
	if !ok {
		return nil, errors.New("could not access community: community doesn't exist")
	}

	// Check if src mac exists
	if _, ok := comm[mac]; !ok {
		return nil, errors.New("could not use MAC address: connection with MAC address doesn't exist")
	}

	return comm, nil
}
