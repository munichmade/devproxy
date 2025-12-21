//go:build linux

// Package ca provides Certificate Authority functionality for generating
// and managing local development certificates.
package ca

import "fmt"

// InstallTrust adds the CA certificate to the Linux system trust store.
// This requires sudo/root privileges.
func InstallTrust() error {
	return fmt.Errorf("Linux trust installation not yet implemented")
}

// UninstallTrust removes the CA certificate from the Linux system trust store.
// This requires sudo/root privileges.
func UninstallTrust() error {
	return fmt.Errorf("Linux trust removal not yet implemented")
}

// IsTrusted checks if the CA certificate is trusted in the system.
func IsTrusted() bool {
	return false
}

// NeedsSudo returns true if trust operations require sudo.
func NeedsSudo() bool {
	return true
}

// TrustStoreName returns a human-readable name for the trust store.
func TrustStoreName() string {
	return "Linux system trust store"
}
