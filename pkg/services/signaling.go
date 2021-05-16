package services

import (
	"log"
	"net/http"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

func Signaling(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(rw, r, nil)
		if err != nil {
			log.Println("could not accept on WebSocket: ", err)

			return
		}
		defer c.Close(websocket.StatusInternalError, "internal error")

		for {
			var v interface{}
			if err := wsjson.Read(r.Context(), c, &v); err != nil {
				log.Println("could not read JSON from WebSocket: ", err)

				break
			}

			log.Println("received: ", v)
		}

		next.ServeHTTP(rw, r)
	})
}
