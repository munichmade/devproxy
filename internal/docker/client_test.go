package docker

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNewClient(t *testing.T) {
	t.Run("creates client with default options", func(t *testing.T) {
		client, err := NewClient(testLogger())
		if err != nil {
			t.Skipf("Docker not available: %v", err)
		}
		defer client.Close()

		if client.cli == nil {
			t.Error("expected docker client to be initialized")
		}
	})

	t.Run("creates client with custom host", func(t *testing.T) {
		client, err := NewClientWithHost("unix:///var/run/docker.sock", testLogger())
		if err != nil {
			t.Skipf("Docker not available: %v", err)
		}
		defer client.Close()

		if client.cli == nil {
			t.Error("expected docker client to be initialized")
		}
	})
}

func TestClient_Ping(t *testing.T) {
	client, err := NewClient(testLogger())
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = client.Ping(ctx)
	if err != nil {
		t.Skipf("Docker daemon not responding: %v", err)
	}
}

func TestClient_Connect(t *testing.T) {
	t.Run("connects successfully", func(t *testing.T) {
		client, err := NewClient(testLogger())
		if err != nil {
			t.Skipf("Docker not available: %v", err)
		}
		defer client.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = client.Connect(ctx)
		if err != nil {
			t.Skipf("Docker daemon not responding: %v", err)
		}

		if !client.IsConnected() {
			t.Error("expected IsConnected to return true after Connect")
		}
	})
}

func TestClient_IsConnected(t *testing.T) {
	t.Run("returns true when connected", func(t *testing.T) {
		client, err := NewClient(testLogger())
		if err != nil {
			t.Skipf("Docker not available: %v", err)
		}
		defer client.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := client.Connect(ctx); err != nil {
			t.Skipf("Docker daemon not responding: %v", err)
		}

		if !client.IsConnected() {
			t.Error("expected IsConnected to return true after successful Connect")
		}
	})

	t.Run("returns false before connect", func(t *testing.T) {
		client, err := NewClient(testLogger())
		if err != nil {
			t.Skipf("Docker not available: %v", err)
		}
		defer client.Close()

		if client.IsConnected() {
			t.Error("expected IsConnected to return false before Connect")
		}
	})
}

func TestClient_Close(t *testing.T) {
	client, err := NewClient(testLogger())
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}

	err = client.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	if client.IsConnected() {
		t.Error("expected IsConnected to return false after Close")
	}
}

func TestHasPrefix(t *testing.T) {
	tests := []struct {
		s      string
		prefix string
		want   bool
	}{
		{"devproxy.host", "devproxy.", true},
		{"devproxy.port", "devproxy.", true},
		{"other.label", "devproxy.", false},
		{"devproxy", "devproxy.", false},
		{"", "devproxy.", false},
		{"devproxy.host", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.s+"_"+tt.prefix, func(t *testing.T) {
			if got := hasPrefix(tt.s, tt.prefix); got != tt.want {
				t.Errorf("hasPrefix(%q, %q) = %v, want %v", tt.s, tt.prefix, got, tt.want)
			}
		})
	}
}
