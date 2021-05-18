package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"

	"github.com/pojntfx/weron/pkg/core"
	"github.com/pojntfx/weron/pkg/services"
)

func main() {
	// Define flags
	laddr := flag.String("laddr", ":15325", "Listen address; the port can also be set using the PORT env variable.")

	// Parse flags
	flag.Parse()

	addr, err := net.ResolveTCPAddr("tcp", *laddr)
	if err != nil {
		log.Fatal("could not resolve address:", err)
	}

	// Parse PORT env variable for Heroku compatibility
	if portEnv := os.Getenv("PORT"); portEnv != "" {
		port, err := strconv.Atoi(portEnv)
		if err != nil {
			log.Fatal("could not parse port:", port)
		}

		addr.Port = port
	}

	// Create core
	signaler := core.NewSignaler()

	// Start
	log.Printf("listening on %v", addr.String())

	log.Fatal(http.ListenAndServe(addr.String(), http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		services.Signaler(signaler, rw, r)
	})))
}
