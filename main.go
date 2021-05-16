package main

import (
	"flag"
	"log"
	"net/http"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

func main() {
	laddr := flag.String("laddr", ":15325", "Listen address")

	flag.Parse()

	log.Printf("listening on %v", *laddr)

	log.Fatal(http.ListenAndServe(*laddr, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
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

				return
			}

			log.Println("received: ", v)
		}
	})))
}
