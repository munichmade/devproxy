package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"

	"github.com/munichmade/devproxy/internal/ca"
	"github.com/munichmade/devproxy/internal/cert"
	"github.com/munichmade/devproxy/internal/config"
	"github.com/munichmade/devproxy/internal/daemon"
	"github.com/munichmade/devproxy/internal/dns"
	"github.com/munichmade/devproxy/internal/docker"
	"github.com/munichmade/devproxy/internal/logging"
	"github.com/munichmade/devproxy/internal/paths"
	"github.com/munichmade/devproxy/internal/proxy"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:    "run",
	Short:  "Run daemon in foreground (for systemd/launchd)",
	Long:   `Run the devproxy daemon in the foreground. Used by systemd/launchd service managers.`,
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runDaemon(); err != nil {
			// Log the error to file since stderr may be nil when running as daemon
			logging.Error("daemon fatal error", "error", err)
			fmt.Fprintf(os.Stderr, "daemon error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
}

func runDaemon() error {
	// Ensure log directory exists
	logFile := paths.LogFile()
	if err := os.MkdirAll(filepath.Dir(logFile), 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Initialize logging to file
	if err := logging.SetupFile(logging.LevelInfo, logFile); err != nil {
		return fmt.Errorf("failed to initialize logging: %w", err)
	}

	logger := slog.Default()

	// Create daemon instance for PID management
	d := daemon.New()

	// Write our PID file
	if err := d.WritePID(); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	// Set up shutdown handler
	shutdown := daemon.NewShutdownHandler()
	ctx := shutdown.Context()

	// Register PID cleanup on shutdown
	shutdown.OnShutdown(func() {
		logging.Info("shutting down daemon")
		if err := d.RemovePID(); err != nil {
			logging.Error("failed to remove PID file", "error", err)
		}
	})

	// Start signal handling
	shutdown.Start()
	defer shutdown.Stop()

	logging.Info("devproxy daemon starting", "pid", os.Getpid())

	// =========================================================================
	// Load Configuration
	// =========================================================================
	cfg, err := config.Load()
	if err != nil {
		// If no config exists, use defaults
		logging.Warn("failed to load config, using defaults", "error", err)
		cfg = config.Default()
	}
	logging.Info("configuration loaded")

	// =========================================================================
	// Initialize CA and Certificate Manager
	// =========================================================================
	_, err = ca.LoadOrGenerate()
	if err != nil {
		return fmt.Errorf("failed to initialize CA: %w", err)
	}
	logging.Info("CA initialized", "path", ca.CertPath())

	certManager, err := cert.NewManager()
	if err != nil {
		return fmt.Errorf("failed to initialize certificate manager: %w", err)
	}
	logging.Info("certificate manager initialized")

	// =========================================================================
	// Initialize Route Registry
	// =========================================================================
	registry := proxy.NewRegistry()
	registry.OnChange(func() {
		logging.Debug("route registry updated", "count", registry.Count())
	})
	logging.Info("route registry initialized")

	// =========================================================================
	// Start DNS Server (optional - can be disabled if using external DNS)
	// =========================================================================
	var dnsServer *dns.Server
	if cfg.DNS.Enabled {
		dnsConfig := dns.Config{
			Addr:      cfg.DNS.Listen,
			Domains:   cfg.DNS.Domains,
			ResolveIP: net.ParseIP("127.0.0.1"),
			Upstream:  cfg.DNS.Upstream,
		}
		dnsServer = dns.New(dnsConfig)
		if err := dnsServer.Start(); err != nil {
			return fmt.Errorf("failed to start DNS server: %w", err)
		}
		shutdown.OnShutdown(func() {
			if err := dnsServer.Stop(); err != nil {
				logging.Error("failed to stop DNS server", "error", err)
			}
		})
		logging.Info("DNS server started", "address", cfg.DNS.Listen, "domains", cfg.DNS.Domains)
	} else {
		logging.Info("DNS server disabled (using external DNS)")
	}

	// =========================================================================
	// Start HTTP Server (redirects to HTTPS)
	// =========================================================================
	httpCfg, _ := cfg.GetEntrypoint("http")
	httpsCfg, _ := cfg.GetEntrypoint("https")

	// Extract HTTPS port for redirect
	httpsPort := 443
	if _, portStr, err := net.SplitHostPort(httpsCfg.Listen); err == nil {
		if p, err := net.LookupPort("tcp", portStr); err == nil {
			httpsPort = p
		}
	}

	httpServer := proxy.NewHTTPServer(httpCfg.Listen, httpsPort)
	if err := httpServer.Start(); err != nil {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}
	shutdown.OnShutdown(func() {
		if err := httpServer.Stop(); err != nil {
			logging.Error("failed to stop HTTP server", "error", err)
		}
	})
	logging.Info("HTTP server started", "address", httpCfg.Listen)

	// =========================================================================
	// Start HTTPS Server
	// =========================================================================
	proxyHandler := proxy.NewProxyHandler(registry)
	httpsServer := proxy.NewHTTPSServer(httpsCfg.Listen, certManager, proxyHandler)
	if err := httpsServer.Start(); err != nil {
		return fmt.Errorf("failed to start HTTPS server: %w", err)
	}
	shutdown.OnShutdown(func() {
		if err := httpsServer.Stop(); err != nil {
			logging.Error("failed to stop HTTPS server", "error", err)
		}
	})
	logging.Info("HTTPS server started", "address", httpsCfg.Listen)

	// =========================================================================
	// Start TCP Entrypoints (postgres, mongo, etc.)
	// =========================================================================
	var tcpEntrypoints []*proxy.TCPEntrypoint
	for name, epCfg := range cfg.Entrypoints {
		// Skip http and https - they're handled above
		if name == "http" || name == "https" {
			continue
		}

		// Only start if target_port is configured (indicates TCP entrypoint)
		if epCfg.TargetPort <= 0 {
			continue
		}

		tcpCfg := proxy.TCPEntrypointConfig{
			Name:        name,
			Listen:      epCfg.Listen,
			TargetPort:  epCfg.TargetPort,
			Registry:    registry,
			CertManager: certManager,
			Logger:      logger,
		}

		tcpEntry := proxy.NewTCPEntrypoint(tcpCfg)
		if err := tcpEntry.Start(ctx); err != nil {
			logging.Error("failed to start TCP entrypoint", "name", name, "error", err)
			continue
		}

		tcpEntrypoints = append(tcpEntrypoints, tcpEntry)
		logging.Info("TCP entrypoint started", "name", name, "address", epCfg.Listen, "target_port", epCfg.TargetPort)
	}

	// Register TCP entrypoint cleanup
	shutdown.OnShutdown(func() {
		for _, ep := range tcpEntrypoints {
			if err := ep.Stop(context.Background()); err != nil {
				logging.Error("failed to stop TCP entrypoint", "error", err)
			}
		}
	})

	// =========================================================================
	// Initialize Docker Integration
	// =========================================================================
	if cfg.Docker.Enabled {
		dockerClient, err := docker.NewClient(logger)
		if err != nil {
			logging.Error("failed to create Docker client", "error", err)
		} else {
			if err := dockerClient.Connect(ctx); err != nil {
				logging.Error("failed to connect to Docker", "error", err)
			} else {
				logging.Info("connected to Docker daemon")

				// Create route sync to handle container events
				routeSync := docker.NewRouteSync(registry, dockerClient, cfg.Docker.LabelPrefix, "", logger)
				routeSync.SetCertManager(certManager)

				// Create and start watcher
				watcher := docker.NewWatcher(dockerClient, cfg.Docker.LabelPrefix, routeSync.HandleEvent, logger)
				if err := watcher.Start(ctx); err != nil {
					logging.Error("failed to start Docker watcher", "error", err)
				} else {
					logging.Info("Docker watcher started", "label_prefix", cfg.Docker.LabelPrefix)

					// Register cleanup
					shutdown.OnShutdown(func() {
						watcher.Stop()
						if err := dockerClient.Close(); err != nil {
							logging.Error("failed to close Docker client", "error", err)
						}
					})
				}
			}
		}
	} else {
		logging.Info("Docker integration disabled")
	}

	// =========================================================================
	// Daemon Ready
	// =========================================================================
	logging.Info("devproxy daemon started successfully",
		"pid", os.Getpid(),
		"dns", cfg.DNS.Listen,
		"http", httpCfg.Listen,
		"https", httpsCfg.Listen,
	)

	// =========================================================================
	// Main Loop - Wait for shutdown or reload signals
	// =========================================================================
	for {
		select {
		case <-shutdown.Done():
			logging.Info("daemon stopped")
			return nil

		case <-shutdown.ReloadChan():
			logging.Info("received SIGHUP, reloading configuration")
			// Reload config
			newCfg, err := config.Load()
			if err != nil {
				logging.Error("failed to reload config", "error", err)
				continue
			}
			cfg = newCfg
			logging.Info("configuration reloaded")
		}
	}
}
