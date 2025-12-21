package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "[stub] Configure system for devproxy (requires sudo)",
	Long: `Setup configures your system for devproxy by:

  1. Generating a local Certificate Authority (CA)
  2. Installing the CA into the system trust store
  3. Configuring DNS resolver for *.localhost domains

This command requires administrator privileges and will prompt for sudo.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("setup: not implemented")
	},
}

func init() {
	rootCmd.AddCommand(setupCmd)
}
