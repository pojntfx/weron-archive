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

func (n *Network) HandleReady(community string, mac string) error {
	n.lock.Lock()
	defer n.lock.Unlock()

	comm, ok := n.communities[community]
	if !ok {
		return errors.New("could not access community: community doesn't exist")
	}

	for candidate, conn := range comm {
		// Ignore the node which sent the ready message
		if candidate == mac {
			continue
		}

		// Send introduction
		if err := wsjson.Write(context.Background(), conn, api.NewIntroduction(mac)); err != nil {
			return err
		}
	}

	return nil
}
