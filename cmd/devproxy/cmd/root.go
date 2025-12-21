// Package cmd provides the CLI commands for devproxy.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Build-time variables set via ldflags.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

var rootCmd = &cobra.Command{
	Use:   "devproxy",
	Short: "Local development reverse proxy with TLS and SNI support",
	Long: `DevProxy is a local development reverse proxy that provides:

  - Automatic TLS certificates for *.localhost domains
  - SNI-based routing for multiple services on standard ports
  - Docker integration with automatic service discovery
  - Built-in DNS server for seamless domain resolution

Start by running 'devproxy setup' to configure your system,
then 'devproxy start' to run the proxy daemon.`,
	Version: Version,
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.SetVersionTemplate(fmt.Sprintf("devproxy version {{.Version}}\ncommit: %s\nbuilt: %s\n", Commit, BuildDate))
}
