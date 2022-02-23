package networking

import (
	"errors"
	"io"
	"net"

	"github.com/songgao/water"
)

const (
	ethernetHeaderLength = 14
)

var (
	ErrAlreadyOpened = errors.New("already opened")
)

type NetworkAdapter struct {
	io.Writer

	name string

	tap *water.Interface
}

func NewNetworkAdapter(name string) *NetworkAdapter {
	return &NetworkAdapter{
		name: name,
	}
}

func (a *NetworkAdapter) Open() (string, error) {
	if a.tap != nil {
		return "", ErrAlreadyOpened
	}

	tap, err := water.New(
		AddPlatformParameters(
			water.Config{
				DeviceType: water.TAP,
			},
			a.name,
		))
	if err != nil {
		return "", err
	}

	a.tap = tap

	return a.tap.Name(), nil
}

func (a *NetworkAdapter) Read(p []byte) (n int, err error) {
	if a.tap == nil {
		return -1, net.ErrClosed
	}

	return a.tap.Read(p)
}

func (a *NetworkAdapter) Write(p []byte) (n int, err error) {
	if a.tap == nil {
		return -1, net.ErrClosed
	}

	return a.tap.Write(p)
}

func (a *NetworkAdapter) Close() error {
	if a.tap == nil {
		return nil // No-op
	}

	if err := a.tap.Close(); err != nil {
		return err
	}

	a.tap = nil

	return nil
}

func (a *NetworkAdapter) GetFrameSize() (int, error) {
	if a.tap == nil {
		return -1, net.ErrClosed
	}

	iface, err := net.InterfaceByName(a.tap.Name())
	if err != nil {
		return -1, err
	}

	return iface.MTU + ethernetHeaderLength, nil
}

func (a *NetworkAdapter) GetMACAddress() (net.HardwareAddr, error) {
	if a.tap == nil {
		return nil, net.ErrClosed
	}

	iface, err := net.InterfaceByName(a.tap.Name())
	if err != nil {
		return nil, err
	}

	return iface.HardwareAddr, nil
}
