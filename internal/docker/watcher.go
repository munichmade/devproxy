// Package docker provides Docker container discovery and event watching.
package docker

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
)

// ContainerEvent represents a container lifecycle event.
type ContainerEvent struct {
	// Type is the event type: "start", "stop", or "die"
	Type string
	// ContainerID is the container's ID
	ContainerID string
	// ContainerName is the container's name (without leading /)
	ContainerName string
	// Labels are the container's labels
	Labels map[string]string
}

// EventHandler is called when container events occur.
type EventHandler func(event ContainerEvent)

// Watcher watches for Docker container events.
type Watcher struct {
	client  *Client
	handler EventHandler
	logger  *slog.Logger

	mu        sync.Mutex
	running   bool
	stopCh    chan struct{}
	stoppedCh chan struct{}
}

// NewWatcher creates a new container event watcher.
func NewWatcher(client *Client, handler EventHandler, logger *slog.Logger) *Watcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &Watcher{
		client:  client,
		handler: handler,
		logger:  logger,
	}
}

// Start begins watching for container events.
// It first scans for existing containers, then watches the event stream.
func (w *Watcher) Start(ctx context.Context) error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return nil
	}
	w.running = true
	w.stopCh = make(chan struct{})
	w.stoppedCh = make(chan struct{})
	w.mu.Unlock()

	// Check if client is connected
	if w.client.APIClient() == nil {
		w.logger.Warn("docker client not connected, skipping container scan")
	} else {
		// Scan existing containers first
		if err := w.scanExistingContainers(ctx); err != nil {
			w.logger.Warn("failed to scan existing containers", "error", err)
		}
	}

	// Start event watching in background
	go w.watchEvents(ctx)

	return nil
}

// Stop stops watching for container events.
func (w *Watcher) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	w.running = false
	close(w.stopCh)
	w.mu.Unlock()

	// Wait for watcher to stop
	<-w.stoppedCh
}

// scanExistingContainers discovers already-running containers with devproxy labels.
func (w *Watcher) scanExistingContainers(ctx context.Context) error {
	enableLabel := LabelPrefix + ".enable"

	// List running containers with our enable label
	opts := container.ListOptions{
		Filters: filters.NewArgs(
			filters.Arg("status", "running"),
			filters.Arg("label", enableLabel+"=true"),
		),
	}

	containers, err := w.client.APIClient().ContainerList(ctx, opts)
	if err != nil {
		return err
	}

	w.logger.Info("scanning existing containers", "count", len(containers))

	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			// Remove leading / from container name
			name = c.Names[0]
			if len(name) > 0 && name[0] == '/' {
				name = name[1:]
			}
		}

		event := ContainerEvent{
			Type:          "start",
			ContainerID:   c.ID,
			ContainerName: name,
			Labels:        c.Labels,
		}

		w.logger.Info("calling handler for container",
			"container", name,
			"labels_count", len(c.Labels))
		w.handler(event)
		w.logger.Info("handler completed for container", "container", name)
	}

	return nil
}

// watchEvents watches the Docker event stream for container events.
func (w *Watcher) watchEvents(ctx context.Context) {
	defer close(w.stoppedCh)

	for {
		select {
		case <-w.stopCh:
			return
		default:
			w.watchEventStream(ctx)
		}

		// If we get here, the stream disconnected. Wait before reconnecting.
		select {
		case <-w.stopCh:
			return
		case <-time.After(time.Second):
			w.logger.Info("reconnecting to Docker event stream")
		}
	}
}

// watchEventStream subscribes to Docker events until disconnection or stop.
func (w *Watcher) watchEventStream(ctx context.Context) {
	enableLabel := LabelPrefix + ".enable"

	// Create filter for container events with our label
	opts := events.ListOptions{
		Filters: filters.NewArgs(
			filters.Arg("type", "container"),
			filters.Arg("event", "start"),
			filters.Arg("event", "stop"),
			filters.Arg("event", "die"),
		),
	}

	eventCh, errCh := w.client.APIClient().Events(ctx, opts)

	for {
		select {
		case <-w.stopCh:
			return

		case err := <-errCh:
			if err != nil {
				w.logger.Warn("Docker event stream error", "error", err)
			}
			return

		case event := <-eventCh:
			// Check if container has our enable label
			if event.Actor.Attributes[enableLabel] != "true" {
				continue
			}

			containerEvent := ContainerEvent{
				Type:          string(event.Action),
				ContainerID:   event.Actor.ID,
				ContainerName: event.Actor.Attributes["name"],
				Labels:        event.Actor.Attributes,
			}

			w.logger.Debug("container event",
				"type", containerEvent.Type,
				"container", containerEvent.ContainerName,
			)

			w.handler(containerEvent)
		}
	}
}

// IsRunning returns true if the watcher is currently running.
func (w *Watcher) IsRunning() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running
}
