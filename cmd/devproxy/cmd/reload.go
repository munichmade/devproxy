package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/munichmade/devproxy/internal/daemon"
	"github.com/spf13/cobra"
)

var reloadCmd = &cobra.Command{
	Use:   "reload",
	Short: "Reload daemon configuration",
	Long: `Reload the daemon configuration without restarting.

This sends a SIGHUP signal to the daemon to reload its configuration file.
Note that some settings (like listen ports) require a full restart.`,
	Run: func(cmd *cobra.Command, args []string) {
		d := daemon.New()

		if err := d.Reload(); err != nil {
			if errors.Is(err, daemon.ErrNotRunning) {
				fmt.Println("devproxy is not running")
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "failed to reload daemon: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("devproxy configuration reloaded")
	},
}

func init() {
	rootCmd.AddCommand(reloadCmd)
}
