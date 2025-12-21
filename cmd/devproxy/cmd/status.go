package cmd

import (
	"fmt"

	"github.com/munichmade/devproxy/internal/daemon"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "[partial] Show daemon status and proxied services",
	Long: `Display the current status of the devproxy daemon including:

  - Whether the daemon is running
  - Configured entrypoints (HTTP, HTTPS, TCP)
  - Currently proxied services
  - Certificate status and expiry dates`,
	Run: func(cmd *cobra.Command, args []string) {
		d := daemon.New()

		if d.IsRunning() {
			pid, _ := d.GetPID()
			fmt.Printf("devproxy is running (pid %d)\n", pid)
		} else {
			fmt.Println("devproxy is not running")
		}

		// TODO: Show entrypoints, proxied services, and certificates
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
