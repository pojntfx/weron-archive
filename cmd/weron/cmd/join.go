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

var (
	errInvalidTURNServerAddr  = errors.New("invalid TURN server address")
	errMissingTURNCredentials = errors.New("missing TURN server credentials")
)

type candidate struct {
	mac string
	i   webrtc.ICECandidate
}

type session struct {
	mac string
	o   webrtc.SessionDescription
}

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
		adapter := adapter.NewTAP(viper.GetString(deviceNameFlag))
		deviceName, err := adapter.Open()
		if err != nil {
			return err
		}

		iceServers := []webrtc.ICEServer{}

		for _, stunServer := range viper.GetStringSlice(stunFlag) {
			iceServers = append(iceServers, webrtc.ICEServer{
				URLs: []string{stunServer},
			})
		}

		for _, turnServer := range viper.GetStringSlice(turnFlag) {
			addrParts := strings.Split(turnServer, "@")
			if len(addrParts) < 2 {
				return errInvalidTURNServerAddr
			}

			authParts := strings.Split(addrParts[0], ":")
			if len(addrParts) < 2 {
				return errMissingTURNCredentials
			}

			iceServers = append(iceServers, webrtc.ICEServer{
				URLs:           []string{addrParts[1]},
				Username:       authParts[0],
				Credential:     authParts[1],
				CredentialType: webrtc.ICECredentialTypePassword,
			})
		}

		candidates := make(chan candidate)
		offers := make(chan session)
		answers := make(chan session)

		peers := transport.NewWebRTCManager(
			iceServers,
			func(mac string, i webrtc.ICECandidate) {
				if viper.GetBool(verboseFlag) {
					log.Println("Handling outgoing candidate for MAC", mac)
				}

				candidates <- candidate{mac, i}
			},
			func(mac string, frame []byte) {
				if viper.GetBool(verboseFlag) {
					log.Println("Handling outgoing frame for MAC", mac)
				}

				frame, err = encryption.Decrypt(frame, []byte(viper.GetString(keyFlag)))
				if err != nil {
					panic(err)
				}

				if _, err := adapter.Write(frame); err != nil {
					panic(err)
				}
			},
			func(mac string, o webrtc.SessionDescription) {
				if viper.GetBool(verboseFlag) {
					log.Println("Handling outgoing offer for MAC", mac)
				}

				offers <- session{mac, o}
			},
			func(mac string, o webrtc.SessionDescription) {
				if viper.GetBool(verboseFlag) {
					log.Println("Handling outgoing answer for MAC", mac)
				}

				answers <- session{mac, o}
			},
			func(mac string) {
				log.Println("Peer with MAC", mac, "connected")
			},
			func(mac string) {
				log.Println("Peer with MAC", mac, "disconnected")
			},
		)

		if err := os.MkdirAll(filepath.Dir(viper.GetString(tlsHostsFlag)), os.ModePerm); err != nil {
			return err
		}

		httpTransport := http.DefaultTransport.(*http.Transport).Clone()
		httpTransport.TLSClientConfig = encryption.GetInteractiveTLSConfig(
			viper.GetBool(tlsInsecureFlag),
			viper.GetString(tlsFingerprintFlag),
			viper.GetString(tlsHostsFlag),
			viper.GetString(raddrFlag),
			func(err error) {
				panic(err)
			},
			cmd.Printf,
			func(s string, i ...interface{}) (string, error) {
				fmt.Printf(s, i...)

				scanner := bufio.NewScanner(os.Stdin)
				scanner.Scan()
				if err := scanner.Err(); err != nil {
					panic(err)
				}

				return strings.TrimSuffix(scanner.Text(), "\n"), nil
			},
		)
		client := &http.Client{Transport: httpTransport}

		conn, _, err := websocket.Dial(context.Background(), viper.GetString(raddrFlag), &websocket.DialOptions{HTTPClient: client})
		if err != nil {
			return err
		}

		mac, err := adapter.GetMACAddress()
		if err != nil {
			return err
		}

		signaler := signaling.NewSignalingClient(
			conn,
			mac.String(),
			viper.GetString(communityFlag),
			viper.GetDuration(timeoutFlag),
			func(mac string) {
				if viper.GetBool(verboseFlag) {
					log.Println("Handling incoming introduction for MAC", mac)
				}

				if err := peers.HandleIntroduction(mac); err != nil {
					panic(err)
				}
			},
			func(mac string, o webrtc.SessionDescription) {
				if viper.GetBool(verboseFlag) {
					log.Println("Handling incoming offer for MAC", mac)
				}

				if err := peers.HandleOffer(mac, o); err != nil {
					panic(err)
				}
			},
			func(mac string, i webrtc.ICECandidateInit) {
				if viper.GetBool(verboseFlag) {
					log.Println("Handling incoming candidate for MAC", mac)
				}

				if err := peers.HandleCandidate(mac, i); err != nil {
					panic(err)
				}
			},
			func(mac string, o webrtc.SessionDescription) {
				if viper.GetBool(verboseFlag) {
					log.Println("Handling incoming answer for MAC", mac)
				}

				if err := peers.HandleAnswer(mac, o); err != nil {
					panic(err)
				}
			},
			func(mac string, blocked bool) {
				if blocked {
					log.Println("Blocked connection to peer", mac, "due to wrong encryption key")
				}

				// Ignore as this can be a no-op
				_ = peers.HandleResignation(mac)
			},
			func(data []byte) ([]byte, error) {
				return encryption.Encrypt(data, []byte(viper.GetString(keyFlag)))
			},
			func(data []byte) ([]byte, error) {
				return encryption.Decrypt(data, []byte(viper.GetString(keyFlag)))
			},
		)

		var command *exec.Cmd
		if len(args) > 0 {
			extraArgs := []string{}
			if len(args) > 1 {
				extraArgs = append(extraArgs, args[1:]...)
			}

			command = exec.Command(args[0], extraArgs...)

			command.Stdin = os.Stdin
			command.Stdout = os.Stdout
			command.Stderr = os.Stderr
			command.Args = append(command.Args, deviceName)

			if err := command.Start(); err != nil {
				return err
			}
		}

		frameSize, err := adapter.GetFrameSize()
		if err != nil {
			return err
		}

		go func() {
			for {
				select {
				case candidate := <-candidates:
					if err := signaler.SignalCandidate(candidate.mac, candidate.i); err != nil {
						panic(err)
					}
				case offer := <-offers:
					if err := signaler.SignalOffer(offer.mac, offer.o); err != nil {
						panic(err)
					}
				case answer := <-answers:
					if err := signaler.SignalAnswer(answer.mac, answer.o); err != nil {
						panic(err)
					}
				}
			}
		}()

		s := make(chan os.Signal)
		signal.Notify(s, os.Interrupt)
		go func() {
			<-s

			log.Println("Gracefully shutting down agent")

			if err := adapter.Close(); err != nil {
				panic(err)
			}

			if err := peers.Close(); len(err) > 1 {
				panic(err)
			}

			if err := signaler.Close(); err != nil {
				panic(err)
			}
		}()

		go func() {
			if err := signaler.Run(); err != nil {
				panic(err)
			}
		}()

		log.Println("Agent connected to signaler", viper.GetString(raddrFlag))

		for {
			frame := make([]byte, frameSize)
			if _, err := adapter.Read(frame); err != nil {
				return err
			}

			var parsedFrame ethernet.Frame
			if err := parsedFrame.UnmarshalBinary(frame); err != nil {
				log.Println("could not parse frame, continuing:", err)

				continue
			}

			frame, err = encryption.Encrypt(frame, []byte(viper.GetString(keyFlag)))
			if err != nil {
				return err
			}

			if err := peers.Write(parsedFrame.Destination.String(), frame); err != nil {
				if viper.GetBool(verboseFlag) {
					log.Println("could not write to peer, continuing:", err)
				}

				continue
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
