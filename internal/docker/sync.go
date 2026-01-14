// Package docker provides Docker integration for the dev proxy.
package docker

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/docker/docker/api/types/container"

	"github.com/munichmade/devproxy/internal/proxy"
)

// IPResolver resolves a container ID to its IP address.
type IPResolver interface {
	ResolveContainerIP(ctx context.Context, containerID string, network string) (string, error)
}

// CertManager provides certificate operations.
type CertManager interface {
	EnsureCertificate(domain string) error
}

// RouteSync synchronizes Docker container events with the route registry.
type RouteSync struct {
	registry    *proxy.Registry
	parser      *LabelParser
	client      *Client
	certManager CertManager
	network     string
	logger      *slog.Logger

	mu         sync.RWMutex
	containers map[string][]string // containerID -> list of hosts
}

// NewRouteSync creates a new route synchronizer.
func NewRouteSync(registry *proxy.Registry, client *Client, network string, logger *slog.Logger) *RouteSync {
	return &RouteSync{
		registry:   registry,
		parser:     NewLabelParser(),
		client:     client,
		network:    network,
		logger:     logger,
		containers: make(map[string][]string),
	}
}

// SetCertManager sets the certificate manager for automatic certificate generation.
func (s *RouteSync) SetCertManager(cm CertManager) {
	s.certManager = cm
}

// HandleEvent processes a container event and updates routes accordingly.
func (s *RouteSync) HandleEvent(event ContainerEvent) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("panic in HandleEvent", "recover", r, "event", event.Type, "container", event.ContainerName)
		}
	}()

	s.logger.Info("HandleEvent called", "type", event.Type, "container", event.ContainerName)

	switch event.Type {
	case "start":
		s.handleStart(event)
	case "stop", "die":
		s.handleStop(event)
	}

	s.logger.Info("HandleEvent completed", "container", event.ContainerName)
}

// handleStart processes a container start event.
func (s *RouteSync) handleStart(event ContainerEvent) {
	ctx := context.Background()

	// Safely truncate container ID for logging
	containerIDShort := event.ContainerID
	if len(containerIDShort) > 12 {
		containerIDShort = containerIDShort[:12]
	}

	s.logger.Debug("processing container start",
		"container", event.ContainerName,
		"id", containerIDShort,
		"labels_count", len(event.Labels))

	// Parse labels to get service configurations
	configs, err := s.parser.ParseLabels(event.Labels)
	s.logger.Debug("parsed labels", "container", event.ContainerName, "configs", len(configs), "error", err)
	if err != nil {
		s.logger.Warn("failed to parse container labels",
			"container", event.ContainerID[:12],
			"error", err)
		return
	}

	if len(configs) == 0 {
		s.logger.Debug("container has no devproxy configuration",
			"container", event.ContainerName)
		return
	}

	s.logger.Debug("parsed service configs",
		"container", event.ContainerName,
		"configs_count", len(configs))

	s.logger.Debug("resolving container IP", "container", event.ContainerName, "id", containerIDShort)

	// Resolve container IP
	ip, err := s.resolveContainerIP(ctx, event.ContainerID)

	s.logger.Debug("resolved container IP", "container", event.ContainerName, "ip", ip, "error", err)
	if err != nil {
		s.logger.Error("failed to resolve container IP",
			"container", event.ContainerID[:12],
			"error", err)
		return
	}

	// Use container name from event or resolve it
	containerName := event.ContainerName
	if containerName == "" {
		containerName = s.getContainerName(ctx, event.ContainerID)
	}

	// Register routes for each service
	var hosts []string
	s.logger.Debug("registering routes", "container", event.ContainerName, "count", len(configs))
	for _, config := range configs {
		// Split comma-separated hosts into individual hosts
		hostList := strings.Split(config.Host, ",")
		for _, host := range hostList {
			host = strings.TrimSpace(host)
			if host == "" {
				continue
			}

			backend := fmt.Sprintf("%s:%d", ip, config.Port)
			s.logger.Debug("creating route", "host", host, "backend", backend)

			route := proxy.Route{
				Host:          host,
				Backend:       backend,
				Protocol:      s.getProtocol(config),
				Entrypoint:    config.Entrypoint,
				ContainerID:   event.ContainerID,
				ContainerName: containerName,
			}

			if err := s.registry.Add(route); err != nil {
				s.logger.Warn("failed to add route",
					"host", host,
					"error", err)
				continue
			}

			s.logger.Info("route added successfully",
				"host", host,
				"backend", backend,
				"container", containerName)

			hosts = append(hosts, host)
			s.logger.Info("route added",
				"host", host,
				"backend", backend,
				"container", containerName)

			// Pre-generate certificate for the domain
			if s.certManager != nil {
				if err := s.certManager.EnsureCertificate(host); err != nil {
					s.logger.Warn("failed to pre-generate certificate",
						"host", host,
						"error", err)
				} else {
					s.logger.Debug("certificate ready",
						"host", host)
				}
			}
		}
	}

	// Track which hosts belong to this container
	if len(hosts) > 0 {
		s.mu.Lock()
		s.containers[event.ContainerID] = hosts
		s.mu.Unlock()
	}
}

// handleStop processes a container stop event.
func (s *RouteSync) handleStop(event ContainerEvent) {
	s.mu.Lock()
	hosts, exists := s.containers[event.ContainerID]
	if exists {
		delete(s.containers, event.ContainerID)
	}
	s.mu.Unlock()

	if !exists {
		// Try registry's RemoveByContainerID as fallback
		removed := s.registry.RemoveByContainerID(event.ContainerID)
		if removed > 0 {
			s.logger.Info("routes removed by container ID",
				"container", event.ContainerID[:12],
				"count", removed)
		}
		return
	}

	// Remove each tracked host
	for _, host := range hosts {
		if err := s.registry.Remove(host); err != nil {
			s.logger.Warn("failed to remove route",
				"host", host,
				"error", err)
			continue
		}
		s.logger.Info("route removed",
			"host", host,
			"container", event.ContainerID[:12])
	}
}

// resolveContainerIP gets the IP address of a container.
func (s *RouteSync) resolveContainerIP(ctx context.Context, containerID string) (string, error) {
	if s.client.APIClient() == nil {
		return "", fmt.Errorf("docker client not connected")
	}

	info, err := s.client.APIClient().ContainerInspect(ctx, containerID)
	if err != nil {
		return "", fmt.Errorf("failed to inspect container: %w", err)
	}

	// Try the specified network first
	if s.network != "" {
		if network, ok := info.NetworkSettings.Networks[s.network]; ok && network.IPAddress != "" {
			return network.IPAddress, nil
		}
	}

	// Fall back to first available network
	for _, network := range info.NetworkSettings.Networks {
		if network.IPAddress != "" {
			return network.IPAddress, nil
		}
	}

	// Fall back to default network IP (deprecated but kept for compatibility)
	//nolint:staticcheck // IPAddress is deprecated but we use Networks first
	if info.NetworkSettings.IPAddress != "" {
		return info.NetworkSettings.IPAddress, nil
	}

	return "", fmt.Errorf("no IP address found for container")
}

// getContainerName gets the display name of a container.
func (s *RouteSync) getContainerName(ctx context.Context, containerID string) string {
	if s.client.APIClient() == nil {
		return containerID[:12]
	}

	info, err := s.client.APIClient().ContainerInspect(ctx, containerID)
	if err != nil {
		return containerID[:12]
	}

	// Remove leading slash from container name
	name := info.Name
	if len(name) > 0 && name[0] == '/' {
		name = name[1:]
	}
	return name
}

// getProtocol determines the protocol based on service config.
func (s *RouteSync) getProtocol(config ServiceConfig) proxy.Protocol {
	if config.Entrypoint != "" {
		return proxy.ProtocolTCP
	}
	return proxy.ProtocolHTTP
}

// ListContainers returns a snapshot of tracked container IDs and their hosts.
func (s *RouteSync) ListContainers() map[string][]string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string][]string, len(s.containers))
	for id, hosts := range s.containers {
		hostsCopy := make([]string, len(hosts))
		copy(hostsCopy, hosts)
		result[id] = hostsCopy
	}
	return result
}

// SyncExisting scans for existing containers and adds their routes.
func (s *RouteSync) SyncExisting(ctx context.Context) error {
	if s.client.APIClient() == nil {
		return fmt.Errorf("docker client not connected")
	}

	containers, err := s.client.APIClient().ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	for _, c := range containers {
		// Create a synthetic start event
		event := ContainerEvent{
			ContainerID: c.ID,
			Labels:      c.Labels,
			Type:        "start",
		}
		s.handleStart(event)
	}

	return nil
}
