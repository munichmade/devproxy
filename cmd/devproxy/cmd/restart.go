package cmd

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/munichmade/devproxy/internal/daemon"
	"github.com/munichmade/devproxy/internal/privilege"
)

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the devproxy daemon",
	Long:  `Stop and start the devproxy daemon.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Elevate to root if needed (for binding ports 80/443)
		if err := privilege.RequireRoot("binding to ports 80 and 443"); err != nil {
			fmt.Fprintf(os.Stderr, "failed to elevate privileges: %v\n", err)
			os.Exit(1)
		}

		d := daemon.New()

		// Stop if running
		if d.IsRunning() {
			if err := d.Stop(); err != nil && !errors.Is(err, daemon.ErrNotRunning) {
				fmt.Fprintf(os.Stderr, "failed to stop daemon: %v\n", err)
				os.Exit(1)
			}

			// Wait a moment for the process to exit
			for i := 0; i < 10; i++ {
				if !d.IsRunning() {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}

			if d.IsRunning() {
				fmt.Fprintln(os.Stderr, "daemon did not stop in time")
				os.Exit(1)
			}
		}

		// Start
		if err := d.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to start daemon: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("devproxy restarted")
	},
}

func init() {
	rootCmd.AddCommand(restartCmd)
}
