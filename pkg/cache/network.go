package cache

import (
	"errors"
	"sync"

	"nhooyr.io/websocket"
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
