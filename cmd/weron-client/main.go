package main

import (
	"encoding/json"
	"flag"
	"log"

	"github.com/gorilla/websocket"
)

type Candidate struct {
	Address string `json:"address"`
	Payload []byte `json:"payload"`
}

func main() {
	raddr := flag.String("raddr", "ws://localhost:15356", "Remote address")

	flag.Parse()

	remote, _, err := websocket.DefaultDialer.Dial(*raddr, nil)
	if err != nil {
		panic(err)
	}
	defer remote.Close()

	for {
		_, msg, err := remote.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNoStatusReceived) {
				break
			}

			panic(err)
		}

		var candidate Candidate
		if err := json.Unmarshal(msg, &candidate); err != nil {
			panic(err)
		}

		log.Println(candidate)
	}
}
