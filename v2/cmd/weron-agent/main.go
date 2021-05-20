package main

import (
	"context"
	"flag"
	"log"
	"net"
	"strings"

	"github.com/pion/webrtc/v3"
	"github.com/pojntfx/weron/v2/pkg/networking"
	"github.com/pojntfx/weron/v2/pkg/signaling"
	"github.com/pojntfx/weron/v2/pkg/utils"
	"nhooyr.io/websocket"
)

func main() {
	// Define flags
	nameFlag := flag.String("dev", "weron0", "Name for the network adapter")
	mtuFlag := flag.Int("mtu", 1500, "MTU for the network adapter")
	macFlag := flag.String("mac", "cc:0b:cf:23:22:0d", "MAC address for the network adapter")
	iceFlag := flag.String("ice", "stun:stun.l.google.com:19302", "Comma-seperated list of STUN servers to use")
	communityFlag := flag.String("community", "cluster1", "Community to join")
	raddrFlag := flag.String("raddr", "wss://weron.herokuapp.com", "Address of the signaler to use")
	keyFlag := flag.String("key", "abcdefghijklmopq", "Key for the community (16, 24 or 32 characters); only relevant if AES encryption is enabled")
	encryptFlag := flag.Bool("encrypt", true, "In addition to WebRTC's built-in wire security, also encrypt frames using AES")

	// Parse flags
	flag.Parse()

	mac, err := net.ParseMAC(*macFlag)
	if err != nil {
		log.Fatal(err)
	}

	key := []byte(*keyFlag)

	stunServers := []webrtc.ICEServer{}
	for _, stunServer := range strings.Split(*iceFlag, ",") {
		stunServers = append(stunServers, webrtc.ICEServer{
			URLs: []string{stunServer},
		})
	}

	conn, _, err := websocket.Dial(context.Background(), *raddrFlag, nil)
	if err != nil {
		log.Fatal("could not dial WebSocket:", err)
	}
	defer conn.Close(websocket.StatusInternalError, "closing")

	// Handle circular dependencies
	candidateChan := make(chan struct {
		mac string
		i   webrtc.ICECandidate
	})

	offerChan := make(chan struct {
		mac string
		o   webrtc.SessionDescription
	})

	answerChan := make(chan struct {
		mac string
		o   webrtc.SessionDescription
	})

	// Create core
	adapter := networking.NewNetworkAdapter(*nameFlag, *mtuFlag, mac)
	peers := networking.NewPeerManager(
		stunServers,
		func(mac string, i webrtc.ICECandidate) {
			candidateChan <- struct {
				mac string
				i   webrtc.ICECandidate
			}{mac, i}
		},
		func(mac string, frame []byte) {
			if *encryptFlag {
				frame, err = utils.Decrypt(frame, key)
				if err != nil {
					log.Fatal(err)
				}
			}

			if err := adapter.Write(frame); err != nil {
				log.Fatal(err)
			}
		},
		func(mac string, o webrtc.SessionDescription) {
			offerChan <- struct {
				mac string
				o   webrtc.SessionDescription
			}{mac, o}
		},
		func(mac string, o webrtc.SessionDescription) {
			answerChan <- struct {
				mac string
				o   webrtc.SessionDescription
			}{mac, o}
		},
		func(mac string) {
			log.Println("connected to peer", mac)
		},
		func(mac string) {
			log.Println("disconnected from peer", mac)
		},
	)
	signaler := signaling.NewSignalingClient(
		conn,
		mac.String(),
		*communityFlag,
		func(mac string) {
			if err := peers.HandleIntroduction(mac); err != nil {
				log.Fatal(err)
			}
		},
		func(mac string, o webrtc.SessionDescription) {
			if err := peers.HandleOffer(mac, o); err != nil {
				log.Fatal(err)
			}
		},
		func(mac string, i webrtc.ICECandidateInit) {
			if err := peers.HandleCandidate(mac, i); err != nil {
				log.Fatal(err)
			}
		},
		func(mac string, o webrtc.SessionDescription) {
			if err := peers.HandleAnswer(mac, o); err != nil {
				log.Fatal(err)
			}
		},
		func(mac string) {
			if err := peers.HandleResignation(mac); err != nil {
				log.Fatal(err)
			}
		},
	)

	// Start
	if err := adapter.Open(); err != nil {
		log.Fatal(err)
	}

	go func() {
		for {
			frame, err := adapter.Read()
			if err != nil {
				log.Fatal(err)
			}

			dst, err := utils.GetDestination(frame)
			if err != nil {
				log.Println("could not get destination from frame, continuing:", err)

				continue
			}

			if *encryptFlag {
				frame, err = utils.Encrypt(frame, key)
				if err != nil {
					log.Fatal(err)
				}
			}

			if err := peers.Write(dst.String(), frame); err != nil {
				log.Println("could not write to peer, continuing:", err)

				continue
			}
		}
	}()

	go func() {
		for candidate := range candidateChan {
			if err := signaler.SignalCandidate(candidate.mac, candidate.i); err != nil {
				log.Fatal(err)
			}
		}
	}()

	go func() {
		for offer := range offerChan {
			if err := signaler.SignalOffer(offer.mac, offer.o); err != nil {
				log.Fatal(err)
			}
		}
	}()

	go func() {
		for answer := range answerChan {
			if err := signaler.SignalAnswer(answer.mac, answer.o); err != nil {
				log.Fatal(err)
			}
		}
	}()

	log.Printf("connected to signaler %v", *raddrFlag)

	log.Fatal(signaler.Run())
}
