package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/pojntfx/weron/pkg/cache"
	"github.com/pojntfx/weron/pkg/services"
)

func main() {
	laddr := flag.String("laddr", ":15325", "Listen address")

	flag.Parse()

	log.Printf("listening on %v", *laddr)

	network := cache.NewNetwork()

	log.Fatal(http.ListenAndServe(*laddr, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		services.Signaling(network, rw, r)
	})))
}
