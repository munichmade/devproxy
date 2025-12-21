package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "[stub] Manage configuration",
	Long:  `View and edit devproxy configuration.`,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Long:  `Display the current configuration file contents.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("config show: not implemented")
	},
}

var configEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Edit configuration file",
	Long:  `Open the configuration file in your default editor ($EDITOR).`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("config edit: not implemented")
	},
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configEditCmd)
	rootCmd.AddCommand(configCmd)
}
