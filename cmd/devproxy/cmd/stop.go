package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/munichmade/devproxy/internal/daemon"
	"github.com/munichmade/devproxy/internal/privilege"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the devproxy daemon",
	Long:  `Stop the running devproxy daemon gracefully.`,
	Run: func(cmd *cobra.Command, args []string) {
		d := daemon.New()

		// Check if running first - don't elevate if not needed
		if !d.IsRunning() {
			fmt.Println("devproxy is not running")
			os.Exit(1)
		}

		// Elevate to root if needed (to send signal to root-started process)
		if err := privilege.RequireRoot("stopping the daemon"); err != nil {
			fmt.Fprintf(os.Stderr, "failed to elevate privileges: %v\n", err)
			os.Exit(1)
		}

		if err := d.Stop(); err != nil {
			if errors.Is(err, daemon.ErrNotRunning) {
				fmt.Println("devproxy is not running")
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "failed to stop daemon: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("devproxy stopped")
	},
}

func init() {
	rootCmd.AddCommand(stopCmd)
}
