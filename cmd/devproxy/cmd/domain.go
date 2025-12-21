package cmd

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/munichmade/devproxy/internal/cert"
	"github.com/munichmade/devproxy/internal/config"
	"github.com/munichmade/devproxy/internal/paths"
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
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load configuration: %w", err)
		}

		certsDir := paths.CertsDir()

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "DOMAIN\tSTATUS\tEXPIRES\tSOURCE")

		// List domains from config
		for _, domain := range cfg.DNS.Domains {
			status, expires := getCertStatus(certsDir, domain)
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", domain, status, expires, "config")
		}

		// Check for additional certificates in certs directory
		entries, err := os.ReadDir(certsDir)
		if err == nil {
			configDomains := make(map[string]bool)
			for _, d := range cfg.DNS.Domains {
				configDomains[d] = true
			}

			for _, entry := range entries {
				if entry.IsDir() {
					domain := entry.Name()
					if !configDomains[domain] {
						status, expires := getCertStatus(certsDir, domain)
						fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", domain, status, expires, "generated")
					}
				}
			}
		}

		w.Flush()
		return nil
	},
}

var domainAddCmd = &cobra.Command{
	Use:   "add <domain>",
	Short: "Add a domain and generate certificate",
	Long: `Manually add a domain and generate its TLS certificate.

Example:
  devproxy domain add myproject.localhost`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain := args[0]

		// Create certificate manager (loads CA internally)
		manager, err := cert.NewManager()
		if err != nil {
			return fmt.Errorf("failed to initialize certificate manager (run 'devproxy setup' first): %w", err)
		}

		// Generate certificate using EnsureCertificate
		if err := manager.EnsureCertificate(domain); err != nil {
			return fmt.Errorf("failed to generate certificate: %w", err)
		}

		// Read the generated certificate to show details
		certsDir := paths.CertsDir()
		status, expires := getCertStatus(certsDir, domain)

		fmt.Printf("Certificate generated for %s\n", domain)
		fmt.Printf("  Status: %s\n", status)
		fmt.Printf("  Expires: %s\n", expires)

		return nil
	},
}

var domainRemoveCmd = &cobra.Command{
	Use:   "remove <domain>",
	Short: "Remove a domain's certificate",
	Long: `Remove a domain's certificate files.

Example:
  devproxy domain remove myproject.localhost`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain := args[0]
		certsDir := paths.CertsDir()
		domainDir := filepath.Join(certsDir, domain)

		// Check if directory exists
		if _, err := os.Stat(domainDir); os.IsNotExist(err) {
			return fmt.Errorf("no certificate found for domain: %s", domain)
		}

		// Remove the certificate directory
		if err := os.RemoveAll(domainDir); err != nil {
			return fmt.Errorf("failed to remove certificate: %w", err)
		}

		fmt.Printf("Removed certificate for %s\n", domain)
		return nil
	},
}

var domainCertCmd = &cobra.Command{
	Use:   "cert <domain>",
	Short: "Show certificate details",
	Long: `Display detailed certificate information for a domain.

Example:
  devproxy domain cert myproject.localhost`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain := args[0]
		certsDir := paths.CertsDir()
		certPath := filepath.Join(certsDir, domain, "cert.pem")

		// Read certificate file
		certData, err := os.ReadFile(certPath)
		if err != nil {
			return fmt.Errorf("no certificate found for domain %s: %w", domain, err)
		}

		// Parse PEM block
		block, _ := pem.Decode(certData)
		if block == nil {
			return fmt.Errorf("failed to parse certificate PEM")
		}

		// Parse certificate
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return fmt.Errorf("failed to parse certificate: %w", err)
		}

		// Display certificate info
		fmt.Printf("Certificate for: %s\n\n", domain)
		fmt.Printf("  Subject:      %s\n", certificate.Subject.CommonName)
		fmt.Printf("  Issuer:       %s\n", certificate.Issuer.CommonName)
		fmt.Printf("  Serial:       %s\n", certificate.SerialNumber.String())
		fmt.Printf("  Not Before:   %s\n", certificate.NotBefore.Format(time.RFC3339))
		fmt.Printf("  Not After:    %s\n", certificate.NotAfter.Format(time.RFC3339))

		if len(certificate.DNSNames) > 0 {
			fmt.Printf("  DNS Names:    %s\n", strings.Join(certificate.DNSNames, ", "))
		}

		// Check expiry status
		now := time.Now()
		if now.After(certificate.NotAfter) {
			fmt.Printf("\n  Status:       EXPIRED\n")
		} else if now.Add(30 * 24 * time.Hour).After(certificate.NotAfter) {
			fmt.Printf("\n  Status:       EXPIRING SOON\n")
		} else {
			daysLeft := int(certificate.NotAfter.Sub(now).Hours() / 24)
			fmt.Printf("\n  Status:       Valid (%d days remaining)\n", daysLeft)
		}

		return nil
	},
}

var domainRenewCmd = &cobra.Command{
	Use:   "renew <domain>",
	Short: "Renew certificate for domain",
	Long: `Force renewal of the TLS certificate for a domain.

Example:
  devproxy domain renew myproject.localhost`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain := args[0]
		certsDir := paths.CertsDir()
		domainDir := filepath.Join(certsDir, domain)

		// Remove existing certificate
		if _, err := os.Stat(domainDir); err == nil {
			if err := os.RemoveAll(domainDir); err != nil {
				return fmt.Errorf("failed to remove old certificate: %w", err)
			}
		}

		// Create certificate manager (loads CA internally)
		manager, err := cert.NewManager()
		if err != nil {
			return fmt.Errorf("failed to initialize certificate manager (run 'devproxy setup' first): %w", err)
		}

		// Generate new certificate using EnsureCertificate
		if err := manager.EnsureCertificate(domain); err != nil {
			return fmt.Errorf("failed to generate certificate: %w", err)
		}

		// Read the generated certificate to show details
		status, expires := getCertStatus(certsDir, domain)

		fmt.Printf("Certificate renewed for %s\n", domain)
		fmt.Printf("  Status: %s\n", status)
		fmt.Printf("  Expires: %s\n", expires)

		return nil
	},
}

// getCertStatus returns the status and expiry date of a domain's certificate
func getCertStatus(certsDir, domain string) (status, expires string) {
	certPath := filepath.Join(certsDir, domain, "cert.pem")

	certData, err := os.ReadFile(certPath)
	if err != nil {
		return "No cert", "-"
	}

	block, _ := pem.Decode(certData)
	if block == nil {
		return "Invalid", "-"
	}

	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "Invalid", "-"
	}

	now := time.Now()
	if now.After(certificate.NotAfter) {
		return "Expired", certificate.NotAfter.Format("2006-01-02")
	} else if now.Add(30 * 24 * time.Hour).After(certificate.NotAfter) {
		return "Expiring", certificate.NotAfter.Format("2006-01-02")
	}

	return "Valid", certificate.NotAfter.Format("2006-01-02")
}

func init() {
	domainCmd.AddCommand(domainListCmd)
	domainCmd.AddCommand(domainAddCmd)
	domainCmd.AddCommand(domainRemoveCmd)
	domainCmd.AddCommand(domainCertCmd)
	domainCmd.AddCommand(domainRenewCmd)
	rootCmd.AddCommand(domainCmd)
}
