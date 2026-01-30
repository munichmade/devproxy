package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/munichmade/devproxy/internal/ca"
	"github.com/munichmade/devproxy/internal/config"
	"github.com/munichmade/devproxy/internal/privilege"
	"github.com/munichmade/devproxy/internal/resolver"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure system for devproxy",
	Long: `Setup configures your system for devproxy by:

  1. Generating a local Certificate Authority (CA) if not present
  2. Installing the CA into the system trust store
  3. Configuring DNS resolver for *.localhost domains

Administrator privileges are required to install the CA and configure DNS.`,
	Run: func(cmd *cobra.Command, args []string) {
		domains, _ := cmd.Flags().GetStringSlice("domain")

		// Elevate to root if needed
		if err := privilege.RequireRoot("installing CA certificate and configuring DNS resolver"); err != nil {
			fmt.Fprintf(os.Stderr, "failed to elevate privileges: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Setting up devproxy...")
		fmt.Println()

		// Load config to get port settings
		cfg, err := config.Load()
		if err != nil {
			// Use defaults if config doesn't exist
			cfg = config.Default()
		}

		// Step 1: Generate CA if needed
		fmt.Print("1. Checking CA... ")
		if ca.Exists() {
			fmt.Println("exists")
		} else {
			fmt.Println("generating")
			if _, err := ca.Generate(); err != nil {
				fmt.Fprintf(os.Stderr, "   Failed to generate CA: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("   CA generated successfully")
		}

		// Step 2: Install CA trust
		fmt.Print("2. Checking trust store... ")
		if ca.IsTrusted() {
			fmt.Println("already trusted")
		} else {
			fmt.Println("installing")
			if err := ca.InstallTrust(); err != nil {
				fmt.Fprintf(os.Stderr, "   Failed to install CA trust: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("   CA installed into trust store")
		}

		// Step 3: Configure DNS resolver
		fmt.Print("3. Checking DNS resolver... ")
		dnsPort := extractPort(cfg.DNS.Listen, 15353)
		if resolver.IsConfigured(domains) {
			fmt.Println("already configured")
		} else {
			fmt.Println("configuring")
			resolverCfg := resolver.Config{
				Domains: domains,
				Port:    dnsPort,
			}
			if err := resolver.Setup(resolverCfg); err != nil {
				fmt.Fprintf(os.Stderr, "   Failed to configure resolver: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("   DNS resolver configured for: %v (port %d)\n", domains, dnsPort)
		}

		fmt.Println()
		fmt.Println("Setup complete! devproxy is ready to use.")
		fmt.Println()
		fmt.Println("Start the daemon with: devproxy start")
	},
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove devproxy system configuration",
	Long: `Uninstall removes devproxy system configuration:

  1. Removes CA from system trust store
  2. Removes DNS resolver configuration

Administrator privileges are required.`,
	Run: func(cmd *cobra.Command, args []string) {
		domains, _ := cmd.Flags().GetStringSlice("domain")

		// Elevate to root if needed
		if err := privilege.RequireRoot("removing CA certificate and DNS resolver configuration"); err != nil {
			fmt.Fprintf(os.Stderr, "failed to elevate privileges: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Removing devproxy configuration...")
		fmt.Println()

		// Step 1: Remove CA trust
		fmt.Print("1. Removing CA from trust store... ")
		if !ca.IsTrusted() {
			fmt.Println("not installed")
		} else {
			if err := ca.UninstallTrust(); err != nil {
				fmt.Fprintf(os.Stderr, "failed: %v\n", err)
			} else {
				fmt.Println("done")
			}
		}

		// Step 2: Remove DNS resolver
		fmt.Print("2. Removing DNS resolver... ")
		if !resolver.IsConfigured(domains) {
			fmt.Println("not configured")
		} else {
			if err := resolver.Remove(domains); err != nil {
				fmt.Fprintf(os.Stderr, "failed: %v\n", err)
			} else {
				fmt.Println("done")
			}
		}

		fmt.Println()
		fmt.Println("Uninstall complete.")
	},
}

// extractPort extracts the port number from an address string like ":8080" or "127.0.0.1:8080"
func extractPort(addr string, defaultPort int) int {
	if addr == "" {
		return defaultPort
	}
	// Find the last colon
	idx := -1
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			idx = i
			break
		}
	}
	if idx == -1 {
		return defaultPort
	}
	portStr := addr[idx+1:]
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		return defaultPort
	}
	return port
}

func init() {
	setupCmd.Flags().StringSliceP("domain", "d", []string{"localhost"}, "Domains to configure")
	uninstallCmd.Flags().StringSliceP("domain", "d", []string{"localhost"}, "Domains to remove")

	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(uninstallCmd)
}
