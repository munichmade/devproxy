package proxy

import (
	"sync"
	"testing"
)

func TestTCPRegistry_Add(t *testing.T) {
	t.Run("adds route successfully", func(t *testing.T) {
		registry := NewTCPRegistry()

		route := TCPRoute{
			Host:        "db.localhost",
			Backend:     "172.18.0.5:5432",
			Entrypoint:  "postgres",
			ContainerID: "abc123",
		}

		registry.Add(route)

		got, found := registry.LookupTCP("db.localhost", "postgres")
		if !found {
			t.Fatal("expected route to be found")
		}
		if got.Backend != "172.18.0.5:5432" {
			t.Errorf("expected backend '172.18.0.5:5432', got '%s'", got.Backend)
		}
	})

	t.Run("replaces existing route with same host and entrypoint", func(t *testing.T) {
		registry := NewTCPRegistry()

		route1 := TCPRoute{
			Host:       "db.localhost",
			Backend:    "172.18.0.5:5432",
			Entrypoint: "postgres",
		}
		route2 := TCPRoute{
			Host:       "db.localhost",
			Backend:    "172.18.0.10:5432",
			Entrypoint: "postgres",
		}

		registry.Add(route1)
		registry.Add(route2)

		got, found := registry.LookupTCP("db.localhost", "postgres")
		if !found {
			t.Fatal("expected route to be found")
		}
		if got.Backend != "172.18.0.10:5432" {
			t.Errorf("expected backend '172.18.0.10:5432', got '%s'", got.Backend)
		}

		if registry.Count() != 1 {
			t.Errorf("expected 1 route, got %d", registry.Count())
		}
	})

	t.Run("allows same host on different entrypoints", func(t *testing.T) {
		registry := NewTCPRegistry()

		route1 := TCPRoute{
			Host:       "db.localhost",
			Backend:    "172.18.0.5:5432",
			Entrypoint: "postgres",
		}
		route2 := TCPRoute{
			Host:       "db.localhost",
			Backend:    "172.18.0.5:3306",
			Entrypoint: "mysql",
		}

		registry.Add(route1)
		registry.Add(route2)

		got1, found1 := registry.LookupTCP("db.localhost", "postgres")
		got2, found2 := registry.LookupTCP("db.localhost", "mysql")

		if !found1 || !found2 {
			t.Fatal("expected both routes to be found")
		}
		if got1.Backend != "172.18.0.5:5432" {
			t.Errorf("expected postgres backend '172.18.0.5:5432', got '%s'", got1.Backend)
		}
		if got2.Backend != "172.18.0.5:3306" {
			t.Errorf("expected mysql backend '172.18.0.5:3306', got '%s'", got2.Backend)
		}

		if registry.Count() != 2 {
			t.Errorf("expected 2 routes, got %d", registry.Count())
		}
	})
}

func TestTCPRegistry_Remove(t *testing.T) {
	t.Run("removes existing route", func(t *testing.T) {
		registry := NewTCPRegistry()

		registry.Add(TCPRoute{
			Host:       "db.localhost",
			Backend:    "172.18.0.5:5432",
			Entrypoint: "postgres",
		})

		removed := registry.Remove("db.localhost", "postgres")
		if !removed {
			t.Error("expected Remove to return true")
		}

		_, found := registry.LookupTCP("db.localhost", "postgres")
		if found {
			t.Error("expected route to be removed")
		}
	})

	t.Run("returns false for non-existent route", func(t *testing.T) {
		registry := NewTCPRegistry()

		removed := registry.Remove("unknown.localhost", "postgres")
		if removed {
			t.Error("expected Remove to return false for non-existent route")
		}
	})

	t.Run("returns false for wrong entrypoint", func(t *testing.T) {
		registry := NewTCPRegistry()

		registry.Add(TCPRoute{
			Host:       "db.localhost",
			Backend:    "172.18.0.5:5432",
			Entrypoint: "postgres",
		})

		removed := registry.Remove("db.localhost", "mysql")
		if removed {
			t.Error("expected Remove to return false for wrong entrypoint")
		}

		// Original route should still exist
		_, found := registry.LookupTCP("db.localhost", "postgres")
		if !found {
			t.Error("expected original route to still exist")
		}
	})
}

func TestTCPRegistry_RemoveByContainerID(t *testing.T) {
	t.Run("removes all routes for container", func(t *testing.T) {
		registry := NewTCPRegistry()

		registry.Add(TCPRoute{
			Host:        "db1.localhost",
			Backend:     "172.18.0.5:5432",
			Entrypoint:  "postgres",
			ContainerID: "container1",
		})
		registry.Add(TCPRoute{
			Host:        "db2.localhost",
			Backend:     "172.18.0.5:3306",
			Entrypoint:  "mysql",
			ContainerID: "container1",
		})
		registry.Add(TCPRoute{
			Host:        "other.localhost",
			Backend:     "172.18.0.10:5432",
			Entrypoint:  "postgres",
			ContainerID: "container2",
		})

		removed := registry.RemoveByContainerID("container1")
		if removed != 2 {
			t.Errorf("expected 2 routes removed, got %d", removed)
		}

		if registry.Count() != 1 {
			t.Errorf("expected 1 route remaining, got %d", registry.Count())
		}

		_, found := registry.LookupTCP("other.localhost", "postgres")
		if !found {
			t.Error("expected container2 route to still exist")
		}
	})

	t.Run("returns 0 for unknown container", func(t *testing.T) {
		registry := NewTCPRegistry()

		registry.Add(TCPRoute{
			Host:        "db.localhost",
			Backend:     "172.18.0.5:5432",
			Entrypoint:  "postgres",
			ContainerID: "container1",
		})

		removed := registry.RemoveByContainerID("unknown")
		if removed != 0 {
			t.Errorf("expected 0 routes removed, got %d", removed)
		}
	})
}

func TestTCPRegistry_LookupTCP(t *testing.T) {
	t.Run("finds existing route", func(t *testing.T) {
		registry := NewTCPRegistry()

		registry.Add(TCPRoute{
			Host:       "db.localhost",
			Backend:    "172.18.0.5:5432",
			Entrypoint: "postgres",
		})

		route, found := registry.LookupTCP("db.localhost", "postgres")
		if !found {
			t.Fatal("expected route to be found")
		}
		if route.Host != "db.localhost" {
			t.Errorf("expected host 'db.localhost', got '%s'", route.Host)
		}
	})

	t.Run("returns false for unknown host", func(t *testing.T) {
		registry := NewTCPRegistry()

		_, found := registry.LookupTCP("unknown.localhost", "postgres")
		if found {
			t.Error("expected route not to be found")
		}
	})

	t.Run("returns false for unknown entrypoint", func(t *testing.T) {
		registry := NewTCPRegistry()

		registry.Add(TCPRoute{
			Host:       "db.localhost",
			Backend:    "172.18.0.5:5432",
			Entrypoint: "postgres",
		})

		_, found := registry.LookupTCP("db.localhost", "unknown")
		if found {
			t.Error("expected route not to be found for unknown entrypoint")
		}
	})
}

func TestTCPRegistry_List(t *testing.T) {
	t.Run("lists routes for specific entrypoint", func(t *testing.T) {
		registry := NewTCPRegistry()

		registry.Add(TCPRoute{Host: "db1.localhost", Backend: "1", Entrypoint: "postgres"})
		registry.Add(TCPRoute{Host: "db2.localhost", Backend: "2", Entrypoint: "postgres"})
		registry.Add(TCPRoute{Host: "db3.localhost", Backend: "3", Entrypoint: "mysql"})

		routes := registry.List("postgres")
		if len(routes) != 2 {
			t.Errorf("expected 2 postgres routes, got %d", len(routes))
		}
	})

	t.Run("lists all routes when entrypoint is empty", func(t *testing.T) {
		registry := NewTCPRegistry()

		registry.Add(TCPRoute{Host: "db1.localhost", Backend: "1", Entrypoint: "postgres"})
		registry.Add(TCPRoute{Host: "db2.localhost", Backend: "2", Entrypoint: "mysql"})

		routes := registry.List("")
		if len(routes) != 2 {
			t.Errorf("expected 2 total routes, got %d", len(routes))
		}
	})

	t.Run("returns empty slice for unknown entrypoint", func(t *testing.T) {
		registry := NewTCPRegistry()

		registry.Add(TCPRoute{Host: "db.localhost", Backend: "1", Entrypoint: "postgres"})

		routes := registry.List("unknown")
		if len(routes) != 0 {
			t.Errorf("expected 0 routes, got %d", len(routes))
		}
	})
}

func TestTCPRegistry_Count(t *testing.T) {
	registry := NewTCPRegistry()

	if registry.Count() != 0 {
		t.Errorf("expected 0 routes initially, got %d", registry.Count())
	}

	registry.Add(TCPRoute{Host: "db1.localhost", Backend: "1", Entrypoint: "postgres"})
	registry.Add(TCPRoute{Host: "db2.localhost", Backend: "2", Entrypoint: "mysql"})

	if registry.Count() != 2 {
		t.Errorf("expected 2 routes, got %d", registry.Count())
	}
}

func TestTCPRegistry_ConcurrentAccess(t *testing.T) {
	registry := NewTCPRegistry()

	var wg sync.WaitGroup
	const numGoroutines = 100

	// Concurrent adds
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			registry.Add(TCPRoute{
				Host:       "db.localhost",
				Backend:    "172.18.0.5:5432",
				Entrypoint: "postgres",
			})
		}(i)
	}

	// Concurrent lookups
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			registry.LookupTCP("db.localhost", "postgres")
		}()
	}

	// Concurrent lists
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			registry.List("")
		}()
	}

	wg.Wait()

	// Should have exactly 1 route (all adds were for the same host/entrypoint)
	if registry.Count() != 1 {
		t.Errorf("expected 1 route after concurrent adds, got %d", registry.Count())
	}
}
