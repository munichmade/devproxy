// Package service provides system service installation and management for devproxy.
package service

import (
	"fmt"
	"os"
	"path/filepath"
)

// Config holds service installation configuration.
type Config struct {
	// BinaryPath is the path to the devproxy binary.
	// If empty, the current executable path is used.
	BinaryPath string
}

// IsInstalled checks if devproxy is installed as a system service.
func IsInstalled() bool {
	return isInstalled()
}

// Install installs devproxy as a system service.
func Install(cfg Config) error {
	if cfg.BinaryPath == "" {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("failed to get executable path: %w", err)
		}
		cfg.BinaryPath, err = filepath.EvalSymlinks(exe)
		if err != nil {
			return fmt.Errorf("failed to resolve executable path: %w", err)
		}
	}

	return install(cfg)
}

// Uninstall removes devproxy from system services.
func Uninstall() error {
	return uninstall()
}

// Start starts the system service.
func Start() error {
	return start()
}

// Stop stops the system service.
func Stop() error {
	return stop()
}

// ServiceName returns the name of the service for the current platform.
func ServiceName() string {
	return serviceName()
}
