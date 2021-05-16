package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"

	api "github.com/pojntfx/weron/pkg/api/websockets/v1"
	"github.com/pojntfx/weron/pkg/cache"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

const (
	invalidCommunity = "-1"
	invalidMAC       = "-1"
)

func Signaling(network *cache.Network, rw http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(rw, r, nil)
	if err != nil {
		log.Println("could not accept on WebSocket:", err)

		return
	}

	go func() {
		// Community and MAC for this connection
		community := invalidCommunity
		mac := invalidMAC

		// Handle errors by closing the connection
		msg := "internal error"
		defer func() {
			log.Println("could not continue in handler:", msg)

			// Handle exited; ignore the error as it might be a no-op
			_ = network.HandleExited(community, mac, msg)
		}()

		for {
			// Read message from connection
			_, data, err := c.Read(context.Background())
			if err != nil {
				msg = "could not read from WebSocket: " + err.Error()

				return
			}

			// Parse message
			var v api.Message
			if err := json.Unmarshal(data, &v); err != nil {
				msg = "could not parse JSON from WebSocket: " + err.Error()

				return
			}

			// Handle different message types
			switch v.Type {
			// Admission
			case api.TypeApplication:
				// Prevent duplicate application
				if community != invalidCommunity || mac != invalidMAC {
					msg = "could not handle application: already applied"

					return
				}

				// Cast to application
				var application api.Application
				if err := json.Unmarshal(data, &application); err != nil {
					msg = "could not parse JSON from WebSocket: " + err.Error()

					return
				}

				log.Println("handling application:", application)

				// Validate incoming community and MAC address
				if _, err := net.ParseMAC(application.Mac); application.Community == invalidCommunity || application.Mac == invalidMAC || err != nil {
					msg = "could not handle application: invalid community or MAC"

					if err != nil {
						msg += ": " + err.Error()
					}

					return
				}

				// Handle application
				if err := network.HandleApplication(application.Community, application.Mac, c); err != nil {
					msg = "could not handle application: " + err.Error()

					// Send rejection on error
					if err := wsjson.Write(context.Background(), c, api.NewRejection()); err != nil {
						msg += ": " + err.Error()
					}

					return
				}

				// Set community and MAC address for this connection
				community = application.Community
				mac = application.Mac

				// Send acceptance
				if err := wsjson.Write(context.Background(), c, api.NewAcceptance()); err != nil {
					msg += ": " + err.Error()
				}
			case api.TypeReady:
				log.Printf("handling ready for community %v and MAC address %v: %v", community, mac, v)

				// Handle ready
				if err := network.HandleReady(community, mac); err != nil {
					msg = "could not handle ready: " + err.Error()

					return
				}

			// Exchange
			case api.TypeOffer:
				fallthrough
			case api.TypeAnswer:
				fallthrough
			case api.TypeCandidate:
				// Cast to exchange
				var exchange api.Exchange
				if err := json.Unmarshal(data, &exchange); err != nil {
					msg = "could not parse JSON from WebSocket: " + err.Error()

					return
				}

				// Validate incoming MAC address
				if _, err := net.ParseMAC(exchange.Mac); err != nil {
					msg = "could not handle application: invalid MAC address: " + err.Error()

					return
				}

				log.Printf("handling exchange for community %v and src MAC address %v: %v", community, mac, exchange)

				// Handle exchange
				if err := network.HandleExchange(community, mac, exchange); err != nil {
					msg = "could not handle ready: " + err.Error()

					return
				}

			// Discharge
			case api.TypeExited:
				log.Printf("handling exited for community %v and MAC address %v: %v", community, mac, v)

				// Handle exited
				if err := network.HandleExited(community, mac, ""); err != nil {
					msg = "could not handle exited: " + err.Error()

					return
				}
			default:
				msg = fmt.Sprintf("could not handle message type, received unknown message type \"%v\"", v.Type)

				return
			}
		}
	}()
}
