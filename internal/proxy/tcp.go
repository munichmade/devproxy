// Package proxy provides HTTP and TCP proxy functionality.
package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/munichmade/devproxy/internal/cert"
)

const (
	// tcpDialTimeout is the timeout for connecting to backend servers.
	tcpDialTimeout = 10 * time.Second

	// tcpCopyBufferSize is the buffer size for TCP data copying.
	tcpCopyBufferSize = 32 * 1024
)

var (
	// ErrEntrypointClosed is returned when the entrypoint is closed.
	ErrEntrypointClosed = errors.New("entrypoint closed")
)

// TCPEntrypoint handles TCP connections with optional TLS termination.
type TCPEntrypoint struct {
	name        string
	listen      string
	targetPort  int
	registry    *Registry
	certManager *cert.Manager
	logger      *slog.Logger

	listener net.Listener
	mu       sync.Mutex
	running  bool
	wg       sync.WaitGroup
}

// TCPEntrypointConfig configures a TCP entrypoint.
type TCPEntrypointConfig struct {
	Name        string
	Listen      string
	TargetPort  int
	Registry    *Registry
	CertManager *cert.Manager
	Logger      *slog.Logger
}

// NewTCPEntrypoint creates a new TCP entrypoint.
func NewTCPEntrypoint(cfg TCPEntrypointConfig) *TCPEntrypoint {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &TCPEntrypoint{
		name:        cfg.Name,
		listen:      cfg.Listen,
		targetPort:  cfg.TargetPort,
		registry:    cfg.Registry,
		certManager: cfg.CertManager,
		logger:      logger.With("entrypoint", cfg.Name),
	}
}

// NewTCPEntrypointWithListener creates a new TCP entrypoint using a pre-bound listener.
// This is used when ports are bound before dropping privileges.
func NewTCPEntrypointWithListener(cfg TCPEntrypointConfig, listener net.Listener) *TCPEntrypoint {
	ep := NewTCPEntrypoint(cfg)
	ep.listener = listener
	return ep
}

// Start begins listening for TCP connections.
func (e *TCPEntrypoint) Start(ctx context.Context) error {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return errors.New("entrypoint already running")
	}

	// If no listener was provided, create one
	if e.listener == nil {
		listener, err := net.Listen("tcp", e.listen)
		if err != nil {
			e.mu.Unlock()
			return fmt.Errorf("failed to listen on %s: %w", e.listen, err)
		}
		e.listener = listener
	}

	e.running = true
	e.mu.Unlock()

	e.logger.Info("TCP entrypoint started", "address", e.listen)

	// Accept connections in a goroutine
	go e.acceptLoop(ctx)

	return nil
}

// Stop gracefully shuts down the entrypoint.
func (e *TCPEntrypoint) Stop(ctx context.Context) error {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return nil
	}
	e.running = false
	listener := e.listener
	e.mu.Unlock()

	// Close listener to stop accepting new connections
	if listener != nil {
		listener.Close()
	}

	// Wait for active connections to finish (with timeout)
	done := make(chan struct{})
	go func() {
		e.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		e.logger.Info("TCP entrypoint stopped gracefully")
	case <-ctx.Done():
		e.logger.Warn("TCP entrypoint shutdown timed out")
	}

	return nil
}

// acceptLoop accepts incoming connections.
func (e *TCPEntrypoint) acceptLoop(ctx context.Context) {
	for {
		conn, err := e.listener.Accept()
		if err != nil {
			e.mu.Lock()
			running := e.running
			e.mu.Unlock()

			if !running {
				return // Normal shutdown
			}

			e.logger.Error("failed to accept connection", "error", err)
			continue
		}

		e.wg.Add(1)
		go func() {
			defer e.wg.Done()
			e.handleConnection(ctx, conn)
		}()
	}
}

// handleConnection processes a single TCP connection.
func (e *TCPEntrypoint) handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	clientAddr := conn.RemoteAddr().String()

	// Extract SNI from TLS ClientHello
	serverName, peekedBytes, err := ExtractSNI(conn)
	if err != nil {
		e.logger.Error("failed to extract SNI", "client", clientAddr, "error", err)
		return
	}
	if serverName == "" {
		e.logger.Warn("no SNI in ClientHello", "client", clientAddr)
		return
	}

	e.logger.Debug("connection received", "client", clientAddr, "sni", serverName)

	// Look up route in registry
	route := e.registry.Lookup(serverName)
	if route == nil {
		e.logger.Warn("no route for SNI", "sni", serverName, "client", clientAddr)
		return
	}

	// Determine backend address
	backendAddr := e.getBackendAddr(*route)

	// Wrap connection to replay peeked bytes for TLS handshake
	peekedConn := NewPeekedConn(conn, peekedBytes)

	// Perform TLS handshake with client using our certificate
	tlsConfig := &tls.Config{
		GetCertificate: e.certManager.GetCertificate,
	}

	tlsConn := tls.Server(peekedConn, tlsConfig)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		e.logger.Error("TLS handshake failed", "sni", serverName, "error", err)
		return
	}
	defer tlsConn.Close()

	// Connect to backend
	backendConn, err := net.DialTimeout("tcp", backendAddr, tcpDialTimeout)
	if err != nil {
		e.logger.Error("failed to connect to backend", "backend", backendAddr, "error", err)
		return
	}
	defer backendConn.Close()

	e.logger.Debug("proxying connection", "sni", serverName, "backend", backendAddr)

	// Proxy data bidirectionally
	e.proxyBidirectional(tlsConn, backendConn)
}

// getBackendAddr returns the backend address for a route.
func (e *TCPEntrypoint) getBackendAddr(route Route) string {
	// If targetPort is configured on the entrypoint, use it instead of route's backend port
	if e.targetPort > 0 {
		// Extract host from route.Backend and use entrypoint's targetPort
		host, _, err := net.SplitHostPort(route.Backend)
		if err != nil {
			// Backend might not have a port, use as-is
			host = route.Backend
		}
		return fmt.Sprintf("%s:%d", host, e.targetPort)
	}
	return route.Backend
}

// proxyBidirectional copies data between client and backend.
func (e *TCPEntrypoint) proxyBidirectional(client, backend net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	// Client -> Backend
	go func() {
		defer wg.Done()
		e.copyData(backend, client)
		// Close write side to signal EOF
		if tcpConn, ok := backend.(*net.TCPConn); ok {
			tcpConn.CloseWrite()
		}
	}()

	// Backend -> Client
	go func() {
		defer wg.Done()
		e.copyData(client, backend)
		// Close write side to signal EOF
		if tlsConn, ok := client.(*tls.Conn); ok {
			tlsConn.CloseWrite()
		}
	}()

	wg.Wait()
}

// copyData copies data from src to dst.
func (e *TCPEntrypoint) copyData(dst, src net.Conn) {
	buf := make([]byte, tcpCopyBufferSize)
	_, err := io.CopyBuffer(dst, src, buf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
		// Only log unexpected errors
		if opErr, ok := err.(*net.OpError); ok && opErr.Err.Error() == "use of closed network connection" {
			return
		}
		e.logger.Debug("copy error", "error", err)
	}
}

// Addr returns the listener's address, or empty string if not listening.
func (e *TCPEntrypoint) Addr() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.listener != nil {
		return e.listener.Addr().String()
	}
	return ""
}
