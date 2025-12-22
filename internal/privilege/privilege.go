// Package privilege provides utilities for managing root privileges.
package privilege

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"
)

// Info contains information about the user to drop privileges to.
type Info struct {
	UID      int
	GID      int
	Username string
	HomeDir  string
}

// GetOriginalUser returns the original user who invoked sudo.
// Returns nil if not running as root or no sudo user is detected.
func GetOriginalUser() (*Info, error) {
	// Not running as root, no need to drop privileges
	if os.Geteuid() != 0 {
		return nil, nil
	}

	// Check SUDO_UID and SUDO_GID (set by sudo)
	sudoUID := os.Getenv("SUDO_UID")
	sudoGID := os.Getenv("SUDO_GID")
	sudoUser := os.Getenv("SUDO_USER")

	if sudoUID == "" || sudoGID == "" {
		// Running as root but not via sudo - can't determine original user
		return nil, nil
	}

	uid, err := strconv.Atoi(sudoUID)
	if err != nil {
		return nil, fmt.Errorf("invalid SUDO_UID: %w", err)
	}

	gid, err := strconv.Atoi(sudoGID)
	if err != nil {
		return nil, fmt.Errorf("invalid SUDO_GID: %w", err)
	}

	// Get home directory from user lookup
	homeDir := ""
	if sudoUser != "" {
		if u, err := user.Lookup(sudoUser); err == nil {
			homeDir = u.HomeDir
		}
	}

	return &Info{
		UID:      uid,
		GID:      gid,
		Username: sudoUser,
		HomeDir:  homeDir,
	}, nil
}

// Drop drops privileges to the specified user.
// This must be called after binding privileged ports but before processing any untrusted data.
func Drop(info *Info) error {
	if info == nil {
		return nil
	}

	// Must set groups first, then GID, then UID (can't change groups after dropping root)

	// Set supplementary groups (clear them for security)
	if err := syscall.Setgroups([]int{info.GID}); err != nil {
		return fmt.Errorf("failed to set supplementary groups: %w", err)
	}

	// Set GID
	if err := syscall.Setgid(info.GID); err != nil {
		return fmt.Errorf("failed to set GID to %d: %w", info.GID, err)
	}

	// Set UID (do this last - after this we can't regain root)
	if err := syscall.Setuid(info.UID); err != nil {
		return fmt.Errorf("failed to set UID to %d: %w", info.UID, err)
	}

	// Update HOME environment variable
	if info.HomeDir != "" {
		os.Setenv("HOME", info.HomeDir)
	}

	// Verify we actually dropped privileges
	if os.Geteuid() == 0 {
		return fmt.Errorf("failed to drop root privileges")
	}

	return nil
}

// IsRoot returns true if the current process is running as root.
func IsRoot() bool {
	return os.Geteuid() == 0
}

// Elevate re-executes the current command with sudo.
// It prints the reason for elevation and replaces the current process.
// This function does not return on success.
func Elevate(reason string) error {
	if IsRoot() {
		return nil // Already root
	}

	// Print reason for elevation
	fmt.Fprintf(os.Stderr, "Requesting administrator privileges: %s\n", reason)

	// Get the path to the current executable
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Build sudo command with all original arguments
	args := append([]string{executable}, os.Args[1:]...)

	// Execute sudo, replacing the current process
	cmd := exec.Command("sudo", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("sudo failed: %w", err)
	}

	os.Exit(0)
	return nil // Never reached
}

// RequireRoot checks if root is needed and elevates if necessary.
// This function does not return if elevation is performed.
func RequireRoot(reason string) error {
	if IsRoot() {
		return nil
	}
	return Elevate(reason)
}
