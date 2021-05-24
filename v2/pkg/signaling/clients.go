package signaling

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/pion/webrtc/v3"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	api "github.com/pojntfx/weron/pkg/api/websockets/v1"
)

type SignalingClient struct {
	conn *websocket.Conn

	mac       string
	community string
	timeout   time.Duration

	onIntroduction func(mac string)
	onOffer        func(mac string, o webrtc.SessionDescription)
	onCandidate    func(mac string, i webrtc.ICECandidateInit)
	onAnswer       func(mac string, o webrtc.SessionDescription)
	onResignation  func(mac string, blocked bool)
	onEncrypt      func(data []byte) ([]byte, error)
	onDecrypt      func(data []byte) ([]byte, error)
}

func NewSignalingClient(
	conn *websocket.Conn,

	mac string,
	community string,
	timeout time.Duration,

	onIntroduction func(mac string),
	onOffer func(mac string, o webrtc.SessionDescription),
	onCandidate func(mac string, i webrtc.ICECandidateInit),
	onAnswer func(mac string, o webrtc.SessionDescription),
	onResignation func(mac string, blocked bool),
	onEncrypt func(data []byte) ([]byte, error),
	onDecrypt func(data []byte) ([]byte, error),
) *SignalingClient {
	return &SignalingClient{
		conn: conn,

		mac:       mac,
		community: community,
		timeout:   timeout,

		onIntroduction: onIntroduction,
		onOffer:        onOffer,
		onCandidate:    onCandidate,
		onAnswer:       onAnswer,
		onResignation:  onResignation,
		onEncrypt:      onEncrypt,
		onDecrypt:      onDecrypt,
	}
}

func (c *SignalingClient) Run() error {
	fatal := make(chan error)
	ready := make(chan struct{})

	keepalive := time.NewTicker(10 * time.Second)
	defer keepalive.Stop()

	go func() {
		for range keepalive.C {
			ctx, cancel := context.WithTimeout(context.Background(), c.timeout)

			err := c.conn.Ping(ctx)
			cancel()

			// If ping failed, reconnect
			if err != nil {
				fatal <- err

				return
			}
		}
	}()

	go func() {
		for {
			// Read message from connection
			_, data, err := c.conn.Read(context.Background())
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
			case api.TypeRejection:
				fatal <- errors.New("could not join community: MAC address rejected")

				return
			case api.TypeAcceptance:
				ready <- struct{}{}
			case api.TypeIntroduction:
				// Cast to introduction
				var introduction api.Introduction
				if err := json.Unmarshal(data, &introduction); err != nil {
					fatal <- err

					return
				}

				c.onIntroduction(introduction.Mac)
			case api.TypeOffer:
				// Cast to exchange
				var exchange api.Exchange
				if err := json.Unmarshal(data, &exchange); err != nil {
					fatal <- err

					return
				}

				// Decrypt payload
				payload, err := c.onDecrypt(exchange.Payload)
				if err != nil {
					c.onResignation(exchange.Mac, true)

					return
				}

				// Parse offer
				var offer webrtc.SessionDescription
				if err := json.Unmarshal(payload, &offer); err != nil {
					fatal <- err

					return
				}

				c.onOffer(exchange.Mac, offer)
			case api.TypeCandidate:
				// Cast to exchange
				var exchange api.Exchange
				if err := json.Unmarshal(data, &exchange); err != nil {
					fatal <- err

					return
				}

				// Decrypt payload
				payload, err := c.onDecrypt(exchange.Payload)
				if err != nil {
					c.onResignation(exchange.Mac, true)

					return
				}

				c.onCandidate(exchange.Mac, webrtc.ICECandidateInit{Candidate: string(payload)})
			case api.TypeAnswer:
				// Cast to exchange
				var exchange api.Exchange
				if err := json.Unmarshal(data, &exchange); err != nil {
					fatal <- err

					return
				}

				// Decrypt payload
				payload, err := c.onDecrypt(exchange.Payload)
				if err != nil {
					c.onResignation(exchange.Mac, true)

					return
				}

				// Parse answer
				var answer webrtc.SessionDescription
				if err := json.Unmarshal(payload, &answer); err != nil {
					fatal <- err

					return
				}

				c.onAnswer(exchange.Mac, answer)

			// Discharge
			case api.TypeResignation:
				// Cast to resignation
				var resignation api.Resignation
				if err := json.Unmarshal(data, &resignation); err != nil {
					fatal <- err

					return
				}

				c.onResignation(resignation.Mac, false)

			// Other messages
			default:
				fatal <- fmt.Errorf("could not handle message type, received unknown message type \"%v\"", v.Type)

				return
			}
		}
	}()

	go func() {
		// Send application
		if err := wsjson.Write(context.Background(), c.conn, api.NewApplication(c.community, c.mac)); err != nil {
			fatal <- err

			return
		}

		<-ready

		// Send ready
		readyMessage := api.NewReady()

		if err := wsjson.Write(context.Background(), c.conn, readyMessage); err != nil {
			fatal <- err
		}
	}()

	err := <-fatal

	return err
}

func (c *SignalingClient) SignalCandidate(mac string, i webrtc.ICECandidate) error {
	// Encrypt payload
	payload, err := c.onEncrypt([]byte(i.ToJSON().Candidate))
	if err != nil {
		return err
	}

	return wsjson.Write(context.Background(), c.conn, api.NewCandidate(mac, payload))
}

func (c *SignalingClient) SignalOffer(mac string, o webrtc.SessionDescription) error {
	data, err := json.Marshal(o)
	if err != nil {
		return err
	}

	// Encrypt payload
	payload, err := c.onEncrypt(data)
	if err != nil {
		return err
	}

	return wsjson.Write(context.Background(), c.conn, api.NewOffer(mac, payload))
}

func (c *SignalingClient) SignalAnswer(mac string, o webrtc.SessionDescription) error {
	data, err := json.Marshal(o)
	if err != nil {
		return err
	}

	// Encrypt payload
	payload, err := c.onEncrypt(data)
	if err != nil {
		return err
	}

	return wsjson.Write(context.Background(), c.conn, api.NewAnswer(mac, payload))
}

func (c *SignalingClient) Close() error {
	if c.conn != nil {
		return c.conn.Close(websocket.StatusGoingAway, "shutting down")
	}

	return nil
}
