//go:build !darwin

package service

import "net"

// ActivatedListener returns nil on non-Darwin platforms.
// Socket activation via launchd is only supported on macOS.
// Linux systemd socket activation can be added in the future.
func ActivatedListener(name string) (net.Listener, error) {
	return nil, nil
}

// IsSocketActivated returns false on non-Darwin platforms.
func IsSocketActivated() bool {
	return false
}
