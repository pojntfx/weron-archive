package main

import (
	"context"
	"flag"
	"log"
	"sync"

	api "github.com/pojntfx/weron/pkg/api/websockets/v1"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

func main() {
	raddr := flag.String("raddr", "ws://localhost:15325", "Signaler address")
	community := flag.String("community", "cluster1", "Community to join")
	mac := flag.String("mac", "cc:0b:cf:23:22:0d", "MAC address to use")

	flag.Parse()

	c, _, err := websocket.Dial(context.Background(), *raddr, nil)
	if err != nil {
		log.Fatal("could not dial WebSocket:", err)
	}
	defer c.Close(websocket.StatusInternalError, "closing")

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		for {
			_, data, err := c.Read(context.Background())
			if err != nil {
				log.Println("could not read from WebSocket:", err)

				return
			}

			log.Println(string(data))
		}
	}()

	if err := wsjson.Write(context.Background(), c, api.NewApplication(*community, *mac)); err != nil {
		log.Fatal("could not send application:", err)
	}

	wg.Wait()
}
