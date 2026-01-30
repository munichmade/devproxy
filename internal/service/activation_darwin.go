//go:build darwin

package service

import (
	"net"
	"sync"
	"syscall"

	launchd "github.com/bored-engineer/go-launchd"
)

var (
	activatedListeners = make(map[string]net.Listener)
	activationMu       sync.Mutex
	activationChecked  = make(map[string]bool)
)

// ActivatedListener returns a listener from launchd socket activation.
// Returns nil, nil if not running under launchd or socket not available.
// Results are cached since launchd only allows one activation per socket name.
func ActivatedListener(name string) (net.Listener, error) {
	activationMu.Lock()
	defer activationMu.Unlock()

	// Return cached listener if already activated
	if l, ok := activatedListeners[name]; ok {
		return l, nil
	}

	// If we already checked this socket and it wasn't available, don't retry
	if activationChecked[name] {
		return nil, nil
	}
	activationChecked[name] = true

	// Try to activate from launchd
	listener, err := launchd.Activate(name)
	if err != nil {
		// ESRCH = not running under launchd
		// ENOENT = socket name not found in plist
		// These are expected when not running as a launchd service
		if err == syscall.ESRCH || err == syscall.ENOENT {
			return nil, nil
		}
		// EALREADY = already activated (shouldn't happen due to caching, but handle it)
		if err == syscall.EALREADY {
			return nil, nil
		}
		// Unexpected error
		return nil, err
	}

	activatedListeners[name] = listener
	return listener, nil
}

// IsSocketActivated returns true if the process was started via launchd
// socket activation and has activated listeners available.
func IsSocketActivated() bool {
	activationMu.Lock()
	defer activationMu.Unlock()
	return len(activatedListeners) > 0
}
