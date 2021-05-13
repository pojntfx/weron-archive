package main

import (
	"encoding/json"
	"flag"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

type Candidate struct {
	Address string `json:"address"`
	Payload []byte `json:"payload"`
}

func main() {
	raddr := flag.String("raddr", "ws://localhost:15356", "Remote address")

	flag.Parse()

	remote, _, err := websocket.DefaultDialer.Dial(*raddr, nil)
	if err != nil {
		panic(err)
	}
	defer remote.Close()

	peers := map[string]*webrtc.PeerConnection{}

	for {
		_, msg, err := remote.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNoStatusReceived) {
				break
			}

			panic(err)
		}

		var candidate Candidate
		if err := json.Unmarshal(msg, &candidate); err != nil {
			panic(err)
		}

		if peer, ok := peers[candidate.Address]; ok {
			iceCandidate := webrtc.ICECandidateInit{}
			if err := json.Unmarshal(candidate.Payload, &iceCandidate); err != nil {
				panic(err)
			}

			if err := peer.AddICECandidate(iceCandidate); err != nil {
				panic(err)
			}

			continue
		}

		conn, err := webrtc.NewPeerConnection(webrtc.Configuration{
			ICEServers: []webrtc.ICEServer{
				{
					URLs: []string{"stun:stun.l.google.com:19302"},
				},
			},
		})
		if err != nil {
			panic(err)
		}

		conn.OnICECandidate(func(iceCandidate *webrtc.ICECandidate) {
			msg, err := json.Marshal(iceCandidate)
			if err != nil {
				panic(err)
			}

			if err := remote.WriteMessage(websocket.TextMessage, msg); err != nil {
				if err.Error() == websocket.ErrCloseSent.Error() || strings.HasSuffix(err.Error(), "write: broken pipe") {
					return
				}

				panic(err)
			}
		})
	}
}
