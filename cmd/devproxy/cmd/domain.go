package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var domainCmd = &cobra.Command{
	Use:   "domain",
	Short: "Manage domains and certificates",
	Long:  `Manage registered domains and their TLS certificates.`,
}

var domainListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered domains",
	Long:  `List all registered domains and their certificate status.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("domain list: not implemented")
	},
}

var domainAddCmd = &cobra.Command{
	Use:   "add <domain>",
	Short: "Add a domain and generate certificate",
	Long: `Manually add a domain and generate its wildcard certificate.

Example:
  devproxy domain add myproject.localhost`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("domain add %s: not implemented\n", args[0])
	},
}

var domainRemoveCmd = &cobra.Command{
	Use:   "remove <domain>",
	Short: "Remove a domain and its certificate",
	Long: `Remove a domain and delete its certificate files.

Example:
  devproxy domain remove myproject.localhost`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("domain remove %s: not implemented\n", args[0])
	},
}

func init() {
	domainCmd.AddCommand(domainListCmd)
	domainCmd.AddCommand(domainAddCmd)
	domainCmd.AddCommand(domainRemoveCmd)
	rootCmd.AddCommand(domainCmd)
}
