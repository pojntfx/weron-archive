package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net"
	"sync"

	api "github.com/pojntfx/weron/pkg/api/websockets/v1"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

func main() {
	// Define flags
	raddr := flag.String("raddr", "ws://localhost:15325", "Signaler address")
	community := flag.String("community", "cluster1", "Community to join")
	macFlag := flag.String("mac", "cc:0b:cf:23:22:0d", "MAC address to use")

	// Parse flags
	flag.Parse()

	mac, err := net.ParseMAC(*macFlag)
	if err != nil {
		log.Fatal("could not parse MAC address:", err)
	}

	// Connect to the signaler
	c, _, err := websocket.Dial(context.Background(), *raddr, nil)
	if err != nil {
		log.Fatal("could not dial WebSocket:", err)
	}
	defer c.Close(websocket.StatusInternalError, "closing")

	var wg sync.WaitGroup
	wg.Add(1)

	ready := make(chan struct{})

	go func() {
		defer wg.Done()

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

				log.Fatal("could not join community: MAC address rejected. Please retry with another MAC address.")
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
			}
		}
	}()

	// Send application
	application := api.NewApplication(*community, mac.String())
	log.Println("sending application:", application)

	if err := wsjson.Write(context.Background(), c, api.NewApplication(*community, mac.String())); err != nil {
		log.Fatal("could not send application:", err)
	}

	<-ready

	// Send ready
	readyMessage := api.NewReady()
	log.Println("sending ready:", readyMessage)

	if err := wsjson.Write(context.Background(), c, readyMessage); err != nil {
		log.Fatal("could not send ready:", err)
	}

	wg.Wait()
}
