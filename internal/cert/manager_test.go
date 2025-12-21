package cert

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"testing"
	"time"

	"github.com/munichmade/devproxy/internal/ca"
	"github.com/munichmade/devproxy/internal/paths"
)

func setupTestEnv(t *testing.T) func() {
	t.Helper()

	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "devproxy-cert-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Set XDG_DATA_HOME to use temp directory
	os.Setenv("XDG_DATA_HOME", tmpDir)
	paths.Reset()

	// Generate a CA for testing
	_, err = ca.Generate()
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to generate CA: %v", err)
	}

	return func() {
		os.RemoveAll(tmpDir)
		os.Unsetenv("XDG_DATA_HOME")
		paths.Reset()
	}
}

func TestNewManager(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	m, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if m == nil {
		t.Fatal("NewManager() returned nil")
	}

	if m.ca == nil {
		t.Error("Manager.ca is nil")
	}

	if m.cache == nil {
		t.Error("Manager.cache is nil")
	}
}

func TestNewManagerNoCA(t *testing.T) {
	// Create temp directory without CA
	tmpDir, err := os.MkdirTemp("", "devproxy-cert-test-noca")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	os.Setenv("XDG_DATA_HOME", tmpDir)
	paths.Reset()
	defer func() {
		os.Unsetenv("XDG_DATA_HOME")
		paths.Reset()
	}()

	_, err = NewManager()
	if err == nil {
		t.Fatal("NewManager() should fail without CA")
	}
}

func TestGetCertificate(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	m, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	tests := []struct {
		name       string
		serverName string
		wantCN     string
	}{
		{
			name:       "simple domain",
			serverName: "example.localhost",
			wantCN:     "example.localhost",
		},
		{
			name:       "subdomain gets wildcard",
			serverName: "api.example.localhost",
			wantCN:     "*.example.localhost",
		},
		{
			name:       "deep subdomain gets wildcard",
			serverName: "v1.api.example.localhost",
			wantCN:     "*.api.example.localhost",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hello := &tls.ClientHelloInfo{
				ServerName: tt.serverName,
			}

			cert, err := m.GetCertificate(hello)
			if err != nil {
				t.Fatalf("GetCertificate() error = %v", err)
			}

			if cert == nil {
				t.Fatal("GetCertificate() returned nil certificate")
			}

			// Parse the certificate to verify
			x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
			if err != nil {
				t.Fatalf("failed to parse certificate: %v", err)
			}

			if x509Cert.Subject.CommonName != tt.wantCN {
				t.Errorf("CommonName = %q, want %q", x509Cert.Subject.CommonName, tt.wantCN)
			}

			// Verify certificate is signed by our CA
			caData, _ := ca.Load()
			roots := x509.NewCertPool()
			roots.AddCert(caData.Certificate)

			_, err = x509Cert.Verify(x509.VerifyOptions{
				Roots: roots,
			})
			if err != nil {
				t.Errorf("certificate verification failed: %v", err)
			}
		})
	}
}

func TestGetCertificateCaching(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	m, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	hello := &tls.ClientHelloInfo{
		ServerName: "test.example.localhost",
	}

	// First call - should generate
	cert1, err := m.GetCertificate(hello)
	if err != nil {
		t.Fatalf("first GetCertificate() error = %v", err)
	}

	// Second call - should return cached
	cert2, err := m.GetCertificate(hello)
	if err != nil {
		t.Fatalf("second GetCertificate() error = %v", err)
	}

	// Should be the same certificate (same serial number)
	x509Cert1, _ := x509.ParseCertificate(cert1.Certificate[0])
	x509Cert2, _ := x509.ParseCertificate(cert2.Certificate[0])

	if x509Cert1.SerialNumber.Cmp(x509Cert2.SerialNumber) != 0 {
		t.Error("cached certificate has different serial number")
	}
}

func TestGetCertificateDiskCache(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	// Generate certificate with first manager
	m1, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	hello := &tls.ClientHelloInfo{
		ServerName: "cached.example.localhost",
	}

	cert1, err := m1.GetCertificate(hello)
	if err != nil {
		t.Fatalf("first GetCertificate() error = %v", err)
	}

	// Create new manager (simulating restart)
	m2, err := NewManager()
	if err != nil {
		t.Fatalf("second NewManager() error = %v", err)
	}

	// Should load from disk
	cert2, err := m2.GetCertificate(hello)
	if err != nil {
		t.Fatalf("second GetCertificate() error = %v", err)
	}

	// Should be the same certificate
	x509Cert1, _ := x509.ParseCertificate(cert1.Certificate[0])
	x509Cert2, _ := x509.ParseCertificate(cert2.Certificate[0])

	if x509Cert1.SerialNumber.Cmp(x509Cert2.SerialNumber) != 0 {
		t.Error("disk-cached certificate has different serial number")
	}
}

func TestCertificateValidity(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	m, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	hello := &tls.ClientHelloInfo{
		ServerName: "validity.example.localhost",
	}

	cert, err := m.GetCertificate(hello)
	if err != nil {
		t.Fatalf("GetCertificate() error = %v", err)
	}

	x509Cert, _ := x509.ParseCertificate(cert.Certificate[0])

	// Check validity period
	now := time.Now()
	if x509Cert.NotBefore.After(now) {
		t.Error("certificate NotBefore is in the future")
	}

	expectedExpiry := now.AddDate(0, 0, certValidityDays)
	if x509Cert.NotAfter.Before(now) {
		t.Error("certificate is already expired")
	}

	// Should expire within certValidityDays (+/- 1 day for timing)
	if x509Cert.NotAfter.After(expectedExpiry.AddDate(0, 0, 1)) {
		t.Errorf("certificate expires too late: %v", x509Cert.NotAfter)
	}
}

func TestToWildcard(t *testing.T) {
	tests := []struct {
		domain string
		want   string
	}{
		{"localhost", "localhost"},
		{"example.localhost", "example.localhost"},
		{"api.example.localhost", "*.example.localhost"},
		{"v1.api.example.localhost", "*.api.example.localhost"},
		{"a.b.c.d.localhost", "*.b.c.d.localhost"},
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			got := toWildcard(tt.domain)
			if got != tt.want {
				t.Errorf("toWildcard(%q) = %q, want %q", tt.domain, got, tt.want)
			}
		})
	}
}

func TestBuildDNSNames(t *testing.T) {
	names := buildDNSNames("*.example.localhost", "api.example.localhost")

	expected := map[string]bool{
		"*.example.localhost":   true,
		"example.localhost":     true,
		"api.example.localhost": true,
	}

	if len(names) != len(expected) {
		t.Errorf("got %d names, want %d", len(names), len(expected))
	}

	for _, name := range names {
		if !expected[name] {
			t.Errorf("unexpected DNS name: %q", name)
		}
	}
}

func TestClearCache(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	m, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// Generate a certificate
	hello := &tls.ClientHelloInfo{
		ServerName: "clear.example.localhost",
	}

	_, err = m.GetCertificate(hello)
	if err != nil {
		t.Fatalf("GetCertificate() error = %v", err)
	}

	// Clear cache
	if err := m.ClearCache(); err != nil {
		t.Fatalf("ClearCache() error = %v", err)
	}

	// Memory cache should be empty
	m.mu.RLock()
	cacheLen := len(m.cache)
	m.mu.RUnlock()

	if cacheLen != 0 {
		t.Errorf("cache length = %d, want 0", cacheLen)
	}

	// Disk cache should be empty
	entries, _ := os.ReadDir(paths.CertsDir())
	if len(entries) != 0 {
		t.Errorf("disk cache has %d files, want 0", len(entries))
	}
}

func TestEnsureCertificate(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	m, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	t.Run("generates certificate for new domain", func(t *testing.T) {
		err := m.EnsureCertificate("new.example.localhost")
		if err != nil {
			t.Errorf("EnsureCertificate() error = %v", err)
		}

		// Verify it's now in cache
		hello := &tls.ClientHelloInfo{ServerName: "new.example.localhost"}
		cert, err := m.GetCertificate(hello)
		if err != nil {
			t.Errorf("GetCertificate() after EnsureCertificate error = %v", err)
		}
		if cert == nil {
			t.Error("expected certificate to be cached")
		}
	})

	t.Run("succeeds for already cached domain", func(t *testing.T) {
		// First call generates
		err := m.EnsureCertificate("cached.example.localhost")
		if err != nil {
			t.Fatalf("first EnsureCertificate() error = %v", err)
		}

		// Second call should succeed without error
		err = m.EnsureCertificate("cached.example.localhost")
		if err != nil {
			t.Errorf("second EnsureCertificate() error = %v", err)
		}
	})

	t.Run("returns error for empty domain", func(t *testing.T) {
		err := m.EnsureCertificate("")
		if err == nil {
			t.Error("expected error for empty domain")
		}
	})

	t.Run("uses wildcard for subdomains", func(t *testing.T) {
		err := m.EnsureCertificate("sub.wildcard.localhost")
		if err != nil {
			t.Fatalf("EnsureCertificate() error = %v", err)
		}

		// Another subdomain should use same wildcard cert
		hello := &tls.ClientHelloInfo{ServerName: "other.wildcard.localhost"}
		cert, err := m.GetCertificate(hello)
		if err != nil {
			t.Errorf("GetCertificate() error = %v", err)
		}

		// Should have wildcard in DNS names
		x509Cert, _ := x509.ParseCertificate(cert.Certificate[0])
		hasWildcard := false
		for _, name := range x509Cert.DNSNames {
			if name == "*.wildcard.localhost" {
				hasWildcard = true
				break
			}
		}
		if !hasWildcard {
			t.Error("expected wildcard in certificate DNS names")
		}
	})
}
