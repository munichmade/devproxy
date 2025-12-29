// Package proxy provides HTTP/HTTPS proxy servers for devproxy.
package proxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

// ReverseProxy routes incoming requests to backend services based on Host header.
type ReverseProxy struct {
	registry *Registry
}

// NewReverseProxy creates a new reverse proxy with the given route registry.
func NewReverseProxy(registry *Registry) *ReverseProxy {
	return &ReverseProxy{
		registry: registry,
	}
}

// ServeHTTP implements http.Handler for the reverse proxy.
func (rp *ReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract host without port
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	// Look up route
	route := rp.registry.Lookup(host)
	if route == nil {
		http.Error(w, fmt.Sprintf("no route configured for host: %s", host), http.StatusNotFound)
		return
	}

	// Only handle HTTP protocol routes
	if route.Protocol != ProtocolHTTP {
		http.Error(w, fmt.Sprintf("route for %s is not HTTP protocol", host), http.StatusBadRequest)
		return
	}

	// Parse backend URL
	backendURL, err := url.Parse("http://" + route.Backend)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid backend URL: %v", err), http.StatusInternalServerError)
		return
	}

	// Create reverse proxy for this request
	proxy := rp.createProxy(backendURL, r)
	proxy.ServeHTTP(w, r)
}

// createProxy creates an httputil.ReverseProxy configured for the given backend.
func (rp *ReverseProxy) createProxy(target *url.URL, originalReq *http.Request) *httputil.ReverseProxy {
	director := func(req *http.Request) {
		// Set target URL
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host

		// Preserve original path if target has a path
		if target.Path != "" && target.Path != "/" {
			req.URL.Path = singleJoiningSlash(target.Path, req.URL.Path)
		}

		// Preserve original Host header for the backend
		// Most applications expect the original Host header for virtual hosting,
		// URL generation, and multi-tenant routing
		req.Host = originalReq.Host

		// Set proxy headers
		clientIP := getClientIP(originalReq)

		// X-Forwarded-For: append client IP
		if prior, ok := req.Header["X-Forwarded-For"]; ok {
			clientIP = strings.Join(prior, ", ") + ", " + clientIP
		}
		req.Header.Set("X-Forwarded-For", clientIP)

		// X-Forwarded-Proto: original scheme
		if originalReq.TLS != nil {
			req.Header.Set("X-Forwarded-Proto", "https")
		} else {
			req.Header.Set("X-Forwarded-Proto", "http")
		}

		// X-Forwarded-Host: original host
		req.Header.Set("X-Forwarded-Host", originalReq.Host)

		// X-Real-IP: client IP
		req.Header.Set("X-Real-IP", getClientIP(originalReq))
	}

	proxy := &httputil.ReverseProxy{
		Director: director,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, fmt.Sprintf("proxy error: %v", err), http.StatusBadGateway)
		},
		// FlushInterval for streaming responses (including WebSocket)
		FlushInterval: -1,
	}

	return proxy
}

// Handler returns an http.Handler that can be used with HTTPSServer.
func (rp *ReverseProxy) Handler() http.Handler {
	return rp
}

// singleJoiningSlash joins two URL path segments with a single slash.
func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}

// getClientIP extracts the client IP from a request.
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For first (in case of upstream proxy)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// Check X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// ProxyHandler wraps the reverse proxy to add context-aware features.
type ProxyHandler struct {
	proxy *ReverseProxy
}

// NewProxyHandler creates a new proxy handler.
func NewProxyHandler(registry *Registry) *ProxyHandler {
	return &ProxyHandler{
		proxy: NewReverseProxy(registry),
	}
}

// ServeHTTP implements http.Handler with additional context handling.
func (ph *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Add timeout context for non-WebSocket requests
	if !isWebSocketRequest(r) {
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
	}

	ph.proxy.ServeHTTP(w, r)
}

// isWebSocketRequest checks if the request is a WebSocket upgrade.
func isWebSocketRequest(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}
