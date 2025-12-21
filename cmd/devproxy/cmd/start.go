package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/munichmade/devproxy/internal/daemon"
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
		d := daemon.New()

		if err := d.Start(); err != nil {
			if errors.Is(err, daemon.ErrAlreadyRunning) {
				fmt.Println("devproxy is already running")
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "failed to start daemon: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("devproxy started")
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
}
