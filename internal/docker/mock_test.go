package docker

import (
	"context"
	"errors"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/network"
)

// mockDockerAPI is a test double for DockerAPI that allows configuring
// behavior per-test via function fields.
type mockDockerAPI struct {
	pingFunc             func(ctx context.Context) (types.Ping, error)
	containerListFunc    func(ctx context.Context, options container.ListOptions) ([]container.Summary, error)
	containerInspectFunc func(ctx context.Context, containerID string) (container.InspectResponse, error)
	eventsFunc           func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error)
	closeFunc            func() error
}

func (m *mockDockerAPI) Ping(ctx context.Context) (types.Ping, error) {
	if m.pingFunc != nil {
		return m.pingFunc(ctx)
	}
	return types.Ping{}, nil
}

func (m *mockDockerAPI) ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
	if m.containerListFunc != nil {
		return m.containerListFunc(ctx, options)
	}
	return nil, nil
}

func (m *mockDockerAPI) ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error) {
	if m.containerInspectFunc != nil {
		return m.containerInspectFunc(ctx, containerID)
	}
	return container.InspectResponse{}, nil
}

func (m *mockDockerAPI) Events(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
	if m.eventsFunc != nil {
		return m.eventsFunc(ctx, options)
	}
	// Return closed channels by default
	eventCh := make(chan events.Message)
	errCh := make(chan error)
	close(eventCh)
	close(errCh)
	return eventCh, errCh
}

func (m *mockDockerAPI) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

// newMockDockerAPI creates a new mock with default no-op implementations.
func newMockDockerAPI() *mockDockerAPI {
	return &mockDockerAPI{}
}

// mockDockerAPIBuilder provides a fluent API for building mock configurations.
type mockDockerAPIBuilder struct {
	mock *mockDockerAPI
}

func newMockBuilder() *mockDockerAPIBuilder {
	return &mockDockerAPIBuilder{mock: &mockDockerAPI{}}
}

func (b *mockDockerAPIBuilder) withPing(fn func(ctx context.Context) (types.Ping, error)) *mockDockerAPIBuilder {
	b.mock.pingFunc = fn
	return b
}

func (b *mockDockerAPIBuilder) withPingError(err error) *mockDockerAPIBuilder {
	b.mock.pingFunc = func(ctx context.Context) (types.Ping, error) {
		return types.Ping{}, err
	}
	return b
}

func (b *mockDockerAPIBuilder) withPingSuccess() *mockDockerAPIBuilder {
	b.mock.pingFunc = func(ctx context.Context) (types.Ping, error) {
		return types.Ping{APIVersion: "1.41"}, nil
	}
	return b
}

func (b *mockDockerAPIBuilder) withContainerListResult(containers []container.Summary) *mockDockerAPIBuilder {
	b.mock.containerListFunc = func(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
		return containers, nil
	}
	return b
}

func (b *mockDockerAPIBuilder) withContainerListError(err error) *mockDockerAPIBuilder {
	b.mock.containerListFunc = func(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
		return nil, err
	}
	return b
}

func (b *mockDockerAPIBuilder) withContainerInspect(fn func(ctx context.Context, containerID string) (container.InspectResponse, error)) *mockDockerAPIBuilder {
	b.mock.containerInspectFunc = fn
	return b
}

func (b *mockDockerAPIBuilder) withContainerInspectResult(response container.InspectResponse) *mockDockerAPIBuilder {
	b.mock.containerInspectFunc = func(ctx context.Context, containerID string) (container.InspectResponse, error) {
		return response, nil
	}
	return b
}

func (b *mockDockerAPIBuilder) withContainerInspectError(err error) *mockDockerAPIBuilder {
	b.mock.containerInspectFunc = func(ctx context.Context, containerID string) (container.InspectResponse, error) {
		return container.InspectResponse{}, err
	}
	return b
}

func (b *mockDockerAPIBuilder) withEvents(fn func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error)) *mockDockerAPIBuilder {
	b.mock.eventsFunc = fn
	return b
}

func (b *mockDockerAPIBuilder) build() *mockDockerAPI {
	return b.mock
}

// Helper function to create a container summary for tests.
func makeContainerSummary(id, name string, labels map[string]string) container.Summary {
	names := []string{}
	if name != "" {
		names = append(names, "/"+name)
	}
	return container.Summary{
		ID:     id,
		Names:  names,
		Labels: labels,
	}
}

// Helper function to create a container inspect response for tests.
func makeContainerInspectResponse(id, name, ip, networkName string) container.InspectResponse {
	return container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{
			ID:   id,
			Name: "/" + name,
		},
		NetworkSettings: &container.NetworkSettings{
			Networks: map[string]*network.EndpointSettings{
				networkName: {
					IPAddress: ip,
				},
			},
		},
	}
}

// Common test errors.
var (
	errMockConnection = errors.New("mock: connection failed")
	errMockNotFound   = errors.New("mock: container not found")
)
