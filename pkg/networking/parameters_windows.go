//go:build windows
// +build windows

package networking

import (
	"github.com/songgao/water"
)

func AddPlatformParameters(config water.Config, name string) water.Config {
	config.PlatformSpecificParams.InterfaceName = name

	return config
}
