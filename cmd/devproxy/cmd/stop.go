package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/munichmade/devproxy/internal/daemon"
	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the devproxy daemon",
	Long:  `Stop the running devproxy daemon gracefully.`,
	Run: func(cmd *cobra.Command, args []string) {
		d := daemon.New()

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
