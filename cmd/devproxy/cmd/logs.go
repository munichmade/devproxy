package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var logsFollow bool

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "View daemon logs",
	Long:  `Display logs from the devproxy daemon.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("logs: not implemented")
	},
}

func init() {
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output")
	rootCmd.AddCommand(logsCmd)
}
