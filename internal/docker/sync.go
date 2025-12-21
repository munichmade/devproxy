// Package docker provides Docker integration for the dev proxy.
package docker

import (
	"context"
	"fmt"
	"log/slog"
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
func NewRouteSync(registry *proxy.Registry, client *Client, labelPrefix, network string, logger *slog.Logger) *RouteSync {
	return &RouteSync{
		registry:   registry,
		parser:     NewLabelParser(labelPrefix),
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
	switch event.Type {
	case "start":
		s.handleStart(event)
	case "stop", "die":
		s.handleStop(event)
	}
}

// handleStart processes a container start event.
func (s *RouteSync) handleStart(event ContainerEvent) {
	ctx := context.Background()

	// Parse labels to get service configurations
	configs, err := s.parser.ParseLabels(event.Labels)
	if err != nil {
		s.logger.Warn("failed to parse container labels",
			"container", event.ContainerID[:12],
			"error", err)
		return
	}

	if len(configs) == 0 {
		// Container doesn't have devproxy enabled
		return
	}

	// Resolve container IP
	ip, err := s.resolveContainerIP(ctx, event.ContainerID)
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
	for _, config := range configs {
		backend := fmt.Sprintf("%s:%d", ip, config.Port)

		route := proxy.Route{
			Host:          config.Host,
			Backend:       backend,
			Protocol:      s.getProtocol(config),
			Entrypoint:    config.Entrypoint,
			ContainerID:   event.ContainerID,
			ContainerName: containerName,
		}

		if err := s.registry.Add(route); err != nil {
			s.logger.Warn("failed to add route",
				"host", config.Host,
				"backend", backend,
				"error", err)
			continue
		}

		hosts = append(hosts, config.Host)
		s.logger.Info("route added",
			"host", config.Host,
			"backend", backend,
			"container", containerName)

		// Pre-generate certificate for the domain
		if s.certManager != nil {
			if err := s.certManager.EnsureCertificate(config.Host); err != nil {
				s.logger.Warn("failed to pre-generate certificate",
					"host", config.Host,
					"error", err)
			} else {
				s.logger.Debug("certificate ready",
					"host", config.Host)
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

	// Fall back to default network IP
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
