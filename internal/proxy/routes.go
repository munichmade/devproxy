// Package proxy provides HTTP and HTTPS proxy servers for devproxy.
package proxy

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/munichmade/devproxy/internal/paths"
)

// Protocol represents the proxy protocol type.
type Protocol string

const (
	// ProtocolHTTP is for HTTP/HTTPS proxying.
	ProtocolHTTP Protocol = "http"

	// ProtocolTCP is for raw TCP proxying.
	ProtocolTCP Protocol = "tcp"
)

// Route represents a proxy route from a host to a backend.
type Route struct {
	// Host is the domain name to match (e.g., "app.localhost" or "*.app.localhost").
	Host string

	// IsWildcard indicates if this is a wildcard route (e.g., "*.app.localhost").
	IsWildcard bool

	// Pattern is the base domain for wildcard routes (e.g., "app.localhost" for "*.app.localhost").
	// Empty for exact routes.
	Pattern string

	// Backend is the upstream address (e.g., "172.18.0.3:3000").
	Backend string

	// Protocol is the proxy type ("http" or "tcp").
	Protocol Protocol

	// Entrypoint is the service type for TCP routes (e.g., "postgres", "redis").
	Entrypoint string

	// ContainerID is the Docker container ID if this route is from Docker.
	ContainerID string

	// ContainerName is the Docker container name for display purposes.
	ContainerName string

	// CreatedAt is when the route was added.
	CreatedAt time.Time
}

// Errors for route operations.
var (
	ErrRouteExists         = errors.New("route already exists")
	ErrWildcardRouteExists = errors.New("wildcard route already exists for this pattern")
	ErrRouteNotFound       = errors.New("route not found")
)

// isWildcardHost checks if host is a wildcard pattern (e.g., "*.app.localhost").
func isWildcardHost(host string) bool {
	return strings.HasPrefix(host, "*.")
}

// wildcardPattern extracts the base domain from a wildcard host.
// e.g., "*.app.localhost" -> "app.localhost"
func wildcardPattern(host string) string {
	return strings.TrimPrefix(host, "*.")
}

// matchWildcard checks if host matches a wildcard pattern (any subdomain depth).
// e.g., host "sub.team.app.localhost" matches pattern "app.localhost"
func matchWildcard(host, pattern string) bool {
	return strings.HasSuffix(host, "."+pattern)
}

// Registry is a thread-safe registry of proxy routes.
type Registry struct {
	mu             sync.RWMutex
	routes         map[string]*Route // exact host -> route
	wildcardRoutes map[string]*Route // pattern (e.g., "app.localhost") -> route

	// onChange is called when routes are added or removed.
	onChange func()
}

// NewRegistry creates a new route registry.
func NewRegistry() *Registry {
	return &Registry{
		routes:         make(map[string]*Route),
		wildcardRoutes: make(map[string]*Route),
	}
}

// OnChange sets a callback to be invoked when routes change.
func (r *Registry) OnChange(fn func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onChange = fn
}

// Add adds a new route to the registry.
// Returns ErrRouteExists if an exact route for the host already exists.
// Returns ErrWildcardRouteExists if a wildcard route for the pattern already exists.
func (r *Registry) Add(route Route) error {
	r.mu.Lock()

	// Set creation time if not provided
	if route.CreatedAt.IsZero() {
		route.CreatedAt = time.Now()
	}

	if isWildcardHost(route.Host) {
		// Handle wildcard route
		route.IsWildcard = true
		route.Pattern = wildcardPattern(route.Host)

		if _, exists := r.wildcardRoutes[route.Pattern]; exists {
			r.mu.Unlock()
			return ErrWildcardRouteExists
		}

		r.wildcardRoutes[route.Pattern] = &route
	} else {
		// Handle exact route
		if _, exists := r.routes[route.Host]; exists {
			r.mu.Unlock()
			return ErrRouteExists
		}

		r.routes[route.Host] = &route
	}

	onChange := r.onChange
	r.mu.Unlock()

	// Call onChange outside the lock to prevent deadlocks
	if onChange != nil {
		onChange()
	}

	return nil
}

// Remove removes a route from the registry.
// Returns ErrRouteNotFound if the route doesn't exist.
func (r *Registry) Remove(host string) error {
	r.mu.Lock()

	if isWildcardHost(host) {
		pattern := wildcardPattern(host)
		if _, exists := r.wildcardRoutes[pattern]; !exists {
			r.mu.Unlock()
			return ErrRouteNotFound
		}
		delete(r.wildcardRoutes, pattern)
	} else {
		if _, exists := r.routes[host]; !exists {
			r.mu.Unlock()
			return ErrRouteNotFound
		}
		delete(r.routes, host)
	}

	onChange := r.onChange
	r.mu.Unlock()

	// Call onChange outside the lock to prevent deadlocks
	if onChange != nil {
		onChange()
	}

	return nil
}

// RemoveByContainerID removes all routes associated with a container.
// Returns the number of routes removed.
func (r *Registry) RemoveByContainerID(containerID string) int {
	r.mu.Lock()

	var removed int

	// Remove from exact routes
	for host, route := range r.routes {
		if route.ContainerID == containerID {
			delete(r.routes, host)
			removed++
		}
	}

	// Remove from wildcard routes
	for pattern, route := range r.wildcardRoutes {
		if route.ContainerID == containerID {
			delete(r.wildcardRoutes, pattern)
			removed++
		}
	}

	onChange := r.onChange
	r.mu.Unlock()

	// Call onChange outside the lock to prevent deadlocks
	if removed > 0 && onChange != nil {
		onChange()
	}

	return removed
}

// findMostSpecificWildcard finds the most specific matching wildcard route.
// More specific = longer pattern (more domain segments).
// Must be called with r.mu held.
func (r *Registry) findMostSpecificWildcard(host string) *Route {
	var bestMatch *Route
	var bestLen int

	for pattern, route := range r.wildcardRoutes {
		if matchWildcard(host, pattern) && len(pattern) > bestLen {
			bestMatch = route
			bestLen = len(pattern)
		}
	}
	return bestMatch
}

// Lookup finds a route by host.
// Priority: exact match > most specific wildcard.
// Returns nil if not found.
func (r *Registry) Lookup(host string) *Route {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 1. Try exact match first (highest priority)
	if route, exists := r.routes[host]; exists {
		copy := *route
		return &copy
	}

	// 2. Try wildcard match (most specific wins)
	if route := r.findMostSpecificWildcard(host); route != nil {
		copy := *route
		return &copy
	}

	return nil
}

// List returns a snapshot of all routes, sorted by host.
func (r *Registry) List() []Route {
	r.mu.RLock()
	defer r.mu.RUnlock()

	routes := make([]Route, 0, len(r.routes)+len(r.wildcardRoutes))

	// Add exact routes
	for _, route := range r.routes {
		routes = append(routes, *route)
	}

	// Add wildcard routes
	for _, route := range r.wildcardRoutes {
		routes = append(routes, *route)
	}

	// Sort by host for consistent ordering
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].Host < routes[j].Host
	})

	return routes
}

// Count returns the number of registered routes.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.routes) + len(r.wildcardRoutes)
}

// GetByEntrypoint returns all routes that match a given entrypoint.
// This is used for TCP routing when no SNI is available.
func (r *Registry) GetByEntrypoint(entrypoint string) []*Route {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*Route
	for _, route := range r.routes {
		if route.Protocol == ProtocolTCP && route.Entrypoint == entrypoint {
			copy := *route
			result = append(result, &copy)
		}
	}
	return result
}

// Clear removes all routes from the registry.
func (r *Registry) Clear() {
	r.mu.Lock()

	hadRoutes := len(r.routes) > 0 || len(r.wildcardRoutes) > 0
	r.routes = make(map[string]*Route)
	r.wildcardRoutes = make(map[string]*Route)
	onChange := r.onChange
	r.mu.Unlock()

	// Call onChange outside the lock to prevent deadlocks
	if hadRoutes && onChange != nil {
		onChange()
	}
}

// StateFile returns the path to the routes state file.
func StateFile() string {
	return filepath.Join(paths.DataDir(), "routes.json")
}

// RouteState represents the serializable state of routes for IPC.
type RouteState struct {
	Routes []Route `json:"routes"`
}

// SaveState writes the current routes to a state file for IPC with CLI.
func (r *Registry) SaveState() error {
	r.mu.RLock()
	routes := make([]Route, 0, len(r.routes)+len(r.wildcardRoutes))

	// Add exact routes
	for _, route := range r.routes {
		routes = append(routes, *route)
	}

	// Add wildcard routes
	for _, route := range r.wildcardRoutes {
		routes = append(routes, *route)
	}
	r.mu.RUnlock()

	// Sort for consistent output
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].Host < routes[j].Host
	})

	state := RouteState{Routes: routes}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	stateFile := StateFile()
	if err := os.MkdirAll(filepath.Dir(stateFile), 0755); err != nil {
		return err
	}

	return os.WriteFile(stateFile, data, 0644)
}

// LoadState reads routes from the state file (used by CLI to query daemon state).
func LoadState() ([]Route, error) {
	data, err := os.ReadFile(StateFile())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var state RouteState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	return state.Routes, nil
}
