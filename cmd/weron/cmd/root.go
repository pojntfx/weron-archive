package cmd

import (
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	verboseFlag = "verbose"
)

var rootCmd = &cobra.Command{
	Use:   "weron",
	Short: "Layer 2 overlay networks based on WebRTC",
	Long: `weron provides lean, fast & secure layer 2 overlay networks based on WebRTC.

Find more information at:
https://github.com/pojntfx/weron.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		viper.SetEnvPrefix("weron")
		viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))

		return nil
	},
}

func Execute() error {
	rootCmd.PersistentFlags().BoolP(verboseFlag, "v", false, "Enable verbose logging")
	rootCmd.PersistentFlags().DurationP(timeoutFlag, "m", time.Second*5, "Duration between reconnects and pings")

	if err := viper.BindPFlags(rootCmd.PersistentFlags()); err != nil {
		return err
	}

	viper.AutomaticEnv()

	return rootCmd.Execute()
}
