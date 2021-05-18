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

	connections map[string]connection
	mac         string
	onReceive   func(mac string, frame []byte)
}

type connection struct {
	connection  *webrtc.PeerConnection
	dataChannel *webrtc.DataChannel
}

func NewAgent(mac string, onReceive func(mac string, frame []byte)) *Agent {
	return &Agent{
		connections: map[string]connection{},
		mac:         mac,
		onReceive:   onReceive,
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

	a.connections[mac] = connection{
		connection: peerConnection,
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
		a.lock.Lock()
		defer a.lock.Unlock()

		log.Printf("data channel for mac %v opened", mac)

		conn := a.connections[mac]
		conn.dataChannel = dataChannel

		a.connections[mac] = conn
	})

	dataChannel.OnClose(func() {
		_ = a.HandleResignation(mac) // Close connection; ignore errors as this might be a no-op
	})

	dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
		log.Printf("received from data channel: %v", string(msg.Data))

		a.onReceive(mac, msg.Data)
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
			dataChannel.OnOpen(func() {
				a.lock.Lock()
				defer a.lock.Unlock()

				log.Printf("data channel for mac %v opened", mac)

				conn := a.connections[mac]
				conn.dataChannel = dataChannel

				a.connections[mac] = conn
			})

			dataChannel.OnClose(func() {
				_ = a.HandleResignation(mac) // Close connection; ignore errors as this might be a no-op
			})

			dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
				log.Printf("received from data channel: %v", string(msg.Data))

				a.onReceive(mac, msg.Data)
			})
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

	peerConnection, err := a.ensureConnection(mac)
	if err != nil {
		return err
	}

	if err := peerConnection.connection.AddICECandidate(webrtc.ICECandidateInit{Candidate: string(data)}); err != nil {
		return err
	}

	return nil
}

func (a *Agent) HandleAnswer(mac string, data []byte, c *websocket.Conn) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	peerConnection, err := a.ensureConnection(mac)
	if err != nil {
		return err
	}

	var answer webrtc.SessionDescription
	if err := json.Unmarshal(data, &answer); err != nil {
		return err
	}

	return peerConnection.connection.SetRemoteDescription(answer)
}

func (a *Agent) HandleResignation(mac string) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	log.Println("closing connection to " + mac)

	peerConnection, err := a.ensureConnection(mac)
	if err != nil {
		return err
	}

	delete(a.connections, mac)

	return peerConnection.connection.Close()
}

func (a *Agent) WriteToDataChannel(mac string, frame []byte) error {
	conn, err := a.ensureConnection(mac)
	if err != nil {
		return err
	}

	if conn.dataChannel == nil {
		return errors.New("could not access data channel: connection for data channel exists, but no data channel")
	}

	log.Printf("sending to data channel for mac %v", mac)

	if err := conn.dataChannel.Send(frame); err != nil {
		_ = a.HandleResignation(mac) // Close connection; ignore errors as this might be a no-op

		return nil
	}

	return nil
}

func (a *Agent) ensureConnection(mac string) (connection, error) {
	// Check if connection exists
	conn, ok := a.connections[mac]
	if !ok {
		return connection{}, errors.New("could not access connection: connection with MAC address doesn't exist")
	}

	return conn, nil
}
