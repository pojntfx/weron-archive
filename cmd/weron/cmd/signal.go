package cmd

import (
	"context"
	"crypto/tls"
	"fmt"
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
	"github.com/pojntfx/weron/pkg/networking"
	"github.com/pojntfx/weron/pkg/signaling"
	"github.com/pojntfx/weron/pkg/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

const (
	laddrKey   = "laddr"
	tlsKey     = "tls"
	tlsKeyKey  = "tls-key"
	tlsCertKey = "tls-cert"
)

var signalCmd = &cobra.Command{
	Use:     "signal",
	Aliases: []string{"sig", "s"},
	Short:   "Start a signaling server",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Handle lifecycle
		fatal := make(chan error)
		done := make(chan struct{})

		go func() {
			for {
				breaker := make(chan error)

				go func() {
					// Parse subsystem-specific flags
					addr, err := net.ResolveTCPAddr("tcp", viper.GetString(laddrKey))
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
					if viper.GetBool(tlsKey) {
						_, keyExists := os.Stat(viper.GetString(tlsKeyKey))
						_, certExists := os.Stat(viper.GetString(tlsCertKey))
						if keyExists != nil || certExists != nil {
							key, cert, err := encryption.GenerateTLSKeyAndCert("weron", time.Duration(time.Hour*24*180))
							if err != nil {
								fatal <- err

								return
							}

							if err := utils.CreateFileAndLeadingDirectories(viper.GetString(tlsKeyKey), key); err != nil {
								fatal <- err

								return
							}

							if err := utils.CreateFileAndLeadingDirectories(viper.GetString(tlsCertKey), cert); err != nil {
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
						_ = communities.Close() // Best effort
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
						_ = signaler.Close() // Best effort
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

						_ = communities.Close() // Best effort
						_ = signaler.Close()    // Best effort

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

					if viper.GetBool(tlsKey) {
						cert, err := tls.LoadX509KeyPair(viper.GetString(tlsCertKey), viper.GetString(tlsKeyKey))
						if err != nil {
							fatal <- err
						}

						log.Printf("TLS certificate SHA1 fingerprint is %v.", encryption.GetFingerprint(cert.Certificate[0]))

						fatal <- http.ListenAndServeTLS(addr.String(), viper.GetString(tlsCertKey), viper.GetString(tlsKeyKey), handler)
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
	// Get working directory
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal("could not get home directory:", err)
	}
	workingDirectoryDefault := filepath.Join(home, ".local", "share", "weron", "etc", "lib", "weron")

	signalCmd.PersistentFlags().String(laddrKey, ":15325", "Listen address; the port can also be set using the PORT env variable.")

	signalCmd.PersistentFlags().Bool(tlsKey, true, "Enable TLS")
	signalCmd.PersistentFlags().String(tlsKeyKey, filepath.Join(workingDirectoryDefault, "key.pem"), "Path to TLS key; will be generated if it does not exist")
	signalCmd.PersistentFlags().String(tlsCertKey, filepath.Join(workingDirectoryDefault, "cert.pem"), "Path to TLS certificate; will be generated if it does not exist")

	signalCmd.PersistentFlags().Bool(verboseKey, false, "Enable verbose logging")

	// Bind env variables
	if err := viper.BindPFlags(signalCmd.PersistentFlags()); err != nil {
		log.Fatal("could not bind flags:", err)
	}
	viper.SetEnvPrefix("weron")
	viper.AutomaticEnv()

	rootCmd.AddCommand(signalCmd)
}
