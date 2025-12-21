package cmd

import (
	"fmt"
	"os"

	"github.com/munichmade/devproxy/internal/resolver"
	"github.com/spf13/cobra"
)

var dnsCmd = &cobra.Command{
	Use:   "dns",
	Short: "Manage DNS resolver configuration",
	Long:  `Manage DNS resolver configuration for local development domains.`,
}

var dnsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List domains resolved by devproxy",
	Long: `List all domains that are currently configured to be resolved by devproxy.

Only shows domains managed by devproxy (created via 'devproxy setup').
Domains configured by other tools (e.g., dnsmasq) are not shown.`,
	Run: func(cmd *cobra.Command, args []string) {
		showAll, _ := cmd.Flags().GetBool("all")

		if showAll {
			// Show all resolver files
			domains, err := resolver.GetConfiguredDomains()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			if len(domains) == 0 {
				fmt.Println("No resolver files found in /etc/resolver/")
				return
			}

			fmt.Println("All resolver files:")
			for _, domain := range domains {
				managed := ""
				if resolver.IsManagedByDevproxy(domain) {
					managed = " (devproxy)"
				}
				fmt.Printf("  *.%s%s\n", domain, managed)
			}
		} else {
			// Show only devproxy-managed domains
			domains, err := resolver.ListManaged()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			if len(domains) == 0 {
				fmt.Println("No domains configured by devproxy.")
				fmt.Println("Run 'devproxy setup' to configure DNS resolution.")
				return
			}

			fmt.Println("Domains resolved by devproxy:")
			for _, domain := range domains {
				fmt.Printf("  *.%s\n", domain)
			}
		}
	},
}

var dnsRemoveCmd = &cobra.Command{
	Use:   "remove [domain...]",
	Short: "Remove domains from devproxy DNS resolution",
	Long: `Remove one or more domains from devproxy DNS resolution.

Only removes domains that were configured by devproxy.
Domains configured by other tools are not affected.

Examples:
  devproxy dns remove localhost      # Remove localhost domain
  devproxy dns remove test local     # Remove multiple domains
  devproxy dns remove --all          # Remove all devproxy-managed domains`,
	Run: func(cmd *cobra.Command, args []string) {
		removeAll, _ := cmd.Flags().GetBool("all")

		if removeAll {
			// Remove all managed domains
			removed, err := resolver.RemoveAll()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			if len(removed) == 0 {
				fmt.Println("No devproxy-managed domains to remove.")
				return
			}

			fmt.Println("Removed domains:")
			for _, domain := range removed {
				fmt.Printf("  *.%s\n", domain)
			}
		} else {
			// Remove specified domains
			if len(args) == 0 {
				fmt.Fprintln(os.Stderr, "Error: specify domains to remove or use --all")
				fmt.Fprintln(os.Stderr, "Usage: devproxy dns remove [domain...] or devproxy dns remove --all")
				os.Exit(1)
			}

			// Check which domains are managed
			var toRemove []string
			var skipped []string
			for _, domain := range args {
				if resolver.IsManagedByDevproxy(domain) {
					toRemove = append(toRemove, domain)
				} else {
					skipped = append(skipped, domain)
				}
			}

			if len(skipped) > 0 {
				fmt.Println("Skipping (not managed by devproxy):")
				for _, domain := range skipped {
					fmt.Printf("  *.%s\n", domain)
				}
			}

			if len(toRemove) == 0 {
				fmt.Println("No devproxy-managed domains to remove.")
				return
			}

			if err := resolver.Remove(toRemove); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			fmt.Println("Removed domains:")
			for _, domain := range toRemove {
				fmt.Printf("  *.%s\n", domain)
			}
		}
	},
}

func init() {
	dnsListCmd.Flags().BoolP("all", "a", false, "Show all resolver files, not just devproxy-managed")

	dnsRemoveCmd.Flags().Bool("all", false, "Remove all devproxy-managed domains")

	dnsCmd.AddCommand(dnsListCmd)
	dnsCmd.AddCommand(dnsRemoveCmd)
	rootCmd.AddCommand(dnsCmd)
}
