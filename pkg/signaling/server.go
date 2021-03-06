package signaling

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	api "github.com/pojntfx/weron/pkg/api/websockets/v1"
	"github.com/pojntfx/weron/pkg/config"
	"nhooyr.io/websocket"
)

const (
	invalidCommunity = "-1"
	invalidMAC       = "-1"
)

type SignalingServer struct {
	conns map[string]*websocket.Conn

	lock sync.Mutex

	ctx     context.Context
	timeout time.Duration

	onApplication func(community string, mac string, conn *websocket.Conn) error
	onRejection   func(community string, mac string, conn *websocket.Conn) error
	onAcceptance  func(community string, mac string, conn *websocket.Conn) error
	onExited      func(community string, mac string, err error) error
	onReady       func(community string, mac string) error
	onExchange    func(community string, mac string, exchange api.Exchange) error
}

func NewSignalingServer(
	ctx context.Context,
	timeout time.Duration,

	onApplication func(community string, mac string, conn *websocket.Conn) error,
	onRejection func(community string, mac string, conn *websocket.Conn) error,
	onAcceptance func(community string, mac string, conn *websocket.Conn) error,
	onExited func(community string, mac string, err error) error,
	onReady func(community string, mac string) error,
	onExchange func(community string, mac string, exchange api.Exchange) error,
) *SignalingServer {
	return &SignalingServer{
		conns: map[string]*websocket.Conn{},

		ctx:     ctx,
		timeout: timeout,

		onApplication: onApplication,
		onRejection:   onRejection,
		onAcceptance:  onAcceptance,
		onExited:      onExited,
		onReady:       onReady,
		onExchange:    onExchange,
	}
}

func (s *SignalingServer) HandleConn(conn *websocket.Conn) error {
	// Create a unique ID for the connection
	id := uuid.New()

	// Register connection
	s.lock.Lock()
	s.conns[id.String()] = conn
	s.lock.Unlock()

	fatal := make(chan error)

	// Community and MAC address for this connection
	community := invalidCommunity
	mac := invalidMAC

	keepalive := time.NewTicker(s.timeout)
	defer keepalive.Stop()
	go func() {
		for range keepalive.C {
			ctx, cancel := context.WithTimeout(s.ctx, s.timeout)

			err := conn.Ping(ctx)
			cancel()

			// If ping failed, remove connection
			if err != nil {
				fatal <- err

				return
			}
		}
	}()

	go func() {
		for {
			// Read message from connection
			_, data, err := conn.Read(s.ctx)
			if err != nil {
				fatal <- err

				return
			}

			// Parse message
			var v api.Message
			if err := json.Unmarshal(data, &v); err != nil {
				fatal <- err

				return
			}

			// Handle different message types
			switch v.Type {
			// Admission
			case api.TypeApplication:
				// Prevent duplicate application
				if community != invalidCommunity || mac != invalidMAC {
					fatal <- config.ErrAlreadyApplied

					return
				}

				// Cast to application
				var application api.Application
				if err := json.Unmarshal(data, &application); err != nil {
					fatal <- fmt.Errorf("%v: %v", config.ErrCouldNotUnmarshalJSON, err.Error())

					return
				}

				// Validate incoming community and MAC address
				incomingMAC, err := net.ParseMAC(application.Mac)
				if application.Community == invalidCommunity || application.Mac == invalidMAC || err != nil {
					msg := config.ErrCouldNotHandleApplication.Error() + ": " + config.ErrInvalidCommunityOrMACAddress.Error()
					if err != nil {
						msg += ": " + err.Error()
					}

					fatal <- errors.New(msg)

					return
				}

				// Handle application
				if err := s.onApplication(application.Community, incomingMAC.String(), conn); err != nil {
					msg := config.ErrCouldNotHandleApplication.Error() + ": " + err.Error()

					// Send rejection on error
					if err := s.onRejection(community, mac, conn); err != nil {
						msg += ": " + err.Error()
					}

					fatal <- errors.New(msg)

					return
				}

				// Set community and MAC address for this connection
				community = application.Community
				mac = incomingMAC.String()

				// Send acceptance
				if err := s.onAcceptance(community, mac, conn); err != nil {
					fatal <- err

					return
				}
			case api.TypeReady:
				// Handle ready
				if err := s.onReady(community, mac); err != nil {
					fatal <- fmt.Errorf("%v: %v", config.ErrCouldNotHandleReady, err)

					return
				}

			// Exchange
			case api.TypeOffer:
				fallthrough
			case api.TypeAnswer:
				fallthrough
			case api.TypeCandidate:
				// Cast to exchange
				var exchange api.Exchange
				if err := json.Unmarshal(data, &exchange); err != nil {
					fatal <- fmt.Errorf("%v: %v", config.ErrCouldNotUnmarshalJSON, err.Error())

					return
				}

				// Validate incoming MAC address
				incomingMAC, err := net.ParseMAC(exchange.Mac)
				if err != nil {
					fatal <- fmt.Errorf("%v: %v: %v", config.ErrCouldNotHandleApplication, config.ErrInvalidMACAddress, err.Error())

					return
				}
				exchange.Mac = incomingMAC.String()

				// Handle exchange
				if err := s.onExchange(community, mac, exchange); err != nil {
					fatal <- fmt.Errorf("%v: %v", config.ErrCouldNotHandleReady, err.Error())

					return
				}

			// Discharge
			case api.TypeExited:
				// Handle exited
				if err := s.onExited(community, mac, nil); err != nil {
					fatal <- fmt.Errorf("%v: %v", config.ErrCouldNotHandleExited, err)

					return
				}

				// "Regular" exit
				fatal <- nil

			// Other messages
			default:
				fatal <- fmt.Errorf("%v: \"%v\"", config.ErrUnknownMessageType, v.Type)

				return
			}
		}
	}()

	err := <-fatal

	// Remove connection
	s.lock.Lock()
	delete(s.conns, id.String())
	s.lock.Unlock()

	// Handle exited; ignore the error as it might be a no-op
	_ = s.onExited(community, mac, err)

	// Handle error during application; the connection might not be added to any community yet, so close directly
	if community == invalidCommunity && mac == invalidMAC && err != nil {
		// Close the connection (irregular)
		msg := err.Error()
		if len(msg) >= 123 {
			msg = msg[:122] // string max is 123 in WebSockets
		}

		if err := conn.Close(websocket.StatusProtocolError, msg); err != nil {
			return err
		}
	}

	return err
}

func (s *SignalingServer) Close() []error {
	s.lock.Lock()
	defer s.lock.Unlock()

	errors := []error{}

	for _, peer := range s.conns {
		if err := peer.Close(websocket.StatusGoingAway, "shutting down"); err != nil {
			errors = append(errors, err)
		}
	}

	return errors
}
