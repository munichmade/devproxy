package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/munichmade/devproxy/internal/ca"
	"github.com/munichmade/devproxy/internal/cert"
	"github.com/munichmade/devproxy/internal/config"
	"github.com/munichmade/devproxy/internal/daemon"
	"github.com/munichmade/devproxy/internal/dns"
	"github.com/munichmade/devproxy/internal/docker"
	"github.com/munichmade/devproxy/internal/logging"
	"github.com/munichmade/devproxy/internal/paths"
	"github.com/munichmade/devproxy/internal/privilege"
	"github.com/munichmade/devproxy/internal/proxy"
)

// chownRecursive changes ownership of a directory and all its contents.
func chownRecursive(path string, uid, gid int) error {
	return filepath.Walk(path, func(name string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return syscall.Lchown(name, uid, gid)
	})
}

var runCmd = &cobra.Command{
	Use:    "run",
	Short:  "Run daemon in foreground",
	Long:   `Run the devproxy daemon in the foreground. Useful for debugging or running under a process supervisor.`,
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

var initConfigCmd = &cobra.Command{
	Use:    "init-config",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := config.Load()
		return err
	},
}

func init() {
	rootCmd.AddCommand(runCmd, initConfigCmd)
}

func listenAll(addresses []string) ([]net.Listener, error) {
	return listenAllWith(addresses, net.Listen)
}

func listenAllWith(addresses []string, listen func(string, string) (net.Listener, error)) ([]net.Listener, error) {
	listeners := make([]net.Listener, 0, len(addresses))
	var unavailableErr error
	for _, address := range addresses {
		listener, err := listen("tcp", address)
		if err != nil {
			if errors.Is(err, syscall.EAFNOSUPPORT) || errors.Is(err, syscall.EADDRNOTAVAIL) {
				unavailableErr = fmt.Errorf("failed to listen on %s: %w", address, err)
				continue
			}
			closeListeners(listeners)
			return nil, fmt.Errorf("failed to listen on %s: %w", address, err)
		}
		listeners = append(listeners, listener)
	}
	if len(listeners) == 0 && unavailableErr != nil {
		return nil, unavailableErr
	}
	return listeners, nil
}

func closeListeners(listeners []net.Listener) {
	for _, listener := range listeners {
		_ = listener.Close()
	}
}

func loadConfig(originalUser *privilege.Info) (*config.Config, error) {
	if originalUser == nil {
		return config.Load()
	}
	if err := chownRecursive(paths.ConfigDir(), originalUser.UID, originalUser.GID); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("failed to chown config directory: %w", err)
	}

	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to get executable path: %w", err)
	}
	cmd := exec.Command(executable, "init-config")
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{
		Uid: uint32(originalUser.UID),
		Gid: uint32(originalUser.GID),
	}}
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to initialize user config: %w", err)
	}
	return config.LoadExisting()
}

func runDaemon() error {
	// =========================================================================
	// Get original user info BEFORE doing anything else
	// This must happen first while SUDO_UID/SUDO_GID are still set
	// =========================================================================
	originalUser, err := privilege.GetOriginalUser()
	if err != nil {
		return fmt.Errorf("failed to get original user: %w", err)
	}

	// Load config first to get port settings
	cfg, err := loadConfig(originalUser)
	if err != nil {
		return err
	}

	// =========================================================================
	// PRIVILEGED SECTION - Bind to privileged ports while running as root
	// =========================================================================
	httpCfg, _ := cfg.GetEntrypoint("http")
	httpsCfg, _ := cfg.GetEntrypoint("https")

	// Bind HTTP ports
	httpListeners, err := listenAll(httpCfg.Listen)
	if err != nil {
		return fmt.Errorf("failed to bind HTTP ports: %w", err)
	}
	defer closeListeners(httpListeners)

	// Bind HTTPS ports
	httpsListeners, err := listenAll(httpsCfg.Listen)
	if err != nil {
		return fmt.Errorf("failed to bind HTTPS ports: %w", err)
	}
	defer closeListeners(httpsListeners)

	// Bind DNS port if enabled
	var dnsListener net.PacketConn
	if cfg.DNS.Enabled {
		dnsListener, err = net.ListenPacket("udp", cfg.DNS.Listen)
		if err != nil {
			return fmt.Errorf("failed to bind DNS port %s: %w", cfg.DNS.Listen, err)
		}
		defer dnsListener.Close()
	}

	// Bind TCP entrypoint ports
	tcpListeners := make(map[string][]net.Listener)
	var allTCPListeners []net.Listener
	for name, epCfg := range cfg.Entrypoints {
		if name == "http" || name == "https" || epCfg.TargetPort <= 0 {
			continue
		}
		listeners, err := listenAll(epCfg.Listen)
		if err != nil {
			closeListeners(allTCPListeners)
			return fmt.Errorf("failed to bind TCP entrypoint %s: %w", name, err)
		}
		tcpListeners[name] = listeners
		allTCPListeners = append(allTCPListeners, listeners...)
	}
	defer closeListeners(allTCPListeners)

	// =========================================================================
	// DROP PRIVILEGES - Run as original user after binding privileged ports
	// =========================================================================
	if originalUser != nil {
		// Before dropping privileges, ensure data directories are owned by the original user
		// This is needed because directories may have been created by root
		dataDir := paths.DataDir()
		runtimeDir := paths.RuntimeDir()

		// Chown data directory and contents
		if err := chownRecursive(dataDir, originalUser.UID, originalUser.GID); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to chown data directory: %v\n", err)
		}

		// Chown runtime directory
		if err := chownRecursive(runtimeDir, originalUser.UID, originalUser.GID); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to chown runtime directory: %v\n", err)
		}

		if err := privilege.Drop(originalUser); err != nil {
			return fmt.Errorf("failed to drop privileges: %w", err)
		}
		// Log after dropping privileges (logging not set up yet)
		fmt.Fprintf(os.Stderr, "dropped privileges to user %s (uid=%d)\n", originalUser.Username, originalUser.UID)
	}

	// =========================================================================
	// UNPRIVILEGED SECTION - Everything below runs as the original user
	// =========================================================================

	// Ensure log directory exists
	logFile := paths.LogFile()
	if err := os.MkdirAll(filepath.Dir(logFile), 0o755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Initialize logging with configured level
	logLevel := logging.ParseLevel(cfg.Logging.Level)
	if err = logging.SetupFile(logLevel, logFile); err != nil {
		return fmt.Errorf("failed to initialize logging: %w", err)
	}

	logger := slog.Default()

	if originalUser != nil {
		logging.Info("privileges dropped", "user", originalUser.Username, "uid", originalUser.UID)
	}

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

	logging.Info("devproxy daemon starting", "pid", os.Getpid(), "log_level", cfg.Logging.Level)

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
		// Save state to file for CLI to read
		if err := registry.SaveState(); err != nil {
			logging.Error("failed to save route state", "error", err)
		}
	})
	logging.Info("route registry initialized")

	// =========================================================================
	// Start DNS Server (using pre-bound listener)
	// =========================================================================
	var dnsServer *dns.Server
	if cfg.DNS.Enabled && dnsListener != nil {
		dnsConfig := dns.Config{
			Addr:      cfg.DNS.Listen,
			Domains:   cfg.DNS.Domains,
			ResolveIP: net.ParseIP("127.0.0.1"),
			Upstream:  cfg.DNS.Upstream,
		}
		dnsServer = dns.NewWithListener(dnsConfig, dnsListener)
		if err := dnsServer.Start(); err != nil {
			return fmt.Errorf("failed to start DNS server: %w", err)
		}
		shutdown.OnShutdown(func() {
			if err := dnsServer.Stop(); err != nil {
				logging.Error("failed to stop DNS server", "error", err)
			}
		})
		logging.Info("DNS server started", "address", cfg.DNS.Listen, "domains", cfg.DNS.Domains)
	} else if !cfg.DNS.Enabled {
		logging.Info("DNS server disabled (using external DNS)")
	}

	// =========================================================================
	// Start HTTP Server (using pre-bound listener)
	// =========================================================================
	// Extract HTTPS port for redirect
	httpsPort := 443
	if _, portStr, err := net.SplitHostPort(httpsCfg.Listen[0]); err == nil {
		if p, err := net.LookupPort("tcp", portStr); err == nil {
			httpsPort = p
		}
	}

	httpServers := make([]*proxy.HTTPServer, 0, len(httpListeners))
	for _, listener := range httpListeners {
		httpServer := proxy.NewHTTPServerWithListener(listener, httpsPort)
		if err := httpServer.Start(); err != nil {
			return fmt.Errorf("failed to start HTTP server: %w", err)
		}
		httpServers = append(httpServers, httpServer)
	}
	shutdown.OnShutdown(func() {
		for _, server := range httpServers {
			if err := server.Stop(); err != nil {
				logging.Error("failed to stop HTTP server", "error", err)
			}
		}
	})
	logging.Info("HTTP servers started", "addresses", httpCfg.Listen)

	// =========================================================================
	// Start HTTPS Server (using pre-bound listener)
	// =========================================================================
	proxyHandler := proxy.NewProxyHandler(registry)
	// Wrap with access logger that checks config dynamically
	// This allows hot-reloading the access_log setting
	// Use a pointer-to-pointer so the closure sees config updates
	cfgPtr := &cfg
	httpsHandler := proxy.NewAccessLogger(proxyHandler, slog.Default(), func() bool {
		return (*cfgPtr).Logging.AccessLog
	})
	httpsServers := make([]*proxy.HTTPSServer, 0, len(httpsListeners))
	for _, listener := range httpsListeners {
		httpsServer := proxy.NewHTTPSServerWithListener(listener, certManager, httpsHandler)
		if err := httpsServer.Start(); err != nil {
			return fmt.Errorf("failed to start HTTPS server: %w", err)
		}
		httpsServers = append(httpsServers, httpsServer)
	}
	shutdown.OnShutdown(func() {
		for _, server := range httpsServers {
			if err := server.Stop(); err != nil {
				logging.Error("failed to stop HTTPS server", "error", err)
			}
		}
	})
	logging.Info("HTTPS servers started", "addresses", httpsCfg.Listen)

	// =========================================================================
	// Start TCP Entrypoints (using pre-bound listeners)
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

		listeners := tcpListeners[name]
		for _, listener := range listeners {
			tcpCfg := proxy.TCPEntrypointConfig{
				Name:        name,
				Listen:      listener.Addr().String(),
				TargetPort:  epCfg.TargetPort,
				Registry:    registry,
				CertManager: certManager,
				Logger:      logger,
			}

			tcpEntry := proxy.NewTCPEntrypointWithListener(tcpCfg, listener)
			if err := tcpEntry.Start(ctx); err != nil {
				logging.Error("failed to start TCP entrypoint", "name", name, "error", err)
				continue
			}

			tcpEntrypoints = append(tcpEntrypoints, tcpEntry)
		}
		logging.Info("TCP entrypoint started", "name", name, "addresses", epCfg.Listen, "target_port", epCfg.TargetPort)
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
				routeSync := docker.NewRouteSync(registry, dockerClient, "", logger)
				routeSync.SetCertManager(certManager)

				// Create and start watcher
				watcher := docker.NewWatcher(dockerClient, routeSync.HandleEvent, logger)
				if err := watcher.Start(ctx); err != nil {
					logging.Error("failed to start Docker watcher", "error", err)
				} else {
					logging.Info("Docker watcher started")

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
	// Start Config File Watcher for Hot Reload
	// =========================================================================
	configPath := paths.ConfigFile()
	configWatcher := config.NewWatcher(configPath, func(newCfg *config.Config) {
		applyConfigChanges(cfg, newCfg, dnsServer)
		cfg = newCfg
	})
	if err := configWatcher.Start(); err != nil {
		logging.Warn("failed to start config watcher", "error", err)
	} else {
		shutdown.OnShutdown(func() {
			configWatcher.Stop()
		})
	}

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
			newCfg, err := config.Load()
			if err != nil {
				logging.Error("failed to reload config", "error", err)
				continue
			}
			applyConfigChanges(cfg, newCfg, dnsServer)
			cfg = newCfg
			logging.Info("configuration reloaded")
		}
	}
}

// applyConfigChanges applies configuration changes that can be hot-reloaded.
func applyConfigChanges(oldCfg, newCfg *config.Config, dnsServer *dns.Server) {
	// Update logging level
	if oldCfg.Logging.Level != newCfg.Logging.Level {
		newLevel := logging.ParseLevel(newCfg.Logging.Level)
		logging.SetLevel(newLevel)
		logging.Info("log level changed", "old", oldCfg.Logging.Level, "new", newCfg.Logging.Level)
	}

	// Update DNS settings (domains and upstream only - listen address requires restart)
	if dnsServer != nil {
		domainsChanged := !slices.Equal(oldCfg.DNS.Domains, newCfg.DNS.Domains)
		upstreamChanged := oldCfg.DNS.Upstream != newCfg.DNS.Upstream

		if domainsChanged || upstreamChanged {
			dnsServer.UpdateConfig(newCfg.DNS.Domains, newCfg.DNS.Upstream)
		}

		// Warn if listen address changed (requires restart)
		if oldCfg.DNS.Listen != newCfg.DNS.Listen {
			logging.Warn("DNS listen address changed - restart required to apply",
				"old", oldCfg.DNS.Listen, "new", newCfg.DNS.Listen)
		}
	}

	// Check for entrypoint changes
	for name, oldEp := range oldCfg.Entrypoints {
		if newEp, exists := newCfg.Entrypoints[name]; exists {
			if !slices.Equal(oldEp.Listen, newEp.Listen) {
				logging.Warn("entrypoint listen address changed - restart required to apply",
					"entrypoint", name, "old", oldEp.Listen, "new", newEp.Listen)
			}
		}
	}
}
