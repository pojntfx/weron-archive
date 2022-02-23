//go:build linux || darwin

package networking

import (
	"github.com/songgao/water"
)

func AddPlatformParameters(config water.Config, name string) water.Config {
	config.PlatformSpecificParams.Name = name

	return config
}
