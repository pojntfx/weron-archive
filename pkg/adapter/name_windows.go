//go:build windows
// +build windows

package adapter

import (
	"github.com/songgao/water"
)

func addPlatformParameters(config water.Config, name string) water.Config {
	config.PlatformSpecificParams.InterfaceName = name

	return config
}
