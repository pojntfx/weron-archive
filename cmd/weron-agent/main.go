package main

import (
	"context"
	"flag"
	"log"
	"net"

	"github.com/pojntfx/weron/pkg/core"
	"github.com/pojntfx/weron/pkg/services"
	"nhooyr.io/websocket"
)

func main() {
	// Define flags
	raddr := flag.String("raddr", "ws://localhost:15325", "Signaler address")
	community := flag.String("community", "cluster1", "Community to join")
	macFlag := flag.String("mac", "cc:0b:cf:23:22:0d", "MAC address to use")

	// Parse flags
	flag.Parse()

	mac, err := net.ParseMAC(*macFlag)
	if err != nil {
		log.Fatal("could not parse MAC address:", err)
	}

	// Create core
	agent := core.NewAgent(mac.String())

	// Start
	c, _, err := websocket.Dial(context.Background(), *raddr, nil)
	if err != nil {
		log.Fatal("could not dial WebSocket:", err)
	}
	defer c.Close(websocket.StatusInternalError, "closing")

	log.Printf("connected to %v", *raddr)

	log.Fatal(services.Agent(agent, *community, mac, c))
}
