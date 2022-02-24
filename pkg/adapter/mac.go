//go:build !(linux || darwin)
// +build !linux,!darwin

package adapter

func refreshMACAddress(name string) error {
	return nil // No-op
}
