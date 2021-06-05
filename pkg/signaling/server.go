package signaling

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/google/uuid"
	api "github.com/pojntfx/weron/pkg/api/websockets/v1"
	"nhooyr.io/websocket"
)

const (
	invalidCommunity = "-1"
	invalidMAC       = "-1"
)

var (
	ErrorAlreadyApplied               = errors.New("cannot apply multiple times")
	ErrorCouldNotUnmarshalJSON        = errors.New("could not unmarshal JSON")
	ErrorCouldNotHandleApplication    = errors.New("could not handle application")
	ErrorInvalidCommunityOrMACAddress = errors.New("invalid community or MAC address")
	ErrorCouldNotHandleReady          = errors.New("could not handle ready")
	ErrorInvalidMACAddress            = errors.New("invalid MAC address")
	ErrorCouldNotHandleExited         = errors.New("could not handle exited")
)

type SignalingServer struct {
	conns map[string]*websocket.Conn

	lock sync.Mutex

	onApplication func(community string, mac string, conn *websocket.Conn) error
	onRejection   func(conn *websocket.Conn) error
	onAcceptance  func(conn *websocket.Conn) error
	onExited      func(community string, mac string, err error) error
	onReady       func(community string, mac string) error
	onExchange    func(community string, mac string, exchange api.Exchange) error
}

func NewSignalingServer(
	onApplication func(community string, mac string, conn *websocket.Conn) error,
	onRejection func(conn *websocket.Conn) error,
	onAcceptance func(conn *websocket.Conn) error,
	onExited func(community string, mac string, err error) error,
	onReady func(community string, mac string) error,
	onExchange func(community string, mac string, exchange api.Exchange) error,
) *SignalingServer {
	return &SignalingServer{
		conns: map[string]*websocket.Conn{},

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

	go func() {
		for {
			// Read message from connection
			_, data, err := conn.Read(context.Background())
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
					fatal <- ErrorAlreadyApplied

					return
				}

				// Cast to application
				var application api.Application
				if err := json.Unmarshal(data, &application); err != nil {
					fatal <- fmt.Errorf("%v: %v", ErrorCouldNotUnmarshalJSON, err.Error())

					return
				}

				// Validate incoming community and MAC address
				incomingMAC, err := net.ParseMAC(application.Mac)
				if application.Community == invalidCommunity || application.Mac == invalidMAC || err != nil {
					msg := ErrorCouldNotHandleApplication.Error() + ": " + ErrorInvalidCommunityOrMACAddress.Error()
					if err != nil {
						msg += ": " + err.Error()
					}

					fatal <- errors.New(msg)

					return
				}

				// Handle application
				if err := s.onApplication(application.Community, incomingMAC.String(), conn); err != nil {
					msg := ErrorCouldNotHandleApplication.Error() + ": " + err.Error()

					// Send rejection on error
					if err := s.onRejection(conn); err != nil {
						msg += ": " + err.Error()
					}

					fatal <- errors.New(msg)

					return
				}

				// Set community and MAC address for this connection
				community = application.Community
				mac = incomingMAC.String()

				// Send acceptance
				if err := s.onAcceptance(conn); err != nil {
					fatal <- err

					return
				}
			case api.TypeReady:
				// Handle ready
				if err := s.onReady(community, mac); err != nil {
					fatal <- fmt.Errorf("%v: %v", ErrorCouldNotHandleReady, err)

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
					fatal <- fmt.Errorf("%v: %v", ErrorCouldNotUnmarshalJSON, err.Error())

					return
				}

				// Validate incoming MAC address
				incomingMAC, err := net.ParseMAC(exchange.Mac)
				if err != nil {
					fatal <- fmt.Errorf("%v: %v: %v", ErrorCouldNotHandleApplication, ErrorInvalidMACAddress, err.Error())

					return
				}
				exchange.Mac = incomingMAC.String()

				// Handle exchange
				if err := s.onExchange(community, mac, exchange); err != nil {
					fatal <- fmt.Errorf("%v: %v", ErrorCouldNotHandleReady, err.Error())

					return
				}

			// Discharge
			case api.TypeExited:
				// Handle exited
				if err := s.onExited(community, mac, nil); err != nil {
					fatal <- fmt.Errorf("%v: %v", ErrorCouldNotHandleExited, err)

					return
				}

				// "Regular" exit
				fatal <- nil

			// Other messages
			default:
				fatal <- fmt.Errorf("%v: \"%v\"", ErrorUnknownMessageType, v.Type)

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
