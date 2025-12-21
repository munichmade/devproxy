package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the devproxy daemon",
	Long:  `Stop and start the devproxy daemon.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("restart: not implemented")
	},
}

func init() {
	rootCmd.AddCommand(restartCmd)
}
