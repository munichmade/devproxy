// Package daemon provides daemon lifecycle management including
// PID file handling, process forking, and signal handling.
package daemon

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// ShutdownHandler manages graceful shutdown of the daemon.
type ShutdownHandler struct {
	ctx        context.Context
	cancel     context.CancelFunc
	sigChan    chan os.Signal
	reloadChan chan struct{}
	callbacks  []func()
	mu         sync.Mutex
	done       chan struct{}
}

// NewShutdownHandler creates a new shutdown handler that listens for
// SIGTERM, SIGINT (for graceful shutdown) and SIGHUP (for config reload).
func NewShutdownHandler() *ShutdownHandler {
	ctx, cancel := context.WithCancel(context.Background())
	return &ShutdownHandler{
		ctx:        ctx,
		cancel:     cancel,
		sigChan:    make(chan os.Signal, 1),
		reloadChan: make(chan struct{}, 1),
		done:       make(chan struct{}),
	}
}

// Start begins listening for signals. This should be called in a goroutine
// or before the main daemon loop.
func (h *ShutdownHandler) Start() {
	signal.Notify(h.sigChan, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	go func() {
		defer close(h.done)
		for {
			select {
			case sig := <-h.sigChan:
				switch sig {
				case syscall.SIGTERM, syscall.SIGINT:
					h.shutdown()
					return
				case syscall.SIGHUP:
					h.triggerReload()
				}
			case <-h.ctx.Done():
				return
			}
		}
	}()
}

// Stop stops the shutdown handler and cleans up resources.
func (h *ShutdownHandler) Stop() {
	signal.Stop(h.sigChan)
	h.cancel()
	<-h.done
}

// Context returns a context that is canceled when shutdown is triggered.
func (h *ShutdownHandler) Context() context.Context {
	return h.ctx
}

// Done returns a channel that is closed when shutdown is complete.
func (h *ShutdownHandler) Done() <-chan struct{} {
	return h.ctx.Done()
}

// ReloadChan returns a channel that receives when SIGHUP is received.
func (h *ShutdownHandler) ReloadChan() <-chan struct{} {
	return h.reloadChan
}

// OnShutdown registers a callback to be called during shutdown.
// Callbacks are called in reverse order of registration (LIFO).
func (h *ShutdownHandler) OnShutdown(fn func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.callbacks = append(h.callbacks, fn)
}

// shutdown triggers graceful shutdown and runs all registered callbacks.
func (h *ShutdownHandler) shutdown() {
	h.mu.Lock()
	callbacks := make([]func(), len(h.callbacks))
	copy(callbacks, h.callbacks)
	h.mu.Unlock()

	// Run callbacks in reverse order (LIFO)
	for i := len(callbacks) - 1; i >= 0; i-- {
		callbacks[i]()
	}

	h.cancel()
}

// triggerReload sends a reload signal to listeners.
func (h *ShutdownHandler) triggerReload() {
	select {
	case h.reloadChan <- struct{}{}:
	default:
		// Already a reload pending, ignore
	}
}

// Trigger manually triggers a shutdown (useful for testing or programmatic shutdown).
func (h *ShutdownHandler) Trigger() {
	h.shutdown()
}
