package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/ugjka/messenger"
)

type Candidate struct {
	Address string `json:"address"`
	Payload []byte `json:"payload"`
}

func main() {
	laddr := flag.String("laddr", ":15356", "Listen address")

	flag.Parse()

	upgrader := websocket.Upgrader{}
	msgr := messenger.New(0, true)

	http.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		remote, err := upgrader.Upgrade(rw, r, nil)
		if err != nil {
			panic(err)
		}
		defer remote.Close()

		address := uuid.New().String()

		bus, err := msgr.Sub()
		if err != nil {
			panic(err)
		}
		defer msgr.Unsub(bus)

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			for {
				_, payload, err := remote.ReadMessage()
				if err != nil {
					if websocket.IsCloseError(err, websocket.CloseNoStatusReceived, websocket.CloseAbnormalClosure) {
						break
					}

					panic(err)
				}

				msgr.Broadcast(Candidate{
					Address: address,
					Payload: payload,
				})
			}

			wg.Done()
		}()

		go func() {
			for candidate := range bus {
				if candidate.(Candidate).Address == address {
					continue
				}

				msg, err := json.Marshal(candidate)
				if err != nil {
					panic(err)
				}

				if err := remote.WriteMessage(websocket.TextMessage, msg); err != nil {
					if err.Error() == websocket.ErrCloseSent.Error() || strings.HasSuffix(err.Error(), "write: broken pipe") {
						break
					}

					panic(err)
				}
			}

			wg.Done()
		}()

		wg.Wait()
	})

	log.Printf("weron server listening on %v", *laddr)

	log.Fatal(http.ListenAndServe(*laddr, nil))
}
