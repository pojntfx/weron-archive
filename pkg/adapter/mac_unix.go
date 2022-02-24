//go:build linux || darwin
// +build linux darwin

package adapter

import "github.com/vishvananda/netlink"

func refreshMACAddress(name string) error {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return err
	}

	return netlink.LinkSetHardwareAddr(link, link.Attrs().HardwareAddr)
}
