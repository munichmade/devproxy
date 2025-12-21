// Package proxy provides HTTP and TCP proxy functionality.
package proxy

import (
	"sync"
)

// TCPRoute represents a TCP route for SNI-based routing.
type TCPRoute struct {
	Host        string // e.g., "db.myproject.localhost"
	Backend     string // e.g., "172.18.0.5:5432"
	Entrypoint  string // e.g., "postgres"
	ContainerID string // Docker container ID
}

// TCPRegistry stores and retrieves TCP routes.
// It is thread-safe for concurrent access.
type TCPRegistry struct {
	mu     sync.RWMutex
	routes map[string]map[string]*TCPRoute // entrypoint -> host -> route
}

// NewTCPRegistry creates a new TCP route registry.
func NewTCPRegistry() *TCPRegistry {
	return &TCPRegistry{
		routes: make(map[string]map[string]*TCPRoute),
	}
}

// Add registers a TCP route.
// If a route with the same host and entrypoint exists, it will be replaced.
func (r *TCPRegistry) Add(route TCPRoute) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.routes[route.Entrypoint] == nil {
		r.routes[route.Entrypoint] = make(map[string]*TCPRoute)
	}

	routeCopy := route
	r.routes[route.Entrypoint][route.Host] = &routeCopy
}

// Remove removes a TCP route by host and entrypoint.
// Returns true if a route was removed.
func (r *TCPRegistry) Remove(host, entrypoint string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if hostRoutes, ok := r.routes[entrypoint]; ok {
		if _, exists := hostRoutes[host]; exists {
			delete(hostRoutes, host)
			// Clean up empty entrypoint map
			if len(hostRoutes) == 0 {
				delete(r.routes, entrypoint)
			}
			return true
		}
	}
	return false
}

// RemoveByContainerID removes all TCP routes associated with a container.
// Returns the number of routes removed.
func (r *TCPRegistry) RemoveByContainerID(containerID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	removed := 0
	for entrypoint, hostRoutes := range r.routes {
		for host, route := range hostRoutes {
			if route.ContainerID == containerID {
				delete(hostRoutes, host)
				removed++
			}
		}
		// Clean up empty entrypoint map
		if len(hostRoutes) == 0 {
			delete(r.routes, entrypoint)
		}
	}
	return removed
}

// LookupTCP finds a route by hostname and entrypoint.
// Returns the route and true if found, nil and false otherwise.
func (r *TCPRegistry) LookupTCP(host, entrypoint string) (*TCPRoute, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if hostRoutes, ok := r.routes[entrypoint]; ok {
		if route, exists := hostRoutes[host]; exists {
			return route, true
		}
	}
	return nil, false
}

// List returns all TCP routes for a given entrypoint.
// If entrypoint is empty, returns all routes.
func (r *TCPRegistry) List(entrypoint string) []TCPRoute {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []TCPRoute

	if entrypoint != "" {
		if hostRoutes, ok := r.routes[entrypoint]; ok {
			for _, route := range hostRoutes {
				result = append(result, *route)
			}
		}
	} else {
		for _, hostRoutes := range r.routes {
			for _, route := range hostRoutes {
				result = append(result, *route)
			}
		}
	}

	return result
}

// Count returns the total number of TCP routes.
func (r *TCPRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, hostRoutes := range r.routes {
		count += len(hostRoutes)
	}
	return count
}
