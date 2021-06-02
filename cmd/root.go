package cmd

import (
	"log"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "weron",
	Short: "Layer 2 overlay networks based on WebRTC.",
	Long: `weron provides lean, fast & secure layer 2 overlay networks based on WebRTC.

For more information, please visit https://github.com/pojntfx/weron.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func init() {
	rootCmd.AddCommand(joinCmd)
	rootCmd.AddCommand(signalCmd)
}
