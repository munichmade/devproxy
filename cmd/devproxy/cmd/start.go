package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the devproxy daemon",
	Long: `Start the devproxy daemon in the background.

The daemon listens on:
  - Port 53 for DNS queries
  - Port 80 for HTTP (redirects to HTTPS)
  - Port 443 for HTTPS
  - Port 15432 for PostgreSQL (SNI-based routing)
  - Additional configured TCP entrypoints

Use 'devproxy status' to check if the daemon is running.
Use 'devproxy stop' to stop the daemon.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("start: not implemented")
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
}
