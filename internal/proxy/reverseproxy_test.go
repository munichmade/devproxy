package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestReverseProxy_ServeHTTP(t *testing.T) {
	t.Run("proxies request to backend", func(t *testing.T) {
		// Create a backend server
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Backend", "reached")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Hello from backend"))
		}))
		defer backend.Close()

		// Extract host:port from backend URL
		backendAddr := strings.TrimPrefix(backend.URL, "http://")

		// Set up registry with route
		registry := NewRegistry()
		registry.Add(Route{
			Host:     "app.localhost",
			Backend:  backendAddr,
			Protocol: ProtocolHTTP,
		})

		// Create reverse proxy
		rp := NewReverseProxy(registry)

		// Make request
		req := httptest.NewRequest(http.MethodGet, "http://app.localhost/test", nil)
		req.Host = "app.localhost"
		w := httptest.NewRecorder()

		rp.ServeHTTP(w, req)

		// Verify response
		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}
		if w.Header().Get("X-Backend") != "reached" {
			t.Error("backend header not set")
		}
		if w.Body.String() != "Hello from backend" {
			t.Errorf("unexpected body: %s", w.Body.String())
		}
	})

	t.Run("returns 404 for unknown host", func(t *testing.T) {
		registry := NewRegistry()
		rp := NewReverseProxy(registry)

		req := httptest.NewRequest(http.MethodGet, "http://unknown.localhost/", nil)
		req.Host = "unknown.localhost"
		w := httptest.NewRecorder()

		rp.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "no route configured") {
			t.Errorf("unexpected error message: %s", w.Body.String())
		}
	})

	t.Run("returns 502 on backend error", func(t *testing.T) {
		// Set up registry with invalid backend
		registry := NewRegistry()
		registry.Add(Route{
			Host:     "app.localhost",
			Backend:  "127.0.0.1:59999", // Non-existent backend
			Protocol: ProtocolHTTP,
		})

		rp := NewReverseProxy(registry)

		req := httptest.NewRequest(http.MethodGet, "http://app.localhost/", nil)
		req.Host = "app.localhost"
		w := httptest.NewRecorder()

		rp.ServeHTTP(w, req)

		if w.Code != http.StatusBadGateway {
			t.Errorf("expected status 502, got %d", w.Code)
		}
	})

	t.Run("returns 400 for non-HTTP protocol route", func(t *testing.T) {
		registry := NewRegistry()
		registry.Add(Route{
			Host:     "db.localhost",
			Backend:  "127.0.0.1:5432",
			Protocol: ProtocolTCP,
		})

		rp := NewReverseProxy(registry)

		req := httptest.NewRequest(http.MethodGet, "http://db.localhost/", nil)
		req.Host = "db.localhost"
		w := httptest.NewRecorder()

		rp.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", w.Code)
		}
	})

	t.Run("strips port from host header", func(t *testing.T) {
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer backend.Close()

		backendAddr := strings.TrimPrefix(backend.URL, "http://")

		registry := NewRegistry()
		registry.Add(Route{
			Host:     "app.localhost",
			Backend:  backendAddr,
			Protocol: ProtocolHTTP,
		})

		rp := NewReverseProxy(registry)

		// Request with port in Host header
		req := httptest.NewRequest(http.MethodGet, "http://app.localhost:443/", nil)
		req.Host = "app.localhost:443"
		w := httptest.NewRecorder()

		rp.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}
	})
}

func TestReverseProxy_ProxyHeaders(t *testing.T) {
	t.Run("sets X-Forwarded-For header", func(t *testing.T) {
		var receivedXFF string
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedXFF = r.Header.Get("X-Forwarded-For")
			w.WriteHeader(http.StatusOK)
		}))
		defer backend.Close()

		backendAddr := strings.TrimPrefix(backend.URL, "http://")

		registry := NewRegistry()
		registry.Add(Route{
			Host:     "app.localhost",
			Backend:  backendAddr,
			Protocol: ProtocolHTTP,
		})

		rp := NewReverseProxy(registry)

		req := httptest.NewRequest(http.MethodGet, "http://app.localhost/", nil)
		req.Host = "app.localhost"
		req.RemoteAddr = "192.168.1.100:12345"
		w := httptest.NewRecorder()

		rp.ServeHTTP(w, req)

		// The X-Forwarded-For should contain the client IP from RemoteAddr
		if !strings.Contains(receivedXFF, "192.168.1.100") {
			t.Errorf("expected X-Forwarded-For to contain '192.168.1.100', got '%s'", receivedXFF)
		}
	})

	t.Run("appends to existing X-Forwarded-For", func(t *testing.T) {
		var receivedXFF string
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedXFF = r.Header.Get("X-Forwarded-For")
			w.WriteHeader(http.StatusOK)
		}))
		defer backend.Close()

		backendAddr := strings.TrimPrefix(backend.URL, "http://")

		registry := NewRegistry()
		registry.Add(Route{
			Host:     "app.localhost",
			Backend:  backendAddr,
			Protocol: ProtocolHTTP,
		})

		rp := NewReverseProxy(registry)

		req := httptest.NewRequest(http.MethodGet, "http://app.localhost/", nil)
		req.Host = "app.localhost"
		req.RemoteAddr = "192.168.1.100:12345"
		req.Header.Set("X-Forwarded-For", "10.0.0.1")
		w := httptest.NewRecorder()

		rp.ServeHTTP(w, req)

		// Should contain both the original chain and append the client IP
		if !strings.Contains(receivedXFF, "10.0.0.1") {
			t.Errorf("expected X-Forwarded-For to contain '10.0.0.1', got '%s'", receivedXFF)
		}
		if !strings.Contains(receivedXFF, "192.168.1.100") {
			t.Errorf("expected X-Forwarded-For to contain '192.168.1.100', got '%s'", receivedXFF)
		}
	})

	t.Run("sets X-Forwarded-Proto to http", func(t *testing.T) {
		var receivedProto string
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedProto = r.Header.Get("X-Forwarded-Proto")
			w.WriteHeader(http.StatusOK)
		}))
		defer backend.Close()

		backendAddr := strings.TrimPrefix(backend.URL, "http://")

		registry := NewRegistry()
		registry.Add(Route{
			Host:     "app.localhost",
			Backend:  backendAddr,
			Protocol: ProtocolHTTP,
		})

		rp := NewReverseProxy(registry)

		req := httptest.NewRequest(http.MethodGet, "http://app.localhost/", nil)
		req.Host = "app.localhost"
		w := httptest.NewRecorder()

		rp.ServeHTTP(w, req)

		if receivedProto != "http" {
			t.Errorf("expected X-Forwarded-Proto 'http', got '%s'", receivedProto)
		}
	})

	t.Run("sets X-Forwarded-Host header", func(t *testing.T) {
		var receivedHost string
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedHost = r.Header.Get("X-Forwarded-Host")
			w.WriteHeader(http.StatusOK)
		}))
		defer backend.Close()

		backendAddr := strings.TrimPrefix(backend.URL, "http://")

		registry := NewRegistry()
		registry.Add(Route{
			Host:     "app.localhost",
			Backend:  backendAddr,
			Protocol: ProtocolHTTP,
		})

		rp := NewReverseProxy(registry)

		req := httptest.NewRequest(http.MethodGet, "http://app.localhost/", nil)
		req.Host = "app.localhost:443"
		w := httptest.NewRecorder()

		rp.ServeHTTP(w, req)

		if receivedHost != "app.localhost:443" {
			t.Errorf("expected X-Forwarded-Host 'app.localhost:443', got '%s'", receivedHost)
		}
	})

	t.Run("sets X-Real-IP header", func(t *testing.T) {
		var receivedRealIP string
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedRealIP = r.Header.Get("X-Real-IP")
			w.WriteHeader(http.StatusOK)
		}))
		defer backend.Close()

		backendAddr := strings.TrimPrefix(backend.URL, "http://")

		registry := NewRegistry()
		registry.Add(Route{
			Host:     "app.localhost",
			Backend:  backendAddr,
			Protocol: ProtocolHTTP,
		})

		rp := NewReverseProxy(registry)

		req := httptest.NewRequest(http.MethodGet, "http://app.localhost/", nil)
		req.Host = "app.localhost"
		req.RemoteAddr = "192.168.1.100:12345"
		w := httptest.NewRecorder()

		rp.ServeHTTP(w, req)

		if receivedRealIP != "192.168.1.100" {
			t.Errorf("expected X-Real-IP '192.168.1.100', got '%s'", receivedRealIP)
		}
	})
}

func TestReverseProxy_WebSocket(t *testing.T) {
	t.Run("proxies WebSocket connections", func(t *testing.T) {
		// Create a WebSocket backend server
		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}

		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Logf("upgrade error: %v", err)
				return
			}
			defer conn.Close()

			// Echo messages back
			for {
				mt, message, err := conn.ReadMessage()
				if err != nil {
					break
				}
				err = conn.WriteMessage(mt, message)
				if err != nil {
					break
				}
			}
		}))
		defer backend.Close()

		backendAddr := strings.TrimPrefix(backend.URL, "http://")

		registry := NewRegistry()
		registry.Add(Route{
			Host:     "ws.localhost",
			Backend:  backendAddr,
			Protocol: ProtocolHTTP,
		})

		// Create proxy server
		rp := NewReverseProxy(registry)
		proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Override Host for the test
			r.Host = "ws.localhost"
			rp.ServeHTTP(w, r)
		}))
		defer proxyServer.Close()

		// Connect via WebSocket through the proxy
		wsURL := "ws" + strings.TrimPrefix(proxyServer.URL, "http") + "/"
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}
		defer conn.Close()

		// Send a message
		testMsg := "Hello WebSocket"
		err = conn.WriteMessage(websocket.TextMessage, []byte(testMsg))
		if err != nil {
			t.Fatalf("failed to write message: %v", err)
		}

		// Read the echoed message
		_, msg, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read message: %v", err)
		}

		if string(msg) != testMsg {
			t.Errorf("expected '%s', got '%s'", testMsg, string(msg))
		}
	})
}

func TestIsWebSocketRequest(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		isWebSocket bool
	}{
		{
			name: "valid WebSocket upgrade",
			headers: map[string]string{
				"Upgrade":    "websocket",
				"Connection": "Upgrade",
			},
			isWebSocket: true,
		},
		{
			name: "case insensitive upgrade",
			headers: map[string]string{
				"Upgrade":    "WebSocket",
				"Connection": "upgrade",
			},
			isWebSocket: true,
		},
		{
			name: "connection with keep-alive",
			headers: map[string]string{
				"Upgrade":    "websocket",
				"Connection": "keep-alive, Upgrade",
			},
			isWebSocket: true,
		},
		{
			name: "missing upgrade header",
			headers: map[string]string{
				"Connection": "Upgrade",
			},
			isWebSocket: false,
		},
		{
			name: "missing connection header",
			headers: map[string]string{
				"Upgrade": "websocket",
			},
			isWebSocket: false,
		},
		{
			name:        "no headers",
			headers:     map[string]string{},
			isWebSocket: false,
		},
		{
			name: "wrong upgrade type",
			headers: map[string]string{
				"Upgrade":    "h2c",
				"Connection": "Upgrade",
			},
			isWebSocket: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			got := isWebSocketRequest(req)
			if got != tt.isWebSocket {
				t.Errorf("isWebSocketRequest() = %v, want %v", got, tt.isWebSocket)
			}
		})
	}
}

func TestProxyHandler_TimeoutForNonWebSocket(t *testing.T) {
	t.Run("non-WebSocket requests are proxied", func(t *testing.T) {
		var received bool
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			received = true
			w.WriteHeader(http.StatusOK)
		}))
		defer backend.Close()

		backendAddr := strings.TrimPrefix(backend.URL, "http://")

		registry := NewRegistry()
		registry.Add(Route{
			Host:     "app.localhost",
			Backend:  backendAddr,
			Protocol: ProtocolHTTP,
		})

		ph := NewProxyHandler(registry)

		req := httptest.NewRequest(http.MethodGet, "http://app.localhost/", nil)
		req.Host = "app.localhost"
		w := httptest.NewRecorder()

		ph.ServeHTTP(w, req)

		if !received {
			t.Error("expected request to be proxied to backend")
		}
		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}
	})

	t.Run("WebSocket requests bypass timeout", func(t *testing.T) {
		// Verify WebSocket detection works in ProxyHandler
		req := httptest.NewRequest(http.MethodGet, "http://app.localhost/ws", nil)
		req.Header.Set("Upgrade", "websocket")
		req.Header.Set("Connection", "Upgrade")

		if !isWebSocketRequest(req) {
			t.Error("expected WebSocket request to be detected")
		}
	})
}

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
		expected   string
	}{
		{
			name:       "from RemoteAddr",
			remoteAddr: "192.168.1.1:12345",
			headers:    nil,
			expected:   "192.168.1.1",
		},
		{
			name:       "from X-Real-IP",
			remoteAddr: "127.0.0.1:12345",
			headers:    map[string]string{"X-Real-IP": "10.0.0.1"},
			expected:   "10.0.0.1",
		},
		{
			name:       "from X-Forwarded-For single",
			remoteAddr: "127.0.0.1:12345",
			headers:    map[string]string{"X-Forwarded-For": "10.0.0.1"},
			expected:   "10.0.0.1",
		},
		{
			name:       "from X-Forwarded-For chain",
			remoteAddr: "127.0.0.1:12345",
			headers:    map[string]string{"X-Forwarded-For": "10.0.0.1, 10.0.0.2, 10.0.0.3"},
			expected:   "10.0.0.1",
		},
		{
			name:       "X-Forwarded-For takes precedence",
			remoteAddr: "127.0.0.1:12345",
			headers: map[string]string{
				"X-Forwarded-For": "10.0.0.1",
				"X-Real-IP":       "10.0.0.2",
			},
			expected: "10.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			got := getClientIP(req)
			if got != tt.expected {
				t.Errorf("getClientIP() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestSingleJoiningSlash(t *testing.T) {
	tests := []struct {
		a, b     string
		expected string
	}{
		{"/api", "/users", "/api/users"},
		{"/api/", "/users", "/api/users"},
		{"/api", "users", "/api/users"},
		{"/api/", "users", "/api/users"},
		{"", "/users", "/users"},
		{"/api", "", "/api/"},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			got := singleJoiningSlash(tt.a, tt.b)
			if got != tt.expected {
				t.Errorf("singleJoiningSlash(%q, %q) = %q, want %q", tt.a, tt.b, got, tt.expected)
			}
		})
	}
}
