package proxy

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/munichmade/devproxy/internal/ca"
	"github.com/munichmade/devproxy/internal/cert"
	"github.com/munichmade/devproxy/internal/paths"
)

func setupTestCA(t *testing.T) *cert.Manager {
	t.Helper()

	// Use temp directory for test
	tmpDir, err := os.MkdirTemp("", "devproxy-https-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	os.Setenv("XDG_DATA_HOME", tmpDir)
	paths.Reset()

	// Generate CA
	if _, err := ca.Generate(); err != nil {
		t.Fatalf("failed to generate CA: %v", err)
	}

	// Create cert manager
	mgr, err := cert.NewManager()
	if err != nil {
		t.Fatalf("failed to create cert manager: %v", err)
	}

	return mgr
}

func TestHTTPSServer_StartStop(t *testing.T) {
	mgr := setupTestCA(t)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "Hello, HTTPS!")
	})

	server := NewHTTPSServer("127.0.0.1:0", mgr, handler)

	if err := server.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer server.Stop()

	addr := server.Addr()
	if addr == "" {
		t.Fatal("expected non-empty address after start")
	}

	// Make HTTPS request with SNI (required for cert generation)
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				ServerName:         "test.localhost",
			},
		},
	}

	req, _ := http.NewRequest("GET", "https://"+addr+"/", nil)
	req.Host = "test.localhost"

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "Hello, HTTPS!" {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestHTTPSServer_DynamicCertificate(t *testing.T) {
	mgr := setupTestCA(t)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Host: %s", r.Host)
	})

	server := NewHTTPSServer("127.0.0.1:0", mgr, handler)

	if err := server.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer server.Stop()

	addr := server.Addr()

	// Request with specific SNI
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				ServerName:         "app.localhost",
			},
		},
	}

	req, _ := http.NewRequest("GET", "https://"+addr+"/", nil)
	req.Host = "app.localhost"

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Verify TLS connection info
	if resp.TLS == nil {
		t.Fatal("expected TLS connection")
	}

	if len(resp.TLS.PeerCertificates) == 0 {
		t.Fatal("expected peer certificates")
	}

	// Certificate should be issued for app.localhost or *.localhost
	cert := resp.TLS.PeerCertificates[0]
	found := false
	for _, name := range cert.DNSNames {
		if name == "app.localhost" || name == "*.localhost" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("certificate doesn't cover app.localhost, DNSNames: %v", cert.DNSNames)
	}
}

func TestHTTPSServer_HTTP2(t *testing.T) {
	mgr := setupTestCA(t)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Protocol: %s", r.Proto)
	})

	server := NewHTTPSServer("127.0.0.1:0", mgr, handler)

	if err := server.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer server.Stop()

	addr := server.Addr()

	// HTTP/2 client with SNI
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				ServerName:         "test.localhost",
			},
			ForceAttemptHTTP2: true,
		},
	}

	req, _ := http.NewRequest("GET", "https://"+addr+"/", nil)
	req.Host = "test.localhost"

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Check that HTTP/2 was negotiated
	if resp.ProtoMajor != 2 {
		t.Errorf("expected HTTP/2, got %s", resp.Proto)
	}
}
