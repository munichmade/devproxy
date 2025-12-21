package daemon

import (
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestNewShutdownHandler(t *testing.T) {
	h := NewShutdownHandler()
	if h == nil {
		t.Fatal("NewShutdownHandler() returned nil")
	}
	if h.ctx == nil {
		t.Error("context should not be nil")
	}
	if h.cancel == nil {
		t.Error("cancel function should not be nil")
	}
	if h.sigChan == nil {
		t.Error("signal channel should not be nil")
	}
	if h.reloadChan == nil {
		t.Error("reload channel should not be nil")
	}
}

func TestShutdownHandler_Context(t *testing.T) {
	h := NewShutdownHandler()
	h.Start()
	defer h.Stop()

	ctx := h.Context()
	if ctx == nil {
		t.Fatal("Context() returned nil")
	}

	select {
	case <-ctx.Done():
		t.Error("context should not be cancelled initially")
	default:
		// Expected
	}
}

func TestShutdownHandler_Trigger(t *testing.T) {
	h := NewShutdownHandler()
	h.Start()
	defer h.Stop()

	// Trigger shutdown
	h.Trigger()

	// Context should be cancelled
	select {
	case <-h.Done():
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Error("context should be cancelled after Trigger()")
	}
}

func TestShutdownHandler_OnShutdown(t *testing.T) {
	h := NewShutdownHandler()
	h.Start()
	defer h.Stop()

	var called atomic.Bool
	h.OnShutdown(func() {
		called.Store(true)
	})

	h.Trigger()

	// Wait a bit for the callback
	time.Sleep(10 * time.Millisecond)

	if !called.Load() {
		t.Error("shutdown callback should have been called")
	}
}

func TestShutdownHandler_OnShutdown_LIFO(t *testing.T) {
	h := NewShutdownHandler()
	h.Start()
	defer h.Stop()

	var order []int
	h.OnShutdown(func() {
		order = append(order, 1)
	})
	h.OnShutdown(func() {
		order = append(order, 2)
	})
	h.OnShutdown(func() {
		order = append(order, 3)
	})

	h.Trigger()

	// Wait a bit for the callbacks
	time.Sleep(10 * time.Millisecond)

	if len(order) != 3 {
		t.Fatalf("expected 3 callbacks, got %d", len(order))
	}

	// Should be in reverse order (LIFO)
	if order[0] != 3 || order[1] != 2 || order[2] != 1 {
		t.Errorf("callbacks should run in LIFO order, got %v", order)
	}
}

func TestShutdownHandler_SIGTERM(t *testing.T) {
	h := NewShutdownHandler()
	h.Start()
	defer h.Stop()

	var called atomic.Bool
	h.OnShutdown(func() {
		called.Store(true)
	})

	// Send SIGTERM through the signal channel
	h.sigChan <- syscall.SIGTERM

	// Wait for shutdown
	select {
	case <-h.Done():
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Error("context should be cancelled after SIGTERM")
	}

	if !called.Load() {
		t.Error("shutdown callback should have been called on SIGTERM")
	}
}

func TestShutdownHandler_SIGINT(t *testing.T) {
	h := NewShutdownHandler()
	h.Start()
	defer h.Stop()

	var called atomic.Bool
	h.OnShutdown(func() {
		called.Store(true)
	})

	// Send SIGINT through the signal channel
	h.sigChan <- syscall.SIGINT

	// Wait for shutdown
	select {
	case <-h.Done():
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Error("context should be cancelled after SIGINT")
	}

	if !called.Load() {
		t.Error("shutdown callback should have been called on SIGINT")
	}
}

func TestShutdownHandler_SIGHUP(t *testing.T) {
	h := NewShutdownHandler()
	h.Start()
	defer h.Stop()

	// Send SIGHUP through the signal channel
	h.sigChan <- syscall.SIGHUP

	// Should receive on reload channel
	select {
	case <-h.ReloadChan():
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Error("should receive reload signal on SIGHUP")
	}

	// Context should NOT be cancelled
	select {
	case <-h.Done():
		t.Error("context should not be cancelled on SIGHUP")
	default:
		// Expected
	}
}

func TestShutdownHandler_ReloadChan_NoBlock(t *testing.T) {
	h := NewShutdownHandler()
	h.Start()
	defer h.Stop()

	// Send multiple SIGHUPs rapidly - should not block
	for i := 0; i < 5; i++ {
		h.sigChan <- syscall.SIGHUP
	}

	// Should receive at least one reload signal
	select {
	case <-h.ReloadChan():
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Error("should receive at least one reload signal")
	}
}

func TestShutdownHandler_Stop(t *testing.T) {
	h := NewShutdownHandler()
	h.Start()

	// Stop should complete without blocking
	done := make(chan struct{})
	go func() {
		h.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Error("Stop() should complete quickly")
	}
}
