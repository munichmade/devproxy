//go:build linux

// Package resolver provides DNS resolver configuration for Linux.
//
// NOTICE: Linux resolver support is currently UNTESTED and EXPERIMENTAL.
// Linux uses various methods for DNS resolution depending on the distribution
// and configuration (systemd-resolved, NetworkManager, /etc/resolv.conf).
package resolver

import (
	"errors"
)

// ErrNotImplemented is returned when resolver functions are called on Linux.
var ErrNotImplemented = errors.New("resolver configuration not yet implemented for Linux")

// Config holds resolver configuration.
type Config struct {
	// Domains to configure resolvers for (e.g., "localhost", "test").
	Domains []string

	// Port the DNS server listens on.
	Port int
}

// Setup configures DNS resolution for the given domains.
// Not yet implemented for Linux.
func Setup(cfg Config) error {
	return ErrNotImplemented
}

// Remove removes DNS resolver configuration.
// Not yet implemented for Linux.
func Remove(domains []string) error {
	return ErrNotImplemented
}

// RemoveAll removes all devproxy-managed resolver files.
// Not yet implemented for Linux.
func RemoveAll() ([]string, error) {
	return nil, ErrNotImplemented
}

// IsManagedByDevproxy checks if a resolver config was created by devproxy.
// Not yet implemented for Linux.
func IsManagedByDevproxy(domain string) bool {
	return false
}

// ListManaged returns a list of domains with resolver config managed by devproxy.
// Not yet implemented for Linux.
func ListManaged() ([]string, error) {
	return nil, ErrNotImplemented
}

// IsConfigured checks if resolver is configured for the given domains.
// Not yet implemented for Linux.
func IsConfigured(domains []string) bool {
	return false
}

// GetConfiguredDomains returns a list of configured domains.
// Not yet implemented for Linux.
func GetConfiguredDomains() ([]string, error) {
	return nil, ErrNotImplemented
}

// NeedsSudo returns true since modifying system DNS config requires root.
func NeedsSudo() bool {
	return true
}
