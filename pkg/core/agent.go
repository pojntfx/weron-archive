package core

import (
	"context"
	"encoding/json"
	"log"
	"sync"

	"github.com/pion/webrtc/v3"
	api "github.com/pojntfx/weron/pkg/api/websockets/v1"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

type Agent struct {
	lock sync.Mutex

	connections map[string]*webrtc.PeerConnection
}

func NewAgent() *Agent {
	return &Agent{
		connections: map[string]*webrtc.PeerConnection{},
	}
}

func (a *Agent) HandleIntroduction(mac string, c *websocket.Conn) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	// TODO: Decompose ICE servers
	peerConnection, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	})
	if err != nil {
		return err
	}

	peerConnection.OnICECandidate(func(i *webrtc.ICECandidate) {
		data, err := json.Marshal(i)
		if err != nil {
			log.Println("could not marshal ICE candidate:", err)

			return
		}

		// Send candidate
		log.Printf("sending candidate %v", i)

		if err := wsjson.Write(context.Background(), c, api.NewCandidate(mac, data)); err != nil {
			log.Println("could not send candidate:", err)

			return
		}
	})

	offer, err := peerConnection.CreateOffer(&webrtc.OfferOptions{})
	if err != nil {
		return err
	}
	peerConnection.SetLocalDescription(offer)

	data, err := json.Marshal(offer)
	if err != nil {
		return err
	}

	// Send offer
	log.Printf("sending offer %v", offer)

	if err := wsjson.Write(context.Background(), c, api.NewOffer(mac, data)); err != nil {
		return err
	}

	a.connections[mac] = peerConnection

	return nil
}
