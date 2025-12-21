package proxy

import (
	"sync"
	"testing"
)

func TestRegistry_AddAndLookup(t *testing.T) {
	reg := NewRegistry()

	route := Route{
		Host:     "app.localhost",
		Backend:  "127.0.0.1:3000",
		Protocol: ProtocolHTTP,
	}

	if err := reg.Add(route); err != nil {
		t.Fatalf("failed to add route: %v", err)
	}

	found := reg.Lookup("app.localhost")
	if found == nil {
		t.Fatal("expected to find route")
	}

	if found.Backend != "127.0.0.1:3000" {
		t.Errorf("expected backend 127.0.0.1:3000, got %s", found.Backend)
	}

	if found.Protocol != ProtocolHTTP {
		t.Errorf("expected protocol http, got %s", found.Protocol)
	}
}

func TestRegistry_AddDuplicate(t *testing.T) {
	reg := NewRegistry()

	route := Route{
		Host:    "app.localhost",
		Backend: "127.0.0.1:3000",
	}

	if err := reg.Add(route); err != nil {
		t.Fatalf("failed to add route: %v", err)
	}

	// Try to add duplicate
	err := reg.Add(route)
	if err != ErrRouteExists {
		t.Errorf("expected ErrRouteExists, got %v", err)
	}
}

func TestRegistry_Remove(t *testing.T) {
	reg := NewRegistry()

	route := Route{
		Host:    "app.localhost",
		Backend: "127.0.0.1:3000",
	}

	reg.Add(route)

	if err := reg.Remove("app.localhost"); err != nil {
		t.Fatalf("failed to remove route: %v", err)
	}

	if reg.Lookup("app.localhost") != nil {
		t.Error("expected route to be removed")
	}

	// Try to remove non-existent
	err := reg.Remove("nonexistent.localhost")
	if err != ErrRouteNotFound {
		t.Errorf("expected ErrRouteNotFound, got %v", err)
	}
}

func TestRegistry_RemoveByContainerID(t *testing.T) {
	reg := NewRegistry()

	// Add routes for same container
	reg.Add(Route{Host: "app.localhost", Backend: "172.18.0.2:3000", ContainerID: "abc123"})
	reg.Add(Route{Host: "api.localhost", Backend: "172.18.0.2:8080", ContainerID: "abc123"})
	reg.Add(Route{Host: "db.localhost", Backend: "172.18.0.3:5432", ContainerID: "def456"})

	removed := reg.RemoveByContainerID("abc123")
	if removed != 2 {
		t.Errorf("expected 2 routes removed, got %d", removed)
	}

	if reg.Lookup("app.localhost") != nil {
		t.Error("expected app.localhost to be removed")
	}

	if reg.Lookup("api.localhost") != nil {
		t.Error("expected api.localhost to be removed")
	}

	if reg.Lookup("db.localhost") == nil {
		t.Error("expected db.localhost to still exist")
	}
}

func TestRegistry_List(t *testing.T) {
	reg := NewRegistry()

	reg.Add(Route{Host: "zebra.localhost", Backend: "127.0.0.1:1"})
	reg.Add(Route{Host: "alpha.localhost", Backend: "127.0.0.1:2"})
	reg.Add(Route{Host: "beta.localhost", Backend: "127.0.0.1:3"})

	routes := reg.List()

	if len(routes) != 3 {
		t.Fatalf("expected 3 routes, got %d", len(routes))
	}

	// Should be sorted by host
	if routes[0].Host != "alpha.localhost" {
		t.Errorf("expected first route to be alpha.localhost, got %s", routes[0].Host)
	}
	if routes[1].Host != "beta.localhost" {
		t.Errorf("expected second route to be beta.localhost, got %s", routes[1].Host)
	}
	if routes[2].Host != "zebra.localhost" {
		t.Errorf("expected third route to be zebra.localhost, got %s", routes[2].Host)
	}
}

func TestRegistry_Count(t *testing.T) {
	reg := NewRegistry()

	if reg.Count() != 0 {
		t.Errorf("expected count 0, got %d", reg.Count())
	}

	reg.Add(Route{Host: "a.localhost", Backend: "127.0.0.1:1"})
	reg.Add(Route{Host: "b.localhost", Backend: "127.0.0.1:2"})

	if reg.Count() != 2 {
		t.Errorf("expected count 2, got %d", reg.Count())
	}
}

func TestRegistry_Clear(t *testing.T) {
	reg := NewRegistry()

	reg.Add(Route{Host: "a.localhost", Backend: "127.0.0.1:1"})
	reg.Add(Route{Host: "b.localhost", Backend: "127.0.0.1:2"})

	reg.Clear()

	if reg.Count() != 0 {
		t.Errorf("expected count 0 after clear, got %d", reg.Count())
	}
}

func TestRegistry_OnChange(t *testing.T) {
	reg := NewRegistry()

	callCount := 0
	reg.OnChange(func() {
		callCount++
	})

	reg.Add(Route{Host: "a.localhost", Backend: "127.0.0.1:1"})
	if callCount != 1 {
		t.Errorf("expected 1 onChange call after Add, got %d", callCount)
	}

	reg.Remove("a.localhost")
	if callCount != 2 {
		t.Errorf("expected 2 onChange calls after Remove, got %d", callCount)
	}

	reg.Add(Route{Host: "b.localhost", Backend: "127.0.0.1:2"})
	reg.Clear()
	if callCount != 4 {
		t.Errorf("expected 4 onChange calls after Clear, got %d", callCount)
	}
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	reg := NewRegistry()

	var wg sync.WaitGroup
	numGoroutines := 100
	numOperations := 100

	// Concurrent adds
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				host := "host-" + string(rune('a'+id%26)) + ".localhost"
				reg.Add(Route{Host: host, Backend: "127.0.0.1:3000"})
			}
		}(i)
	}
	wg.Wait()

	// Concurrent reads
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				reg.List()
				reg.Count()
				reg.Lookup("host-a.localhost")
			}
		}()
	}
	wg.Wait()

	// Concurrent mixed operations
	wg.Add(numGoroutines * 3)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			host := "mixed-" + string(rune('a'+id%26)) + ".localhost"
			reg.Add(Route{Host: host, Backend: "127.0.0.1:3000"})
		}(i)
		go func() {
			defer wg.Done()
			reg.List()
		}()
		go func(id int) {
			defer wg.Done()
			host := "mixed-" + string(rune('a'+id%26)) + ".localhost"
			reg.Remove(host)
		}(i)
	}
	wg.Wait()

	// If we got here without deadlock or panic, the test passed
}

func TestRegistry_LookupReturnsACopy(t *testing.T) {
	reg := NewRegistry()

	reg.Add(Route{Host: "app.localhost", Backend: "127.0.0.1:3000"})

	found := reg.Lookup("app.localhost")
	found.Backend = "modified"

	// Original should be unchanged
	original := reg.Lookup("app.localhost")
	if original.Backend != "127.0.0.1:3000" {
		t.Error("expected Lookup to return a copy, but original was modified")
	}
}
