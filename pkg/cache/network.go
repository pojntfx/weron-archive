package cache

import (
	"context"
	"errors"
	"sync"

	api "github.com/pojntfx/weron/pkg/api/websockets/v1"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

type Network struct {
	lock sync.Mutex

	communities map[string]connections
}

type connections map[string]*websocket.Conn

func NewNetwork() *Network {
	return &Network{
		communities: map[string]connections{},
	}
}

func (n *Network) HandleApplication(community string, mac string, conn *websocket.Conn) error {
	n.lock.Lock()
	defer n.lock.Unlock()

	// Initialize or copy community
	comm := make(connections)
	if candidate, ok := n.communities[community]; ok {
		comm = candidate
	}

	// Prevent duplicate MAC addresses
	if _, ok := comm[mac]; ok {
		return errors.New("could not add MAC address to community: MAC address is already in community")
	}
	comm[mac] = conn

	// Apply changes
	n.communities[community] = comm

	return nil
}

func (n *Network) HandleReady(community string, srcMAC string) error {
	n.lock.Lock()
	defer n.lock.Unlock()

	// Check if community exists
	comm, ok := n.communities[community]
	if !ok {
		return errors.New("could not access community: community doesn't exist")
	}

	// Check if src mac exists
	if _, ok := comm[srcMAC]; !ok {
		return errors.New("could not use MAC address: connection with MAC address doesn't exist")
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

func (n *Network) HandleExchange(community string, srcMAC string, exchange api.Exchange) error {
	n.lock.Lock()
	defer n.lock.Unlock()

	// Check if community exists
	comm, ok := n.communities[community]
	if !ok {
		return errors.New("could not access community: community doesn't exist")
	}

	// Check if src mac exists
	if _, ok := comm[srcMAC]; !ok {
		return errors.New("could not use MAC address: connection with src MAC address doesn't exist")
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
