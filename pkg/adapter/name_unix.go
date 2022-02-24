//go:build linux || darwin
// +build linux darwin

package adapter

import (
	"github.com/songgao/water"
)

func addPlatformParameters(config water.Config, name string) water.Config {
	config.PlatformSpecificParams.Name = name

	return config
}
