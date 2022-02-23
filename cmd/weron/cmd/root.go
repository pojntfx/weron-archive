package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "weron",
	Short: "Layer 2 overlay networks based on WebRTC",
	Long: `weron provides lean, fast & secure layer 2 overlay networks based on WebRTC.

Find more information at:
https://github.com/pojntfx/weron.`,
}

func Execute() error {
	return rootCmd.Execute()
}
