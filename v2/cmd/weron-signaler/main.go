package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"

	api "github.com/pojntfx/weron/pkg/api/websockets/v1"
	"github.com/pojntfx/weron/v2/pkg/networking"
	"github.com/pojntfx/weron/v2/pkg/signaling"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
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
	communities := networking.NewCommunitiesManager(
		func(mac string, conn *websocket.Conn) error {
			return wsjson.Write(context.Background(), conn, api.NewIntroduction(mac))
		},
		func(exchange api.Exchange, conn *websocket.Conn) error {
			return wsjson.Write(context.Background(), conn, exchange)
		},
		func(mac string, conn *websocket.Conn) error {
			return wsjson.Write(context.Background(), conn, api.NewResignation(mac))
		},
	)
	signaler := signaling.NewSignalingServer(
		func(community string, mac string, conn *websocket.Conn) error {
			return communities.HandleApplication(community, mac, conn)
		},
		func(conn *websocket.Conn) error {
			return wsjson.Write(context.Background(), conn, api.NewRejection())
		},
		func(conn *websocket.Conn) error {
			return wsjson.Write(context.Background(), conn, api.NewAcceptance())
		},
		func(community, mac string, err error) error {
			return communities.HandleExited(community, mac, err)
		},
		func(community, mac string) error {
			return communities.HandleReady(community, mac)
		},
		func(community, mac string, exchange api.Exchange) error {
			return communities.HandleExchange(community, mac, exchange)
		},
	)

	// Start
	log.Printf("listening on %v", addr.String())

	log.Fatal(http.ListenAndServe(addr.String(), http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(rw, r, nil)
		if err != nil {
			log.Println("could not accept on WebSocket:", err)

			return
		}

		log.Println("client connected")

		go func() {
			if err := signaler.HandleConn(conn); err != nil {
				log.Println("client disconnected:", err)

				return
			}
		}()
	})))
}
