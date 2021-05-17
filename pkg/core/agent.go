package core

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync"

	"github.com/pion/webrtc/v3"
	api "github.com/pojntfx/weron/pkg/api/websockets/v1"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

const (
	dataChannelName = "data"
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

	// Create a datachannel
	if _, err = peerConnection.CreateDataChannel(dataChannelName, nil); err != nil {
		panic(err)
	}

	offer, err := peerConnection.CreateOffer(nil)
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

func (a *Agent) HandleOffer(mac string, data []byte, c *websocket.Conn) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	peerConnection, ok := a.connections[mac]
	if !ok {
		// TODO: Decompose ICE servers
		newPeerConnection, err := webrtc.NewPeerConnection(webrtc.Configuration{
			ICEServers: []webrtc.ICEServer{
				{
					URLs: []string{"stun:stun.l.google.com:19302"},
				},
			},
		})
		if err != nil {
			return err
		}

		newPeerConnection.OnICECandidate(func(i *webrtc.ICECandidate) {
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

		a.connections[mac] = newPeerConnection

		peerConnection = newPeerConnection

	}

	var offer webrtc.SessionDescription
	if err := json.Unmarshal(data, &offer); err != nil {
		return err
	}

	if err := peerConnection.SetRemoteDescription(offer); err != nil {
		return err
	}

	answer, err := peerConnection.CreateAnswer(&webrtc.AnswerOptions{})
	if err != nil {
		return err
	}
	peerConnection.SetLocalDescription(answer)

	outData, err := json.Marshal(answer)
	if err != nil {
		return err
	}

	// Send answer
	log.Printf("sending answer %v", answer)

	if err := wsjson.Write(context.Background(), c, api.NewAnswer(mac, outData)); err != nil {
		return err
	}

	a.connections[mac] = peerConnection

	return nil
}

func (a *Agent) HandleCandidate(mac string, data []byte, c *websocket.Conn) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	peerConnection, ok := a.connections[mac]
	if !ok {
		return errors.New("could not access peer connection: peer connection doesn't exist")
	}

	var candidate webrtc.ICECandidateInit
	if err := json.Unmarshal(data, &candidate); err != nil {
		return err
	}

	if err := peerConnection.AddICECandidate(candidate); err != nil {
		return err
	}

	return nil
}
