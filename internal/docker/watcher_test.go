package docker

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
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
