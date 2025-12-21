package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the devproxy daemon",
	Long:  `Stop the running devproxy daemon gracefully.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("stop: not implemented")
	},
}

func init() {
	rootCmd.AddCommand(stopCmd)
}
