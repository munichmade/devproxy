package cmd

import (
	"context"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/docker/docker/client"
	"github.com/spf13/cobra"

	"github.com/munichmade/devproxy/internal/ca"
	"github.com/munichmade/devproxy/internal/paths"
)

// CheckResult represents the result of a single check.
type CheckResult struct {
	Name       string
	Passed     bool
	Message    string
	Suggestion string
}

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Run diagnostic checks",
	Long: `Run diagnostic checks to verify devproxy is configured correctly.

Checks include:
  - CA certificate exists and is valid
  - CA is trusted by the system
  - DNS resolver is configured
  - DNS resolution working
  - Docker socket is accessible
  - Docker daemon is running
  - Required ports are available`,
	Run: runCheck,
}

func init() {
	rootCmd.AddCommand(checkCmd)
}

func runCheck(cmd *cobra.Command, args []string) {
	fmt.Println("\nChecking system configuration...")

	var failures int

	// Run all checks
	checks := []func() CheckResult{
		checkCAExists,
		checkCATrusted,
		checkDNSResolver,
		checkDNSWorking,
		checkDockerSocket,
		checkDockerRunning,
		checkPort80,
		checkPort443,
	}

	for _, check := range checks {
		result := check()
		printResult(result)
		if !result.Passed {
			failures++
		}
	}

	// Print summary
	fmt.Println()
	if failures == 0 {
		fmt.Println("All checks passed!")
	} else {
		fmt.Printf("%d check(s) failed\n", failures)
		os.Exit(1)
	}
}

func printResult(r CheckResult) {
	if r.Passed {
		fmt.Printf("  ✓ %s\n", r.Message)
	} else {
		fmt.Printf("  ✗ %s\n", r.Message)
		if r.Suggestion != "" {
			fmt.Printf("    → %s\n", r.Suggestion)
		}
	}
}

func checkCAExists() CheckResult {
	result := CheckResult{Name: "ca_exists"}

	caPath := filepath.Join(paths.CADir(), "root-ca.pem")
	if _, err := os.Stat(caPath); os.IsNotExist(err) {
		result.Passed = false
		result.Message = "CA certificate not found"
		result.Suggestion = "Run: sudo devproxy setup"
		return result
	}

	// Try to load and parse the CA
	caCert, err := os.ReadFile(caPath)
	if err != nil {
		result.Passed = false
		result.Message = "CA certificate not readable"
		result.Suggestion = "Run: sudo devproxy setup"
		return result
	}

	// Verify it's a valid certificate
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caCert) {
		result.Passed = false
		result.Message = "CA certificate is invalid"
		result.Suggestion = "Run: sudo devproxy setup"
		return result
	}

	result.Passed = true
	result.Message = "CA certificate exists"
	return result
}

func checkCATrusted() CheckResult {
	result := CheckResult{Name: "ca_trusted"}

	if !ca.IsTrusted() {
		result.Passed = false
		result.Message = "CA not trusted by system"
		result.Suggestion = "Run: sudo devproxy setup"
		return result
	}

	result.Passed = true
	result.Message = "CA trusted by system"
	return result
}

func checkDNSResolver() CheckResult {
	result := CheckResult{Name: "dns_resolver"}

	if runtime.GOOS == "darwin" {
		resolverPath := "/etc/resolver/localhost"
		if _, err := os.Stat(resolverPath); os.IsNotExist(err) {
			result.Passed = false
			result.Message = "DNS resolver not configured"
			result.Suggestion = "Run: sudo devproxy setup"
			return result
		}
		result.Passed = true
		result.Message = "DNS resolver configured"
	} else {
		// Linux - check systemd-resolved or /etc/hosts
		result.Passed = true
		result.Message = "DNS resolver check skipped (Linux)"
	}

	return result
}

func checkDNSWorking() CheckResult {
	result := CheckResult{Name: "dns_working"}

	// Try to resolve a .localhost domain
	addrs, err := net.LookupHost("test.localhost")
	if err != nil {
		result.Passed = false
		result.Message = "DNS resolution failed for *.localhost"
		result.Suggestion = "Run: sudo devproxy setup"
		return result
	}

	// Check if it resolves to localhost
	for _, addr := range addrs {
		if addr == "127.0.0.1" || addr == "::1" {
			result.Passed = true
			result.Message = "DNS resolution working (*.localhost → 127.0.0.1)"
			return result
		}
	}

	result.Passed = false
	result.Message = fmt.Sprintf("DNS resolves to wrong address: %v", addrs)
	result.Suggestion = "Run: sudo devproxy setup"
	return result
}

func checkDockerSocket() CheckResult {
	result := CheckResult{Name: "docker_socket"}

	socketPath := "/var/run/docker.sock"
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		result.Passed = false
		result.Message = "Docker socket not found"
		result.Suggestion = "Start Docker Desktop or the Docker daemon"
		return result
	}

	// Check if readable
	if _, err := os.Stat(socketPath); err != nil {
		result.Passed = false
		result.Message = "Docker socket not accessible"
		result.Suggestion = "Check Docker socket permissions"
		return result
	}

	result.Passed = true
	result.Message = "Docker socket accessible"
	return result
}

func checkDockerRunning() CheckResult {
	result := CheckResult{Name: "docker_running"}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		result.Passed = false
		result.Message = "Could not create Docker client"
		result.Suggestion = "Start Docker Desktop or the Docker daemon"
		return result
	}
	defer cli.Close()

	_, err = cli.Ping(ctx)
	if err != nil {
		result.Passed = false
		result.Message = "Docker daemon not responding"
		result.Suggestion = "Start Docker Desktop or the Docker daemon"
		return result
	}

	result.Passed = true
	result.Message = "Docker daemon running"
	return result
}

func checkPort80() CheckResult {
	return checkPort(80)
}

func checkPort443() CheckResult {
	return checkPort(443)
}

func checkPort(port int) CheckResult {
	result := CheckResult{Name: fmt.Sprintf("port_%d", port)}

	// First try to bind - this works if we have permission and port is free
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err == nil {
		ln.Close()
		result.Passed = true
		result.Message = fmt.Sprintf("Port %d available", port)
		return result
	}

	// Binding failed - check if it's a permission issue or actually in use
	// Try connecting to the port to see if something is listening
	conn, connErr := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 1*time.Second)
	if connErr != nil {
		// Connection refused or timed out - port is likely free but we lack permission to bind
		result.Passed = true
		result.Message = fmt.Sprintf("Port %d available (requires sudo to bind)", port)
		return result
	}
	conn.Close()

	// Something is actually listening on this port
	result.Passed = false
	result.Message = fmt.Sprintf("Port %d is in use", port)
	result.Suggestion = fmt.Sprintf("Stop the service using port %d or run devproxy with sudo", port)
	return result
}
