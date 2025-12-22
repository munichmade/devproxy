// Package proxy provides HTTP/HTTPS proxy functionality including access logging.
package proxy

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// AccessLogger wraps an http.Handler to log requests.
// Logs at DEBUG level for access logs to avoid noise in normal operation.
type AccessLogger struct {
	handler http.Handler
	logger  *slog.Logger
}

// NewAccessLogger creates a new AccessLogger middleware.
// Access logs are always written at DEBUG level.
func NewAccessLogger(handler http.Handler, logger *slog.Logger) *AccessLogger {
	if logger == nil {
		logger = slog.Default()
	}
	return &AccessLogger{
		handler: handler,
		logger:  logger,
	}
}

// ServeHTTP implements http.Handler.
func (a *AccessLogger) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Wrap the response writer to capture status and size
	wrapped := &responseRecorder{
		ResponseWriter: w,
		statusCode:     http.StatusOK, // Default if WriteHeader not called
		bytesWritten:   0,
	}

	// Call the wrapped handler
	a.handler.ServeHTTP(wrapped, r)

	// Calculate duration
	duration := time.Since(start)

	// Log at DEBUG level
	a.logRequest(r, wrapped.statusCode, wrapped.bytesWritten, duration)
}

// logRequest logs a request at INFO level.
// Format: <method> <path> -> <status> <bytes> <duration>ms (host: <host>)
func (a *AccessLogger) logRequest(r *http.Request, status int, bytes int64, duration time.Duration) {
	host := r.Host
	if host == "" {
		host = "-"
	}

	// Get request path with query string
	path := r.URL.RequestURI()
	if path == "" {
		path = r.URL.Path
		if r.URL.RawQuery != "" {
			path = path + "?" + r.URL.RawQuery
		}
	}

	// Get client IP
	clientIP := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		clientIP = forwarded
	}

	// Log at INFO level with structured fields
	a.logger.Info("access",
		"method", r.Method,
		"host", host,
		"path", path,
		"status", status,
		"bytes", bytes,
		"duration_ms", duration.Milliseconds(),
		"client", clientIP,
	)
}

// responseRecorder wraps http.ResponseWriter to capture status code and bytes written.
type responseRecorder struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
	wroteHeader  bool
}

// WriteHeader captures the status code.
func (r *responseRecorder) WriteHeader(statusCode int) {
	if r.wroteHeader {
		return
	}
	r.wroteHeader = true
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

// Write captures the bytes written and writes to the underlying writer.
func (r *responseRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytesWritten += int64(n)
	return n, err
}

// Unwrap returns the underlying ResponseWriter for middleware compatibility.
func (r *responseRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// Flush implements http.Flusher for streaming responses.
func (r *responseRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Hijack implements http.Hijacker for WebSocket support.
func (r *responseRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := r.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not support hijacking")
}
