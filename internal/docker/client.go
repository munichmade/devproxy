// Package docker provides Docker integration for automatic container discovery.
package docker

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

// Client wraps the Docker API client with connection management.
type Client struct {
	api    DockerAPI
	logger *slog.Logger

	mu        sync.RWMutex
	connected bool
}

// NewClient creates a new Docker client using environment configuration.
// It uses DOCKER_HOST, DOCKER_CERT_PATH, etc. from environment.
func NewClient(logger *slog.Logger) (*Client, error) {
	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	return &Client{
		api:    cli,
		logger: logger,
	}, nil
}

// NewClientWithHost creates a Docker client connecting to a specific host.
func NewClientWithHost(host string, logger *slog.Logger) (*Client, error) {
	cli, err := client.NewClientWithOpts(
		client.WithHost(host),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	return &Client{
		api:    cli,
		logger: logger,
	}, nil
}

// NewClientWithAPI creates a Docker client with a custom DockerAPI implementation.
// This is primarily useful for testing.
func NewClientWithAPI(api DockerAPI, logger *slog.Logger) *Client {
	return &Client{
		api:    api,
		logger: logger,
	}
}

// Connect verifies the connection to Docker daemon.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Ping Docker daemon to verify connection
	_, err := c.api.Ping(ctx)
	if err != nil {
		c.connected = false
		return fmt.Errorf("failed to connect to Docker daemon: %w", err)
	}

	c.connected = true
	c.logger.Info("connected to Docker daemon")
	return nil
}

// IsConnected returns whether the client is connected to Docker daemon.
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// Close closes the Docker client connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.connected = false
	if c.api != nil {
		return c.api.Close()
	}
	return nil
}

// Ping checks if the Docker daemon is responsive.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.api.Ping(ctx)
	if err != nil {
		c.mu.Lock()
		c.connected = false
		c.mu.Unlock()
		return err
	}
	return nil
}

// ListContainers returns all running containers.
func (c *Client) ListContainers(ctx context.Context) ([]container.Summary, error) {
	return c.api.ContainerList(ctx, container.ListOptions{
		All: false, // Only running containers
	})
}

// ListContainersWithLabel returns running containers that have a specific label prefix.
func (c *Client) ListContainersWithLabel(ctx context.Context, labelPrefix string) ([]container.Summary, error) {
	filterArgs := filters.NewArgs()
	filterArgs.Add("status", "running")
	// Filter for containers that have any label starting with prefix
	// Docker's filter doesn't support prefix matching, so we filter all and check labels ourselves

	containers, err := c.api.ContainerList(ctx, container.ListOptions{
		All:     false,
		Filters: filterArgs,
	})
	if err != nil {
		return nil, err
	}

	// Filter containers that have at least one label with the prefix
	var result []container.Summary
	for _, ctr := range containers {
		for label := range ctr.Labels {
			if hasPrefix(label, labelPrefix) {
				result = append(result, ctr)
				break
			}
		}
	}

	return result, nil
}

// InspectContainer returns detailed information about a container.
func (c *Client) InspectContainer(ctx context.Context, containerID string) (container.InspectResponse, error) {
	return c.api.ContainerInspect(ctx, containerID)
}

// API returns the underlying DockerAPI for advanced operations.
func (c *Client) API() DockerAPI {
	return c.api
}

// WaitForConnection attempts to connect to Docker with retries.
func (c *Client) WaitForConnection(ctx context.Context, maxRetries int, retryInterval time.Duration) error {
	for i := 0; i < maxRetries; i++ {
		if err := c.Connect(ctx); err == nil {
			return nil
		}

		c.logger.Debug("waiting for Docker daemon", "attempt", i+1, "max", maxRetries)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryInterval):
			continue
		}
	}

	return fmt.Errorf("failed to connect to Docker after %d attempts", maxRetries)
}

// hasPrefix checks if a string has a given prefix.
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
