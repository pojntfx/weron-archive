package transport

import (
	"errors"
	"net"
	"sync"

	"github.com/pion/webrtc/v3"
	"github.com/pojntfx/weron/pkg/config"
)

const (
	dataChannelName = "data"
)

var (
	broadcastMAC = net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}.String()

	ErrorConnectionHasNoDataChannel = errors.New("connection has no data channel")
)

type WebRTCManager struct {
	peers map[string]*peer

	ice  []webrtc.ICEServer
	lock sync.Mutex

	onCandidate        func(mac string, i webrtc.ICECandidate)
	onReceive          func(mac string, frame []byte)
	onOffer            func(mac string, o webrtc.SessionDescription)
	onAnswer           func(mac string, o webrtc.SessionDescription)
	onDataChannelOpen  func(mac string)
	onDataChannelClose func(mac string)
}

func NewWebRTCManager(
	ice []webrtc.ICEServer,

	onCandidate func(mac string, i webrtc.ICECandidate),
	onReceive func(mac string, frame []byte),
	onOffer func(mac string, o webrtc.SessionDescription),
	onAnswer func(mac string, o webrtc.SessionDescription),
	onDataChannelOpen func(mac string),
	onDataChannelClose func(mac string),
) *WebRTCManager {
	return &WebRTCManager{
		peers: map[string]*peer{},

		ice: ice,

		onCandidate:        onCandidate,
		onReceive:          onReceive,
		onOffer:            onOffer,
		onAnswer:           onAnswer,
		onDataChannelOpen:  onDataChannelOpen,
		onDataChannelClose: onDataChannelClose,
	}
}

type peer struct {
	connection *webrtc.PeerConnection
	channel    *webrtc.DataChannel
	candidates []webrtc.ICECandidateInit
}

func (m *WebRTCManager) HandleIntroduction(mac string) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	c, err := m.createPeer(mac)
	if err != nil {
		return err
	}

	if err := m.createDataChannel(mac, c); err != nil {
		return err
	}

	offer, err := c.CreateOffer(nil)
	if err != nil {
		if err := m.HandleResignation(mac); err != nil {
			return err
		}

		return err
	}

	if err := c.SetLocalDescription(offer); err != nil {
		return err
	}

	m.onOffer(mac, offer)

	return nil
}

func (m *WebRTCManager) HandleOffer(mac string, offer webrtc.SessionDescription) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	c, err := m.createPeer(mac)
	if err != nil {
		return err
	}

	if err := m.subscribeToDataChannels(mac, c); err != nil {
		return err
	}

	if err := c.SetRemoteDescription(offer); err != nil {
		return err
	}

	// No need to loop over queued candidates here, as the peer
	// has just been created above so there can't be any

	answer, err := c.CreateAnswer(nil)
	if err != nil {
		if err := m.HandleResignation(mac); err != nil {
			return err
		}

		return err
	}

	if err := c.SetLocalDescription(answer); err != nil {
		return err
	}

	m.onAnswer(mac, answer)

	return nil
}

func (m *WebRTCManager) HandleCandidate(mac string, candidate webrtc.ICECandidateInit) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	c, err := m.getConnection(mac)
	if err != nil {
		return err
	}

	// If remote description has been set, continue
	if c.connection.RemoteDescription() != nil {
		return c.connection.AddICECandidate(candidate)
	}

	// If remote description has not been set, queue it
	c.candidates = append(c.candidates, candidate)

	return nil
}

func (m *WebRTCManager) HandleAnswer(mac string, answer webrtc.SessionDescription) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	c, err := m.getConnection(mac)
	if err != nil {
		return err
	}

	if err := c.connection.SetRemoteDescription(answer); err != nil {
		return err
	}

	// Add queued candidates if there are any
	if len(c.candidates) > 0 {
		for _, candidate := range c.candidates {
			if err := c.connection.AddICECandidate(candidate); err != nil {
				return err
			}
		}

		// Clear now-added candidates
		c.candidates = []webrtc.ICECandidateInit{}
	}

	return nil
}

func (m *WebRTCManager) HandleResignation(mac string) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	c, err := m.getConnection(mac)
	if err != nil {
		return err
	}

	delete(m.peers, mac)

	return c.connection.Close()
}

func (m *WebRTCManager) Write(mac string, frame []byte) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	peers := []*peer{}
	if mac == broadcastMAC {
		for candidate, p := range m.peers {
			if candidate != mac {
				peers = append(peers, p)
			}
		}
	} else {
		p, err := m.getConnection(mac)
		if err != nil {
			return err
		}

		peers = append(peers, p)
	}

	for _, p := range peers {
		if p.channel == nil {
			return ErrorConnectionHasNoDataChannel
		}

		if err := p.channel.Send(frame); err != nil {
			if err := m.HandleResignation(mac); err != nil {
				return err
			}

			return nil
		}
	}

	return nil
}

func (m *WebRTCManager) Close() []error {
	m.lock.Lock()
	defer m.lock.Unlock()

	errors := []error{}

	for mac := range m.peers {
		if err := m.HandleResignation(mac); err != nil {
			errors = append(errors, err)
		}
	}

	return errors
}

func (m *WebRTCManager) createPeer(mac string) (*webrtc.PeerConnection, error) {
	c, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: m.ice,
	})
	if err != nil {
		return nil, err
	}

	m.peers[mac] = &peer{
		connection: c,
		candidates: []webrtc.ICECandidateInit{},
	}

	c.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i != nil {
			m.onCandidate(mac, *i)
		}
	})

	return c, nil
}

func (m *WebRTCManager) createDataChannel(mac string, c *webrtc.PeerConnection) error {
	dc, err := c.CreateDataChannel(dataChannelName, nil)
	if err != nil {
		if err := m.HandleResignation(mac); err != nil {
			return err
		}

		return err
	}

	dc.OnOpen(func() {
		m.lock.Lock()
		defer m.lock.Unlock()

		peer := m.peers[mac]
		peer.channel = dc

		m.peers[mac] = peer

		m.onDataChannelOpen(mac)
	})

	dc.OnClose(func() {
		m.onDataChannelClose(mac)

		_ = m.HandleResignation(mac)
	})

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		go m.onReceive(mac, msg.Data)
	})

	return nil
}

func (m *WebRTCManager) subscribeToDataChannels(mac string, c *webrtc.PeerConnection) error {
	c.OnDataChannel(func(dc *webrtc.DataChannel) {
		dc.OnOpen(func() {
			m.lock.Lock()
			defer m.lock.Unlock()

			peer := m.peers[mac]
			peer.channel = dc

			m.peers[mac] = peer

			m.onDataChannelOpen(mac)
		})

		dc.OnClose(func() {
			m.onDataChannelClose(mac)

			_ = m.HandleResignation(mac)
		})

		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			go m.onReceive(mac, msg.Data)
		})
	})

	return nil
}

func (m *WebRTCManager) getConnection(mac string) (*peer, error) {
	peers, ok := m.peers[mac]
	if !ok {
		return &peer{}, config.ErrConnectionDoesNotExist
	}

	return peers, nil
}
