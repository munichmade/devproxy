// Package proxy provides HTTP and TCP proxy functionality.
package proxy

import (
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

// ProxyTCP copies data bidirectionally between client and backend connections.
// It handles half-close scenarios properly and waits for both directions to complete.
// Returns nil on successful completion, or an error if either direction fails.
func ProxyTCP(client, backend net.Conn) error {
	var wg sync.WaitGroup
	wg.Add(2)

	var clientErr, backendErr error

	// Client -> Backend
	go func() {
		defer wg.Done()
		_, clientErr = io.Copy(backend, client)
		// Signal half-close to backend
		closeWrite(backend)
	}()

	// Backend -> Client
	go func() {
		defer wg.Done()
		_, backendErr = io.Copy(client, backend)
		// Signal half-close to client
		closeWrite(client)
	}()

	wg.Wait()

	// Return first non-nil error, ignoring EOF which is normal
	if clientErr != nil && !isNormalClose(clientErr) {
		return clientErr
	}
	if backendErr != nil && !isNormalClose(backendErr) {
		return backendErr
	}

	return nil
}

// ProxyTCPWithTimeout copies data bidirectionally with idle timeout.
// If no data is transferred in either direction for the specified duration,
// the connections are closed.
func ProxyTCPWithTimeout(client, backend net.Conn, idleTimeout time.Duration) error {
	var wg sync.WaitGroup
	wg.Add(2)

	var clientErr, backendErr error

	// Client -> Backend
	go func() {
		defer wg.Done()
		_, clientErr = copyWithIdleTimeout(backend, client, idleTimeout)
		closeWrite(backend)
	}()

	// Backend -> Client
	go func() {
		defer wg.Done()
		_, backendErr = copyWithIdleTimeout(client, backend, idleTimeout)
		closeWrite(client)
	}()

	wg.Wait()

	if clientErr != nil && !isNormalClose(clientErr) {
		return clientErr
	}
	if backendErr != nil && !isNormalClose(backendErr) {
		return backendErr
	}

	return nil
}

// copyWithIdleTimeout copies from src to dst, resetting the deadline on each read.
func copyWithIdleTimeout(dst io.Writer, src net.Conn, idleTimeout time.Duration) (int64, error) {
	buf := make([]byte, 32*1024) // 32KB buffer
	var total int64

	for {
		// Set read deadline for idle timeout
		if err := src.SetReadDeadline(time.Now().Add(idleTimeout)); err != nil {
			return total, err
		}

		n, readErr := src.Read(buf)
		if n > 0 {
			written, writeErr := dst.Write(buf[:n])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
			if written != n {
				return total, io.ErrShortWrite
			}
		}
		if readErr != nil {
			return total, readErr
		}
	}
}

// closeWrite performs a half-close on the connection if it supports it.
// This signals to the peer that no more data will be sent, while still
// allowing data to be received.
func closeWrite(conn net.Conn) {
	// Try TCP half-close
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.CloseWrite()
		return
	}

	// Try to unwrap and find a TCP connection
	if wrapper, ok := conn.(interface{ NetConn() net.Conn }); ok {
		if tcpConn, ok := wrapper.NetConn().(*net.TCPConn); ok {
			_ = tcpConn.CloseWrite()
			return
		}
	}

	// For TLS connections, we can't do half-close, so just let it be
	// The full close will happen when the connection is closed
}

// isNormalClose returns true if the error represents a normal connection close.
func isNormalClose(err error) bool {
	if err == nil || errors.Is(err, io.EOF) {
		return true
	}

	// Check for network closed errors
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		if netErr.Err.Error() == "use of closed network connection" {
			return true
		}
	}

	return false
}

// ProxyResult contains the result of a proxy operation.
type ProxyResult struct {
	ClientToBackend int64 // Bytes copied from client to backend
	BackendToClient int64 // Bytes copied from backend to client
	Error           error // First error encountered, if any
}

// ProxyTCPWithStats copies data bidirectionally and returns statistics.
func ProxyTCPWithStats(client, backend net.Conn) ProxyResult {
	var wg sync.WaitGroup
	wg.Add(2)

	var result ProxyResult
	var clientErr, backendErr error
	var clientToBackend, backendToClient int64

	// Client -> Backend
	go func() {
		defer wg.Done()
		clientToBackend, clientErr = io.Copy(backend, client)
		closeWrite(backend)
	}()

	// Backend -> Client
	go func() {
		defer wg.Done()
		backendToClient, backendErr = io.Copy(client, backend)
		closeWrite(client)
	}()

	wg.Wait()

	result.ClientToBackend = clientToBackend
	result.BackendToClient = backendToClient

	if clientErr != nil && !isNormalClose(clientErr) {
		result.Error = clientErr
	} else if backendErr != nil && !isNormalClose(backendErr) {
		result.Error = backendErr
	}

	return result
}
