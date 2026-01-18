package docker

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
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

		if client.API() == nil {
			t.Error("expected docker client to be initialized")
		}
	})

	t.Run("creates client with custom host", func(t *testing.T) {
		client, err := NewClientWithHost("unix:///var/run/docker.sock", testLogger())
		if err != nil {
			t.Skipf("Docker not available: %v", err)
		}
		defer client.Close()

		if client.API() == nil {
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

func TestNewClientWithAPI(t *testing.T) {
	t.Run("creates client with custom API", func(t *testing.T) {
		mockAPI := newMockDockerAPI()
		client := NewClientWithAPI(mockAPI, testLogger())

		if client.API() != mockAPI {
			t.Error("expected API to return the provided mock")
		}
	})
}

func TestClient_Connect_WithMock(t *testing.T) {
	t.Run("sets connected on successful ping", func(t *testing.T) {
		mockAPI := newMockBuilder().
			withPingSuccess().
			build()

		client := NewClientWithAPI(mockAPI, testLogger())

		ctx := context.Background()
		err := client.Connect(ctx)
		if err != nil {
			t.Fatalf("Connect failed: %v", err)
		}

		if !client.IsConnected() {
			t.Error("expected IsConnected to be true after successful Connect")
		}
	})

	t.Run("returns error and sets disconnected on ping failure", func(t *testing.T) {
		mockAPI := newMockBuilder().
			withPingError(errMockConnection).
			build()

		client := NewClientWithAPI(mockAPI, testLogger())

		ctx := context.Background()
		err := client.Connect(ctx)
		if err == nil {
			t.Error("expected Connect to fail")
		}

		if client.IsConnected() {
			t.Error("expected IsConnected to be false after failed Connect")
		}
	})
}

func TestClient_Ping_WithMock(t *testing.T) {
	t.Run("sets disconnected on ping failure", func(t *testing.T) {
		pingCount := 0
		mockAPI := newMockBuilder().
			withPing(func(ctx context.Context) (types.Ping, error) {
				pingCount++
				if pingCount == 1 {
					return types.Ping{}, nil
				}
				return types.Ping{}, errMockConnection
			}).
			build()

		client := NewClientWithAPI(mockAPI, testLogger())

		ctx := context.Background()

		// First connect succeeds
		_ = client.Connect(ctx)
		if !client.IsConnected() {
			t.Fatal("expected to be connected after first ping")
		}

		// Second ping fails
		err := client.Ping(ctx)
		if err == nil {
			t.Error("expected Ping to fail")
		}

		if client.IsConnected() {
			t.Error("expected IsConnected to be false after failed Ping")
		}
	})
}

func TestClient_ListContainers_WithMock(t *testing.T) {
	t.Run("returns container list", func(t *testing.T) {
		mockAPI := newMockBuilder().
			withContainerListResult([]container.Summary{
				makeContainerSummary("container1", "web", nil),
				makeContainerSummary("container2", "api", nil),
			}).
			build()

		client := NewClientWithAPI(mockAPI, testLogger())

		ctx := context.Background()
		containers, err := client.ListContainers(ctx)
		if err != nil {
			t.Fatalf("ListContainers failed: %v", err)
		}

		if len(containers) != 2 {
			t.Errorf("expected 2 containers, got %d", len(containers))
		}
	})

	t.Run("returns error on failure", func(t *testing.T) {
		mockAPI := newMockBuilder().
			withContainerListError(errMockConnection).
			build()

		client := NewClientWithAPI(mockAPI, testLogger())

		ctx := context.Background()
		_, err := client.ListContainers(ctx)
		if err == nil {
			t.Error("expected ListContainers to fail")
		}
	})
}

func TestClient_ListContainersWithLabel_WithMock(t *testing.T) {
	t.Run("filters containers by label prefix", func(t *testing.T) {
		mockAPI := newMockBuilder().
			withContainerListResult([]container.Summary{
				makeContainerSummary("container1", "web", map[string]string{
					"devproxy.host": "web.localhost",
				}),
				makeContainerSummary("container2", "api", map[string]string{
					"other.label": "value",
				}),
				makeContainerSummary("container3", "db", map[string]string{
					"devproxy.port": "5432",
				}),
			}).
			build()

		client := NewClientWithAPI(mockAPI, testLogger())

		ctx := context.Background()
		containers, err := client.ListContainersWithLabel(ctx, "devproxy.")
		if err != nil {
			t.Fatalf("ListContainersWithLabel failed: %v", err)
		}

		if len(containers) != 2 {
			t.Errorf("expected 2 containers with devproxy labels, got %d", len(containers))
		}

		// Verify the right containers were returned
		ids := make(map[string]bool)
		for _, c := range containers {
			ids[c.ID] = true
		}
		if !ids["container1"] {
			t.Error("expected container1 to be returned")
		}
		if ids["container2"] {
			t.Error("container2 should not be returned (no devproxy label)")
		}
		if !ids["container3"] {
			t.Error("expected container3 to be returned")
		}
	})

	t.Run("returns empty slice when no matches", func(t *testing.T) {
		mockAPI := newMockBuilder().
			withContainerListResult([]container.Summary{
				makeContainerSummary("container1", "web", map[string]string{
					"other.label": "value",
				}),
			}).
			build()

		client := NewClientWithAPI(mockAPI, testLogger())

		ctx := context.Background()
		containers, err := client.ListContainersWithLabel(ctx, "devproxy.")
		if err != nil {
			t.Fatalf("ListContainersWithLabel failed: %v", err)
		}

		if len(containers) != 0 {
			t.Errorf("expected 0 containers, got %d", len(containers))
		}
	})

	t.Run("returns error on list failure", func(t *testing.T) {
		mockAPI := newMockBuilder().
			withContainerListError(errMockConnection).
			build()

		client := NewClientWithAPI(mockAPI, testLogger())

		ctx := context.Background()
		_, err := client.ListContainersWithLabel(ctx, "devproxy.")
		if err == nil {
			t.Error("expected ListContainersWithLabel to fail")
		}
	})
}

func TestClient_InspectContainer_WithMock(t *testing.T) {
	t.Run("returns container info", func(t *testing.T) {
		mockAPI := newMockBuilder().
			withContainerInspectResult(makeContainerInspectResponse("container123", "web-app", "172.17.0.5", "bridge")).
			build()

		client := NewClientWithAPI(mockAPI, testLogger())

		ctx := context.Background()
		info, err := client.InspectContainer(ctx, "container123")
		if err != nil {
			t.Fatalf("InspectContainer failed: %v", err)
		}

		if info.ID != "container123" {
			t.Errorf("expected ID 'container123', got '%s'", info.ID)
		}
	})

	t.Run("returns error on failure", func(t *testing.T) {
		mockAPI := newMockBuilder().
			withContainerInspectError(errMockNotFound).
			build()

		client := NewClientWithAPI(mockAPI, testLogger())

		ctx := context.Background()
		_, err := client.InspectContainer(ctx, "nonexistent")
		if err == nil {
			t.Error("expected InspectContainer to fail")
		}
	})
}

func TestClient_WaitForConnection_WithMock(t *testing.T) {
	t.Run("succeeds on first try", func(t *testing.T) {
		mockAPI := newMockBuilder().
			withPingSuccess().
			build()

		client := NewClientWithAPI(mockAPI, testLogger())

		ctx := context.Background()
		err := client.WaitForConnection(ctx, 3, 10*time.Millisecond)
		if err != nil {
			t.Fatalf("WaitForConnection failed: %v", err)
		}

		if !client.IsConnected() {
			t.Error("expected IsConnected to be true")
		}
	})

	t.Run("succeeds after retries", func(t *testing.T) {
		attempts := 0
		mockAPI := newMockBuilder().
			withPing(func(ctx context.Context) (types.Ping, error) {
				attempts++
				if attempts < 3 {
					return types.Ping{}, errMockConnection
				}
				return types.Ping{APIVersion: "1.41"}, nil
			}).
			build()

		client := NewClientWithAPI(mockAPI, testLogger())

		ctx := context.Background()
		err := client.WaitForConnection(ctx, 5, 10*time.Millisecond)
		if err != nil {
			t.Fatalf("WaitForConnection failed: %v", err)
		}

		if attempts != 3 {
			t.Errorf("expected 3 attempts, got %d", attempts)
		}
	})

	t.Run("fails after max retries", func(t *testing.T) {
		mockAPI := newMockBuilder().
			withPingError(errMockConnection).
			build()

		client := NewClientWithAPI(mockAPI, testLogger())

		ctx := context.Background()
		err := client.WaitForConnection(ctx, 3, 10*time.Millisecond)
		if err == nil {
			t.Error("expected WaitForConnection to fail after max retries")
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		mockAPI := newMockBuilder().
			withPingError(errMockConnection).
			build()

		client := NewClientWithAPI(mockAPI, testLogger())

		ctx, cancel := context.WithCancel(context.Background())

		// Cancel after a short delay
		go func() {
			time.Sleep(25 * time.Millisecond)
			cancel()
		}()

		err := client.WaitForConnection(ctx, 10, 20*time.Millisecond)
		if err == nil {
			t.Error("expected WaitForConnection to fail on context cancellation")
		}

		if err != context.Canceled {
			t.Errorf("expected context.Canceled error, got %v", err)
		}
	})
}

func TestClient_Close_WithMock(t *testing.T) {
	t.Run("calls close on API", func(t *testing.T) {
		closeCalled := false
		mockAPI := &mockDockerAPI{
			closeFunc: func() error {
				closeCalled = true
				return nil
			},
		}

		client := NewClientWithAPI(mockAPI, testLogger())

		// First connect
		client.Connect(context.Background())

		err := client.Close()
		if err != nil {
			t.Errorf("Close returned error: %v", err)
		}

		if !closeCalled {
			t.Error("expected Close to call API.Close()")
		}

		if client.IsConnected() {
			t.Error("expected IsConnected to be false after Close")
		}
	})

	t.Run("handles nil API gracefully", func(t *testing.T) {
		client := &Client{logger: testLogger()}

		err := client.Close()
		if err != nil {
			t.Errorf("Close with nil API returned error: %v", err)
		}
	})
}
