// Package proxy provides HTTP and HTTPS proxy servers.
package proxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/munichmade/devproxy/internal/cert"
)

// HTTPSServer is an HTTPS server with dynamic certificate generation.
type HTTPSServer struct {
	addr        string
	certManager *cert.Manager
	server      *http.Server
	listener    net.Listener
	handler     http.Handler
}

// NewHTTPSServer creates a new HTTPS server.
// The certManager is used for on-demand certificate generation during TLS handshake.
func NewHTTPSServer(addr string, certManager *cert.Manager, handler http.Handler) *HTTPSServer {
	return &HTTPSServer{
		addr:        addr,
		certManager: certManager,
		handler:     handler,
	}
}

// NewHTTPSServerWithListener creates a new HTTPS server using a pre-bound listener.
// This is used when ports are bound before dropping privileges.
func NewHTTPSServerWithListener(listener net.Listener, certManager *cert.Manager, handler http.Handler) *HTTPSServer {
	return &HTTPSServer{
		addr:        listener.Addr().String(),
		certManager: certManager,
		handler:     handler,
		listener:    listener,
	}
}

// Start starts the HTTPS server in the background.
func (s *HTTPSServer) Start() error {
	// Create TLS config with dynamic certificate generation
	tlsConfig := &tls.Config{
		GetCertificate: s.certManager.GetCertificate,
		MinVersion:     tls.VersionTLS12,
		// Enable HTTP/2 by default
		NextProtos: []string{"h2", "http/1.1"},
	}

	s.server = &http.Server{
		Addr:      s.addr,
		Handler:   s.handler,
		TLSConfig: tlsConfig,
		// Timeouts for security
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// If no listener was provided, create one
	if s.listener == nil {
		ln, err := net.Listen("tcp", s.addr)
		if err != nil {
			return fmt.Errorf("failed to listen on %s: %w", s.addr, err)
		}
		s.listener = ln
	}

	// Wrap with TLS
	s.listener = tls.NewListener(s.listener, tlsConfig)

	// Start serving in background
	go func() {
		if err := s.server.Serve(s.listener); err != nil && err != http.ErrServerClosed {
			fmt.Printf("HTTPS server error: %v\n", err)
		}
	}()

	return nil
}

// Stop gracefully stops the HTTPS server.
func (s *HTTPSServer) Stop() error {
	if s.server == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return s.server.Shutdown(ctx)
}

// Addr returns the actual address the server is listening on.
func (s *HTTPSServer) Addr() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}
