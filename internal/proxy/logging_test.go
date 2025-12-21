package proxy

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAccessLogger(t *testing.T) {
	t.Run("logs request details when enabled", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, nil))

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Hello, World!"))
		})

		middleware := NewAccessLogger(handler, true, logger)

		req := httptest.NewRequest(http.MethodGet, "http://example.com/path", nil)
		req.Host = "example.com"
		req.RemoteAddr = "192.168.1.100:12345"
		w := httptest.NewRecorder()

		middleware.ServeHTTP(w, req)

		logOutput := buf.String()

		// Check method is logged
		if !strings.Contains(logOutput, "GET") {
			t.Errorf("expected log to contain method 'GET', got: %s", logOutput)
		}

		// Check path is logged
		if !strings.Contains(logOutput, "/path") {
			t.Errorf("expected log to contain path '/path', got: %s", logOutput)
		}

		// Check host is logged
		if !strings.Contains(logOutput, "example.com") {
			t.Errorf("expected log to contain host 'example.com', got: %s", logOutput)
		}

		// Check status is logged
		if !strings.Contains(logOutput, "200") {
			t.Errorf("expected log to contain status '200', got: %s", logOutput)
		}
	})

	t.Run("does not log when disabled", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, nil))

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		middleware := NewAccessLogger(handler, false, logger)

		req := httptest.NewRequest(http.MethodGet, "http://example.com/path", nil)
		w := httptest.NewRecorder()

		middleware.ServeHTTP(w, req)

		logOutput := buf.String()

		if logOutput != "" {
			t.Errorf("expected no log output when disabled, got: %s", logOutput)
		}
	})

	t.Run("logs response size", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, nil))

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("12345")) // 5 bytes
		})

		middleware := NewAccessLogger(handler, true, logger)

		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		w := httptest.NewRecorder()

		middleware.ServeHTTP(w, req)

		logOutput := buf.String()

		// Check that 5 bytes are logged (the response body size)
		if !strings.Contains(logOutput, " 5 ") {
			t.Errorf("expected log to contain response size '5', got: %s", logOutput)
		}
	})

	t.Run("logs duration in milliseconds", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, nil))

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		middleware := NewAccessLogger(handler, true, logger)

		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		w := httptest.NewRecorder()

		middleware.ServeHTTP(w, req)

		logOutput := buf.String()

		// Check duration is logged (ends with "ms")
		if !strings.Contains(logOutput, "ms") {
			t.Errorf("expected log to contain duration in 'ms', got: %s", logOutput)
		}
	})

	t.Run("logs error status codes", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, nil))

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})

		middleware := NewAccessLogger(handler, true, logger)

		req := httptest.NewRequest(http.MethodPost, "http://example.com/api", nil)
		w := httptest.NewRecorder()

		middleware.ServeHTTP(w, req)

		logOutput := buf.String()

		// Check status 500 is logged
		if !strings.Contains(logOutput, "500") {
			t.Errorf("expected log to contain status '500', got: %s", logOutput)
		}

		// Check method POST is logged
		if !strings.Contains(logOutput, "POST") {
			t.Errorf("expected log to contain method 'POST', got: %s", logOutput)
		}
	})

	t.Run("uses default logger when nil", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		middleware := NewAccessLogger(handler, true, nil)

		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		w := httptest.NewRecorder()

		// Should not panic
		middleware.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}
	})

	t.Run("logs user agent and referer", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, nil))

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		middleware := NewAccessLogger(handler, true, logger)

		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		req.Header.Set("User-Agent", "TestAgent/1.0")
		req.Header.Set("Referer", "http://referrer.com/page")
		w := httptest.NewRecorder()

		middleware.ServeHTTP(w, req)

		logOutput := buf.String()

		if !strings.Contains(logOutput, "TestAgent/1.0") {
			t.Errorf("expected log to contain user agent 'TestAgent/1.0', got: %s", logOutput)
		}

		if !strings.Contains(logOutput, "http://referrer.com/page") {
			t.Errorf("expected log to contain referer 'http://referrer.com/page', got: %s", logOutput)
		}
	})
}

func TestResponseRecorder(t *testing.T) {
	t.Run("captures status code", func(t *testing.T) {
		w := httptest.NewRecorder()
		rr := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}

		rr.WriteHeader(http.StatusNotFound)

		if rr.statusCode != http.StatusNotFound {
			t.Errorf("expected status %d, got %d", http.StatusNotFound, rr.statusCode)
		}
	})

	t.Run("captures bytes written", func(t *testing.T) {
		w := httptest.NewRecorder()
		rr := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}

		n, err := rr.Write([]byte("hello"))

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if n != 5 {
			t.Errorf("expected 5 bytes written, got %d", n)
		}
		if rr.bytesWritten != 5 {
			t.Errorf("expected bytesWritten=5, got %d", rr.bytesWritten)
		}
	})

	t.Run("accumulates bytes across multiple writes", func(t *testing.T) {
		w := httptest.NewRecorder()
		rr := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}

		rr.Write([]byte("hello"))
		rr.Write([]byte(" world"))

		if rr.bytesWritten != 11 {
			t.Errorf("expected bytesWritten=11, got %d", rr.bytesWritten)
		}
	})

	t.Run("implements Flush", func(t *testing.T) {
		w := httptest.NewRecorder()
		rr := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}

		// Should not panic
		rr.Flush()
	})

	t.Run("only writes header once", func(t *testing.T) {
		w := httptest.NewRecorder()
		rr := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}

		rr.WriteHeader(http.StatusNotFound)
		rr.WriteHeader(http.StatusInternalServerError) // Should be ignored

		if rr.statusCode != http.StatusNotFound {
			t.Errorf("expected status %d (first call), got %d", http.StatusNotFound, rr.statusCode)
		}
	})

	t.Run("Write triggers implicit WriteHeader", func(t *testing.T) {
		w := httptest.NewRecorder()
		rr := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}

		rr.Write([]byte("hello"))

		if !rr.wroteHeader {
			t.Error("expected wroteHeader to be true after Write")
		}
		if rr.statusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.statusCode)
		}
	})

	t.Run("Unwrap returns underlying ResponseWriter", func(t *testing.T) {
		w := httptest.NewRecorder()
		rr := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}

		unwrapped := rr.Unwrap()

		if unwrapped != w {
			t.Error("expected Unwrap to return underlying ResponseWriter")
		}
	})
}
