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
)

func Signaling(network *cache.Network, rw http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(rw, r, nil)
	if err != nil {
		log.Println("could not accept on WebSocket:", err)

		return
	}

	go func() {
		msg := "internal error"
		defer func() {
			log.Println("could not continue in handler:", msg)

			conn.Close(websocket.StatusInternalError, msg)
		}()

		for {
			_, data, err := conn.Read(context.Background())
			if err != nil {
				msg = "could not read from WebSocket: " + err.Error()

				return
			}

			var v api.Message
			if err := json.Unmarshal(data, &v); err != nil {
				msg = "could not parse JSON from WebSocket: " + err.Error()

				return
			}

			switch v.Type {
			case api.TypeApplication:
				var application api.Application
				if err := json.Unmarshal(data, &application); err != nil {
					msg = "could not parse JSON from WebSocket: " + err.Error()

					return
				}

				log.Println("handling application:", application)

				if err := network.HandleApplication(application.Community, application.Mac, conn); err != nil {
					msg = "could not handle application: " + err.Error()

					return
				}
			case api.TypeAcceptance:
				log.Println("handling acceptance:", v)
			case api.TypeRejection:
				log.Println("handling rejection:", v)
			case api.TypeReady:
				log.Println("handling ready:", v)
			case api.TypeIntroduction:
				log.Println("handling introduction:", v)
			default:
				msg = fmt.Sprintf("could not handle message type, received unknown message type \"%v\"", v.Type)

				return
			}
		}
	}()
}
