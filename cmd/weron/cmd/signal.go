package cmd

import (
	"context"
	"crypto/tls"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"time"

	api "github.com/pojntfx/weron/pkg/api/websockets/v1"
	"github.com/pojntfx/weron/pkg/encryption"
	"github.com/pojntfx/weron/pkg/signaling"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

const (
	laddrFlag   = "laddr"
	tlsFlag     = "tls"
	tlsKeyFlag  = "tls-key"
	tlsCertFlag = "tls-cert"
)

var signalCmd = &cobra.Command{
	Use:     "signal",
	Aliases: []string{"sig", "s"},
	Short:   "Start a signaling server",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return viper.BindPFlags(cmd.PersistentFlags())
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		addr, err := net.ResolveTCPAddr("tcp", viper.GetString(laddrFlag))
		if err != nil {
			return err
		}

		if port := os.Getenv("PORT"); port != "" {
			if viper.GetBool(verboseFlag) {
				log.Println("Using port from PORT env variable")
			}

			p, err := strconv.Atoi(port)
			if err != nil {
				return err
			}

			addr.Port = p
		}

		if viper.GetBool(tlsFlag) {
			_, keyExists := os.Stat(viper.GetString(tlsKeyFlag))
			_, certExists := os.Stat(viper.GetString(tlsCertFlag))

			if keyExists != nil || certExists != nil {
				if viper.GetBool(verboseFlag) {
					log.Println("Generating TLS cert and key")
				}

				key, cert, err := encryption.GenerateTLSKeyAndCert("weron", time.Hour*24*365)
				if err != nil {
					return err
				}

				for _, file := range [][2]string{
					{key, viper.GetString(tlsKeyFlag)},
					{cert, viper.GetString(tlsCertFlag)},
				} {
					if err := os.MkdirAll(filepath.Base(file[1]), os.ModePerm); err != nil {
						return err
					}

					if err := ioutil.WriteFile(file[1], []byte(file[0]), os.ModePerm); err != nil {
						return err
					}
				}
			}
		}

		communities := signaling.NewCommunitiesManager(
			func(mac string, conn *websocket.Conn) error {
				if viper.GetBool(verboseFlag) {
					log.Println("Handling introduction for MAC", mac)
				}

				return wsjson.Write(context.Background(), conn, api.NewIntroduction(mac))
			},
			func(mac string, exchange api.Exchange, conn *websocket.Conn) error {
				if viper.GetBool(verboseFlag) {
					log.Println("Handling exchange for MAC", mac)
				}

				return wsjson.Write(context.Background(), conn, exchange)
			},
			func(mac string, conn *websocket.Conn) error {
				if viper.GetBool(verboseFlag) {
					log.Println("Handling resignation for MAC", mac)
				}

				return wsjson.Write(context.Background(), conn, api.NewResignation(mac))
			},
		)

		signaler := signaling.NewSignalingServer(
			func(community, mac string, conn *websocket.Conn) error {
				if viper.GetBool(verboseFlag) {
					log.Println("Handling application for community", community, "and MAC", mac)
				}

				return communities.HandleApplication(community, mac, conn)
			},
			func(community, mac string, conn *websocket.Conn) error {
				if viper.GetBool(verboseFlag) {
					log.Println("Handling rejection for community", community, "and MAC", mac)
				}

				return wsjson.Write(context.Background(), conn, api.NewRejection())
			},
			func(community, mac string, conn *websocket.Conn) error {
				if viper.GetBool(verboseFlag) {
					log.Println("Handling acceptance for community", community, "and MAC", mac)
				}

				return wsjson.Write(context.Background(), conn, api.NewAcceptance())
			},
			func(community, mac string, err error) error {
				if viper.GetBool(verboseFlag) {
					log.Println("Handling exited for community", community, "and MAC", mac)
				}

				return communities.HandleExited(community, mac, err)
			},
			func(community, mac string) error {
				if viper.GetBool(verboseFlag) {
					log.Println("Handling ready for community", community, "and MAC", mac)
				}

				return communities.HandleReady(community, mac)
			},
			func(community, mac string, exchange api.Exchange) error {
				if viper.GetBool(verboseFlag) {
					log.Println("Handling exchange for community", community, "and MAC", mac)
				}

				return communities.HandleExchange(community, mac, exchange)
			},
		)

		srv := &http.Server{
			Addr: addr.String(),
			Handler: http.HandlerFunc(
				func(rw http.ResponseWriter, r *http.Request) {
					conn, err := websocket.Accept(rw, r, nil)
					if err != nil {
						log.Println("could not accept on WebSocket:", err)

						return
					}

					log.Println("Client with address", r.RemoteAddr, "connected")

					if err := signaler.HandleConn(conn); err != nil {
						log.Println("Client with address", r.RemoteAddr, "disconnected")

						return
					}
				},
			),
		}

		s := make(chan os.Signal)
		signal.Notify(s, os.Interrupt)
		go func() {
			<-s

			log.Println("Gracefully shutting down server")

			if err := communities.Close(); len(err) > 1 {
				panic(err)
			}

			if err := signaler.Close(); len(err) > 1 {
				panic(err)
			}

			if err := srv.Shutdown(context.Background()); err != nil {
				panic(err)
			}
		}()

		log.Println("Signaler listening on", addr)

		if viper.GetBool(tlsFlag) {
			cert, err := tls.LoadX509KeyPair(viper.GetString(tlsCertFlag), viper.GetString(tlsKeyFlag))
			if err != nil {
				return err
			}

			log.Println("TLS certificate SHA-1 fingerprint:", encryption.GetFingerprint(cert.Certificate[0]))

			if err := srv.ListenAndServeTLS(viper.GetString(tlsCertFlag), viper.GetString(tlsKeyFlag)); err != http.ErrServerClosed {
				return err
			}

			return nil
		}

		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			return err
		}

		return nil
	},
}

func init() {
	// Get default working dir
	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	workingDirectoryDefault := filepath.Join(home, ".local", "share", "weron", "var", "lib", "weron")

	signalCmd.PersistentFlags().StringP(laddrFlag, "a", ":15325", "Listen address (port can also be set with PORT env variable)")
	signalCmd.PersistentFlags().BoolP(tlsFlag, "t", true, "Enable TLS")
	signalCmd.PersistentFlags().StringP(tlsKeyFlag, "k", filepath.Join(workingDirectoryDefault, "key.pem"), "Path to the TLS private key (will be generated if it does not exist)")
	signalCmd.PersistentFlags().StringP(tlsCertFlag, "c", filepath.Join(workingDirectoryDefault, "cert.crt"), "Path to the TLS certificate (will be generated if it does not exist)")

	viper.AutomaticEnv()

	rootCmd.AddCommand(signalCmd)
}
