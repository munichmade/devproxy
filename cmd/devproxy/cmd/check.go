package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "[stub] Run diagnostic checks",
	Long: `Run diagnostic checks to verify devproxy is configured correctly.

Checks include:
  - CA certificate exists and is valid
  - CA is trusted by the system
  - DNS resolver is configured
  - Docker socket is accessible
  - Required ports are available`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("check: not implemented")
	},
}

func init() {
	rootCmd.AddCommand(checkCmd)
}
