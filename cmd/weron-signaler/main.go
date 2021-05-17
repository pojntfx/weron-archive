package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/pojntfx/weron/pkg/core"
	"github.com/pojntfx/weron/pkg/services"
)

func main() {
	// Define flags
	laddr := flag.String("laddr", ":15325", "Listen address")

	// Parse flags
	flag.Parse()

	// Create core
	signaler := core.NewSignaler()

	// Start
	log.Printf("listening on %v", *laddr)

	log.Fatal(http.ListenAndServe(*laddr, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		services.Signaler(signaler, rw, r)
	})))
}
