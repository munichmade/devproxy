//go:build darwin

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	plistName = "com.devproxy.daemon.plist"
	plistDir  = "/Library/LaunchDaemons"
)

func serviceName() string {
	return "com.devproxy.daemon"
}

func plistPath() string {
	return filepath.Join(plistDir, plistName)
}

func isInstalled() bool {
	_, err := os.Stat(plistPath())
	return err == nil
}

func install(cfg Config) error {
	plist := generatePlist(cfg.BinaryPath)

	// Ensure the directory exists
	if err := os.MkdirAll(plistDir, 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", plistDir, err)
	}

	// Write the plist file
	if err := os.WriteFile(plistPath(), []byte(plist), 0644); err != nil {
		return fmt.Errorf("failed to write plist: %w", err)
	}

	return nil
}

func uninstall() error {
	// Stop the service first if running
	_ = stop()

	// Remove the plist file
	if err := os.Remove(plistPath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove plist: %w", err)
	}

	return nil
}

func start() error {
	cmd := exec.Command("launchctl", "load", plistPath())
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to load service: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func stop() error {
	cmd := exec.Command("launchctl", "unload", plistPath())
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to unload service: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func generatePlist(binaryPath string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.devproxy.daemon</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>run</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/var/log/devproxy.log</string>
    <key>StandardErrorPath</key>
    <string>/var/log/devproxy.log</string>
</dict>
</plist>
`, binaryPath)
}
