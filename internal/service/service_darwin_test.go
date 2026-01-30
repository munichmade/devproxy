//go:build darwin

package service

import (
	"net"
	"strings"
	"testing"
)

func TestGeneratePlist(t *testing.T) {
	binaryPath := "/usr/local/bin/devproxy"
	plist := generatePlist(binaryPath)

	t.Run("contains binary path", func(t *testing.T) {
		if !strings.Contains(plist, binaryPath) {
			t.Errorf("plist should contain binary path %s", binaryPath)
		}
	})

	t.Run("contains service label", func(t *testing.T) {
		if !strings.Contains(plist, "com.devproxy.daemon") {
			t.Error("plist should contain service label com.devproxy.daemon")
		}
	})

	t.Run("contains run argument", func(t *testing.T) {
		if !strings.Contains(plist, "<string>run</string>") {
			t.Error("plist should contain 'run' argument")
		}
	})

	t.Run("contains Sockets dictionary", func(t *testing.T) {
		if !strings.Contains(plist, "<key>Sockets</key>") {
			t.Error("plist should contain Sockets key for socket activation")
		}
	})

	t.Run("contains HTTPListener socket", func(t *testing.T) {
		if !strings.Contains(plist, "<key>HTTPListener</key>") {
			t.Error("plist should contain HTTPListener socket")
		}
		if !strings.Contains(plist, "<string>80</string>") {
			t.Error("plist should configure port 80 for HTTPListener")
		}
	})

	t.Run("contains HTTPSListener socket", func(t *testing.T) {
		if !strings.Contains(plist, "<key>HTTPSListener</key>") {
			t.Error("plist should contain HTTPSListener socket")
		}
		if !strings.Contains(plist, "<string>443</string>") {
			t.Error("plist should configure port 443 for HTTPSListener")
		}
	})

	t.Run("does not contain KeepAlive", func(t *testing.T) {
		if strings.Contains(plist, "<key>KeepAlive</key>") {
			t.Error("plist should not contain KeepAlive (using socket activation instead)")
		}
	})

	t.Run("contains RunAtLoad", func(t *testing.T) {
		if !strings.Contains(plist, "<key>RunAtLoad</key>") {
			t.Error("plist should contain RunAtLoad")
		}
	})

	t.Run("contains log paths", func(t *testing.T) {
		if !strings.Contains(plist, "/var/log/devproxy.log") {
			t.Error("plist should configure log path")
		}
	})
}

func TestActivatedListener_NotUnderLaunchd(t *testing.T) {
	// When not running under launchd, ActivatedListener should return nil
	// without error (graceful fallback)
	listener, err := ActivatedListener("HTTPListener")
	if err != nil {
		t.Errorf("ActivatedListener should not return error when not under launchd: %v", err)
	}
	if listener != nil {
		listener.Close()
		t.Error("ActivatedListener should return nil when not running under launchd")
	}
}

func TestActivatedListener_Caching(t *testing.T) {
	// Reset state for test
	activationMu.Lock()
	activatedListeners = make(map[string]net.Listener)
	activationChecked = make(map[string]bool)
	activationMu.Unlock()

	// First call should check launchd
	listener1, _ := ActivatedListener("TestSocket")

	// Second call should use cached result (nil in this case since not under launchd)
	listener2, _ := ActivatedListener("TestSocket")

	// Both should be nil (not under launchd)
	if listener1 != nil || listener2 != nil {
		t.Error("expected nil listeners when not under launchd")
	}

	// Verify cache was used
	activationMu.Lock()
	checked := activationChecked["TestSocket"]
	activationMu.Unlock()

	if !checked {
		t.Error("expected socket to be marked as checked after first call")
	}
}

func TestIsSocketActivated(t *testing.T) {
	// Reset state
	activationMu.Lock()
	activatedListeners = make(map[string]net.Listener)
	activationMu.Unlock()

	if IsSocketActivated() {
		t.Error("IsSocketActivated should return false when no listeners are activated")
	}
}
