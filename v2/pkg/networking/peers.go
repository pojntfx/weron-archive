package networking

import (
	"sync"

	"github.com/pion/webrtc/v3"
)

const (
	dataChannelName = "data"
)

type PeerManager struct {
	peers map[string]peer

	ice  []webrtc.ICEServer
	lock sync.Mutex

	onCandidate func(mac string, i *webrtc.ICECandidate)
	onReceive   func(mac string, frame []byte)
	onOffer     func(mac string, o webrtc.SessionDescription)
	onAnswer    func(mac string, o webrtc.SessionDescription)
}

type peer struct {
	connection *webrtc.PeerConnection
	channel    *webrtc.DataChannel
}

func (m *PeerManager) HandleIntroduction(mac string) error {
	c, err := m.createPeer(mac)
	if err != nil {
		return err
	}

	if err := m.createDataChannel(mac, c); err != nil {
		return err
	}

	offer, err := c.CreateOffer(nil)
	if err != nil {
		_ = m.HandleResignation(mac)

		return err
	}
	c.SetLocalDescription(offer)

	m.onOffer(mac, offer)

	return nil
}

func (m *PeerManager) HandleOffer(mac string, offer webrtc.SessionDescription) error {
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

	answer, err := c.CreateAnswer(nil)
	if err != nil {
		_ = m.HandleResignation(mac)

		return err
	}
	c.SetLocalDescription(answer)

	m.onAnswer(mac, answer)

	return nil
}

func (m *PeerManager) createPeer(mac string) (*webrtc.PeerConnection, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	c, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: m.ice,
	})
	if err != nil {
		return nil, err
	}

	m.peers[mac] = peer{
		connection: c,
	}

	c.OnICECandidate(func(i *webrtc.ICECandidate) {
		m.onCandidate(mac, i)
	})

	return c, nil
}

func (m *PeerManager) createDataChannel(mac string, c *webrtc.PeerConnection) error {
	dc, err := c.CreateDataChannel(dataChannelName, nil)
	if err != nil {
		_ = m.HandleResignation(mac)

		return err
	}

	dc.OnOpen(func() {
		m.lock.Lock()
		defer m.lock.Unlock()

		peer := m.peers[mac]
		peer.channel = dc

		m.peers[mac] = peer
	})

	dc.OnClose(func() {
		_ = m.HandleResignation(mac)
	})

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		m.onReceive(mac, msg.Data)
	})

	return nil
}

func (m *PeerManager) subscribeToDataChannels(mac string, c *webrtc.PeerConnection) error {
	c.OnDataChannel(func(dc *webrtc.DataChannel) {
		dc.OnOpen(func() {
			m.lock.Lock()
			defer m.lock.Unlock()

			peer := m.peers[mac]
			peer.channel = dc

			m.peers[mac] = peer
		})

		dc.OnClose(func() {
			_ = m.HandleResignation(mac)
		})

		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			m.onReceive(mac, msg.Data)
		})
	})

	return nil
}
