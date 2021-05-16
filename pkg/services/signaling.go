package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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

			c.Close(websocket.StatusInternalError, msg)
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
			case api.TypeApplication:
				// Cast to application
				var application api.Application
				if err := json.Unmarshal(data, &application); err != nil {
					msg = "could not parse JSON from WebSocket: " + err.Error()

					return
				}

				log.Println("handling application:", application)

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
			case api.TypeIntroduction:
				log.Println("handling introduction:", v)
			default:
				msg = fmt.Sprintf("could not handle message type, received unknown message type \"%v\"", v.Type)

				return
			}
		}
	}()
}
