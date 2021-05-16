package services

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	api "github.com/pojntfx/weron/pkg/api/websockets/v1"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

func Signaling(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(rw, r, nil)
		if err != nil {
			log.Println("could not accept on WebSocket:", err)

			return
		}

		msg := "internal error"
		defer func() {
			log.Println("could not continue in handler:", msg)

			c.Close(websocket.StatusInternalError, msg)
		}()

	l:
		for {
			var v interface{}
			if err := wsjson.Read(r.Context(), c, &v); err != nil {
				msg = "could not read JSON from WebSocket: " + err.Error()

				break
			}

			vv, ok := v.(map[string]interface{})
			if !ok {
				msg = "could not read cast message from WebSocket"

				break
			}

			t, ok := vv["type"]
			if !ok {
				msg = "could not read message type from WebSocket"

				break
			}

			tt, err := strconv.Atoi(fmt.Sprintf("%v", t))
			if err != nil {
				msg = "could not cast message type from WebSocket: " + err.Error()

				break
			}

			switch tt {
			case api.TypeApplication:
				log.Println("received application: ", v)
			case api.TypeAcceptance:
				log.Println("received acceptance: ", v)
			case api.TypeRejection:
				log.Println("received rejection: ", v)
			case api.TypeReady:
				log.Println("received ready: ", v)
			case api.TypeIntroduction:
				log.Println("received introduction: ", v)
			default:
				msg = fmt.Sprintf("could not handle message type, received unknown message type \"%v\"", tt)

				break l
			}
		}

		next.ServeHTTP(rw, r)
	})
}
