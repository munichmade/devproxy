// Package proxy provides HTTP and HTTPS proxy servers for devproxy.
package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// HTTPServer handles HTTP requests and redirects them to HTTPS.
type HTTPServer struct {
	addr      string
	httpsPort int
	server    *http.Server
	listener  net.Listener
}

// NewHTTPServer creates a new HTTP server that redirects to HTTPS.
// The httpsPort parameter specifies which port the HTTPS server is running on.
func NewHTTPServer(addr string, httpsPort int) *HTTPServer {
	return &HTTPServer{
		addr:      addr,
		httpsPort: httpsPort,
	}
}

// NewHTTPServerWithListener creates a new HTTP server using a pre-bound listener.
// This is used when ports are bound before dropping privileges.
func NewHTTPServerWithListener(listener net.Listener, httpsPort int) *HTTPServer {
	return &HTTPServer{
		addr:      listener.Addr().String(),
		httpsPort: httpsPort,
		listener:  listener,
	}
}

// Start begins listening for HTTP requests.
func (s *HTTPServer) Start() error {
	// If no listener was provided, create one
	if s.listener == nil {
		listener, err := net.Listen("tcp", s.addr)
		if err != nil {
			return fmt.Errorf("failed to listen on %s: %w", s.addr, err)
		}
		s.listener = listener
	}

	s.server = &http.Server{
		Handler:      s,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		if err := s.server.Serve(s.listener); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
		}
	}()

	return nil
}

// Stop gracefully shuts down the HTTP server.
func (s *HTTPServer) Stop() error {
	if s.server == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return s.server.Shutdown(ctx)
}

// Addr returns the address the server is listening on.
// Returns empty string if not started.
func (s *HTTPServer) Addr() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// ServeHTTP handles incoming HTTP requests by redirecting to HTTPS.
func (s *HTTPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Build the HTTPS URL preserving the original path and query
	host := r.Host

	// Remove port from host if present
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	// Add HTTPS port if not the default 443
	if s.httpsPort != 443 {
		host = fmt.Sprintf("%s:%d", host, s.httpsPort)
	}

	// Build redirect URL
	redirectURL := fmt.Sprintf("https://%s%s", host, r.RequestURI)

	// Use 301 Moved Permanently for GET/HEAD, 308 Permanent Redirect for others
	// This preserves the request method for POST/PUT/DELETE etc.
	statusCode := http.StatusMovedPermanently
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		statusCode = http.StatusPermanentRedirect
	}

	http.Redirect(w, r, redirectURL, statusCode)
}
