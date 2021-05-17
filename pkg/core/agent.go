package core

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

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

	connections map[string]connection
	mac         string
}

type connection struct {
	connection  *webrtc.PeerConnection
	dataChannel *webrtc.DataChannel
}

func NewAgent(mac string) *Agent {
	return &Agent{
		connections: map[string]connection{},
		mac:         mac,
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
		// Send candidate
		if i != nil {
			log.Printf("sending candidate %v", i)

			if err := wsjson.Write(context.Background(), c, api.NewCandidate(mac, []byte(i.ToJSON().Candidate))); err != nil {
				log.Println("could not send candidate:", err)

				return
			}
		}
	})

	// Create a datachannel
	dataChannel, err := peerConnection.CreateDataChannel(dataChannelName, nil)
	if err != nil {
		panic(err)
	}

	dataChannel.OnOpen(func() {
		for {
			log.Printf("sending to data channel for mac %v", mac)

			dataChannel.Send([]byte("Hello, world from " + a.mac + "!"))

			time.Sleep(time.Second)
		}
	})

	dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
		log.Printf("received from data channel: %v", string(msg.Data))
	})

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

	a.connections[mac] = connection{
		connection:  peerConnection,
		dataChannel: dataChannel,
	}

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

		a.connections[mac] = connection{
			connection: newPeerConnection,
		}

		newPeerConnection.OnICECandidate(func(i *webrtc.ICECandidate) {
			// Send candidate
			if i != nil {
				log.Printf("sending candidate %v", i)

				if err := wsjson.Write(context.Background(), c, api.NewCandidate(mac, []byte(i.ToJSON().Candidate))); err != nil {
					log.Println("could not send candidate:", err)

					return
				}
			}
		})

		newPeerConnection.OnDataChannel(func(dataChannel *webrtc.DataChannel) {
			a.lock.Lock()
			defer a.lock.Unlock()

			dataChannel.OnOpen(func() {
				for {
					log.Printf("sending to data channel for mac %v", mac)

					dataChannel.Send([]byte("Hello, world from " + a.mac + "!"))

					time.Sleep(time.Second)
				}
			})

			dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
				log.Printf("received from data channel: %v", string(msg.Data))
			})

			conn := a.connections[mac]
			conn.dataChannel = dataChannel

			a.connections[mac] = conn
		})

		peerConnection = a.connections[mac]
	}

	var offer webrtc.SessionDescription
	if err := json.Unmarshal(data, &offer); err != nil {
		return err
	}

	if err := peerConnection.connection.SetRemoteDescription(offer); err != nil {
		return err
	}

	answer, err := peerConnection.connection.CreateAnswer(nil)
	if err != nil {
		return err
	}
	peerConnection.connection.SetLocalDescription(answer)

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

	if err := peerConnection.connection.AddICECandidate(webrtc.ICECandidateInit{Candidate: string(data)}); err != nil {
		return err
	}

	return nil
}

func (a *Agent) HandleAnswer(mac string, data []byte, c *websocket.Conn) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	peerConnection, ok := a.connections[mac]
	if !ok {
		return errors.New("could not access peer connection: peer connection doesn't exist")
	}

	var answer webrtc.SessionDescription
	if err := json.Unmarshal(data, &answer); err != nil {
		return err
	}

	return peerConnection.connection.SetRemoteDescription(answer)
}
