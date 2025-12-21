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

// AccessLogger wraps an http.Handler to log requests in Apache Combined Log Format.
type AccessLogger struct {
	handler http.Handler
	enabled bool
	logger  *slog.Logger
}

// NewAccessLogger creates a new AccessLogger middleware.
// If enabled is false, requests pass through without logging.
func NewAccessLogger(handler http.Handler, enabled bool, logger *slog.Logger) *AccessLogger {
	if logger == nil {
		logger = slog.Default()
	}
	return &AccessLogger{
		handler: handler,
		enabled: enabled,
		logger:  logger,
	}
}

// ServeHTTP implements http.Handler.
func (a *AccessLogger) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !a.enabled {
		a.handler.ServeHTTP(w, r)
		return
	}

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

	// Log in Apache Combined Log Format with duration
	a.logRequest(r, wrapped.statusCode, wrapped.bytesWritten, duration)
}

// logRequest logs a request in Apache Combined Log Format.
// Format: <host> - - [<timestamp>] "<method> <path> <proto>" <status> <bytes> "<referer>" "<user-agent>" <duration>ms
func (a *AccessLogger) logRequest(r *http.Request, status int, bytes int64, duration time.Duration) {
	host := r.Host
	if host == "" {
		host = "-"
	}

	referer := r.Header.Get("Referer")
	if referer == "" {
		referer = "-"
	}

	userAgent := r.Header.Get("User-Agent")
	if userAgent == "" {
		userAgent = "-"
	}

	// Get request path with query string
	path := r.URL.RequestURI()
	if path == "" {
		path = r.URL.Path
		if r.URL.RawQuery != "" {
			path = path + "?" + r.URL.RawQuery
		}
	}

	// Format timestamp in Apache common log format
	timestamp := time.Now().Format("02/Jan/2006:15:04:05 -0700")

	// Log message in Combined Log Format with duration
	logLine := fmt.Sprintf("%s - - [%s] \"%s %s %s\" %d %d \"%s\" \"%s\" %dms",
		host,
		timestamp,
		r.Method,
		path,
		r.Proto,
		status,
		bytes,
		referer,
		userAgent,
		duration.Milliseconds(),
	)

	a.logger.Info(logLine)
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
