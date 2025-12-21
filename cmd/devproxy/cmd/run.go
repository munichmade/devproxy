package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:    "run",
	Short:  "Run daemon in foreground (for systemd/launchd)",
	Long:   `Run the devproxy daemon in the foreground. Used by systemd/launchd service managers.`,
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("run: not implemented")
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
}
