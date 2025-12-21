package proxy

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/munichmade/devproxy/internal/cert"
)

// mockCertManager implements a minimal cert manager for testing.
type mockCertManager struct {
	cert tls.Certificate
}

func (m *mockCertManager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	return &m.cert, nil
}

// generateTestCert creates a self-signed certificate for testing.
func generateTestCert(t *testing.T) tls.Certificate {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"test.localhost", "*.localhost"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("failed to load certificate: %v", err)
	}

	return tlsCert
}

func TestTCPEntrypoint_StartStop(t *testing.T) {
	t.Run("starts and stops cleanly", func(t *testing.T) {
		registry := NewRegistry()
		testCert := generateTestCert(t)
		mockCM := &mockCertManager{cert: testCert}

		// Create a real cert.Manager that wraps our mock
		// For testing, we'll use the TCPEntrypoint with a nil certManager and test Start/Stop
		ep := NewTCPEntrypoint(TCPEntrypointConfig{
			Name:        "test",
			Listen:      "127.0.0.1:0", // Random port
			Registry:    registry,
			CertManager: (*cert.Manager)(nil), // Will be replaced in actual test
		})

		// We can't easily test with nil certManager, so just test start/stop mechanics
		// by creating a custom entrypoint with a discard logger
		ep2 := &TCPEntrypoint{
			name:     "test",
			listen:   "127.0.0.1:0",
			registry: registry,
			logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		}

		ctx := context.Background()
		err := ep2.Start(ctx)
		if err != nil {
			t.Fatalf("failed to start: %v", err)
		}

		addr := ep2.Addr()
		if addr == "" {
			t.Error("expected non-empty address after start")
		}

		// Verify we can connect
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}
		conn.Close()

		// Stop
		stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		err = ep2.Stop(stopCtx)
		if err != nil {
			t.Fatalf("failed to stop: %v", err)
		}

		// Verify we can't connect anymore
		_, err = net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			t.Error("expected connection to fail after stop")
		}

		// Unused variable fix
		_ = ep
		_ = mockCM
	})

	t.Run("returns error when already running", func(t *testing.T) {
		registry := NewRegistry()

		ep := &TCPEntrypoint{
			name:     "test",
			listen:   "127.0.0.1:0",
			registry: registry,
			logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		}

		ctx := context.Background()
		err := ep.Start(ctx)
		if err != nil {
			t.Fatalf("failed to start: %v", err)
		}
		defer ep.Stop(ctx)

		err = ep.Start(ctx)
		if err == nil {
			t.Error("expected error when starting already running entrypoint")
		}
	})

	t.Run("returns error for invalid address", func(t *testing.T) {
		registry := NewRegistry()

		ep := &TCPEntrypoint{
			name:     "test",
			listen:   "invalid:address:format",
			registry: registry,
			logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		}

		ctx := context.Background()
		err := ep.Start(ctx)
		if err == nil {
			t.Error("expected error for invalid listen address")
		}
	})
}

func TestTCPEntrypoint_GetBackendAddr(t *testing.T) {
	t.Run("uses route backend when no targetPort", func(t *testing.T) {
		ep := &TCPEntrypoint{
			targetPort: 0,
		}

		route := Route{
			Host:    "test.localhost",
			Backend: "container:8080",
		}

		addr := ep.getBackendAddr(route)
		if addr != "container:8080" {
			t.Errorf("expected 'container:8080', got '%s'", addr)
		}
	})

	t.Run("uses targetPort when configured", func(t *testing.T) {
		ep := &TCPEntrypoint{
			targetPort: 3000,
		}

		route := Route{
			Host:    "test.localhost",
			Backend: "container:8080",
		}

		addr := ep.getBackendAddr(route)
		if addr != "container:3000" {
			t.Errorf("expected 'container:3000', got '%s'", addr)
		}
	})

	t.Run("handles backend without port", func(t *testing.T) {
		ep := &TCPEntrypoint{
			targetPort: 3000,
		}

		route := Route{
			Host:    "test.localhost",
			Backend: "container",
		}

		addr := ep.getBackendAddr(route)
		if addr != "container:3000" {
			t.Errorf("expected 'container:3000', got '%s'", addr)
		}
	})
}

func TestTCPEntrypoint_Addr(t *testing.T) {
	t.Run("returns empty when not listening", func(t *testing.T) {
		ep := &TCPEntrypoint{}
		if ep.Addr() != "" {
			t.Error("expected empty address when not listening")
		}
	})

	t.Run("returns address when listening", func(t *testing.T) {
		ep := &TCPEntrypoint{
			listen: "127.0.0.1:0",
			logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		}

		ctx := context.Background()
		err := ep.Start(ctx)
		if err != nil {
			t.Fatalf("failed to start: %v", err)
		}
		defer ep.Stop(ctx)

		addr := ep.Addr()
		if addr == "" {
			t.Error("expected non-empty address")
		}

		// Should be a valid address
		_, _, err = net.SplitHostPort(addr)
		if err != nil {
			t.Errorf("invalid address format: %v", err)
		}
	})
}

func TestNewTCPEntrypoint(t *testing.T) {
	t.Run("creates entrypoint with config", func(t *testing.T) {
		registry := NewRegistry()

		ep := NewTCPEntrypoint(TCPEntrypointConfig{
			Name:       "https",
			Listen:     ":443",
			TargetPort: 8080,
			Registry:   registry,
		})

		if ep.name != "https" {
			t.Errorf("expected name 'https', got '%s'", ep.name)
		}
		if ep.listen != ":443" {
			t.Errorf("expected listen ':443', got '%s'", ep.listen)
		}
		if ep.targetPort != 8080 {
			t.Errorf("expected targetPort 8080, got %d", ep.targetPort)
		}
		if ep.registry != registry {
			t.Error("registry not set correctly")
		}
	})

	t.Run("uses default logger when nil", func(t *testing.T) {
		ep := NewTCPEntrypoint(TCPEntrypointConfig{
			Name:   "test",
			Listen: ":8443",
		})

		if ep.logger == nil {
			t.Error("expected non-nil logger")
		}
	})
}
