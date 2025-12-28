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

// Wildcard routing tests

func TestRegistry_WildcardBasicMatching(t *testing.T) {
	reg := NewRegistry()

	// Add wildcard route
	err := reg.Add(Route{
		Host:    "*.app.localhost",
		Backend: "127.0.0.1:3000",
	})
	if err != nil {
		t.Fatalf("failed to add wildcard route: %v", err)
	}

	t.Run("matches single-level subdomain", func(t *testing.T) {
		found := reg.Lookup("team-a.app.localhost")
		if found == nil {
			t.Fatal("expected to find route for team-a.app.localhost")
		}
		if found.Backend != "127.0.0.1:3000" {
			t.Errorf("expected backend 127.0.0.1:3000, got %s", found.Backend)
		}
	})

	t.Run("matches multi-level subdomain", func(t *testing.T) {
		found := reg.Lookup("sub.team-a.app.localhost")
		if found == nil {
			t.Fatal("expected to find route for sub.team-a.app.localhost")
		}
	})

	t.Run("matches deep subdomain", func(t *testing.T) {
		found := reg.Lookup("a.b.c.d.app.localhost")
		if found == nil {
			t.Fatal("expected to find route for a.b.c.d.app.localhost")
		}
	})

	t.Run("does not match base domain", func(t *testing.T) {
		found := reg.Lookup("app.localhost")
		if found != nil {
			t.Error("wildcard should not match base domain app.localhost")
		}
	})

	t.Run("does not match unrelated domain", func(t *testing.T) {
		found := reg.Lookup("other.localhost")
		if found != nil {
			t.Error("wildcard should not match unrelated domain")
		}
	})
}

func TestRegistry_WildcardIsWildcardFlag(t *testing.T) {
	reg := NewRegistry()

	reg.Add(Route{Host: "*.app.localhost", Backend: "127.0.0.1:3000"})

	found := reg.Lookup("sub.app.localhost")
	if found == nil {
		t.Fatal("expected to find route")
	}
	if !found.IsWildcard {
		t.Error("expected IsWildcard to be true")
	}
	if found.Pattern != "app.localhost" {
		t.Errorf("expected Pattern 'app.localhost', got %q", found.Pattern)
	}
}

func TestRegistry_WildcardPriorityExactBeatsWildcard(t *testing.T) {
	reg := NewRegistry()

	// Add wildcard first
	reg.Add(Route{Host: "*.app.localhost", Backend: "127.0.0.1:3000"})
	// Add exact match
	reg.Add(Route{Host: "admin.app.localhost", Backend: "127.0.0.1:4000"})

	t.Run("exact match wins over wildcard", func(t *testing.T) {
		found := reg.Lookup("admin.app.localhost")
		if found == nil {
			t.Fatal("expected to find route")
		}
		if found.Backend != "127.0.0.1:4000" {
			t.Errorf("expected exact match backend 127.0.0.1:4000, got %s", found.Backend)
		}
		if found.IsWildcard {
			t.Error("expected exact match, not wildcard")
		}
	})

	t.Run("wildcard still matches other subdomains", func(t *testing.T) {
		found := reg.Lookup("team-a.app.localhost")
		if found == nil {
			t.Fatal("expected to find route")
		}
		if found.Backend != "127.0.0.1:3000" {
			t.Errorf("expected wildcard backend 127.0.0.1:3000, got %s", found.Backend)
		}
	})
}

func TestRegistry_WildcardPriorityMostSpecificWins(t *testing.T) {
	reg := NewRegistry()

	// Add broader wildcard first
	reg.Add(Route{Host: "*.app.localhost", Backend: "127.0.0.1:3000"})
	// Add more specific wildcard
	reg.Add(Route{Host: "*.team-a.app.localhost", Backend: "127.0.0.1:4000"})

	t.Run("more specific wildcard wins", func(t *testing.T) {
		found := reg.Lookup("sub.team-a.app.localhost")
		if found == nil {
			t.Fatal("expected to find route")
		}
		if found.Backend != "127.0.0.1:4000" {
			t.Errorf("expected more specific wildcard backend 127.0.0.1:4000, got %s", found.Backend)
		}
	})

	t.Run("broader wildcard matches other subdomains", func(t *testing.T) {
		found := reg.Lookup("team-b.app.localhost")
		if found == nil {
			t.Fatal("expected to find route")
		}
		if found.Backend != "127.0.0.1:3000" {
			t.Errorf("expected broader wildcard backend 127.0.0.1:3000, got %s", found.Backend)
		}
	})
}

func TestRegistry_WildcardDuplicateRejection(t *testing.T) {
	reg := NewRegistry()

	err := reg.Add(Route{Host: "*.app.localhost", Backend: "127.0.0.1:3000"})
	if err != nil {
		t.Fatalf("failed to add first wildcard: %v", err)
	}

	// Try to add duplicate wildcard
	err = reg.Add(Route{Host: "*.app.localhost", Backend: "127.0.0.1:4000"})
	if err == nil {
		t.Error("expected error when adding duplicate wildcard")
	}
	if err != ErrWildcardRouteExists {
		t.Errorf("expected ErrWildcardRouteExists, got %v", err)
	}
}

func TestRegistry_WildcardMixedWithExact(t *testing.T) {
	reg := NewRegistry()

	// Add both exact and wildcard for same base domain
	reg.Add(Route{Host: "app.localhost", Backend: "127.0.0.1:3000"})
	reg.Add(Route{Host: "*.app.localhost", Backend: "127.0.0.1:4000"})

	t.Run("exact base domain matches exact route", func(t *testing.T) {
		found := reg.Lookup("app.localhost")
		if found == nil {
			t.Fatal("expected to find route")
		}
		if found.Backend != "127.0.0.1:3000" {
			t.Errorf("expected exact backend 127.0.0.1:3000, got %s", found.Backend)
		}
		if found.IsWildcard {
			t.Error("expected exact match, not wildcard")
		}
	})

	t.Run("subdomain matches wildcard route", func(t *testing.T) {
		found := reg.Lookup("sub.app.localhost")
		if found == nil {
			t.Fatal("expected to find route")
		}
		if found.Backend != "127.0.0.1:4000" {
			t.Errorf("expected wildcard backend 127.0.0.1:4000, got %s", found.Backend)
		}
		if !found.IsWildcard {
			t.Error("expected wildcard match")
		}
	})
}

func TestRegistry_WildcardRemove(t *testing.T) {
	reg := NewRegistry()

	reg.Add(Route{Host: "*.app.localhost", Backend: "127.0.0.1:3000"})

	// Verify it exists
	if reg.Lookup("sub.app.localhost") == nil {
		t.Fatal("expected wildcard route to exist")
	}

	// Remove wildcard
	err := reg.Remove("*.app.localhost")
	if err != nil {
		t.Fatalf("failed to remove wildcard: %v", err)
	}

	// Verify it's gone
	if reg.Lookup("sub.app.localhost") != nil {
		t.Error("expected wildcard route to be removed")
	}
}

func TestRegistry_WildcardRemoveByContainerID(t *testing.T) {
	reg := NewRegistry()

	reg.Add(Route{Host: "*.app.localhost", Backend: "127.0.0.1:3000", ContainerID: "abc123"})
	reg.Add(Route{Host: "exact.localhost", Backend: "127.0.0.1:4000", ContainerID: "abc123"})
	reg.Add(Route{Host: "*.other.localhost", Backend: "127.0.0.1:5000", ContainerID: "def456"})

	removed := reg.RemoveByContainerID("abc123")
	if removed != 2 {
		t.Errorf("expected 2 routes removed, got %d", removed)
	}

	if reg.Lookup("sub.app.localhost") != nil {
		t.Error("expected wildcard route to be removed")
	}
	if reg.Lookup("exact.localhost") != nil {
		t.Error("expected exact route to be removed")
	}
	if reg.Lookup("sub.other.localhost") == nil {
		t.Error("expected other wildcard route to still exist")
	}
}

func TestRegistry_WildcardList(t *testing.T) {
	reg := NewRegistry()

	reg.Add(Route{Host: "*.app.localhost", Backend: "127.0.0.1:3000"})
	reg.Add(Route{Host: "app.localhost", Backend: "127.0.0.1:4000"})
	reg.Add(Route{Host: "*.alpha.localhost", Backend: "127.0.0.1:5000"})

	routes := reg.List()

	if len(routes) != 3 {
		t.Fatalf("expected 3 routes, got %d", len(routes))
	}

	// Should be sorted by host (wildcards sort with their pattern)
	expectedOrder := []string{"*.alpha.localhost", "*.app.localhost", "app.localhost"}
	for i, expected := range expectedOrder {
		if routes[i].Host != expected {
			t.Errorf("expected routes[%d].Host = %q, got %q", i, expected, routes[i].Host)
		}
	}
}

func TestRegistry_WildcardCount(t *testing.T) {
	reg := NewRegistry()

	reg.Add(Route{Host: "*.app.localhost", Backend: "127.0.0.1:3000"})
	reg.Add(Route{Host: "app.localhost", Backend: "127.0.0.1:4000"})

	if reg.Count() != 2 {
		t.Errorf("expected count 2, got %d", reg.Count())
	}
}

func TestRegistry_WildcardClear(t *testing.T) {
	reg := NewRegistry()

	reg.Add(Route{Host: "*.app.localhost", Backend: "127.0.0.1:3000"})
	reg.Add(Route{Host: "app.localhost", Backend: "127.0.0.1:4000"})

	reg.Clear()

	if reg.Count() != 0 {
		t.Errorf("expected count 0 after clear, got %d", reg.Count())
	}
	if reg.Lookup("sub.app.localhost") != nil {
		t.Error("expected wildcard route to be cleared")
	}
}
