// Package docker provides Docker integration for automatic container discovery.
package docker

import (
	"context"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
)

// DockerAPI defines the Docker client operations used by devproxy.
// This interface enables testing without a real Docker daemon.
type DockerAPI interface {
	// Ping checks if the Docker daemon is responsive.
	Ping(ctx context.Context) (types.Ping, error)

	// ContainerList returns a list of containers matching the options.
	ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error)

	// ContainerInspect returns detailed information about a container.
	ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error)

	// Events returns a stream of Docker events.
	Events(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error)

	// Close closes the connection to the Docker daemon.
	Close() error
}
