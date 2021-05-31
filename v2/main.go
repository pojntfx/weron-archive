package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
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
	// Get default working dir
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal("could not get home directory", err)
	}
	prefix := filepath.Join(home, ".local", "share", "weron", "etc", "lib", "weron")

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
	timeoutFlag := flag.Int("timeout", 5, "Maximum time in seconds to wait for the signaling server to respond before reconnecting")
	cmdFlag := flag.String("cmd", "", "Command to run after the interface is up, i.e. 'avahi-autoipd weron0' for ipv4ll")
	tlsKeyFlag := flag.String("tlsKey", filepath.Join(prefix, "key.pem"), "TLS key")
	tlsCertFlag := flag.String("tlsCert", filepath.Join(prefix, "cert.pem"), "TLS certificate")
	tlsFingerprintFlag := flag.String("tlsFingerprint", "", "Instead of using a CA, validate the signaling server's TLS cert using it's fingerprint")
	tlsInsecureSkipVerifyFlag := flag.Bool("tlsInsecureSkipVerify", false, "Skip TLS certificate validation (insecure)")
	tlsEnabled := flag.Bool("tlsEnabled", true, "Serve signaling server using TLS")
	knownHostsFile := flag.String("knownHostsFile", filepath.Join(prefix, "known_hosts"), "Known hosts file")

	// Parse flags
	flag.Parse()

	// Exit if no subsystems where enabled
	if !*signalFlag && !*agentFlag {
		log.Fatal("could not start: No subsystems enabled. Please pass either --signal (to run the signaling server), --agent (to run the agent) or both to start.")
	}

	// Handle cross-subsystem concerns
	fatal := make(chan error)
	done := make(chan struct{})

	// Agent subsystem
	if *agentFlag {
		go func() {
			retryWithFingerprint := false

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

					// Create the config file if it does not exist
					if err := utils.CreateFileAndLeadingDirectories(*knownHostsFile, ""); err != nil {
						fatal <- err

						return
					}

					// Interactively verify TLS certificate if fingerprint is given
					client := http.DefaultClient
					if *tlsFingerprintFlag != "" || retryWithFingerprint || *tlsInsecureSkipVerifyFlag {
						customTransport := http.DefaultTransport.(*http.Transport).Clone()

						customTransport.TLSClientConfig = utils.GetInteractiveTLSConfig(
							*tlsInsecureSkipVerifyFlag,
							*tlsFingerprintFlag,
							*knownHostsFile,
							*raddrFlag,
							func(e error) {
								fatal <- e
							})

						client = &http.Client{Transport: customTransport}
					}

					conn, _, err := websocket.Dial(context.Background(), *raddrFlag, &websocket.DialOptions{HTTPClient: client})
					if err != nil {
						if strings.Contains(err.Error(), "x509:") {
							retryWithFingerprint = true

							breaker <- fmt.Errorf("")
						}

						breaker <- fmt.Errorf("could not dial WebSocket: %v", err)

						return
					}

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
						time.Duration(*timeoutFlag)*time.Second,
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

					cmd := exec.Command("/bin/sh", "-c", *cmdFlag)

					// Start
					if err := adapter.Open(); err != nil {
						breaker <- err

						return
					}

					if *cmdFlag != "" {
						cmd.Stdout = os.Stdout
						cmd.Stderr = os.Stderr
						cmd.SysProcAttr = &syscall.SysProcAttr{
							Pdeathsig: syscall.SIGKILL,
							Setpgid:   true,
						}

						if err := cmd.Start(); err != nil {
							breaker <- err

							return
						}
						defer func() {
							if cmd.Process != nil {
								_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) // Ignored as it can be a no-op

								_ = cmd.Wait() // Ignored as it can be a no-op
							}
						}()
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

					// Register interrrupt handler
					go func() {
						s := make(chan os.Signal, 1)
						signal.Notify(s, os.Interrupt)
						<-s

						log.Println("gracefully shutting down agent")

						// Register secondary interrupt handler (which hard-exits)
						go func() {
							s := make(chan os.Signal, 1)
							signal.Notify(s, os.Interrupt)
							<-s

							log.Fatal("cancelled graceful agent shutdown, exiting immediately")
						}()

						breaker <- nil

						_ = adapter.Close()  // Ignored as it can be a no-op
						_ = peers.Close()    // Ignored as it can be a no-op
						_ = signaler.Close() // Ignored as it can be a no-op
						if cmd.Process != nil {
							_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) // Ignored as it can be a no-op

							_ = cmd.Wait() // Ignored as it can be a no-op
						}

						done <- struct{}{}
					}()

					breaker <- signaler.Run()
				}()

				err := <-breaker

				// Interrupting
				if err == nil {
					break
				}

				// Custom error handling
				if err.Error() == "" {
					continue
				}

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

					// Generate TLS cert if it doesn't exist
					if *tlsEnabled {
						_, keyExists := os.Stat(*tlsKeyFlag)
						_, certExists := os.Stat(*tlsCertFlag)
						if keyExists != nil || certExists != nil {
							key, cert, err := utils.GenerateTLSKeyAndCert("weron", time.Duration(time.Hour*24*180))
							if err != nil {
								fatal <- err

								return
							}

							if err := utils.CreateFileAndLeadingDirectories(*tlsKeyFlag, key); err != nil {
								fatal <- err

								return
							}

							if err := utils.CreateFileAndLeadingDirectories(*tlsCertFlag, cert); err != nil {
								fatal <- err

								return
							}
						}
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

					// Register interrrupt handler
					go func() {
						s := make(chan os.Signal, 1)
						signal.Notify(s, os.Interrupt)
						<-s

						log.Println("gracefully shutting down signaling server")

						// Register secondary interrupt handler (which hard-exits)
						go func() {
							s := make(chan os.Signal, 1)
							signal.Notify(s, os.Interrupt)
							<-s

							log.Fatal("cancelled graceful signaling server shutdown, exiting immediately")
						}()

						breaker <- nil

						_ = communities.Close() // Ignored as it can be a no-op
						_ = signaler.Close()    // Ignored as it can be a no-op

						done <- struct{}{}
					}()

					handler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
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
					})

					if *tlsEnabled {
						cert, err := tls.LoadX509KeyPair(*tlsCertFlag, *tlsKeyFlag)
						if err != nil {
							fatal <- err
						}

						log.Printf("Using TLS; SHA1 Fingerprint=%v", utils.GetFingerprint(cert.Certificate[0]))

						fatal <- http.ListenAndServeTLS(addr.String(), *tlsCertFlag, *tlsKeyFlag, handler)
					} else {
						fatal <- http.ListenAndServe(addr.String(), handler)
					}
				}()

				err := <-breaker

				// Interrupting
				if err == nil {
					break
				}

				log.Println("signaling server crashed, restarting in 1s:", err)

				time.Sleep(time.Second)
			}
		}()
	}

	doneSystems := 0
	for {
		select {
		case err := <-fatal:
			log.Fatal(err)
		case <-done:
			doneSystems++
			if *agentFlag && *signalFlag {
				if doneSystems == 2 {
					os.Exit(0)
				}
			} else if doneSystems == 1 {
				os.Exit(0)
			}
		}
	}
}
