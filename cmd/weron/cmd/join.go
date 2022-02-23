package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/pion/webrtc/v3"
	"github.com/pojntfx/weron/pkg/encryption"
	"github.com/pojntfx/weron/pkg/networking"
	"github.com/pojntfx/weron/pkg/signaling"
	"github.com/pojntfx/weron/pkg/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"nhooyr.io/websocket"
)

const (
	raddrKey          = "raddr"
	keyKey            = "key"
	mtuKey            = "mtu"
	iceKey            = "ice"
	timeoutKey        = "timeout"
	tlsFingerprintKey = "tls-fingerprint"
	tlsInsecureKey    = "tls-insecure"
	tlsHostsKey       = "tls-hosts"
	verboseKey        = "verbose"
)

var joinCmd = &cobra.Command{
	Use:     "join <community>/<mac>[%<device>] [cmd]",
	Aliases: []string{"joi", "j", "c"},
	Short:   "Join a community",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get the URI part of the arguments
		uri := ""
		if len(args) > 0 {
			uri = args[0]
		} else {
			return errors.New("community and MAC address are missing")
		}

		// Parse community from parts
		uriParts := strings.Split(uri, "/")
		community := ""
		macAndDevice := ""
		if len(uriParts) > 1 {
			community = uriParts[0]
			macAndDevice = uriParts[1]
		} else {
			return errors.New("MAC address is missing")
		}

		// Parse raw MAC address and device name from parts
		macAndDeviceParts := strings.Split(macAndDevice, "%")
		rawMAC := ""
		device := "weron0"
		if len(macAndDeviceParts) > 0 {
			rawMAC = macAndDeviceParts[0]

			if len(macAndDeviceParts) > 1 {
				device = macAndDeviceParts[1]
			}
		}

		// Check if MAC address is valid
		mac, err := net.ParseMAC(rawMAC)
		if err != nil {
			return err
		}

		// Parse key
		key := viper.GetString(keyKey)
		if !(key == "" || len(key) == 16 || len(key) == 24 || len(key) == 32) {
			return errors.New("key is not 16, 24 or 32 characters long")
		}

		// Handle lifecycle
		fatal := make(chan error)
		done := make(chan struct{})

		go func() {
			retryWithFingerprint := false

			for {
				breaker := make(chan error)

				go func() {
					// Parse subsystem-specific flags
					mac, err := net.ParseMAC(mac.String())
					if err != nil {
						fatal <- err

						return
					}

					parsedKey := []byte(key)

					stunServers := []webrtc.ICEServer{}
					for _, stunServer := range strings.Split(viper.GetString(iceKey), ",") {
						stunServers = append(stunServers, webrtc.ICEServer{
							URLs: []string{stunServer},
						})
					}

					// Create the utils file if it does not exist
					if err := utils.CreateFileAndLeadingDirectories(viper.GetString(tlsHostsKey), ""); err != nil {
						fatal <- err

						return
					}

					// Interactively verify TLS certificate if fingerprint is given
					client := http.DefaultClient
					if viper.GetString(tlsFingerprintKey) != "" || retryWithFingerprint || viper.GetBool(tlsInsecureKey) {
						customTransport := http.DefaultTransport.(*http.Transport).Clone()

						customTransport.TLSClientConfig = encryption.GetInteractiveTLSConfig(
							viper.GetBool(tlsInsecureKey),
							viper.GetString(tlsFingerprintKey),
							viper.GetString(tlsHostsKey),
							viper.GetString(raddrKey),
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
								if scanner.Err() != nil {
									fatal <- err

									return "", err
								}

								// Trim the trailing newline
								return strings.TrimSuffix(scanner.Text(), "\n"), nil
							},
						)

						client = &http.Client{Transport: customTransport}
					}

					conn, _, err := websocket.Dial(context.Background(), viper.GetString(raddrKey), &websocket.DialOptions{HTTPClient: client})
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
					adapter := networking.NewNetworkAdapter(device, viper.GetInt(mtuKey), mac)
					defer func() {
						_ = adapter.Close() // Best effort
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
							frame, err = encryption.Decrypt(frame, parsedKey)
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
						_ = peers.Close() // Best effort
					}()

					signaler := signaling.NewSignalingClient(
						conn,
						mac.String(),
						community,
						time.Duration(viper.GetInt(timeoutKey))*time.Second,
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
					if err := adapter.Open(); err != nil {
						breaker <- err

						return
					}

					var cmd *exec.Cmd
					if len(args) > 1 {
						extraArgs := []string{}
						if len(args) > 2 {
							extraArgs = append(extraArgs, args[2:]...)
						}

						cmd = exec.Command(args[1], extraArgs...)
					}

					if cmd != nil {
						cmd.Stdin = os.Stdin
						cmd.Stdout = os.Stdout
						cmd.Stderr = os.Stderr

						if err := cmd.Start(); err != nil {
							breaker <- err

							return
						}
					}

					go func() {
						for {
							frame, err := adapter.Read()
							if err != nil {
								breaker <- err

								return
							}

							dst, err := networking.GetDestination(frame)
							if err != nil {
								log.Println("could not get destination from frame, continuing:", err)

								continue
							}

							frame, err = encryption.Encrypt(frame, parsedKey)
							if err != nil {
								breaker <- err

								return
							}

							if err := peers.Write(dst.String(), frame); err != nil {
								if viper.GetBool(verboseKey) {
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

					log.Printf("agent connected to signaler %v", viper.GetString(raddrKey))

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
						if cmd.Process != nil {
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
	// Get home directory
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal("could not get home directory:", err)
	}

	joinCmd.PersistentFlags().String(raddrKey, "wss://weron.herokuapp.com/", "Signaler address")
	joinCmd.PersistentFlags().String(keyKey, "", "Key for community (16, 24 or 32 characters)")

	joinCmd.PersistentFlags().Int(mtuKey, 1500, "MTU for the TAP device")
	joinCmd.PersistentFlags().String(iceKey, "stun:stun.l.google.com:19302", "Comma-seperated list of STUN servers to use")
	joinCmd.PersistentFlags().Int(timeoutKey, 5, "Seconds to wait for the signaler to respond")

	joinCmd.PersistentFlags().String(tlsFingerprintKey, "", "Signaler TLS certificate SHA1 fingerprint")
	joinCmd.PersistentFlags().Bool(tlsInsecureKey, false, "Skip TLS certificate validation")
	joinCmd.PersistentFlags().String(tlsHostsKey, filepath.Join(home, ".local", "share", "weron", "etc", "lib", "weron", "known_hosts"), "Path to TLS known_hosts file")

	joinCmd.PersistentFlags().Bool(verboseKey, false, "Enable verbose logging")

	// Bind env variables
	if err := viper.BindPFlags(joinCmd.PersistentFlags()); err != nil {
		log.Fatal("could not bind flags:", err)
	}
	viper.SetEnvPrefix("weron")
	viper.AutomaticEnv()

	rootCmd.AddCommand(joinCmd)
}
