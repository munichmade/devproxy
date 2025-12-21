package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPServer_RedirectToHTTPS(t *testing.T) {
	server := NewHTTPServer("127.0.0.1:0", 443)

	tests := []struct {
		name           string
		requestURL     string
		host           string
		expectedURL    string
		expectedStatus int
	}{
		{
			name:           "simple path",
			requestURL:     "/",
			host:           "app.localhost",
			expectedURL:    "https://app.localhost/",
			expectedStatus: http.StatusMovedPermanently,
		},
		{
			name:           "path with segments",
			requestURL:     "/api/users",
			host:           "app.localhost",
			expectedURL:    "https://app.localhost/api/users",
			expectedStatus: http.StatusMovedPermanently,
		},
		{
			name:           "path with query string",
			requestURL:     "/search?q=test&page=1",
			host:           "app.localhost",
			expectedURL:    "https://app.localhost/search?q=test&page=1",
			expectedStatus: http.StatusMovedPermanently,
		},
		{
			name:           "host with port",
			requestURL:     "/",
			host:           "app.localhost:80",
			expectedURL:    "https://app.localhost/",
			expectedStatus: http.StatusMovedPermanently,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.requestURL, nil)
			req.Host = tt.host
			w := httptest.NewRecorder()

			server.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			location := w.Header().Get("Location")
			if location != tt.expectedURL {
				t.Errorf("expected redirect to %q, got %q", tt.expectedURL, location)
			}
		})
	}
}

func TestHTTPServer_RedirectPreservesMethod(t *testing.T) {
	server := NewHTTPServer("127.0.0.1:0", 443)

	tests := []struct {
		method         string
		expectedStatus int
	}{
		{http.MethodGet, http.StatusMovedPermanently},
		{http.MethodHead, http.StatusMovedPermanently},
		{http.MethodPost, http.StatusPermanentRedirect},
		{http.MethodPut, http.StatusPermanentRedirect},
		{http.MethodDelete, http.StatusPermanentRedirect},
		{http.MethodPatch, http.StatusPermanentRedirect},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/api/resource", nil)
			req.Host = "app.localhost"
			w := httptest.NewRecorder()

			server.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d for %s, got %d", tt.expectedStatus, tt.method, w.Code)
			}
		})
	}
}

func TestHTTPServer_NonStandardHTTPSPort(t *testing.T) {
	server := NewHTTPServer("127.0.0.1:0", 8443)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "app.localhost"
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	location := w.Header().Get("Location")
	expected := "https://app.localhost:8443/"
	if location != expected {
		t.Errorf("expected redirect to %q, got %q", expected, location)
	}
}

func TestHTTPServer_StartStop(t *testing.T) {
	server := NewHTTPServer("127.0.0.1:0", 443)

	if err := server.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer server.Stop()

	addr := server.Addr()
	if addr == "" {
		t.Fatal("expected non-empty address after start")
	}

	// Make a request to verify server is running
	// Use a client with no redirect following to check the redirect response
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusMovedPermanently {
		t.Errorf("expected redirect status %d, got %d", http.StatusMovedPermanently, resp.StatusCode)
	}

	if err := server.Stop(); err != nil {
		t.Errorf("failed to stop server: %v", err)
	}
}
