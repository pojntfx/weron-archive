package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/mdlayher/ethernet"
	"github.com/pion/webrtc/v3"
	"github.com/pojntfx/weron/pkg/adapter"
	"github.com/pojntfx/weron/pkg/encryption"
	"github.com/pojntfx/weron/pkg/signaling"
	"github.com/pojntfx/weron/pkg/transport"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"nhooyr.io/websocket"
)

const (
	raddrFlag          = "raddr"
	keyFlag            = "key"
	stunFlag           = "stun"
	turnFlag           = "turn"
	timeoutFlag        = "timeout"
	tlsFingerprintFlag = "tls-fingerprint"
	tlsInsecureFlag    = "tls-insecure"
	tlsHostsFlag       = "tls-hosts"
	communityFlag      = "community"
	deviceNameFlag     = "device-name"
)

var joinCmd = &cobra.Command{
	Use:     "join [cmd]",
	Aliases: []string{"joi", "j", "c"},
	Short:   "Join a community",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if err := viper.BindPFlags(cmd.PersistentFlags()); err != nil {
			return nil
		}

		if key := viper.GetString(keyFlag); !(key == "" || len(key) == 16 || len(key) == 24 || len(key) == 32) {
			return errors.New("key is not 16, 24 or 32 characters long")
		}

		if strings.TrimSpace(viper.GetString(communityFlag)) == "" {
			return errors.New("invalid community name")
		}

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Handle lifecycle
		fatal := make(chan error)
		done := make(chan struct{})

		go func() {
			retryWithFingerprint := false

			for {
				breaker := make(chan error)

				go func() {
					// Parse subsystem-specific flags
					parsedKey := []byte(viper.GetString(keyFlag))

					iceServers := []webrtc.ICEServer{}
					for _, stunServer := range viper.GetStringSlice(stunFlag) {
						iceServers = append(iceServers, webrtc.ICEServer{
							URLs: []string{stunServer},
						})
					}

					for _, turnServer := range viper.GetStringSlice(turnFlag) {
						parts := strings.Split(turnServer, "@")
						if len(parts) < 2 {
							fatal <- errors.New("missing authentication or domain parameters in TURN server")

							return
						}

						auth := strings.Split(parts[0], ":")
						if len(parts) < 2 {
							fatal <- errors.New("missing username or credential parameters in TURN server")

							return
						}

						iceServers = append(iceServers, webrtc.ICEServer{
							URLs:           []string{parts[1]},
							Username:       auth[0],
							Credential:     auth[1],
							CredentialType: webrtc.ICECredentialTypePassword,
						})
					}

					log.Println(iceServers)

					// Create the utils dir if it does not exist
					if err := os.MkdirAll(filepath.Dir(viper.GetString(tlsHostsFlag)), os.ModePerm); err != nil {
						fatal <- err

						return
					}

					// Interactively verify TLS certificate if fingerprint is given
					client := http.DefaultClient
					if viper.GetString(tlsFingerprintFlag) != "" || retryWithFingerprint || viper.GetBool(tlsInsecureFlag) {
						customTransport := http.DefaultTransport.(*http.Transport).Clone()

						customTransport.TLSClientConfig = encryption.GetInteractiveTLSConfig(
							viper.GetBool(tlsInsecureFlag),
							viper.GetString(tlsFingerprintFlag),
							viper.GetString(tlsHostsFlag),
							viper.GetString(raddrFlag),
							func(e error) {
								fatal <- e
							},
							func(s string, i ...interface{}) {
								fmt.Printf(s, i...)
							},
							func(s string, i ...interface{}) (string, error) {
								// Print the prompt
								fmt.Printf(s, i...)

								// Read answer
								scanner := bufio.NewScanner(os.Stdin)
								scanner.Scan()
								if err := scanner.Err(); err != nil {
									fatal <- err

									return "", err
								}

								// Trim the trailing newline
								return strings.TrimSuffix(scanner.Text(), "\n"), nil
							},
						)

						client = &http.Client{Transport: customTransport}
					}

					conn, _, err := websocket.Dial(context.Background(), viper.GetString(raddrFlag), &websocket.DialOptions{HTTPClient: client})
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
					adapter := adapter.NewTAP(viper.GetString(deviceNameFlag))
					deviceName, err := adapter.Open()
					if err != nil {
						breaker <- err

						return
					}
					defer func() {
						_ = adapter.Close() // Best effort
					}()

					peers := transport.NewWebRTCManager(
						iceServers,
						func(mac string, i webrtc.ICECandidate) {
							candidateChan <- struct {
								mac string
								i   webrtc.ICECandidate
							}{mac, i}
						},
						func(mac string, frame []byte) {
							frame, err = encryption.Decrypt(frame, parsedKey)
							if err != nil {
								breaker <- err

								return
							}

							if _, err := adapter.Write(frame); err != nil {
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
						_ = peers.Close() // Best effort
					}()

					mac, err := adapter.GetMACAddress()
					if err != nil {
						breaker <- err

						return
					}

					signaler := signaling.NewSignalingClient(
						conn,
						mac.String(),
						viper.GetString(communityFlag),
						viper.GetDuration(timeoutFlag),
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
							return encryption.Encrypt(data, parsedKey)
						},
						func(data []byte) ([]byte, error) {
							return encryption.Decrypt(data, parsedKey)
						},
					)
					defer func() {
						_ = signaler.Close() // Best effort
					}()

					// Start
					var cmd *exec.Cmd
					if len(args) > 0 {
						extraArgs := []string{}
						if len(args) > 1 {
							extraArgs = append(extraArgs, args[1:]...)
						}

						cmd = exec.Command(args[0], extraArgs...)
					}

					if cmd != nil {
						cmd.Stdin = os.Stdin
						cmd.Stdout = os.Stdout
						cmd.Stderr = os.Stderr
						cmd.Args = append(cmd.Args, deviceName)

						if err := cmd.Start(); err != nil {
							breaker <- err

							return
						}
					}

					go func() {
						frameSize, err := adapter.GetFrameSize()
						if err != nil {
							breaker <- err

							return
						}

						for {
							frame := make([]byte, frameSize)
							if _, err := adapter.Read(frame); err != nil {
								breaker <- err

								return
							}

							var parsedFrame ethernet.Frame
							if err := parsedFrame.UnmarshalBinary(frame); err != nil {
								log.Println("could not parse frame, continuing:", err)

								continue
							}

							frame, err = encryption.Encrypt(frame, parsedKey)
							if err != nil {
								breaker <- err

								return
							}

							if err := peers.Write(parsedFrame.Destination.String(), frame); err != nil {
								if viper.GetBool(verboseFlag) {
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

					log.Printf("agent connected to signaler %v", viper.GetString(raddrFlag))

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

						_ = adapter.Close()  // Best effort
						_ = peers.Close()    // Best effort
						_ = signaler.Close() // Best effort
						if cmd != nil && cmd.Process != nil {
							_ = cmd.Process.Kill() // Best effort
							_ = cmd.Wait()         // Best effort
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

		for {
			select {
			case err := <-fatal:
				log.Fatal(err)
			case <-done:
				os.Exit(0)
			}
		}
	},
}

func init() {
	// Get default working dir
	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	workingDirectoryDefault := filepath.Join(home, ".local", "share", "weron", "var", "lib", "weron")

	joinCmd.PersistentFlags().StringP(raddrFlag, "r", "wss://weron.herokuapp.com/", "Signaler address")
	joinCmd.PersistentFlags().StringP(keyFlag, "k", "", "Key for community (16, 24 or 32 characters long)")
	joinCmd.PersistentFlags().StringSliceP(stunFlag, "s", []string{"stun:stun.l.google.com:19302"}, "Comma-seperated list of STUN servers to use")
	joinCmd.PersistentFlags().StringSliceP(turnFlag, "t", []string{}, "Comma-seperated list of TURN servers to use (i.e. username:credential@turn:global.turn.twilio.com:3478?transport=tcp")
	joinCmd.PersistentFlags().DurationP(timeoutFlag, "m", time.Second*5, "Seconds to wait for the signaler to respond")
	joinCmd.PersistentFlags().StringP(tlsFingerprintFlag, "f", "", "Key for community (16, 24 or 32 characters long)")
	joinCmd.PersistentFlags().BoolP(tlsInsecureFlag, "i", false, "Skip TLS certificate validation")
	joinCmd.PersistentFlags().StringP(tlsHostsFlag, "o", filepath.Join(workingDirectoryDefault, "known_hosts"), "Path to the TLS known_hosts file")
	joinCmd.PersistentFlags().StringP(communityFlag, "c", "", "Name of the community to join")
	joinCmd.PersistentFlags().StringP(deviceNameFlag, "d", "", "Name to give the created network interface (if supported by the OS; if not specified, a random name will be chosen)")

	viper.AutomaticEnv()

	rootCmd.AddCommand(joinCmd)
}
