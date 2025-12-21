package cmd

import (
	"fmt"
	"os"

	"github.com/munichmade/devproxy/internal/ca"
	"github.com/spf13/cobra"
)

var caCmd = &cobra.Command{
	Use:   "ca",
	Short: "Manage the local Certificate Authority",
	Long:  `Manage the local Certificate Authority used for signing development certificates.`,
}

var caGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate a new root CA",
	Long: `Generate a new root Certificate Authority keypair.

This creates an ECDSA P-384 private key and a self-signed CA certificate
valid for 10 years. The CA is used to sign certificates for local development
domains.

The CA files are stored in:
  - Certificate: ~/Library/Application Support/devproxy/ca/ca.crt (macOS)
  - Private key: ~/Library/Application Support/devproxy/ca/ca.key (macOS)

WARNING: Regenerating the CA will invalidate all existing certificates.
         You will need to re-trust the new CA in your system keychain.`,
	Run: func(cmd *cobra.Command, args []string) {
		force, _ := cmd.Flags().GetBool("force")

		if ca.Exists() && !force {
			fmt.Println("CA already exists. Use --force to regenerate.")
			fmt.Printf("  Certificate: %s\n", ca.CertPath())
			fmt.Printf("  Private key: %s\n", ca.KeyPath())
			os.Exit(1)
		}

		generated, err := ca.Generate()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to generate CA: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("CA generated successfully.")
		fmt.Printf("  Certificate: %s\n", ca.CertPath())
		fmt.Printf("  Private key: %s\n", ca.KeyPath())
		fmt.Printf("  Valid until: %s\n", generated.Certificate.NotAfter.Format("2006-01-02"))
		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Println("  1. Trust the CA in your system keychain:")
		fmt.Println("     sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain", ca.CertPath())
		fmt.Println("  2. Or run: devproxy setup (recommended)")
	},
}

var caInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show CA information",
	Long:  `Display information about the current root CA certificate.`,
	Run: func(cmd *cobra.Command, args []string) {
		if !ca.Exists() {
			fmt.Println("No CA found. Run 'devproxy ca generate' to create one.")
			os.Exit(1)
		}

		loaded, err := ca.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load CA: %v\n", err)
			os.Exit(1)
		}

		cert := loaded.Certificate
		fmt.Println("CA Information:")
		fmt.Printf("  Subject:      %s\n", cert.Subject.CommonName)
		fmt.Printf("  Issuer:       %s\n", cert.Issuer.CommonName)
		fmt.Printf("  Serial:       %s\n", cert.SerialNumber.Text(16))
		fmt.Printf("  Valid from:   %s\n", cert.NotBefore.Format("2006-01-02 15:04:05"))
		fmt.Printf("  Valid until:  %s\n", cert.NotAfter.Format("2006-01-02 15:04:05"))
		fmt.Printf("  Key type:     ECDSA P-384\n")
		fmt.Println()
		fmt.Printf("  Certificate:  %s\n", ca.CertPath())
		fmt.Printf("  Private key:  %s\n", ca.KeyPath())
	},
}

func init() {
	caGenerateCmd.Flags().BoolP("force", "f", false, "Regenerate CA even if one exists")
	caCmd.AddCommand(caGenerateCmd)
	caCmd.AddCommand(caInfoCmd)
	rootCmd.AddCommand(caCmd)
}
