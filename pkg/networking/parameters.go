//go:build !(windows || linux || darwin)
// +build !windows,!linux,!darwin

package networking

import (
	"github.com/songgao/water"
)

func AddPlatformParameters(config water.Config, name string) water.Config {
	return config
}
