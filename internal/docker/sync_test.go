package docker

import (
	"io"
	"log/slog"
	"testing"

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
