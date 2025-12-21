package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon status and proxied services",
	Long: `Display the current status of the devproxy daemon including:

  - Whether the daemon is running
  - Configured entrypoints (HTTP, HTTPS, TCP)
  - Currently proxied services
  - Certificate status and expiry dates`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("status: not implemented")
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
