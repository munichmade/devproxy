package docker

import (
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
)

func TestContainerResolver_extractIP(t *testing.T) {
	tests := []struct {
		name      string
		network   string
		settings  *container.NetworkSettings
		wantIP    string
		wantError bool
	}{
		{
			name:    "preferred network exists",
			network: "mynetwork",
			settings: &container.NetworkSettings{
				Networks: map[string]*network.EndpointSettings{
					"bridge":    {IPAddress: "172.17.0.2"},
					"mynetwork": {IPAddress: "10.0.0.5"},
				},
			},
			wantIP: "10.0.0.5",
		},
		{
			name:    "preferred network missing, use first available",
			network: "nonexistent",
			settings: &container.NetworkSettings{
				Networks: map[string]*network.EndpointSettings{
					"bridge": {IPAddress: "172.17.0.2"},
				},
			},
			wantIP: "172.17.0.2",
		},
		{
			name:    "no preferred network, use first available",
			network: "",
			settings: &container.NetworkSettings{
				Networks: map[string]*network.EndpointSettings{
					"custom": {IPAddress: "192.168.1.100"},
				},
			},
			wantIP: "192.168.1.100",
		},
		{
			name:    "no networks available",
			network: "",
			settings: &container.NetworkSettings{
				Networks: map[string]*network.EndpointSettings{},
			},
			wantError: true,
		},
		{
			name:      "nil settings",
			network:   "",
			settings:  nil,
			wantError: true,
		},
		{
			name:    "no IP anywhere",
			network: "",
			settings: &container.NetworkSettings{
				Networks: map[string]*network.EndpointSettings{
					"bridge": {IPAddress: ""},
				},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := &ContainerResolver{network: tt.network}
			ip, err := resolver.extractIP(tt.settings)

			if tt.wantError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if ip != tt.wantIP {
				t.Errorf("got IP %q, want %q", ip, tt.wantIP)
			}
		})
	}
}

func TestContainerResolver_extractName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "removes leading slash",
			input:    "/mycontainer",
			expected: "mycontainer",
		},
		{
			name:     "no leading slash",
			input:    "mycontainer",
			expected: "mycontainer",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "just slash",
			input:    "/",
			expected: "",
		},
		{
			name:     "nested name with slashes",
			input:    "/project_container_1",
			expected: "project_container_1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := &ContainerResolver{}
			result := resolver.extractName(tt.input)

			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestContainerResolver_SetNetwork(t *testing.T) {
	resolver := NewContainerResolver(nil, "initial")

	if resolver.network != "initial" {
		t.Errorf("expected initial network, got %q", resolver.network)
	}

	resolver.SetNetwork("updated")

	if resolver.network != "updated" {
		t.Errorf("expected updated network, got %q", resolver.network)
	}
}

func TestNewContainerResolver(t *testing.T) {
	client := &Client{}
	resolver := NewContainerResolver(client, "mynetwork")

	if resolver.client != client {
		t.Error("client not set correctly")
	}

	if resolver.network != "mynetwork" {
		t.Errorf("network = %q, want 'mynetwork'", resolver.network)
	}
}
