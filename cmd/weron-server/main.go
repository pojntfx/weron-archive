package main

import (
	"flag"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/pojntfx/weron/pkg/addressing"
	"github.com/pojntfx/weron/pkg/messages"
	"github.com/ugjka/messenger"
)

type Message struct {
	Address string `json:"address"`
	Payload []byte `json:"payload"`
}

func main() {
	laddr := flag.String("laddr", ":15356", "Listen address")

	flag.Parse()

	upgrader := websocket.Upgrader{}
	msgr := messenger.New(0, true)
	addrDB := addressing.NewMACAddressDB()

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

				msg := messages.Message{}
				if err := messages.Decode(payload, &msg); err != nil {
					panic(err)
				}

				switch msg.Type {
				case messages.MessageTypeApplication:
					{
						addr, err := addrDB.AddAddress()
						if err != nil {
							panic(err)
						}

						resp, err := messages.Encode(messages.Acknowledgement{
							Mac: addr,
						})
						if err != nil {
							panic(err)
						}

						msgr.Broadcast(Message{
							Address: address,
							Payload: resp,
						})
					}
				}

			}

			wg.Done()
		}()

		go func() {
			for msg := range bus {
				if err := remote.WriteMessage(websocket.TextMessage, msg.(Message).Payload); err != nil {
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
