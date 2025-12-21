//go:build linux

// Package ca provides certificate authority management for devproxy.
//
// NOTICE: Linux support is currently UNTESTED and EXPERIMENTAL.
// The trust store integration has been implemented based on documentation
// but has not been validated on actual Linux systems. Use with caution
// and please report any issues encountered.
package ca

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Linux distribution types
type distro int

const (
	distroUnknown distro = iota
	distroDebian         // Debian, Ubuntu, and derivatives
	distroRHEL           // RHEL, Fedora, CentOS, and derivatives
	distroArch           // Arch Linux and derivatives
)

// Trust store paths for different distributions
const (
	// Debian/Ubuntu
	debianCertDir    = "/usr/local/share/ca-certificates"
	debianCertName   = "devproxy-ca.crt"
	debianUpdateCmd  = "update-ca-certificates"

	// RHEL/Fedora
	rhelCertDir    = "/etc/pki/ca-trust/source/anchors"
	rhelCertName   = "devproxy-ca.pem"
	rhelUpdateCmd  = "update-ca-trust"

	// Arch Linux
	archTrustCmd = "trust"
)

// detectDistro attempts to detect the Linux distribution.
func detectDistro() distro {
	// Check for os-release file (modern standard)
	data, err := os.ReadFile("/etc/os-release")
	if err == nil {
		content := strings.ToLower(string(data))

		// Check for Arch first (can also have ID_LIKE=arch)
		if strings.Contains(content, "id=arch") || strings.Contains(content, "id_like=arch") {
			return distroArch
		}

		// Check for Debian-based (Ubuntu, Debian, Linux Mint, etc.)
		if strings.Contains(content, "id=debian") ||
			strings.Contains(content, "id=ubuntu") ||
			strings.Contains(content, "id_like=debian") ||
			strings.Contains(content, "id_like=ubuntu") {
			return distroDebian
		}

		// Check for RHEL-based (Fedora, CentOS, RHEL, Rocky, Alma, etc.)
		if strings.Contains(content, "id=fedora") ||
			strings.Contains(content, "id=rhel") ||
			strings.Contains(content, "id=centos") ||
			strings.Contains(content, "id=rocky") ||
			strings.Contains(content, "id=almalinux") ||
			strings.Contains(content, "id_like=fedora") ||
			strings.Contains(content, "id_like=rhel") {
			return distroRHEL
		}
	}

	// Fallback: check for existence of distro-specific files/commands
	if _, err := os.Stat("/etc/debian_version"); err == nil {
		return distroDebian
	}
	if _, err := os.Stat("/etc/redhat-release"); err == nil {
		return distroRHEL
	}
	if _, err := exec.LookPath(archTrustCmd); err == nil {
		return distroArch
	}

	return distroUnknown
}

// InstallTrust installs the CA certificate into the Linux system trust store.
func InstallTrust() error {
	if !Exists() {
		return fmt.Errorf("CA certificate not found at %s", CertPath())
	}

	d := detectDistro()

	switch d {
	case distroDebian:
		return installTrustDebian()
	case distroRHEL:
		return installTrustRHEL()
	case distroArch:
		return installTrustArch()
	default:
		return fmt.Errorf("unsupported Linux distribution; please install the CA certificate manually from %s", CertPath())
	}
}

// installTrustDebian installs trust for Debian/Ubuntu systems.
func installTrustDebian() error {
	destPath := filepath.Join(debianCertDir, debianCertName)

	// Copy certificate to ca-certificates directory
	if err := copyFile(CertPath(), destPath); err != nil {
		return fmt.Errorf("failed to copy certificate (are you running as root?): %w", err)
	}

	// Run update-ca-certificates
	cmd := exec.Command(debianUpdateCmd)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to run %s: %w\n%s", debianUpdateCmd, err, output)
	}

	return nil
}

// installTrustRHEL installs trust for RHEL/Fedora systems.
func installTrustRHEL() error {
	destPath := filepath.Join(rhelCertDir, rhelCertName)

	// Copy certificate to anchors directory
	if err := copyFile(CertPath(), destPath); err != nil {
		return fmt.Errorf("failed to copy certificate (are you running as root?): %w", err)
	}

	// Run update-ca-trust
	cmd := exec.Command(rhelUpdateCmd)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to run %s: %w\n%s", rhelUpdateCmd, err, output)
	}

	return nil
}

// installTrustArch installs trust for Arch Linux systems.
func installTrustArch() error {
	// Use trust anchor --store which handles everything
	cmd := exec.Command(archTrustCmd, "anchor", "--store", CertPath())
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to run trust anchor: %w\n%s", err, output)
	}

	return nil
}

// UninstallTrust removes the CA certificate from the Linux system trust store.
func UninstallTrust() error {
	d := detectDistro()

	switch d {
	case distroDebian:
		return uninstallTrustDebian()
	case distroRHEL:
		return uninstallTrustRHEL()
	case distroArch:
		return uninstallTrustArch()
	default:
		return fmt.Errorf("unsupported Linux distribution; please remove the CA certificate manually")
	}
}

// uninstallTrustDebian removes trust for Debian/Ubuntu systems.
func uninstallTrustDebian() error {
	destPath := filepath.Join(debianCertDir, debianCertName)

	// Remove certificate
	if err := os.Remove(destPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove certificate (are you running as root?): %w", err)
	}

	// Run update-ca-certificates
	cmd := exec.Command(debianUpdateCmd)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to run %s: %w\n%s", debianUpdateCmd, err, output)
	}

	return nil
}

// uninstallTrustRHEL removes trust for RHEL/Fedora systems.
func uninstallTrustRHEL() error {
	destPath := filepath.Join(rhelCertDir, rhelCertName)

	// Remove certificate
	if err := os.Remove(destPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove certificate (are you running as root?): %w", err)
	}

	// Run update-ca-trust
	cmd := exec.Command(rhelUpdateCmd)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to run %s: %w\n%s", rhelUpdateCmd, err, output)
	}

	return nil
}

// uninstallTrustArch removes trust for Arch Linux systems.
func uninstallTrustArch() error {
	// trust anchor --remove requires the certificate path or a pkcs11 URI
	// We need to find our certificate in the trust store first
	cmd := exec.Command(archTrustCmd, "anchor", "--remove", CertPath())
	if output, err := cmd.CombinedOutput(); err != nil {
		// Ignore errors if certificate wasn't in the store
		if !strings.Contains(string(output), "no such") {
			return fmt.Errorf("failed to run trust anchor --remove: %w\n%s", err, output)
		}
	}

	return nil
}

// IsTrusted checks if the CA certificate is trusted in the Linux system trust store.
func IsTrusted() bool {
	if !Exists() {
		return false
	}

	d := detectDistro()

	switch d {
	case distroDebian:
		destPath := filepath.Join(debianCertDir, debianCertName)
		_, err := os.Stat(destPath)
		return err == nil
	case distroRHEL:
		destPath := filepath.Join(rhelCertDir, rhelCertName)
		_, err := os.Stat(destPath)
		return err == nil
	case distroArch:
		// Check if trust list contains our CA
		cmd := exec.Command(archTrustCmd, "list")
		output, err := cmd.Output()
		if err != nil {
			return false
		}
		return strings.Contains(string(output), caCommonName)
	default:
		return false
	}
}

// NeedsSudo returns true if trust operations require root privileges.
func NeedsSudo() bool {
	return true
}

// TrustStoreName returns a human-readable name for the trust store.
func TrustStoreName() string {
	d := detectDistro()

	switch d {
	case distroDebian:
		return "system CA certificates (Debian/Ubuntu)"
	case distroRHEL:
		return "system CA trust (RHEL/Fedora)"
	case distroArch:
		return "system trust anchors (Arch)"
	default:
		return "system trust store"
	}
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	return os.WriteFile(dst, data, 0644)
}
