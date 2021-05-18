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
	dev := flag.String("dev", "weron0", "Device name to use")
	mtu := flag.Int("mtu", 1500, "MTU to set for device")

	// Parse flags
	flag.Parse()

	mac, err := net.ParseMAC(*macFlag)
	if err != nil {
		log.Fatal("could not parse MAC address:", err)
	}

	// Create core
	tap := core.NewTAPDevice(*dev, *mtu, mac)
	agent := core.NewAgent(mac.String(), func(mac string, frame []byte) {
		if err := tap.Write(frame); err != nil {
			log.Printf("couldn't write frame from %v to TAP device: %v", mac, err)

			return
		}
	})

	// Start
	c, _, err := websocket.Dial(context.Background(), *raddr, nil)
	if err != nil {
		log.Fatal("could not dial WebSocket:", err)
	}
	defer c.Close(websocket.StatusInternalError, "closing")

	if err := tap.Open(); err != nil {
		log.Fatal("could not open TAP device:", err)
	}

	go func() {
		for {
			frame, err := tap.Read()
			if err != nil {
				log.Println("could not read from TAP device, continuing:", err)

				continue
			}

			dst, err := core.GetDestinationMACFromEthernetFrame(frame)
			if err != nil {
				log.Println("could not get destination MAC from ethernet frame, continuing:", err)

				continue
			}

			if err := agent.WriteToDataChannel(dst.String(), frame); err != nil {
				log.Println("could not write frame to data channel:", err)

				continue
			}
		}
	}()

	log.Printf("connected to %v", *raddr)

	log.Fatal(services.Agent(agent, *community, mac, c))
}
