//go:build linux

package service

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	serviceName_    = "devproxy"
	serviceDir      = "/etc/systemd/system"
	serviceFileName = "devproxy.service"
)

func serviceName() string {
	return serviceName_
}

func servicePath() string {
	return serviceDir + "/" + serviceFileName
}

func isInstalled() bool {
	_, err := os.Stat(servicePath())
	return err == nil
}

func install(cfg Config) error {
	unit := generateUnit(cfg.BinaryPath)

	// Ensure the directory exists
	if err := os.MkdirAll(serviceDir, 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", serviceDir, err)
	}

	// Write the service file
	if err := os.WriteFile(servicePath(), []byte(unit), 0644); err != nil {
		return fmt.Errorf("failed to write service file: %w", err)
	}

	// Reload systemd daemon
	cmd := exec.Command("systemctl", "daemon-reload")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to reload systemd: %w: %s", err, strings.TrimSpace(string(output)))
	}

	// Enable the service
	cmd = exec.Command("systemctl", "enable", serviceName_)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to enable service: %w: %s", err, strings.TrimSpace(string(output)))
	}

	return nil
}

func uninstall() error {
	// Stop the service first
	_ = stop()

	// Disable the service
	cmd := exec.Command("systemctl", "disable", serviceName_)
	_ = cmd.Run() // Ignore errors if not enabled

	// Remove the service file
	if err := os.Remove(servicePath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove service file: %w", err)
	}

	// Reload systemd daemon
	cmd = exec.Command("systemctl", "daemon-reload")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to reload systemd: %w: %s", err, strings.TrimSpace(string(output)))
	}

	return nil
}

func start() error {
	cmd := exec.Command("systemctl", "start", serviceName_)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to start service: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func stop() error {
	cmd := exec.Command("systemctl", "stop", serviceName_)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to stop service: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func generateUnit(binaryPath string) string {
	return fmt.Sprintf(`[Unit]
Description=Development Proxy
After=network.target docker.service
Wants=docker.service

[Service]
Type=simple
ExecStart=%s run
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`, binaryPath)
}
