package adapter

import (
	"io"
	"net"

	"github.com/pojntfx/weron/pkg/config"
	"github.com/songgao/water"
)

const (
	ethernetHeaderLength = 14
)

type TAP struct {
	io.Writer

	name string

	tap *water.Interface
}

func NewTAP(name string) *TAP {
	return &TAP{
		name: name,
	}
}

func (a *TAP) Open() (string, error) {
	if a.tap != nil {
		return "", config.ErrAlreadyOpened
	}

	tap, err := water.New(
		addPlatformParameters(
			water.Config{
				DeviceType: water.TAP,
			},
			a.name,
		))
	if err != nil {
		return "", err
	}

	a.tap = tap

	if err := refreshMACAddress(a.tap.Name()); err != nil {
		return "", err
	}

	return a.tap.Name(), nil
}

func (a *TAP) Read(p []byte) (n int, err error) {
	if a.tap == nil {
		return -1, net.ErrClosed
	}

	return a.tap.Read(p)
}

func (a *TAP) Write(p []byte) (n int, err error) {
	if a.tap == nil {
		return -1, net.ErrClosed
	}

	return a.tap.Write(p)
}

func (a *TAP) Close() error {
	if a.tap == nil {
		return nil // No-op
	}

	if err := a.tap.Close(); err != nil {
		return err
	}

	a.tap = nil

	return nil
}

func (a *TAP) GetFrameSize() (int, error) {
	if a.tap == nil {
		return -1, net.ErrClosed
	}

	iface, err := net.InterfaceByName(a.tap.Name())
	if err != nil {
		return -1, err
	}

	return iface.MTU + ethernetHeaderLength, nil
}

func (a *TAP) GetMACAddress() (net.HardwareAddr, error) {
	if a.tap == nil {
		return nil, net.ErrClosed
	}

	iface, err := net.InterfaceByName(a.tap.Name())
	if err != nil {
		return nil, err
	}

	return iface.HardwareAddr, nil
}
