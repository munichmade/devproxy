package docker

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
)

func TestNewWatcher(t *testing.T) {
	t.Run("creates watcher with all parameters", func(t *testing.T) {
		client := &Client{}
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		handler := func(event ContainerEvent) {}

		watcher := NewWatcher(client, handler, logger)

		if watcher == nil {
			t.Fatal("expected watcher to be created")
		}
		if watcher.client != client {
			t.Error("expected watcher to have the provided client")
		}
	})

	t.Run("uses default logger when nil", func(t *testing.T) {
		client := &Client{}
		handler := func(event ContainerEvent) {}

		watcher := NewWatcher(client, handler, nil)

		if watcher.logger == nil {
			t.Error("expected default logger to be set")
		}
	})
}

func TestWatcher_Stop(t *testing.T) {
	t.Run("stops without starting", func(t *testing.T) {
		client := &Client{}
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		handler := func(event ContainerEvent) {}

		watcher := NewWatcher(client, handler, logger)

		// Should not panic
		watcher.Stop()
	})
}

func TestWatcher_IsRunning(t *testing.T) {
	t.Run("returns false when not started", func(t *testing.T) {
		client := &Client{}
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		handler := func(event ContainerEvent) {}

		watcher := NewWatcher(client, handler, logger)

		if watcher.IsRunning() {
			t.Error("expected IsRunning to be false")
		}
	})
}

func TestContainerEvent(t *testing.T) {
	t.Run("has correct fields", func(t *testing.T) {
		event := ContainerEvent{
			Type:          "start",
			ContainerID:   "abc123",
			ContainerName: "test-container",
			Labels: map[string]string{
				"devproxy.host": "test.localhost",
			},
		}

		if event.Type != "start" {
			t.Errorf("expected 'start', got %v", event.Type)
		}
		if event.ContainerID != "abc123" {
			t.Errorf("expected abc123, got %s", event.ContainerID)
		}
		if event.ContainerName != "test-container" {
			t.Errorf("expected test-container, got %s", event.ContainerName)
		}
		if event.Labels["devproxy.host"] != "test.localhost" {
			t.Errorf("expected test.localhost, got %s", event.Labels["devproxy.host"])
		}
	})
}

func TestWatcher_StartRequiresConnection(t *testing.T) {
	t.Run("fails without docker connection", func(t *testing.T) {
		client := &Client{} // Not connected
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		eventReceived := make(chan ContainerEvent, 1)
		handler := func(event ContainerEvent) {
			eventReceived <- event
		}

		watcher := NewWatcher(client, handler, logger)

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		// Start will try to scan existing containers, which will fail
		// because the client is not connected
		_ = watcher.Start(ctx)

		// Should be able to stop cleanly
		watcher.Stop()
	})
}

func TestWatcher_HandlerCallback(t *testing.T) {
	t.Run("handler is stored", func(t *testing.T) {
		client := &Client{}
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		handler := func(event ContainerEvent) {}

		watcher := NewWatcher(client, handler, logger)

		// Verify handler is set by checking it's not nil
		if watcher.handler == nil {
			t.Error("expected handler to be set")
		}
	})
}

func TestWatcher_scanExistingContainers(t *testing.T) {
	t.Run("scans and calls handler for each container", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		// Create mock with containers
		mockAPI := newMockBuilder().
			withContainerListResult([]container.Summary{
				makeContainerSummary("container1", "web-app", map[string]string{
					"devproxy.enable": "true",
					"devproxy.host":   "app.localhost",
				}),
				makeContainerSummary("container2", "api-service", map[string]string{
					"devproxy.enable": "true",
					"devproxy.host":   "api.localhost",
				}),
			}).
			build()

		client := NewClientWithAPI(mockAPI, logger)

		var mu sync.Mutex
		var receivedEvents []ContainerEvent
		handler := func(event ContainerEvent) {
			mu.Lock()
			receivedEvents = append(receivedEvents, event)
			mu.Unlock()
		}

		watcher := NewWatcher(client, handler, logger)

		ctx := context.Background()
		err := watcher.scanExistingContainers(ctx)
		if err != nil {
			t.Fatalf("scanExistingContainers failed: %v", err)
		}

		mu.Lock()
		defer mu.Unlock()

		if len(receivedEvents) != 2 {
			t.Errorf("expected 2 events, got %d", len(receivedEvents))
		}

		// Verify first event
		if receivedEvents[0].Type != "start" {
			t.Errorf("expected event type 'start', got '%s'", receivedEvents[0].Type)
		}
		if receivedEvents[0].ContainerID != "container1" {
			t.Errorf("expected container ID 'container1', got '%s'", receivedEvents[0].ContainerID)
		}
		if receivedEvents[0].ContainerName != "web-app" {
			t.Errorf("expected container name 'web-app', got '%s'", receivedEvents[0].ContainerName)
		}
	})

	t.Run("strips leading slash from container names", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		mockAPI := newMockBuilder().
			withContainerListResult([]container.Summary{
				{
					ID:    "container1",
					Names: []string{"/my-container"},
					Labels: map[string]string{
						"devproxy.enable": "true",
					},
				},
			}).
			build()

		client := NewClientWithAPI(mockAPI, logger)

		var receivedEvent ContainerEvent
		handler := func(event ContainerEvent) {
			receivedEvent = event
		}

		watcher := NewWatcher(client, handler, logger)

		ctx := context.Background()
		_ = watcher.scanExistingContainers(ctx)

		if receivedEvent.ContainerName != "my-container" {
			t.Errorf("expected name without slash 'my-container', got '%s'", receivedEvent.ContainerName)
		}
	})

	t.Run("handles list error gracefully", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		mockAPI := newMockBuilder().
			withContainerListError(errMockConnection).
			build()

		client := NewClientWithAPI(mockAPI, logger)

		handlerCalled := false
		handler := func(event ContainerEvent) {
			handlerCalled = true
		}

		watcher := NewWatcher(client, handler, logger)

		ctx := context.Background()
		err := watcher.scanExistingContainers(ctx)

		if err == nil {
			t.Error("expected error from scanExistingContainers")
		}

		if handlerCalled {
			t.Error("handler should not be called when list fails")
		}
	})

	t.Run("handles empty container list", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		mockAPI := newMockBuilder().
			withContainerListResult([]container.Summary{}).
			build()

		client := NewClientWithAPI(mockAPI, logger)

		handlerCalled := false
		handler := func(event ContainerEvent) {
			handlerCalled = true
		}

		watcher := NewWatcher(client, handler, logger)

		ctx := context.Background()
		err := watcher.scanExistingContainers(ctx)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if handlerCalled {
			t.Error("handler should not be called for empty list")
		}
	})
}

func TestWatcher_watchEventStream(t *testing.T) {
	t.Run("processes start events", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		eventCh := make(chan events.Message, 1)
		errCh := make(chan error, 1)

		mockAPI := newMockBuilder().
			withEvents(func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
				return eventCh, errCh
			}).
			build()

		client := NewClientWithAPI(mockAPI, logger)

		receivedCh := make(chan ContainerEvent, 1)
		handler := func(event ContainerEvent) {
			receivedCh <- event
		}

		watcher := NewWatcher(client, handler, logger)
		watcher.mu.Lock()
		watcher.running = true
		watcher.stopCh = make(chan struct{})
		watcher.stoppedCh = make(chan struct{})
		watcher.mu.Unlock()

		// Start watching in background
		done := make(chan struct{})
		go func() {
			watcher.watchEventStream(context.Background())
			close(done)
		}()

		// Send a start event
		eventCh <- events.Message{
			Action: events.ActionStart,
			Actor: events.Actor{
				ID: "container123",
				Attributes: map[string]string{
					"devproxy.enable": "true",
					"name":            "test-container",
				},
			},
		}

		// Wait for handler to receive event
		select {
		case event := <-receivedCh:
			if event.Type != "start" {
				t.Errorf("expected type 'start', got '%s'", event.Type)
			}
			if event.ContainerID != "container123" {
				t.Errorf("expected ID 'container123', got '%s'", event.ContainerID)
			}
			if event.ContainerName != "test-container" {
				t.Errorf("expected name 'test-container', got '%s'", event.ContainerName)
			}
		case <-time.After(time.Second):
			t.Error("timeout waiting for event")
		}

		// Stop the watcher by closing stopCh and wait for watchEventStream to exit
		close(watcher.stopCh)
		select {
		case <-done:
			// Success
		case <-time.After(time.Second):
			t.Error("watchEventStream did not exit")
		}
	})

	t.Run("processes stop events", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		eventCh := make(chan events.Message, 1)
		errCh := make(chan error, 1)

		mockAPI := newMockBuilder().
			withEvents(func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
				return eventCh, errCh
			}).
			build()

		client := NewClientWithAPI(mockAPI, logger)

		receivedCh := make(chan ContainerEvent, 1)
		handler := func(event ContainerEvent) {
			receivedCh <- event
		}

		watcher := NewWatcher(client, handler, logger)
		watcher.mu.Lock()
		watcher.running = true
		watcher.stopCh = make(chan struct{})
		watcher.stoppedCh = make(chan struct{})
		watcher.mu.Unlock()

		done := make(chan struct{})
		go func() {
			watcher.watchEventStream(context.Background())
			close(done)
		}()

		// Send a stop event
		eventCh <- events.Message{
			Action: events.ActionStop,
			Actor: events.Actor{
				ID: "container456",
				Attributes: map[string]string{
					"devproxy.enable": "true",
					"name":            "stopped-container",
				},
			},
		}

		select {
		case event := <-receivedCh:
			if event.Type != "stop" {
				t.Errorf("expected type 'stop', got '%s'", event.Type)
			}
		case <-time.After(time.Second):
			t.Error("timeout waiting for event")
		}

		close(watcher.stopCh)
		select {
		case <-done:
			// Success
		case <-time.After(time.Second):
			t.Error("watchEventStream did not exit")
		}
	})

	t.Run("filters events without devproxy enable label", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		eventCh := make(chan events.Message, 2)
		errCh := make(chan error, 1)

		mockAPI := newMockBuilder().
			withEvents(func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
				return eventCh, errCh
			}).
			build()

		client := NewClientWithAPI(mockAPI, logger)

		receivedCh := make(chan ContainerEvent, 2)
		handler := func(event ContainerEvent) {
			receivedCh <- event
		}

		watcher := NewWatcher(client, handler, logger)
		watcher.mu.Lock()
		watcher.running = true
		watcher.stopCh = make(chan struct{})
		watcher.stoppedCh = make(chan struct{})
		watcher.mu.Unlock()

		done := make(chan struct{})
		go func() {
			watcher.watchEventStream(context.Background())
			close(done)
		}()

		// Send event without devproxy label (should be filtered)
		eventCh <- events.Message{
			Action: events.ActionStart,
			Actor: events.Actor{
				ID: "filtered-container",
				Attributes: map[string]string{
					"name": "no-devproxy",
				},
			},
		}

		// Send event with devproxy label (should pass)
		eventCh <- events.Message{
			Action: events.ActionStart,
			Actor: events.Actor{
				ID: "devproxy-container",
				Attributes: map[string]string{
					"devproxy.enable": "true",
					"name":            "with-devproxy",
				},
			},
		}

		// Should only receive the devproxy event
		select {
		case event := <-receivedCh:
			if event.ContainerID != "devproxy-container" {
				t.Errorf("expected devproxy-container, got %s", event.ContainerID)
			}
		case <-time.After(500 * time.Millisecond):
			t.Error("timeout waiting for event")
		}

		// Verify no more events (the filtered one)
		select {
		case event := <-receivedCh:
			t.Errorf("unexpected event received: %+v", event)
		case <-time.After(100 * time.Millisecond):
			// Expected - no more events
		}

		close(watcher.stopCh)
		select {
		case <-done:
			// Success
		case <-time.After(time.Second):
			t.Error("watchEventStream did not exit")
		}
	})

	t.Run("exits on error channel", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		eventCh := make(chan events.Message)
		errCh := make(chan error, 1)

		mockAPI := newMockBuilder().
			withEvents(func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
				return eventCh, errCh
			}).
			build()

		client := NewClientWithAPI(mockAPI, logger)

		handler := func(event ContainerEvent) {}

		watcher := NewWatcher(client, handler, logger)
		watcher.mu.Lock()
		watcher.running = true
		watcher.stopCh = make(chan struct{})
		watcher.stoppedCh = make(chan struct{})
		watcher.mu.Unlock()

		done := make(chan struct{})
		go func() {
			watcher.watchEventStream(context.Background())
			close(done)
		}()

		// Send error to cause exit
		errCh <- errMockConnection

		select {
		case <-done:
			// Success - watchEventStream returned
		case <-time.After(time.Second):
			t.Error("watchEventStream did not exit on error")
			close(watcher.stopCh)
		}
	})

	t.Run("exits on stop signal", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		eventCh := make(chan events.Message)
		errCh := make(chan error)

		mockAPI := newMockBuilder().
			withEvents(func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
				return eventCh, errCh
			}).
			build()

		client := NewClientWithAPI(mockAPI, logger)

		handler := func(event ContainerEvent) {}

		watcher := NewWatcher(client, handler, logger)
		watcher.mu.Lock()
		watcher.running = true
		watcher.stopCh = make(chan struct{})
		watcher.stoppedCh = make(chan struct{})
		watcher.mu.Unlock()

		done := make(chan struct{})
		go func() {
			watcher.watchEventStream(context.Background())
			close(done)
		}()

		// Close stop channel to signal stop
		close(watcher.stopCh)

		select {
		case <-done:
			// Success
		case <-time.After(time.Second):
			t.Error("watchEventStream did not exit on stop signal")
		}
	})
}

func TestWatcher_Start_Idempotent(t *testing.T) {
	t.Run("second start is no-op when already running", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		mockAPI := newMockBuilder().
			withContainerListResult([]container.Summary{}).
			withEvents(func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
				eventCh := make(chan events.Message)
				errCh := make(chan error)
				return eventCh, errCh
			}).
			build()

		client := NewClientWithAPI(mockAPI, logger)

		handler := func(event ContainerEvent) {}

		watcher := NewWatcher(client, handler, logger)

		ctx := context.Background()

		// First start
		err1 := watcher.Start(ctx)
		if err1 != nil {
			t.Fatalf("first Start failed: %v", err1)
		}

		if !watcher.IsRunning() {
			t.Error("expected IsRunning to be true after first Start")
		}

		// Second start should be no-op
		err2 := watcher.Start(ctx)
		if err2 != nil {
			t.Fatalf("second Start failed: %v", err2)
		}

		// Should still be running
		if !watcher.IsRunning() {
			t.Error("expected IsRunning to still be true after second Start")
		}

		// Clean up
		watcher.Stop()

		if watcher.IsRunning() {
			t.Error("expected IsRunning to be false after Stop")
		}
	})
}

func TestWatcher_StartAndStop(t *testing.T) {
	t.Run("starts and stops cleanly with mock", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		mockAPI := newMockBuilder().
			withContainerListResult([]container.Summary{}).
			withEvents(func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
				eventCh := make(chan events.Message)
				errCh := make(chan error)
				return eventCh, errCh
			}).
			build()

		client := NewClientWithAPI(mockAPI, logger)

		handler := func(event ContainerEvent) {}

		watcher := NewWatcher(client, handler, logger)

		ctx := context.Background()

		err := watcher.Start(ctx)
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		if !watcher.IsRunning() {
			t.Error("expected IsRunning to be true")
		}

		// Give the goroutine time to start
		time.Sleep(10 * time.Millisecond)

		watcher.Stop()

		if watcher.IsRunning() {
			t.Error("expected IsRunning to be false after Stop")
		}
	})
}
