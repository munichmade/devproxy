package ca

import (
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/munichmade/devproxy/internal/paths"
)

func TestGenerate(t *testing.T) {
	// Use temp directory for testing
	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)
	paths.Reset()

	ca, err := Generate()
	if err != nil {
		t.Fatalf("Generate() failed: %v", err)
	}

	// Verify CA struct is populated
	if ca.Certificate == nil {
		t.Error("Certificate is nil")
	}
	if ca.PrivateKey == nil {
		t.Error("PrivateKey is nil")
	}
	if len(ca.CertPEM) == 0 {
		t.Error("CertPEM is empty")
	}
	if len(ca.KeyPEM) == 0 {
		t.Error("KeyPEM is empty")
	}

	// Verify certificate properties
	cert := ca.Certificate
	if !cert.IsCA {
		t.Error("Certificate is not a CA")
	}
	if cert.Subject.CommonName != caCommonName {
		t.Errorf("CommonName = %q, want %q", cert.Subject.CommonName, caCommonName)
	}
	if len(cert.Subject.Organization) == 0 || cert.Subject.Organization[0] != caOrganization {
		t.Errorf("Organization = %v, want [%q]", cert.Subject.Organization, caOrganization)
	}

	// Verify key usage
	expectedKeyUsage := x509.KeyUsageCertSign | x509.KeyUsageCRLSign
	if cert.KeyUsage != expectedKeyUsage {
		t.Errorf("KeyUsage = %v, want %v", cert.KeyUsage, expectedKeyUsage)
	}

	// Verify validity period (approximately 10 years)
	validity := cert.NotAfter.Sub(cert.NotBefore)
	expectedValidity := time.Duration(caValidityYears) * 365 * 24 * time.Hour
	// Allow for leap years (up to 3 extra days over 10 years)
	if validity < expectedValidity || validity > expectedValidity+72*time.Hour {
		t.Errorf("Validity = %v, want approximately %v", validity, expectedValidity)
	}

	// Verify files were created
	certPath := filepath.Join(paths.CADir(), CACertFilename)
	keyPath := filepath.Join(paths.CADir(), CAKeyFilename)

	certInfo, err := os.Stat(certPath)
	if err != nil {
		t.Fatalf("Certificate file not found: %v", err)
	}
	keyInfo, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("Key file not found: %v", err)
	}

	// Verify file permissions
	if certInfo.Mode().Perm() != 0644 {
		t.Errorf("Certificate permissions = %o, want 0644", certInfo.Mode().Perm())
	}
	if keyInfo.Mode().Perm() != 0600 {
		t.Errorf("Key permissions = %o, want 0600", keyInfo.Mode().Perm())
	}
}

func TestExists(t *testing.T) {
	// Use temp directory for testing
	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)
	paths.Reset()

	// Initially should not exist
	if Exists() {
		t.Error("Exists() = true before generation, want false")
	}

	// Generate CA
	_, err := Generate()
	if err != nil {
		t.Fatalf("Generate() failed: %v", err)
	}

	// Now should exist
	if !Exists() {
		t.Error("Exists() = false after generation, want true")
	}

	// Remove cert, should return false
	os.Remove(filepath.Join(paths.CADir(), CACertFilename))
	if Exists() {
		t.Error("Exists() = true after removing cert, want false")
	}
}

func TestLoad(t *testing.T) {
	// Use temp directory for testing
	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)
	paths.Reset()

	// Generate CA first
	generated, err := Generate()
	if err != nil {
		t.Fatalf("Generate() failed: %v", err)
	}

	// Load CA
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify loaded CA matches generated CA
	if loaded.Certificate.SerialNumber.Cmp(generated.Certificate.SerialNumber) != 0 {
		t.Error("Loaded certificate serial number does not match generated")
	}
	if loaded.Certificate.Subject.CommonName != generated.Certificate.Subject.CommonName {
		t.Error("Loaded certificate common name does not match generated")
	}

	// Verify private key matches by comparing public keys
	if !loaded.PrivateKey.PublicKey.Equal(&generated.PrivateKey.PublicKey) {
		t.Error("Loaded private key does not match generated")
	}
}

func TestLoadOrGenerate(t *testing.T) {
	// Use temp directory for testing
	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)
	paths.Reset()

	// First call should generate
	ca1, err := LoadOrGenerate()
	if err != nil {
		t.Fatalf("LoadOrGenerate() failed on first call: %v", err)
	}

	// Second call should load the same CA
	ca2, err := LoadOrGenerate()
	if err != nil {
		t.Fatalf("LoadOrGenerate() failed on second call: %v", err)
	}

	// Verify same CA was loaded
	if ca1.Certificate.SerialNumber.Cmp(ca2.Certificate.SerialNumber) != 0 {
		t.Error("LoadOrGenerate() returned different CAs on subsequent calls")
	}
}

func TestLoad_NotExists(t *testing.T) {
	// Use temp directory for testing
	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)
	paths.Reset()

	// Load should fail when CA doesn't exist
	_, err := Load()
	if err == nil {
		t.Error("Load() succeeded when CA doesn't exist, want error")
	}
}

func TestCertPath(t *testing.T) {
	path := CertPath()
	if filepath.Base(path) != CACertFilename {
		t.Errorf("CertPath() base = %q, want %q", filepath.Base(path), CACertFilename)
	}
}

func TestKeyPath(t *testing.T) {
	path := KeyPath()
	if filepath.Base(path) != CAKeyFilename {
		t.Errorf("KeyPath() base = %q, want %q", filepath.Base(path), CAKeyFilename)
	}
}
