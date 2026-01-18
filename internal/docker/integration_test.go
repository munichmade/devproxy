//go:build integration

package docker

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"

	"github.com/munichmade/devproxy/internal/proxy"
)

const (
	testImage         = "alpine:latest"
	testContainerName = "devproxy-integration-test"
	testNetwork       = "bridge"
)

// testHelper manages Docker resources for integration tests.
type testHelper struct {
	t      *testing.T
	client *client.Client
	logger *slog.Logger
}

func newTestHelper(t *testing.T) *testHelper {
	t.Helper()

	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = cli.Ping(ctx)
	if err != nil {
		t.Skipf("Docker daemon not responding: %v", err)
	}

	return &testHelper{
		t:      t,
		client: cli,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func (h *testHelper) close() {
	h.client.Close()
}

func (h *testHelper) pullImage(ctx context.Context) {
	h.t.Helper()

	reader, err := h.client.ImagePull(ctx, testImage, image.PullOptions{})
	if err != nil {
		h.t.Fatalf("Failed to pull image: %v", err)
	}
	defer reader.Close()
	// Drain the reader to complete the pull
	_, _ = io.Copy(io.Discard, reader)
}

func (h *testHelper) createContainer(ctx context.Context, name string, labels map[string]string) string {
	h.t.Helper()

	resp, err := h.client.ContainerCreate(ctx,
		&container.Config{
			Image:  testImage,
			Cmd:    []string{"sleep", "300"},
			Labels: labels,
		},
		&container.HostConfig{},
		&network.NetworkingConfig{},
		nil,
		name,
	)
	if err != nil {
		h.t.Fatalf("Failed to create container: %v", err)
	}

	h.t.Cleanup(func() {
		h.removeContainer(context.Background(), resp.ID)
	})

	return resp.ID
}

func (h *testHelper) startContainer(ctx context.Context, containerID string) {
	h.t.Helper()

	err := h.client.ContainerStart(ctx, containerID, container.StartOptions{})
	if err != nil {
		h.t.Fatalf("Failed to start container: %v", err)
	}
}

func (h *testHelper) stopContainer(ctx context.Context, containerID string) {
	h.t.Helper()

	timeout := 5
	err := h.client.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout})
	if err != nil {
		h.t.Logf("Warning: failed to stop container: %v", err)
	}
}

func (h *testHelper) removeContainer(ctx context.Context, containerID string) {
	h.t.Helper()

	err := h.client.ContainerRemove(ctx, containerID, container.RemoveOptions{
		Force: true,
	})
	if err != nil {
		h.t.Logf("Warning: failed to remove container: %v", err)
	}
}

func TestIntegration_ClientConnect(t *testing.T) {
	helper := newTestHelper(t)
	defer helper.close()

	devproxyClient, err := NewClient(helper.logger)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer devproxyClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = devproxyClient.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	if !devproxyClient.IsConnected() {
		t.Error("expected IsConnected to be true")
	}
}

func TestIntegration_ClientListContainers(t *testing.T) {
	helper := newTestHelper(t)
	defer helper.close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Ensure image is available
	helper.pullImage(ctx)

	// Create and start a container
	containerID := helper.createContainer(ctx, testContainerName+"-list", map[string]string{
		"devproxy.enable": "true",
		"devproxy.host":   "list-test.localhost",
	})
	helper.startContainer(ctx, containerID)

	// Use devproxy client to list containers
	devproxyClient, err := NewClient(helper.logger)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer devproxyClient.Close()

	containers, err := devproxyClient.ListContainers(ctx)
	if err != nil {
		t.Fatalf("ListContainers failed: %v", err)
	}

	// Should have at least our test container
	found := false
	for _, c := range containers {
		if c.ID == containerID {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected to find test container in list")
	}
}

func TestIntegration_ClientListContainersWithLabel(t *testing.T) {
	helper := newTestHelper(t)
	defer helper.close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	helper.pullImage(ctx)

	// Create container with devproxy labels
	containerID := helper.createContainer(ctx, testContainerName+"-label", map[string]string{
		"devproxy.enable": "true",
		"devproxy.host":   "label-test.localhost",
		"devproxy.port":   "8080",
	})
	helper.startContainer(ctx, containerID)

	devproxyClient, err := NewClient(helper.logger)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer devproxyClient.Close()

	containers, err := devproxyClient.ListContainersWithLabel(ctx, "devproxy.")
	if err != nil {
		t.Fatalf("ListContainersWithLabel failed: %v", err)
	}

	found := false
	for _, c := range containers {
		if c.ID == containerID {
			found = true
			// Verify labels are present
			if c.Labels["devproxy.host"] != "label-test.localhost" {
				t.Errorf("expected host label 'label-test.localhost', got '%s'", c.Labels["devproxy.host"])
			}
			break
		}
	}

	if !found {
		t.Error("expected to find labeled container")
	}
}

func TestIntegration_WatcherScansExisting(t *testing.T) {
	helper := newTestHelper(t)
	defer helper.close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	helper.pullImage(ctx)

	// Create and start container BEFORE starting watcher
	containerID := helper.createContainer(ctx, testContainerName+"-scan", map[string]string{
		"devproxy.enable": "true",
		"devproxy.host":   "scan-test.localhost",
		"devproxy.port":   "8080",
	})
	helper.startContainer(ctx, containerID)

	// Create devproxy client and watcher
	devproxyClient, err := NewClient(helper.logger)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer devproxyClient.Close()

	if err := devproxyClient.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	receivedEvents := make(chan ContainerEvent, 10)
	handler := func(event ContainerEvent) {
		receivedEvents <- event
	}

	watcher := NewWatcher(devproxyClient, handler, helper.logger)

	err = watcher.Start(ctx)
	if err != nil {
		t.Fatalf("Watcher.Start failed: %v", err)
	}
	defer watcher.Stop()

	// Wait for scan to complete and event to be received
	select {
	case event := <-receivedEvents:
		if event.Type != "start" {
			t.Errorf("expected start event, got %s", event.Type)
		}
		if event.ContainerID != containerID {
			t.Errorf("expected container ID %s, got %s", containerID, event.ContainerID)
		}
		if event.Labels["devproxy.host"] != "scan-test.localhost" {
			t.Errorf("expected host label 'scan-test.localhost', got '%s'", event.Labels["devproxy.host"])
		}
	case <-time.After(5 * time.Second):
		t.Error("timeout waiting for scan event")
	}
}

func TestIntegration_WatcherReceivesStartEvent(t *testing.T) {
	helper := newTestHelper(t)
	defer helper.close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	helper.pullImage(ctx)

	// Create devproxy client and watcher FIRST
	devproxyClient, err := NewClient(helper.logger)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer devproxyClient.Close()

	if err := devproxyClient.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	receivedEvents := make(chan ContainerEvent, 10)
	handler := func(event ContainerEvent) {
		receivedEvents <- event
	}

	watcher := NewWatcher(devproxyClient, handler, helper.logger)

	err = watcher.Start(ctx)
	if err != nil {
		t.Fatalf("Watcher.Start failed: %v", err)
	}
	defer watcher.Stop()

	// Give watcher time to start watching events
	time.Sleep(100 * time.Millisecond)

	// Now create and start container
	containerID := helper.createContainer(ctx, testContainerName+"-start-event", map[string]string{
		"devproxy.enable": "true",
		"devproxy.host":   "start-event.localhost",
		"devproxy.port":   "3000",
	})
	helper.startContainer(ctx, containerID)

	// Wait for start event
	select {
	case event := <-receivedEvents:
		if event.Type != "start" {
			t.Errorf("expected start event, got %s", event.Type)
		}
		if event.ContainerID != containerID {
			t.Errorf("expected container ID %s, got %s", containerID, event.ContainerID)
		}
	case <-time.After(10 * time.Second):
		t.Error("timeout waiting for start event")
	}
}

func TestIntegration_WatcherReceivesStopEvent(t *testing.T) {
	helper := newTestHelper(t)
	defer helper.close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	helper.pullImage(ctx)

	devproxyClient, err := NewClient(helper.logger)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer devproxyClient.Close()

	if err := devproxyClient.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	receivedEvents := make(chan ContainerEvent, 10)
	handler := func(event ContainerEvent) {
		receivedEvents <- event
	}

	watcher := NewWatcher(devproxyClient, handler, helper.logger)

	err = watcher.Start(ctx)
	if err != nil {
		t.Fatalf("Watcher.Start failed: %v", err)
	}
	defer watcher.Stop()

	time.Sleep(100 * time.Millisecond)

	// Create, start, then stop container
	containerID := helper.createContainer(ctx, testContainerName+"-stop-event", map[string]string{
		"devproxy.enable": "true",
		"devproxy.host":   "stop-event.localhost",
	})
	helper.startContainer(ctx, containerID)

	// Wait for start event first
	select {
	case <-receivedEvents:
		// Got start event
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for start event")
	}

	// Now stop the container
	helper.stopContainer(ctx, containerID)

	// Wait for stop event
	select {
	case event := <-receivedEvents:
		if event.Type != "stop" && event.Type != "die" {
			t.Errorf("expected stop or die event, got %s", event.Type)
		}
		if event.ContainerID != containerID {
			t.Errorf("expected container ID %s, got %s", containerID, event.ContainerID)
		}
	case <-time.After(10 * time.Second):
		t.Error("timeout waiting for stop event")
	}
}

func TestIntegration_RouteSyncFullFlow(t *testing.T) {
	helper := newTestHelper(t)
	defer helper.close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	helper.pullImage(ctx)

	devproxyClient, err := NewClient(helper.logger)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer devproxyClient.Close()

	if err := devproxyClient.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// Create registry and route sync
	registry := proxy.NewRegistry()
	routeSync := NewRouteSync(registry, devproxyClient, testNetwork, helper.logger)

	// Create watcher with routeSync as handler
	watcher := NewWatcher(devproxyClient, routeSync.HandleEvent, helper.logger)

	err = watcher.Start(ctx)
	if err != nil {
		t.Fatalf("Watcher.Start failed: %v", err)
	}
	defer watcher.Stop()

	time.Sleep(100 * time.Millisecond)

	// Start container
	uniqueHost := fmt.Sprintf("fullflow-%d.localhost", time.Now().UnixNano())
	containerID := helper.createContainer(ctx, testContainerName+"-fullflow", map[string]string{
		"devproxy.enable": "true",
		"devproxy.host":   uniqueHost,
		"devproxy.port":   "8080",
	})
	helper.startContainer(ctx, containerID)

	// Wait for route to be added
	var route *proxy.Route
	for i := 0; i < 50; i++ {
		route = registry.Lookup(uniqueHost)
		if route != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if route == nil {
		t.Fatalf("expected route for %s to be added", uniqueHost)
	}

	if route.ContainerID != containerID {
		t.Errorf("expected route container ID %s, got %s", containerID, route.ContainerID)
	}

	// Backend should have container IP
	if route.Backend == "" {
		t.Error("expected route backend to be set")
	}

	// Stop container
	helper.stopContainer(ctx, containerID)

	// Wait for route to be removed
	for i := 0; i < 50; i++ {
		route = registry.Lookup(uniqueHost)
		if route == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if route != nil {
		t.Error("expected route to be removed after container stop")
	}
}
