//go:build !(windows || linux || darwin)
// +build !windows,!linux,!darwin

package adapter

import (
	"github.com/songgao/water"
)

func addPlatformParameters(config water.Config, name string) water.Config {
	return config
}
