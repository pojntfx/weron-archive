package networking

import (
	"net"

	"github.com/songgao/water"
	"github.com/vishvananda/netlink"
)

const (
	ethernetHeaderLength = 14
)

type NetworkAdapter struct {
	name string
	mtu  int
	mac  net.HardwareAddr
	tap  *water.Interface
}

func NewNetworkAdapter(name string, mtu int, mac net.HardwareAddr) *NetworkAdapter {
	return &NetworkAdapter{
		name: name,
		mtu:  mtu,
		mac:  mac,
	}
}

func (a *NetworkAdapter) Open() error {
	tap, err := water.New(water.Config{
		DeviceType: water.TAP,
		PlatformSpecificParams: water.PlatformSpecificParams{
			Name: a.name,
		},
	})
	if err != nil {
		return err
	}

	link, err := netlink.LinkByName(a.name)
	if err != nil {
		return err
	}

	if err := netlink.LinkSetMTU(link, a.mtu); err != nil {
		return err
	}

	if err := netlink.LinkSetHardwareAddr(link, a.mac); err != nil {
		return err
	}

	if err := netlink.LinkSetUp(link); err != nil {
		return err
	}

	a.tap = tap

	return nil
}

func (a *NetworkAdapter) Close() error {
	if a.tap != nil {
		return a.tap.Close()
	}

	return nil
}

func (a *NetworkAdapter) Write(frame []byte) error {
	_, err := a.tap.Write(frame)

	return err
}

func (a *NetworkAdapter) Read() ([]byte, error) {
	frame := make([]byte, a.mtu+ethernetHeaderLength)

	_, err := a.tap.Read(frame)

	return frame, err
}
