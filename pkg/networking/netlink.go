//go:build !(linux || darwin)
// +build !linux,!darwin

package networking

func RefreshMACAddress(name string) error {
	return nil // No-op
}
