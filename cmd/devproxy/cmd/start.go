package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/munichmade/devproxy/internal/daemon"
	"github.com/munichmade/devproxy/internal/privilege"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the devproxy daemon",
	Long: `Start the devproxy daemon in the background.

The daemon listens on:
  - Port 80 for HTTP (redirects to HTTPS)
  - Port 443 for HTTPS
  - Port 15353 for DNS queries
  - Port 15432 for PostgreSQL (SNI-based routing)
  - Additional configured TCP entrypoints

Administrator privileges are required to bind to ports 80 and 443.
The daemon will drop privileges after binding these ports.

Use 'devproxy status' to check if the daemon is running.
Use 'devproxy stop' to stop the daemon.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Elevate to root if needed (for binding ports 80/443)
		if err := privilege.RequireRoot("binding to ports 80 and 443"); err != nil {
			fmt.Fprintf(os.Stderr, "failed to elevate privileges: %v\n", err)
			os.Exit(1)
		}

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
