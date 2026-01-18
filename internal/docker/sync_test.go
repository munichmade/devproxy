package docker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"

	"github.com/munichmade/devproxy/internal/proxy"
)

func TestNewRouteSync(t *testing.T) {
	t.Run("creates sync with all fields", func(t *testing.T) {
		registry := proxy.NewRegistry()
		client := &Client{}
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		sync := NewRouteSync(registry, client, "bridge", logger)

		if sync.registry != registry {
			t.Error("expected registry to be set")
		}
		if sync.client != client {
			t.Error("expected client to be set")
		}
		if sync.network != "bridge" {
			t.Errorf("expected network 'bridge', got '%s'", sync.network)
		}
		if sync.containers == nil {
			t.Error("expected containers map to be initialized")
		}
	})
}

func TestRouteSync_HandleEvent(t *testing.T) {
	t.Run("ignores containers without devproxy labels", func(t *testing.T) {
		registry := proxy.NewRegistry()
		client := &Client{}
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		sync := NewRouteSync(registry, client, "bridge", logger)

		event := ContainerEvent{
			ContainerID: "abc123",
			Labels: map[string]string{
				"some.other.label": "value",
			},
			Type: "start",
		}

		sync.HandleEvent(event)

		// No routes should be added
		routes := registry.List()
		if len(routes) != 0 {
			t.Errorf("expected 0 routes, got %d", len(routes))
		}
	})

	t.Run("handles stop event for untracked container", func(t *testing.T) {
		registry := proxy.NewRegistry()
		client := &Client{}
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		sync := NewRouteSync(registry, client, "bridge", logger)

		event := ContainerEvent{
			ContainerID: "abc123def456",
			Labels:      map[string]string{},
			Type:        "stop",
		}

		// Should not panic
		sync.HandleEvent(event)
	})

	t.Run("tracks container hosts on start", func(t *testing.T) {
		registry := proxy.NewRegistry()
		client := &Client{}
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		sync := NewRouteSync(registry, client, "bridge", logger)

		// Simulate a start event with labels but no docker client
		// The IP resolution will fail, so no routes will be added
		// This just tests the tracking logic doesn't panic
		event := ContainerEvent{
			ContainerID: "abc123def456",
			Labels: map[string]string{
				"devproxy.enable": "true",
				"devproxy.host":   "test.localhost",
				"devproxy.port":   "8080",
			},
			Type: "start",
		}

		sync.HandleEvent(event)

		// Route won't be added because IP resolution fails without docker
		// But the code shouldn't panic
	})
}

func TestRouteSync_GetProtocol(t *testing.T) {
	registry := proxy.NewRegistry()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sync := NewRouteSync(registry, nil, "bridge", logger)

	t.Run("returns HTTP by default", func(t *testing.T) {
		config := ServiceConfig{
			Host: "test.localhost",
			Port: 8080,
		}

		protocol := sync.getProtocol(config)
		if protocol != proxy.ProtocolHTTP {
			t.Errorf("expected HTTP, got %s", protocol)
		}
	})

	t.Run("returns TCP for entrypoint config", func(t *testing.T) {
		config := ServiceConfig{
			Host:       "db.localhost",
			Port:       5432,
			Entrypoint: "postgres",
		}

		protocol := sync.getProtocol(config)
		if protocol != proxy.ProtocolTCP {
			t.Errorf("expected TCP, got %s", protocol)
		}
	})
}

func TestRouteSync_CommaSeparatedHosts(t *testing.T) {
	t.Run("splits comma-separated hosts into separate routes", func(t *testing.T) {
		registry := proxy.NewRegistry()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		sync := NewRouteSync(registry, nil, "bridge", logger)

		// Manually simulate what handleStart does after IP resolution
		// by adding routes for each host in a comma-separated list
		hosts := []string{"app.localhost", "*.app.localhost"}
		containerID := "container789"

		for _, host := range hosts {
			registry.Add(proxy.Route{
				Host:        host,
				Backend:     "172.17.0.5:4000",
				ContainerID: containerID,
			})
		}

		sync.mu.Lock()
		sync.containers[containerID] = hosts
		sync.mu.Unlock()

		// Verify both routes exist
		exactRoute := registry.Lookup("app.localhost")
		if exactRoute == nil {
			t.Error("expected exact route to exist")
		}

		wildcardRoute := registry.Lookup("team-a.app.localhost")
		if wildcardRoute == nil {
			t.Error("expected wildcard route to match subdomain")
		}

		// Verify count
		if registry.Count() != 2 {
			t.Errorf("expected 2 routes, got %d", registry.Count())
		}

		// Stop should remove both
		event := ContainerEvent{
			ContainerID: containerID,
			Type:        "stop",
		}
		sync.HandleEvent(event)

		if registry.Count() != 0 {
			t.Errorf("expected 0 routes after stop, got %d", registry.Count())
		}
	})
}

func TestRouteSync_ContainerTracking(t *testing.T) {
	t.Run("removes tracked hosts on stop", func(t *testing.T) {
		registry := proxy.NewRegistry()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		sync := NewRouteSync(registry, nil, "bridge", logger)

		// Manually add a route and track it
		registry.Add(proxy.Route{
			Host:        "test.localhost",
			Backend:     "172.17.0.2:8080",
			ContainerID: "container123",
		})

		sync.mu.Lock()
		sync.containers["container123"] = []string{"test.localhost"}
		sync.mu.Unlock()

		// Stop event should remove the route
		event := ContainerEvent{
			ContainerID: "container123",
			Type:        "stop",
		}

		sync.HandleEvent(event)

		// Route should be removed
		route := registry.Lookup("test.localhost")
		if route != nil {
			t.Error("expected route to be removed")
		}

		// Container should no longer be tracked
		sync.mu.RLock()
		_, exists := sync.containers["container123"]
		sync.mu.RUnlock()
		if exists {
			t.Error("expected container to be untracked")
		}
	})

	t.Run("handles die event same as stop", func(t *testing.T) {
		registry := proxy.NewRegistry()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		sync := NewRouteSync(registry, nil, "bridge", logger)

		// Manually add a route and track it
		registry.Add(proxy.Route{
			Host:        "app.localhost",
			Backend:     "172.17.0.3:3000",
			ContainerID: "container456",
		})

		sync.mu.Lock()
		sync.containers["container456"] = []string{"app.localhost"}
		sync.mu.Unlock()

		// Die event should also remove the route
		event := ContainerEvent{
			ContainerID: "container456",
			Type:        "die",
		}

		sync.HandleEvent(event)

		route := registry.Lookup("app.localhost")
		if route != nil {
			t.Error("expected route to be removed on die event")
		}
	})
}

func TestRouteSync_handleStart_FullFlow(t *testing.T) {
	t.Run("creates route with resolved IP", func(t *testing.T) {
		registry := proxy.NewRegistry()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		mockAPI := newMockBuilder().
			withContainerInspect(func(ctx context.Context, containerID string) (container.InspectResponse, error) {
				return makeContainerInspectResponse(containerID, "web-app", "172.17.0.5", "bridge"), nil
			}).
			build()

		client := NewClientWithAPI(mockAPI, logger)
		sync := NewRouteSync(registry, client, "bridge", logger)

		event := ContainerEvent{
			ContainerID:   "container123abc",
			ContainerName: "web-app",
			Labels: map[string]string{
				"devproxy.enable": "true",
				"devproxy.host":   "app.localhost",
				"devproxy.port":   "8080",
			},
			Type: "start",
		}

		sync.HandleEvent(event)

		// Verify route was added
		route := registry.Lookup("app.localhost")
		if route == nil {
			t.Fatal("expected route to be added")
		}

		if route.Backend != "172.17.0.5:8080" {
			t.Errorf("expected backend '172.17.0.5:8080', got '%s'", route.Backend)
		}

		if route.ContainerID != "container123abc" {
			t.Errorf("expected container ID 'container123abc', got '%s'", route.ContainerID)
		}

		if route.ContainerName != "web-app" {
			t.Errorf("expected container name 'web-app', got '%s'", route.ContainerName)
		}
	})

	t.Run("handles multiple service configs", func(t *testing.T) {
		registry := proxy.NewRegistry()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		mockAPI := newMockBuilder().
			withContainerInspect(func(ctx context.Context, containerID string) (container.InspectResponse, error) {
				return makeContainerInspectResponse(containerID, "multi-service", "172.17.0.10", "bridge"), nil
			}).
			build()

		client := NewClientWithAPI(mockAPI, logger)
		sync := NewRouteSync(registry, client, "bridge", logger)

		event := ContainerEvent{
			ContainerID:   "multicontainer123",
			ContainerName: "multi-service",
			Labels: map[string]string{
				"devproxy.enable":            "true",
				"devproxy.services.web.host": "web.localhost",
				"devproxy.services.web.port": "8080",
				"devproxy.services.api.host": "api.localhost",
				"devproxy.services.api.port": "3000",
			},
			Type: "start",
		}

		sync.HandleEvent(event)

		// Verify both routes were added
		webRoute := registry.Lookup("web.localhost")
		if webRoute == nil {
			t.Error("expected web route to be added")
		}

		apiRoute := registry.Lookup("api.localhost")
		if apiRoute == nil {
			t.Error("expected api route to be added")
		}

		if registry.Count() != 2 {
			t.Errorf("expected 2 routes, got %d", registry.Count())
		}
	})

	t.Run("handles comma-separated hosts", func(t *testing.T) {
		registry := proxy.NewRegistry()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		mockAPI := newMockBuilder().
			withContainerInspect(func(ctx context.Context, containerID string) (container.InspectResponse, error) {
				return makeContainerInspectResponse(containerID, "multi-host", "172.17.0.15", "bridge"), nil
			}).
			build()

		client := NewClientWithAPI(mockAPI, logger)
		sync := NewRouteSync(registry, client, "bridge", logger)

		event := ContainerEvent{
			ContainerID:   "multihostcontainer",
			ContainerName: "multi-host",
			Labels: map[string]string{
				"devproxy.enable": "true",
				"devproxy.host":   "app.localhost, *.app.localhost",
				"devproxy.port":   "8080",
			},
			Type: "start",
		}

		sync.HandleEvent(event)

		// Verify both hosts were added
		exactRoute := registry.Lookup("app.localhost")
		if exactRoute == nil {
			t.Error("expected exact route to be added")
		}

		// Wildcard should match subdomains
		wildcardRoute := registry.Lookup("sub.app.localhost")
		if wildcardRoute == nil {
			t.Error("expected wildcard route to match subdomain")
		}

		if registry.Count() != 2 {
			t.Errorf("expected 2 routes, got %d", registry.Count())
		}
	})

	t.Run("logs error when IP resolution fails", func(t *testing.T) {
		registry := proxy.NewRegistry()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		mockAPI := newMockBuilder().
			withContainerInspectError(errMockNotFound).
			build()

		client := NewClientWithAPI(mockAPI, logger)
		sync := NewRouteSync(registry, client, "bridge", logger)

		event := ContainerEvent{
			ContainerID:   "failingcontainer",
			ContainerName: "failing",
			Labels: map[string]string{
				"devproxy.enable": "true",
				"devproxy.host":   "failing.localhost",
				"devproxy.port":   "8080",
			},
			Type: "start",
		}

		// Should not panic
		sync.HandleEvent(event)

		// No route should be added
		if registry.Count() != 0 {
			t.Errorf("expected 0 routes when IP resolution fails, got %d", registry.Count())
		}
	})
}

func TestRouteSync_handleStart_CertGeneration(t *testing.T) {
	t.Run("pre-generates certificate when cert manager is set", func(t *testing.T) {
		registry := proxy.NewRegistry()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		mockAPI := newMockBuilder().
			withContainerInspect(func(ctx context.Context, containerID string) (container.InspectResponse, error) {
				return makeContainerInspectResponse(containerID, "cert-test", "172.17.0.20", "bridge"), nil
			}).
			build()

		client := NewClientWithAPI(mockAPI, logger)
		sync := NewRouteSync(registry, client, "bridge", logger)

		var certGenerated string
		mockCertManager := &mockCertManager{
			ensureCertificateFunc: func(domain string) error {
				certGenerated = domain
				return nil
			},
		}
		sync.SetCertManager(mockCertManager)

		event := ContainerEvent{
			ContainerID:   "certcontainer",
			ContainerName: "cert-test",
			Labels: map[string]string{
				"devproxy.enable": "true",
				"devproxy.host":   "secure.localhost",
				"devproxy.port":   "443",
			},
			Type: "start",
		}

		sync.HandleEvent(event)

		if certGenerated != "secure.localhost" {
			t.Errorf("expected cert generated for 'secure.localhost', got '%s'", certGenerated)
		}
	})

	t.Run("continues when cert generation fails", func(t *testing.T) {
		registry := proxy.NewRegistry()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		mockAPI := newMockBuilder().
			withContainerInspect(func(ctx context.Context, containerID string) (container.InspectResponse, error) {
				return makeContainerInspectResponse(containerID, "cert-fail", "172.17.0.21", "bridge"), nil
			}).
			build()

		client := NewClientWithAPI(mockAPI, logger)
		sync := NewRouteSync(registry, client, "bridge", logger)

		mockCertManager := &mockCertManager{
			ensureCertificateFunc: func(domain string) error {
				return errors.New("cert generation failed")
			},
		}
		sync.SetCertManager(mockCertManager)

		event := ContainerEvent{
			ContainerID:   "certfailcontainer",
			ContainerName: "cert-fail",
			Labels: map[string]string{
				"devproxy.enable": "true",
				"devproxy.host":   "failing-cert.localhost",
				"devproxy.port":   "443",
			},
			Type: "start",
		}

		// Should not panic, route should still be added
		sync.HandleEvent(event)

		route := registry.Lookup("failing-cert.localhost")
		if route == nil {
			t.Error("expected route to be added even when cert generation fails")
		}
	})
}

func TestRouteSync_resolveContainerIP(t *testing.T) {
	t.Run("uses preferred network when available", func(t *testing.T) {
		registry := proxy.NewRegistry()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		mockAPI := newMockBuilder().
			withContainerInspect(func(ctx context.Context, containerID string) (container.InspectResponse, error) {
				return container.InspectResponse{
					ContainerJSONBase: &container.ContainerJSONBase{
						ID:   containerID,
						Name: "/test",
					},
					NetworkSettings: &container.NetworkSettings{
						Networks: map[string]*network.EndpointSettings{
							"bridge":     {IPAddress: "172.17.0.2"},
							"my-network": {IPAddress: "10.0.0.5"},
						},
					},
				}, nil
			}).
			build()

		client := NewClientWithAPI(mockAPI, logger)
		sync := NewRouteSync(registry, client, "my-network", logger)

		ip, err := sync.resolveContainerIP(context.Background(), "container123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if ip != "10.0.0.5" {
			t.Errorf("expected IP '10.0.0.5' from preferred network, got '%s'", ip)
		}
	})

	t.Run("falls back to first available network", func(t *testing.T) {
		registry := proxy.NewRegistry()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		mockAPI := newMockBuilder().
			withContainerInspect(func(ctx context.Context, containerID string) (container.InspectResponse, error) {
				return container.InspectResponse{
					ContainerJSONBase: &container.ContainerJSONBase{
						ID:   containerID,
						Name: "/test",
					},
					NetworkSettings: &container.NetworkSettings{
						Networks: map[string]*network.EndpointSettings{
							"bridge": {IPAddress: "172.17.0.2"},
						},
					},
				}, nil
			}).
			build()

		client := NewClientWithAPI(mockAPI, logger)
		sync := NewRouteSync(registry, client, "nonexistent", logger)

		ip, err := sync.resolveContainerIP(context.Background(), "container123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if ip != "172.17.0.2" {
			t.Errorf("expected IP '172.17.0.2' from fallback network, got '%s'", ip)
		}
	})

	t.Run("returns error when no IP found", func(t *testing.T) {
		registry := proxy.NewRegistry()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		mockAPI := newMockBuilder().
			withContainerInspect(func(ctx context.Context, containerID string) (container.InspectResponse, error) {
				return container.InspectResponse{
					ContainerJSONBase: &container.ContainerJSONBase{
						ID:   containerID,
						Name: "/test",
					},
					NetworkSettings: &container.NetworkSettings{
						Networks: map[string]*network.EndpointSettings{},
					},
				}, nil
			}).
			build()

		client := NewClientWithAPI(mockAPI, logger)
		sync := NewRouteSync(registry, client, "bridge", logger)

		_, err := sync.resolveContainerIP(context.Background(), "container123")
		if err == nil {
			t.Error("expected error when no IP found")
		}
	})

	t.Run("returns error when client not connected", func(t *testing.T) {
		registry := proxy.NewRegistry()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		client := &Client{} // No API set
		sync := NewRouteSync(registry, client, "bridge", logger)

		_, err := sync.resolveContainerIP(context.Background(), "container123")
		if err == nil {
			t.Error("expected error when client not connected")
		}
	})
}

func TestRouteSync_getContainerName(t *testing.T) {
	t.Run("returns truncated ID when client not connected", func(t *testing.T) {
		registry := proxy.NewRegistry()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		client := &Client{} // No API set
		sync := NewRouteSync(registry, client, "bridge", logger)

		name := sync.getContainerName(context.Background(), "abcdef123456789")
		if name != "abcdef123456" {
			t.Errorf("expected truncated ID 'abcdef123456', got '%s'", name)
		}
	})

	t.Run("strips leading slash from name", func(t *testing.T) {
		registry := proxy.NewRegistry()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		mockAPI := newMockBuilder().
			withContainerInspect(func(ctx context.Context, containerID string) (container.InspectResponse, error) {
				return container.InspectResponse{
					ContainerJSONBase: &container.ContainerJSONBase{
						ID:   containerID,
						Name: "/my-container",
					},
				}, nil
			}).
			build()

		client := NewClientWithAPI(mockAPI, logger)
		sync := NewRouteSync(registry, client, "bridge", logger)

		name := sync.getContainerName(context.Background(), "container123")
		if name != "my-container" {
			t.Errorf("expected 'my-container', got '%s'", name)
		}
	})

	t.Run("returns truncated ID on inspect error", func(t *testing.T) {
		registry := proxy.NewRegistry()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		mockAPI := newMockBuilder().
			withContainerInspectError(errMockNotFound).
			build()

		client := NewClientWithAPI(mockAPI, logger)
		sync := NewRouteSync(registry, client, "bridge", logger)

		name := sync.getContainerName(context.Background(), "abcdef123456789")
		if name != "abcdef123456" {
			t.Errorf("expected truncated ID 'abcdef123456', got '%s'", name)
		}
	})
}

func TestRouteSync_SyncExisting(t *testing.T) {
	t.Run("lists containers and creates start events", func(t *testing.T) {
		registry := proxy.NewRegistry()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		var inspectCalls []string
		var mu sync.Mutex

		mockAPI := newMockBuilder().
			withContainerListResult([]container.Summary{
				makeContainerSummary("container1", "web1", map[string]string{
					"devproxy.enable": "true",
					"devproxy.host":   "web1.localhost",
					"devproxy.port":   "8080",
				}),
				makeContainerSummary("container2", "web2", map[string]string{
					"devproxy.enable": "true",
					"devproxy.host":   "web2.localhost",
					"devproxy.port":   "8081",
				}),
			}).
			withContainerInspect(func(ctx context.Context, containerID string) (container.InspectResponse, error) {
				mu.Lock()
				inspectCalls = append(inspectCalls, containerID)
				mu.Unlock()
				return makeContainerInspectResponse(containerID, "test", "172.17.0."+containerID[len(containerID)-1:], "bridge"), nil
			}).
			build()

		client := NewClientWithAPI(mockAPI, logger)
		sync := NewRouteSync(registry, client, "bridge", logger)

		err := sync.SyncExisting(context.Background())
		if err != nil {
			t.Fatalf("SyncExisting failed: %v", err)
		}

		// Both containers should have been inspected (2 calls per container: IP + name)
		mu.Lock()
		defer mu.Unlock()
		if len(inspectCalls) != 4 {
			t.Errorf("expected 4 inspect calls (2 per container), got %d", len(inspectCalls))
		}

		// Both routes should exist
		if registry.Count() != 2 {
			t.Errorf("expected 2 routes, got %d", registry.Count())
		}
	})

	t.Run("returns error when client not connected", func(t *testing.T) {
		registry := proxy.NewRegistry()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		client := &Client{} // No API set
		sync := NewRouteSync(registry, client, "bridge", logger)

		err := sync.SyncExisting(context.Background())
		if err == nil {
			t.Error("expected error when client not connected")
		}
	})

	t.Run("returns error when list fails", func(t *testing.T) {
		registry := proxy.NewRegistry()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		mockAPI := newMockBuilder().
			withContainerListError(errMockConnection).
			build()

		client := NewClientWithAPI(mockAPI, logger)
		sync := NewRouteSync(registry, client, "bridge", logger)

		err := sync.SyncExisting(context.Background())
		if err == nil {
			t.Error("expected error when list fails")
		}
	})
}

func TestRouteSync_ListContainers(t *testing.T) {
	t.Run("returns copy of tracked containers", func(t *testing.T) {
		registry := proxy.NewRegistry()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		sync := NewRouteSync(registry, nil, "bridge", logger)

		// Manually track some containers
		sync.mu.Lock()
		sync.containers["container1"] = []string{"app1.localhost", "app2.localhost"}
		sync.containers["container2"] = []string{"api.localhost"}
		sync.mu.Unlock()

		result := sync.ListContainers()

		if len(result) != 2 {
			t.Errorf("expected 2 containers, got %d", len(result))
		}

		if len(result["container1"]) != 2 {
			t.Errorf("expected 2 hosts for container1, got %d", len(result["container1"]))
		}

		if len(result["container2"]) != 1 {
			t.Errorf("expected 1 host for container2, got %d", len(result["container2"]))
		}

		// Verify it's a copy by modifying the result
		result["container1"][0] = "modified"
		sync.mu.RLock()
		if sync.containers["container1"][0] == "modified" {
			t.Error("ListContainers should return a copy, not the original")
		}
		sync.mu.RUnlock()
	})
}

func TestRouteSync_SetCertManager(t *testing.T) {
	t.Run("sets cert manager correctly", func(t *testing.T) {
		registry := proxy.NewRegistry()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		sync := NewRouteSync(registry, nil, "bridge", logger)

		if sync.certManager != nil {
			t.Error("cert manager should be nil initially")
		}

		mockCM := &mockCertManager{}
		sync.SetCertManager(mockCM)

		if sync.certManager == nil {
			t.Error("cert manager should be set")
		}
	})
}

func TestRouteSync_handleStop_FallbackToRegistry(t *testing.T) {
	t.Run("uses registry RemoveByContainerID when container not tracked", func(t *testing.T) {
		registry := proxy.NewRegistry()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		sync := NewRouteSync(registry, nil, "bridge", logger)

		// Add route directly to registry without tracking in sync
		registry.Add(proxy.Route{
			Host:        "orphan.localhost",
			Backend:     "172.17.0.99:8080",
			ContainerID: "orphancontainer123",
		})

		if registry.Count() != 1 {
			t.Fatalf("expected 1 route, got %d", registry.Count())
		}

		// Stop event for untracked container
		event := ContainerEvent{
			ContainerID: "orphancontainer123",
			Type:        "stop",
		}

		sync.HandleEvent(event)

		// Route should be removed via RemoveByContainerID fallback
		if registry.Count() != 0 {
			t.Errorf("expected 0 routes after stop, got %d", registry.Count())
		}
	})
}

// mockCertManager is a test double for CertManager.
type mockCertManager struct {
	ensureCertificateFunc func(domain string) error
}

func (m *mockCertManager) EnsureCertificate(domain string) error {
	if m.ensureCertificateFunc != nil {
		return m.ensureCertificateFunc(domain)
	}
	return nil
}
