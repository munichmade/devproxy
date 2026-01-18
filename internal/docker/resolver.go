// Package docker provides Docker integration for the proxy.
package docker

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/container"
)

// ContainerResolver resolves container information from Docker.
type ContainerResolver struct {
	client  *Client
	network string // preferred network name
}

// NewContainerResolver creates a new container resolver.
func NewContainerResolver(client *Client, network string) *ContainerResolver {
	return &ContainerResolver{
		client:  client,
		network: network,
	}
}

// ResolveIP gets the IP address of a container.
// It tries the preferred network first, then falls back to any available network.
func (r *ContainerResolver) ResolveIP(ctx context.Context, containerID string) (string, error) {
	if r.client.API() == nil {
		return "", fmt.Errorf("docker client not connected")
	}

	info, err := r.client.API().ContainerInspect(ctx, containerID)
	if err != nil {
		return "", fmt.Errorf("failed to inspect container: %w", err)
	}

	return r.extractIP(info.NetworkSettings)
}

// extractIP extracts the IP from network settings.
func (r *ContainerResolver) extractIP(settings *container.NetworkSettings) (string, error) {
	if settings == nil {
		return "", fmt.Errorf("no network settings")
	}

	// Try the specified network first
	if r.network != "" {
		if network, ok := settings.Networks[r.network]; ok && network.IPAddress != "" {
			return network.IPAddress, nil
		}
	}

	// Fall back to first available network
	for _, network := range settings.Networks {
		if network.IPAddress != "" {
			return network.IPAddress, nil
		}
	}

	return "", fmt.Errorf("no IP address found for container")
}

// ResolveName gets the display name of a container.
func (r *ContainerResolver) ResolveName(ctx context.Context, containerID string) (string, error) {
	if r.client.API() == nil {
		return "", fmt.Errorf("docker client not connected")
	}

	info, err := r.client.API().ContainerInspect(ctx, containerID)
	if err != nil {
		return "", fmt.Errorf("failed to inspect container: %w", err)
	}

	return r.extractName(info.Name), nil
}

// extractName extracts and cleans the container name.
func (r *ContainerResolver) extractName(name string) string {
	// Remove leading slash from container name
	if len(name) > 0 && name[0] == '/' {
		return name[1:]
	}
	return name
}

// ResolveInfo gets both IP and name for a container in a single call.
func (r *ContainerResolver) ResolveInfo(ctx context.Context, containerID string) (ip, name string, err error) {
	if r.client.API() == nil {
		return "", "", fmt.Errorf("docker client not connected")
	}

	info, err := r.client.API().ContainerInspect(ctx, containerID)
	if err != nil {
		return "", "", fmt.Errorf("failed to inspect container: %w", err)
	}

	ip, err = r.extractIP(info.NetworkSettings)
	if err != nil {
		return "", "", err
	}

	name = r.extractName(info.Name)
	return ip, name, nil
}

// SetNetwork changes the preferred network for IP resolution.
func (r *ContainerResolver) SetNetwork(network string) {
	r.network = network
}
