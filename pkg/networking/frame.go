package networking

import (
	"net"

	"github.com/mdlayher/ethernet"
)

func GetDestination(frame []byte) (net.HardwareAddr, error) {
	var parsedFrame ethernet.Frame

	if err := parsedFrame.UnmarshalBinary(frame); err != nil {
		return nil, err
	}

	return parsedFrame.Destination, nil
}
