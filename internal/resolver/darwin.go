//go:build darwin

// Package resolver provides DNS resolver configuration for macOS.
//
// On macOS, the /etc/resolver directory contains files that configure
// how the system resolves specific domains. Each file is named after
// the domain and contains resolver configuration.
//
// # Coexistence with dnsmasq
//
// If you have dnsmasq or another local DNS server running on port 53,
// you have two options:
//
//  1. Use a different port for devproxy's DNS server:
//     sudo devproxy setup --dns-port 5353
//     This creates resolver files pointing to 127.0.0.1:5353
//
//  2. Disable dnsmasq for localhost/test domains and let devproxy handle them:
//     Remove any localhost/test entries from dnsmasq configuration
//     Then run: sudo devproxy setup
//
// The resolver files created by devproxy include a header comment to identify
// them. If a resolver file exists without this header, it was likely created
// by another tool (e.g., dnsmasq) and will be overwritten by devproxy setup.
package resolver

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// resolverDir is where macOS looks for custom resolver configurations.
	resolverDir = "/etc/resolver"

	// defaultPort is the port devproxy DNS server listens on.
	defaultPort = 53

	// managedHeader identifies resolver files created by devproxy.
	managedHeader = "# devproxy resolver configuration"
)

// Config holds resolver configuration.
type Config struct {
	// Domains to configure resolvers for (e.g., "localhost", "test").
	Domains []string

	// Port the DNS server listens on.
	Port int
}

// Setup creates resolver files in /etc/resolver for each configured domain.
// Requires root/sudo privileges.
func Setup(cfg Config) error {
	port := cfg.Port
	if port == 0 {
		port = defaultPort
	}

	// Ensure resolver directory exists
	if err := os.MkdirAll(resolverDir, 0o755); err != nil {
		return fmt.Errorf("failed to create resolver directory: %w", err)
	}

	for _, domain := range cfg.Domains {
		if err := setupDomain(domain, port); err != nil {
			return err
		}
	}

	return nil
}

// setupDomain creates a resolver file for a single domain.
func setupDomain(domain string, port int) error {
	filePath := filepath.Join(resolverDir, domain)

	content := fmt.Sprintf(`# devproxy resolver configuration for *.%s
# Auto-generated - do not edit manually
nameserver 127.0.0.1
port %d
`, domain, port)

	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write resolver file for %s: %w", domain, err)
	}

	return nil
}

// Remove deletes resolver files for the configured domains.
// Only removes files that were created by devproxy (have the managed header).
// Requires root/sudo privileges.
func Remove(domains []string) error {
	var lastErr error

	for _, domain := range domains {
		filePath := filepath.Join(resolverDir, domain)

		// Check if file is managed by devproxy
		if !IsManagedByDevproxy(domain) {
			continue // Skip non-managed files
		}

		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			lastErr = fmt.Errorf("failed to remove resolver file for %s: %w", domain, err)
		}
	}

	return lastErr
}

// RemoveAll removes all devproxy-managed resolver files.
// Requires root/sudo privileges.
func RemoveAll() ([]string, error) {
	managed, err := ListManaged()
	if err != nil {
		return nil, err
	}

	if err := Remove(managed); err != nil {
		return managed, err
	}

	return managed, nil
}

// IsManagedByDevproxy checks if a resolver file was created by devproxy.
func IsManagedByDevproxy(domain string) bool {
	filePath := filepath.Join(resolverDir, domain)

	content, err := os.ReadFile(filePath)
	if err != nil {
		return false
	}

	return strings.HasPrefix(string(content), managedHeader)
}

// ListManaged returns a list of domains with resolver files managed by devproxy.
func ListManaged() ([]string, error) {
	entries, err := os.ReadDir(resolverDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read resolver directory: %w", err)
	}

	var domains []string
	for _, entry := range entries {
		if !entry.IsDir() && IsManagedByDevproxy(entry.Name()) {
			domains = append(domains, entry.Name())
		}
	}

	return domains, nil
}

// IsConfigured checks if resolver files exist for the given domains.
func IsConfigured(domains []string) bool {
	for _, domain := range domains {
		filePath := filepath.Join(resolverDir, domain)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			return false
		}
	}
	return len(domains) > 0
}

// GetConfiguredDomains returns a list of domains that have resolver files.
func GetConfiguredDomains() ([]string, error) {
	entries, err := os.ReadDir(resolverDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read resolver directory: %w", err)
	}

	var domains []string
	for _, entry := range entries {
		if !entry.IsDir() {
			domains = append(domains, entry.Name())
		}
	}

	return domains, nil
}

// NeedsSudo returns true since modifying /etc/resolver requires root.
func NeedsSudo() bool {
	return true
}
