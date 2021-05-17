package services

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"

	api "github.com/pojntfx/weron/pkg/api/websockets/v1"
	"github.com/pojntfx/weron/pkg/core"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

func Agent(agent *core.Agent, community string, mac net.HardwareAddr, c *websocket.Conn) error {
	ready := make(chan struct{})
	fatal := make(chan string)
	done := make(chan struct{})

	go func() {
		defer func() {
			done <- struct{}{}
		}()

		for {
			// Read message from connection
			_, data, err := c.Read(context.Background())
			if err != nil {
				log.Println("could not read from WebSocket:", err)

				return
			}

			// Parse message
			var v api.Message
			if err := json.Unmarshal(data, &v); err != nil {
				log.Println("could not parse JSON from WebSocket: ", err)

				return
			}

			// Handle different message types
			switch v.Type {
			// Admission
			case api.TypeRejection:
				log.Println("handling rejection:", v)

				fatal <- "could not join community: MAC address rejected. Please retry with another MAC address."
			case api.TypeAcceptance:
				log.Println("handling acceptance:", v)

				ready <- struct{}{}
			case api.TypeIntroduction:
				// Cast to introduction
				var introduction api.Introduction
				if err := json.Unmarshal(data, &introduction); err != nil {
					log.Println("could not parse JSON from WebSocket:", err)

					return
				}

				log.Println("handling introduction:", introduction)

				// TODO: Call agent.HandleIntroduction which creates WebRTC connection, add to map based on MAC, send offer to MAC

			// Discharge
			default:
				log.Printf("could not handle message type, received unknown message type \"%v\"", v.Type)

				return
			}
		}
	}()

	// Send application
	application := api.NewApplication(community, mac.String())
	log.Println("sending application:", application)

	if err := wsjson.Write(context.Background(), c, api.NewApplication(community, mac.String())); err != nil {
		log.Fatal("could not send application:", err)
	}

	<-ready

	// Send ready
	readyMessage := api.NewReady()
	log.Println("sending ready:", readyMessage)

	if err := wsjson.Write(context.Background(), c, readyMessage); err != nil {
		log.Fatal("could not send ready:", err)
	}

	select {
	case <-done:
		return nil
	case err := <-fatal:
		return errors.New(err)
	}
}
