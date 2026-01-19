//go:build darwin

// Package ca provides Certificate Authority functionality for generating
// and managing local development certificates.
package ca

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// isRoot returns true if the current process is running as root.
func isRoot() bool {
	return os.Geteuid() == 0
}

// InstallTrust adds the CA certificate to the macOS System Keychain.
// This requires sudo/admin privileges.
func InstallTrust() error {
	certPath := CertPath()

	if !Exists() {
		return fmt.Errorf("CA certificate not found at %s, run 'devproxy ca generate' first", certPath)
	}

	// Check if already trusted
	if IsTrusted() {
		return nil // Already trusted, nothing to do
	}

	// Add to System Keychain with trust settings
	// -d: add to admin cert store
	// -r trustRoot: trust as root certificate
	// -k: keychain to add to
	var cmd *exec.Cmd
	if isRoot() {
		// Already running as root, no need for sudo
		cmd = exec.Command("security", "add-trusted-cert",
			"-d",
			"-r", "trustRoot",
			"-k", "/Library/Keychains/System.keychain",
			certPath,
		)
	} else {
		cmd = exec.Command("sudo", "security", "add-trusted-cert",
			"-d",
			"-r", "trustRoot",
			"-k", "/Library/Keychains/System.keychain",
			certPath,
		)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add CA to System Keychain: %w\n%s", err, stderr.String())
	}

	return nil
}

// UninstallTrust removes the CA certificate from the macOS System Keychain.
// This requires sudo/admin privileges.
func UninstallTrust() error {
	// Find and delete the certificate by name
	var cmd *exec.Cmd
	if isRoot() {
		// Already running as root, no need for sudo
		cmd = exec.Command("security", "delete-certificate",
			"-c", caCommonName,
			"/Library/Keychains/System.keychain",
		)
	} else {
		cmd = exec.Command("sudo", "security", "delete-certificate",
			"-c", caCommonName,
			"/Library/Keychains/System.keychain",
		)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// If the certificate doesn't exist, that's fine
		if strings.Contains(stderr.String(), "could not be found") ||
			strings.Contains(stderr.String(), "SecKeychainSearchCopyNext") {
			return nil
		}
		return fmt.Errorf("failed to remove CA from System Keychain: %w\n%s", err, stderr.String())
	}

	return nil
}

// IsTrusted checks if the CA certificate is trusted in the macOS System Keychain.
func IsTrusted() bool {
	if !Exists() {
		return false
	}

	// Check if the certificate exists in the System Keychain
	// This is the correct way to verify installation, not verify-cert
	// (verify-cert returns success for self-signed CAs even if not installed)
	cmd := exec.Command("security", "find-certificate",
		"-c", caCommonName,
		"/Library/Keychains/System.keychain",
	)

	// Suppress output - we only care about the exit code
	cmd.Stdout = nil
	cmd.Stderr = nil

	// find-certificate returns 0 if found, non-zero otherwise
	return cmd.Run() == nil
}

// NeedsSudo returns true if trust operations require sudo.
func NeedsSudo() bool {
	return true
}

// TrustStoreName returns a human-readable name for the trust store.
func TrustStoreName() string {
	return "macOS System Keychain"
}
