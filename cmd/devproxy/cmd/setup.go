package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/munichmade/devproxy/internal/ca"
	"github.com/munichmade/devproxy/internal/config"
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
  4. Setting up port forwarding (80->8080, 443->8443) on macOS

This command requires administrator privileges and will prompt for sudo.`,
	Run: func(cmd *cobra.Command, args []string) {
		domains, _ := cmd.Flags().GetStringSlice("domain")
		skipPortForward, _ := cmd.Flags().GetBool("skip-port-forward")

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
		dnsPort := extractPort(cfg.DNS.Listen, 5353)
		if resolver.IsConfigured(domains) {
			fmt.Println("already configured")
		} else {
			fmt.Println("configuring (requires sudo)")
			resolverCfg := resolver.Config{
				Domains: domains,
				Port:    dnsPort,
			}
			if err := resolver.Setup(resolverCfg); err != nil {
				fmt.Fprintf(os.Stderr, "   Failed to configure resolver: %v\n", err)
				fmt.Fprintf(os.Stderr, "   Try running: sudo devproxy dns setup\n")
				os.Exit(1)
			}
			fmt.Printf("   DNS resolver configured for: %v (port %d)\n", domains, dnsPort)
		}

		// Step 4: Configure port forwarding (macOS only)
		if runtime.GOOS == "darwin" && !skipPortForward {
			fmt.Print("4. Checking port forwarding... ")
			httpPort := extractPort(cfg.Entrypoints["http"].Listen, 8080)
			httpsPort := extractPort(cfg.Entrypoints["https"].Listen, 8443)

			if httpPort != 80 || httpsPort != 443 {
				// Need port forwarding
				if isPortForwardingConfigured(httpPort, httpsPort) {
					fmt.Println("already configured")
				} else {
					fmt.Println("configuring (requires sudo)")
					if err := setupPortForwarding(httpPort, httpsPort); err != nil {
						fmt.Fprintf(os.Stderr, "   Failed to configure port forwarding: %v\n", err)
						fmt.Fprintf(os.Stderr, "   You can skip this with --skip-port-forward\n")
						fmt.Fprintf(os.Stderr, "   Or manually configure: 80->%d, 443->%d\n", httpPort, httpsPort)
					} else {
						fmt.Printf("   Port forwarding configured: 80->%d, 443->%d\n", httpPort, httpsPort)
					}
				}
			} else {
				fmt.Println("not needed (using privileged ports)")
			}
		}

		fmt.Println()
		fmt.Println("Setup complete! devproxy is ready to use.")
		fmt.Println()
		if cfg.DNS.Enabled {
			fmt.Println("Start the daemon with: devproxy start")
		} else {
			fmt.Println("Note: Built-in DNS is disabled. Make sure your external DNS")
			fmt.Println("      (e.g., dnsmasq) resolves *.localhost to 127.0.0.1")
			fmt.Println()
			fmt.Println("Start the daemon with: devproxy start")
		}
	},
}

var teardownCmd = &cobra.Command{
	Use:   "teardown",
	Short: "Remove devproxy system configuration (requires sudo)",
	Long: `Teardown removes devproxy system configuration:

  1. Removes CA from system trust store
  2. Removes DNS resolver configuration
  3. Removes port forwarding rules (macOS)

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

		// Step 3: Remove port forwarding (macOS)
		if runtime.GOOS == "darwin" {
			fmt.Print("3. Removing port forwarding... ")
			if err := removePortForwarding(); err != nil {
				fmt.Fprintf(os.Stderr, "failed: %v\n", err)
			} else {
				fmt.Println("done")
			}
		}

		fmt.Println()
		fmt.Println("Teardown complete.")
	},
}

// extractPort extracts the port number from an address string like ":8080" or "127.0.0.1:8080"
func extractPort(addr string, defaultPort int) int {
	if addr == "" {
		return defaultPort
	}
	// Find the last colon
	idx := strings.LastIndex(addr, ":")
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

// isPortForwardingConfigured checks if pf rules for devproxy exist
func isPortForwardingConfigured(httpPort, httpsPort int) bool {
	// Check if our anchor exists in pf
	cmd := exec.Command("sudo", "pfctl", "-s", "Anchors")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), "devproxy")
}

// setupPortForwarding configures macOS pf to forward ports 80->httpPort and 443->httpsPort
func setupPortForwarding(httpPort, httpsPort int) error {
	// Create pf anchor file
	rules := fmt.Sprintf(`# devproxy port forwarding rules
rdr pass on lo0 inet proto tcp from any to 127.0.0.1 port 80 -> 127.0.0.1 port %d
rdr pass on lo0 inet proto tcp from any to 127.0.0.1 port 443 -> 127.0.0.1 port %d
`, httpPort, httpsPort)

	// Write rules to a temporary file
	tmpFile := "/tmp/devproxy-pf-rules.conf"
	if err := os.WriteFile(tmpFile, []byte(rules), 0644); err != nil {
		return fmt.Errorf("failed to write pf rules: %w", err)
	}

	// Create the anchor file in /etc/pf.anchors/
	anchorFile := "/etc/pf.anchors/devproxy"
	cmd := exec.Command("sudo", "cp", tmpFile, anchorFile)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install pf anchor: %w", err)
	}

	// Load the anchor
	cmd = exec.Command("sudo", "pfctl", "-a", "devproxy", "-f", anchorFile)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to load pf anchor: %w", err)
	}

	// Enable pf if not already enabled
	cmd = exec.Command("sudo", "pfctl", "-e")
	_ = cmd.Run() // Ignore error if already enabled

	// Add anchor reference to /etc/pf.conf if not present
	if err := ensurePfAnchorReference(); err != nil {
		return fmt.Errorf("failed to update pf.conf: %w", err)
	}

	return nil
}

// ensurePfAnchorReference adds the devproxy anchor reference to /etc/pf.conf if not present
func ensurePfAnchorReference() error {
	pfConf := "/etc/pf.conf"
	content, err := os.ReadFile(pfConf)
	if err != nil {
		return err
	}

	anchorLine := "rdr-anchor \"devproxy\""
	loadLine := "load anchor \"devproxy\" from \"/etc/pf.anchors/devproxy\""

	if strings.Contains(string(content), anchorLine) {
		return nil // Already configured
	}

	// Append anchor references
	newContent := string(content) + "\n# devproxy port forwarding\n" + anchorLine + "\n" + loadLine + "\n"

	// Write to temp file and copy with sudo
	tmpFile := "/tmp/devproxy-pf.conf"
	if err := os.WriteFile(tmpFile, []byte(newContent), 0644); err != nil {
		return err
	}

	cmd := exec.Command("sudo", "cp", tmpFile, pfConf)
	return cmd.Run()
}

// removePortForwarding removes the devproxy pf rules
func removePortForwarding() error {
	// Flush the anchor
	cmd := exec.Command("sudo", "pfctl", "-a", "devproxy", "-F", "all")
	_ = cmd.Run()

	// Remove anchor file
	cmd = exec.Command("sudo", "rm", "-f", "/etc/pf.anchors/devproxy")
	_ = cmd.Run()

	// Note: We don't remove the reference from pf.conf to avoid breaking things
	// The anchor will just be empty

	return nil
}

func init() {
	setupCmd.Flags().StringSliceP("domain", "d", []string{"localhost"}, "Domains to configure")
	setupCmd.Flags().Bool("skip-port-forward", false, "Skip port forwarding setup (macOS)")

	teardownCmd.Flags().StringSliceP("domain", "d", []string{"localhost"}, "Domains to remove")

	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(teardownCmd)
}
