// Package config provides configuration loading and management for devproxy.
package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/munichmade/devproxy/internal/paths"
)

// Config represents the complete devproxy configuration.
type Config struct {
	DNS         DNSConfig                   `yaml:"dns"`
	Entrypoints map[string]EntrypointConfig `yaml:"entrypoints"`
	Docker      DockerConfig                `yaml:"docker"`
	Logging     LoggingConfig               `yaml:"logging"`
}

// DNSConfig configures the built-in DNS server.
type DNSConfig struct {
	Enabled  bool     `yaml:"enabled"` // Enable built-in DNS server (can be disabled if using dnsmasq)
	Listen   string   `yaml:"listen"`
	Domains  []string `yaml:"domains"`
	Upstream string   `yaml:"upstream"`
}

// ListenAddresses is one or more addresses for an entrypoint.
type ListenAddresses []string

// UnmarshalYAML accepts either a single address or a list of addresses.
func (a *ListenAddresses) UnmarshalYAML(value *yaml.Node) error {
	var addresses []string
	switch value.Kind {
	case yaml.ScalarNode:
		var address string
		if err := value.Decode(&address); err != nil {
			return err
		}
		addresses = []string{address}
	case yaml.SequenceNode:
		if err := value.Decode(&addresses); err != nil {
			return err
		}
	default:
		return fmt.Errorf("listen must be a string or list of strings")
	}

	*a = nil
	for _, address := range addresses {
		for _, normalized := range loopbackAddresses(address) {
			if !slices.Contains(*a, normalized) {
				*a = append(*a, normalized)
			}
		}
	}
	return nil
}

func loopbackAddresses(address string) ListenAddresses {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return ListenAddresses{address}
	}
	ip := net.ParseIP(host)
	if host == "" || strings.EqualFold(strings.TrimSuffix(host, "."), "localhost") || ip != nil && ip.IsUnspecified() {
		return ListenAddresses{net.JoinHostPort("127.0.0.1", port), net.JoinHostPort("::1", port)}
	}
	return ListenAddresses{address}
}

// EntrypointConfig configures a single entrypoint (HTTP, HTTPS, or TCP).
type EntrypointConfig struct {
	Listen     ListenAddresses `yaml:"listen"`
	TargetPort int             `yaml:"target_port,omitempty"`
}

// DockerConfig configures Docker integration.
type DockerConfig struct {
	Enabled bool   `yaml:"enabled"`
	Socket  string `yaml:"socket"`
}

// LoggingConfig configures logging behavior.
type LoggingConfig struct {
	Level     string `yaml:"level"`
	AccessLog bool   `yaml:"access_log"`
}

// Default returns a Config with sensible default values.
// HTTP/HTTPS use privileged ports 80/443 (requires running as root).
// DNS uses unprivileged port 15353 to avoid conflicts with system DNS.
func Default() *Config {
	return &Config{
		DNS: DNSConfig{
			Listen:   ":15353", // Unprivileged port (resolver configured via setup)
			Domains:  []string{"localhost"},
			Upstream: "8.8.8.8:53",
			Enabled:  true,
		},
		Entrypoints: map[string]EntrypointConfig{
			"http": {
				Listen: ListenAddresses{"127.0.0.1:80", "[::1]:80"}, // Privileged port (requires root)
			},
			"https": {
				Listen: ListenAddresses{"127.0.0.1:443", "[::1]:443"}, // Privileged port (requires root)
			},
			"postgres": {
				Listen:     ListenAddresses{"127.0.0.1:15432", "[::1]:15432"},
				TargetPort: 5432,
			},
			"mongo": {
				Listen:     ListenAddresses{"127.0.0.1:27017", "[::1]:27017"},
				TargetPort: 27017,
			},
		},
		Docker: DockerConfig{
			Enabled: true,
			Socket:  "unix:///var/run/docker.sock",
		},
		Logging: LoggingConfig{
			Level:     "info",
			AccessLog: false,
		},
	}
}

// Load reads the configuration from the default config file.
// If the file doesn't exist, it creates a default configuration file.
func Load() (*Config, error) {
	return LoadFromFile(paths.ConfigFile())
}

// LoadExisting reads the configuration without creating a default file.
func LoadExisting() (*Config, error) {
	return loadExisting(paths.ConfigFile())
}

// LoadFromFile reads the configuration from the specified file path.
// If the file doesn't exist, it creates a default configuration file.
func LoadFromFile(path string) (*Config, error) {
	// Check if config file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// Create default config
		cfg := Default()
		if err := cfg.SaveToFile(path); err != nil {
			return nil, fmt.Errorf("failed to create default config: %w", err)
		}
		return cfg, nil
	}

	return loadExisting(path)
}

func loadExisting(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	return cfg, nil
}

// Save writes the configuration to the default config file.
func (c *Config) Save() error {
	return c.SaveToFile(paths.ConfigFile())
}

// SaveToFile writes the configuration to the specified file path.
func (c *Config) SaveToFile(path string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal to YAML
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write file
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	// Validate DNS config
	if c.DNS.Listen == "" {
		return fmt.Errorf("dns.listen is required")
	}
	if len(c.DNS.Domains) == 0 {
		return fmt.Errorf("dns.domains must have at least one domain")
	}

	// Validate entrypoints
	if len(c.Entrypoints) == 0 {
		return fmt.Errorf("at least one entrypoint is required")
	}
	for name, ep := range c.Entrypoints {
		if len(ep.Listen) == 0 {
			return fmt.Errorf("entrypoint %q: listen address is required", name)
		}
		port := -1
		for _, address := range ep.Listen {
			if address == "" {
				return fmt.Errorf("entrypoint %q: listen address is required", name)
			}
			host, portString, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("entrypoint %q: invalid listen address %q: %w", name, address, err)
			}
			addressPort, err := net.LookupPort("tcp", portString)
			if err != nil {
				return fmt.Errorf("entrypoint %q: invalid listen port %q: %w", name, portString, err)
			}
			if port != -1 && port != addressPort {
				return fmt.Errorf("entrypoint %q: listen addresses must use the same port", name)
			}
			port = addressPort
			ip := net.ParseIP(host)
			if ip == nil || !ip.IsLoopback() {
				return fmt.Errorf("entrypoint %q: listen address %q must use a loopback IP", name, address)
			}
		}
	}

	// Validate Docker config
	if c.Docker.Enabled && c.Docker.Socket == "" {
		return fmt.Errorf("docker.socket is required when docker is enabled")
	}

	// Validate logging config
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[c.Logging.Level] {
		return fmt.Errorf("logging.level must be one of: debug, info, warn, error")
	}

	return nil
}

// GetEntrypoint returns the entrypoint configuration by name.
func (c *Config) GetEntrypoint(name string) (EntrypointConfig, bool) {
	ep, ok := c.Entrypoints[name]
	return ep, ok
}
