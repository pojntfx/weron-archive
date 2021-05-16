package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/pojntfx/weron/pkg/services"
)

func main() {
	laddr := flag.String("laddr", ":15325", "Listen address")

	flag.Parse()

	log.Printf("listening on %v", *laddr)

	log.Fatal(http.ListenAndServe(*laddr, services.Signaling(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {}))))
}
