package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/munichmade/devproxy/internal/daemon"
	"github.com/munichmade/devproxy/internal/logging"
	"github.com/munichmade/devproxy/internal/paths"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:    "run",
	Short:  "Run daemon in foreground (for systemd/launchd)",
	Long:   `Run the devproxy daemon in the foreground. Used by systemd/launchd service managers.`,
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		// Ensure log directory exists
		logFile := paths.LogFile()
		if err := os.MkdirAll(filepath.Dir(logFile), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "failed to create log directory: %v\n", err)
			os.Exit(1)
		}

		// Initialize logging to file
		if err := logging.SetupFile(logging.LevelInfo, logFile); err != nil {
			fmt.Fprintf(os.Stderr, "failed to initialize logging: %v\n", err)
			os.Exit(1)
		}

		// Create daemon instance for PID management
		d := daemon.New()

		// Write our PID file (the daemon process writes its own PID)
		if err := d.WritePID(); err != nil {
			logging.Error("failed to write PID file", "error", err)
			os.Exit(1)
		}

		// Set up shutdown handler
		shutdown := daemon.NewShutdownHandler()

		// Register cleanup on shutdown
		shutdown.OnShutdown(func() {
			logging.Info("shutting down daemon")
			if err := d.RemovePID(); err != nil {
				logging.Error("failed to remove PID file", "error", err)
			}
		})

		// Start signal handling
		shutdown.Start()
		defer shutdown.Stop()

		logging.Info("devproxy daemon started", "pid", os.Getpid())

		// Main daemon loop
		for {
			select {
			case <-shutdown.Done():
				logging.Info("daemon stopped")
				return
			case <-shutdown.ReloadChan():
				logging.Info("received SIGHUP, reloading configuration")
				// TODO: Implement config reload when config loading is ready
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
}
