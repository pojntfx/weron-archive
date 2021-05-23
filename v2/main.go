package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pion/webrtc/v3"
	api "github.com/pojntfx/weron/pkg/api/websockets/v1"
	"github.com/pojntfx/weron/v2/pkg/networking"
	"github.com/pojntfx/weron/v2/pkg/signaling"
	"github.com/pojntfx/weron/v2/pkg/utils"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

func main() {
	// Define flags
	laddr := flag.String("laddr", ":15325", "Listen address; the port can also be set using the PORT env variable.")
	nameFlag := flag.String("dev", "weron0", "Name for the network adapter")
	mtuFlag := flag.Int("mtu", 1500, "MTU for the network adapter")
	macFlag := flag.String("mac", "cc:0b:cf:23:22:0d", "MAC address for the network adapter")
	iceFlag := flag.String("ice", "stun:stun.l.google.com:19302", "Comma-seperated list of STUN servers to use")
	communityFlag := flag.String("community", "cluster1", "Community to join")
	raddrFlag := flag.String("raddr", "wss://weron.herokuapp.com", "Address of the signaler to use")
	keyFlag := flag.String("key", "abcdefghijklmopq", "Key for the community (16, 24 or 32 characters)")
	verboseFlag := flag.Bool("verbose", false, "Enable verbose logging")
	signalFlag := flag.Bool("signal", false, "Enable signaling server subsystem")
	agentFlag := flag.Bool("agent", false, "Enable agent subsystem")

	// Parse flags
	flag.Parse()

	// Exit if no subsystems where enabled
	if !*signalFlag && !*agentFlag {
		log.Fatal("could not start: No subsystems enabled. Please pass either --signal (to run the signaling server), --agent (to run the agent) or both to start.")
	}

	// Handle cross-subsystem concerns
	fatal := make(chan error)

	// Agent subsystem
	if *agentFlag {
		go func() {
			for {
				breaker := make(chan error)

				go func() {
					// Parse subsystem-specific flags
					mac, err := net.ParseMAC(*macFlag)
					if err != nil {
						fatal <- err

						return
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
						breaker <- fmt.Errorf("could not dial WebSocket: %v", err)

						return
					}
					defer func() {
						_ = conn.Close(websocket.StatusInternalError, "closing") // Ignored as it can be a no-op
					}()

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
					defer func() {
						_ = adapter.Close() // Ignored as it can be a no-op
					}()

					peers := networking.NewPeerManager(
						stunServers,
						func(mac string, i webrtc.ICECandidate) {
							candidateChan <- struct {
								mac string
								i   webrtc.ICECandidate
							}{mac, i}
						},
						func(mac string, frame []byte) {
							frame, err = utils.Decrypt(frame, key)
							if err != nil {
								breaker <- err

								return
							}

							if err := adapter.Write(frame); err != nil {
								breaker <- err

								return
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
					defer func() {
						_ = peers.Close() // Ignored as it can be a no-op
					}()

					signaler := signaling.NewSignalingClient(
						conn,
						mac.String(),
						*communityFlag,
						func(mac string) {
							if err := peers.HandleIntroduction(mac); err != nil {
								breaker <- err

								return
							}
						},
						func(mac string, o webrtc.SessionDescription) {
							if err := peers.HandleOffer(mac, o); err != nil {
								breaker <- err

								return
							}
						},
						func(mac string, i webrtc.ICECandidateInit) {
							if err := peers.HandleCandidate(mac, i); err != nil {
								breaker <- err

								return
							}
						},
						func(mac string, o webrtc.SessionDescription) {
							if err := peers.HandleAnswer(mac, o); err != nil {
								breaker <- err

								return
							}
						},
						func(mac string, blocked bool) {
							if blocked {
								log.Printf("blocked connection to peer %v due to wrong key", mac)
							}

							// Ignore as this can be a no-op
							_ = peers.HandleResignation(mac)
						},
						func(data []byte) ([]byte, error) {
							return utils.Encrypt(data, key)
						},
						func(data []byte) ([]byte, error) {
							return utils.Decrypt(data, key)
						},
					)
					defer func() {
						_ = signaler.Close() // Ignored as it can be a no-op
					}()

					// Start
					if err := adapter.Open(); err != nil {
						breaker <- err

						return
					}

					go func() {
						for {
							frame, err := adapter.Read()
							if err != nil {
								breaker <- err

								return
							}

							dst, err := utils.GetDestination(frame)
							if err != nil {
								log.Println("could not get destination from frame, continuing:", err)

								continue
							}

							frame, err = utils.Encrypt(frame, key)
							if err != nil {
								breaker <- err

								return
							}

							if err := peers.Write(dst.String(), frame); err != nil {
								if *verboseFlag {
									log.Println("could not write to peer, continuing:", err)
								}

								continue
							}
						}
					}()

					go func() {
						for candidate := range candidateChan {
							if err := signaler.SignalCandidate(candidate.mac, candidate.i); err != nil {
								breaker <- err

								return
							}
						}
					}()

					go func() {
						for offer := range offerChan {
							if err := signaler.SignalOffer(offer.mac, offer.o); err != nil {
								breaker <- err

								return
							}
						}
					}()

					go func() {
						for answer := range answerChan {
							if err := signaler.SignalAnswer(answer.mac, answer.o); err != nil {
								breaker <- err

								return
							}
						}
					}()

					log.Printf("agent connected to signaler %v", *raddrFlag)

					breaker <- signaler.Run()
				}()

				err := <-breaker

				log.Println("agent crashed, restarting in 1s:", err)

				time.Sleep(time.Second)
			}
		}()
	}

	// Signaling server subsystem
	if *signalFlag {
		go func() {
			for {
				breaker := make(chan error)

				go func() {
					// Parse subsystem-specific flags
					addr, err := net.ResolveTCPAddr("tcp", *laddr)
					if err != nil {
						fatal <- fmt.Errorf("could not resolve address: %v", err)

						return
					}

					// Parse PORT env variable for Heroku compatibility
					if portEnv := os.Getenv("PORT"); portEnv != "" {
						port, err := strconv.Atoi(portEnv)
						if err != nil {
							fatal <- fmt.Errorf("could not parse port: %v", port)

							return
						}

						addr.Port = port
					}

					// Create core
					communities := networking.NewCommunitiesManager(
						func(mac string, conn *websocket.Conn) error {
							return wsjson.Write(context.Background(), conn, api.NewIntroduction(mac))
						},
						func(exchange api.Exchange, conn *websocket.Conn) error {
							return wsjson.Write(context.Background(), conn, exchange)
						},
						func(mac string, conn *websocket.Conn) error {
							return wsjson.Write(context.Background(), conn, api.NewResignation(mac))
						},
					)
					defer func() {
						_ = communities.Close() // Ignored as it can be a no-op
					}()

					signaler := signaling.NewSignalingServer(
						func(community string, mac string, conn *websocket.Conn) error {
							return communities.HandleApplication(community, mac, conn)
						},
						func(conn *websocket.Conn) error {
							return wsjson.Write(context.Background(), conn, api.NewRejection())
						},
						func(conn *websocket.Conn) error {
							return wsjson.Write(context.Background(), conn, api.NewAcceptance())
						},
						func(community, mac string, err error) error {
							return communities.HandleExited(community, mac, err)
						},
						func(community, mac string) error {
							return communities.HandleReady(community, mac)
						},
						func(community, mac string, exchange api.Exchange) error {
							return communities.HandleExchange(community, mac, exchange)
						},
					)
					defer func() {
						_ = signaler.Close() // Ignored as it can be a no-op
					}()

					// Start
					log.Printf("signaling server listening on %v", addr.String())

					breaker <- http.ListenAndServe(addr.String(), http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
						conn, err := websocket.Accept(rw, r, nil)
						if err != nil {
							log.Println("could not accept on WebSocket:", err)

							return
						}

						log.Println("client connected")

						go func() {
							if err := signaler.HandleConn(conn); err != nil {
								log.Println("client disconnected:", err)

								return
							}
						}()
					}))
				}()

				err := <-breaker

				log.Println("signaling server crashed, restarting in 1s:", err)

				time.Sleep(time.Second)
			}
		}()
	}

	err := <-fatal

	log.Fatal(err)
}