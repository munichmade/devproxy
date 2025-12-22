// Package paths provides XDG Base Directory Specification compliant path resolution.
// On macOS, it falls back to standard macOS locations when XDG variables are not set.
package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

const appName = "devproxy"

// Paths holds all resolved paths for the application.
type Paths struct {
	// ConfigDir is the directory for configuration files.
	// XDG: $XDG_CONFIG_HOME/devproxy or ~/.config/devproxy
	ConfigDir string

	// DataDir is the directory for persistent data (CA, certs).
	// XDG: $XDG_DATA_HOME/devproxy or ~/.local/share/devproxy
	// macOS fallback: ~/Library/Application Support/devproxy
	DataDir string

	// RuntimeDir is the directory for runtime files (PID, sockets).
	// XDG: $XDG_RUNTIME_DIR/devproxy or fallback to DataDir
	RuntimeDir string

	// CADir is the directory for CA certificates.
	CADir string

	// CertsDir is the directory for generated certificates.
	CertsDir string

	// ConfigFile is the path to the main configuration file.
	ConfigFile string

	// PIDFile is the path to the daemon PID file.
	PIDFile string

	// LogFile is the path to the daemon log file.
	LogFile string
}

var (
	defaultPaths *Paths
	pathsOnce    sync.Once
)

// Default returns the default paths for the current system.
// The result is cached after the first call.
func Default() *Paths {
	pathsOnce.Do(func() {
		defaultPaths = resolve()
	})
	return defaultPaths
}

// resolve determines all paths based on environment and platform.
func resolve() *Paths {
	home := homeDir()

	p := &Paths{}

	// When running as root, use system-wide paths
	if os.Geteuid() == 0 {
		p.ConfigDir = "/etc/devproxy"
		p.DataDir = "/var/lib/devproxy"
		p.RuntimeDir = "/var/run/devproxy"
	} else {
		// Config directory
		p.ConfigDir = resolveConfigDir(home)

		// Data directory
		p.DataDir = resolveDataDir(home)

		// Runtime directory
		p.RuntimeDir = resolveRuntimeDir(home, p.DataDir)
	}

	// Subdirectories
	p.CADir = filepath.Join(p.DataDir, "ca")
	p.CertsDir = filepath.Join(p.DataDir, "certs")

	// Files
	p.ConfigFile = filepath.Join(p.ConfigDir, "config.yaml")
	p.PIDFile = filepath.Join(p.RuntimeDir, "devproxy.pid")
	p.LogFile = filepath.Join(p.DataDir, "devproxy.log")

	return p
}

// resolveConfigDir determines the configuration directory.
func resolveConfigDir(home string) string {
	// Check XDG_CONFIG_HOME first (works on all platforms)
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, appName)
	}

	// Default: ~/.config/devproxy (same on macOS and Linux)
	return filepath.Join(home, ".config", appName)
}

// resolveDataDir determines the data directory.
func resolveDataDir(home string) string {
	// Check XDG_DATA_HOME first
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, appName)
	}

	// Platform-specific defaults
	if runtime.GOOS == "darwin" {
		// macOS: ~/Library/Application Support/devproxy
		return filepath.Join(home, "Library", "Application Support", appName)
	}

	// Linux/others: ~/.local/share/devproxy
	return filepath.Join(home, ".local", "share", appName)
}

// resolveRuntimeDir determines the runtime directory.
func resolveRuntimeDir(home string, dataDir string) string {
	// Check XDG_RUNTIME_DIR first
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return filepath.Join(xdg, appName)
	}

	// Linux: /run/user/<uid> is typically set as XDG_RUNTIME_DIR
	// If not set, fall back to data directory
	// macOS: No standard runtime dir, use data directory
	return dataDir
}

// homeDir returns the user's home directory.
func homeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	// Fallback for Windows (though we don't officially support it)
	if home := os.Getenv("USERPROFILE"); home != "" {
		return home
	}
	return "/"
}

// EnsureDirectories creates all necessary directories with proper permissions.
func (p *Paths) EnsureDirectories() error {
	dirs := []string{
		p.ConfigDir,
		p.DataDir,
		p.RuntimeDir,
		p.CADir,
		p.CertsDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}

	return nil
}

// Reset clears the cached default paths.
// Useful for testing with different environment variables.
func Reset() {
	defaultPaths = nil
	pathsOnce = sync.Once{}
}

// Convenience functions for common path access

// ConfigDir returns the configuration directory path.
func ConfigDir() string {
	return Default().ConfigDir
}

// DataDir returns the data directory path.
func DataDir() string {
	return Default().DataDir
}

// RuntimeDir returns the runtime directory path.
func RuntimeDir() string {
	return Default().RuntimeDir
}

// CADir returns the CA certificates directory path.
func CADir() string {
	return Default().CADir
}

// CertsDir returns the generated certificates directory path.
func CertsDir() string {
	return Default().CertsDir
}

// ConfigFile returns the main configuration file path.
func ConfigFile() string {
	return Default().ConfigFile
}

// PIDFile returns the daemon PID file path.
func PIDFile() string {
	return Default().PIDFile
}

// LogFile returns the daemon log file path.
func LogFile() string {
	return Default().LogFile
}

// EnsureDirectories creates all necessary directories using default paths.
func EnsureDirectories() error {
	return Default().EnsureDirectories()
}
