package addressing

import (
	"crypto/rand"
	"errors"
	"net"
)

type MACAddressDB struct {
	addresses []string
}

func NewMACAddressDB() *MACAddressDB {
	return &MACAddressDB{
		addresses: []string{},
	}
}

func (d *MACAddressDB) AddAddress() (string, error) {
	for {
		buf := make([]byte, 6)
		var mac net.HardwareAddr

		if _, err := rand.Read(buf); err != nil {
			return "", err
		}

		// Set the local bit
		buf[0] |= 2

		mac = append(mac, buf[0], buf[1], buf[2], buf[3], buf[4], buf[5])

		// Regenerate if duplicate
		if d.addressExists(mac.String()) {
			continue
		}

		d.addresses = append(d.addresses, mac.String())

		return mac.String(), nil
	}
}

func (d *MACAddressDB) RemoveAddress(address string) error {
	for i, addr := range d.addresses {
		if addr == address {
			d.addresses = append(d.addresses[:i], d.addresses[i+1:]...)

			return nil
		}
	}

	return errors.New("address not known")
}

func (d *MACAddressDB) addressExists(address string) bool {
	for _, addr := range d.addresses {
		if addr == address {
			return true
		}
	}

	return false
}
