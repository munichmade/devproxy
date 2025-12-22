//go:build darwin

// Package ca provides Certificate Authority functionality for generating
// and managing local development certificates.
package ca

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

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
	cmd := exec.Command("sudo", "security", "add-trusted-cert",
		"-d",
		"-r", "trustRoot",
		"-k", "/Library/Keychains/System.keychain",
		certPath,
	)

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
	// First, we need to find the SHA-1 hash of our certificate
	cmd := exec.Command("sudo", "security", "delete-certificate",
		"-c", caCommonName,
		"/Library/Keychains/System.keychain",
	)

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

	// Use security verify-cert to check if the CA is actually trusted
	// This verifies the trust chain, not just presence in keychain
	cmd := exec.Command("security", "verify-cert",
		"-c", CertPath(),
	)

	// verify-cert returns 0 if trusted, non-zero otherwise
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
