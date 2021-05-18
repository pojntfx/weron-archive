package core

import (
	"net"

	"github.com/mdlayher/ethernet"
)

func GetDestinationMACFromEthernetFrame(frame []byte) (net.HardwareAddr, error) {
	var parsed ethernet.Frame

	if err := parsed.UnmarshalBinary(frame); err != nil {
		return nil, err
	}

	return parsed.Destination, nil
}
