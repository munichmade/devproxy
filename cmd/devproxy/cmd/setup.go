package cmd

import (
	"fmt"
	"os"

	"github.com/munichmade/devproxy/internal/ca"
	"github.com/munichmade/devproxy/internal/resolver"
	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure system for devproxy (requires sudo)",
	Long: `Setup configures your system for devproxy by:

  1. Generating a local Certificate Authority (CA) if not present
  2. Installing the CA into the system trust store
  3. Configuring DNS resolver for *.localhost domains

This command requires administrator privileges and will prompt for sudo.`,
	Run: func(cmd *cobra.Command, args []string) {
		domains, _ := cmd.Flags().GetStringSlice("domain")

		fmt.Println("Setting up devproxy...")
		fmt.Println()

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
			fmt.Println("installing (requires sudo)")
			if err := ca.InstallTrust(); err != nil {
				fmt.Fprintf(os.Stderr, "   Failed to install CA trust: %v\n", err)
				fmt.Fprintf(os.Stderr, "   Try running: sudo devproxy ca trust\n")
				os.Exit(1)
			}
			fmt.Println("   CA installed into trust store")
		}

		// Step 3: Configure DNS resolver
		fmt.Print("3. Checking DNS resolver... ")
		if resolver.IsConfigured(domains) {
			fmt.Println("already configured")
		} else {
			fmt.Println("configuring (requires sudo)")
			cfg := resolver.Config{
				Domains: domains,
				Port:    53, // Always use standard DNS port
			}
			if err := resolver.Setup(cfg); err != nil {
				fmt.Fprintf(os.Stderr, "   Failed to configure resolver: %v\n", err)
				fmt.Fprintf(os.Stderr, "   Try running: sudo devproxy dns setup\n")
				os.Exit(1)
			}
			fmt.Printf("   DNS resolver configured for: %v\n", domains)
		}

		fmt.Println()
		fmt.Println("Setup complete! devproxy is ready to use.")
		fmt.Println()
		fmt.Println("Start the daemon with: devproxy start")
	},
}

var teardownCmd = &cobra.Command{
	Use:   "teardown",
	Short: "Remove devproxy system configuration (requires sudo)",
	Long: `Teardown removes devproxy system configuration:

  1. Removes CA from system trust store
  2. Removes DNS resolver configuration

This command requires administrator privileges.`,
	Run: func(cmd *cobra.Command, args []string) {
		domains, _ := cmd.Flags().GetStringSlice("domain")

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
		fmt.Println("Teardown complete.")
	},
}

func init() {
	setupCmd.Flags().StringSliceP("domain", "d", []string{"localhost"}, "Domains to configure")

	teardownCmd.Flags().StringSliceP("domain", "d", []string{"localhost"}, "Domains to remove")

	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(teardownCmd)
}
