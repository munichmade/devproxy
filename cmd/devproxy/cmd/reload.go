package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var reloadCmd = &cobra.Command{
	Use:   "reload",
	Short: "Reload daemon configuration",
	Long: `Reload the daemon configuration without restarting.

This sends a SIGHUP signal to the daemon to reload its configuration file.
Note that some settings (like listen ports) require a full restart.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("reload: not implemented")
	},
}

func init() {
	rootCmd.AddCommand(reloadCmd)
}
