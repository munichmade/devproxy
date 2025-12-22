// Package config provides configuration loading and management for devproxy.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/munichmade/devproxy/internal/paths"
	"gopkg.in/yaml.v3"
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

// EntrypointConfig configures a single entrypoint (HTTP, HTTPS, or TCP).
type EntrypointConfig struct {
	Listen     string `yaml:"listen"`
	TargetPort int    `yaml:"target_port,omitempty"`
}

// DockerConfig configures Docker integration.
type DockerConfig struct {
	Enabled     bool   `yaml:"enabled"`
	Socket      string `yaml:"socket"`
	LabelPrefix string `yaml:"label_prefix"`
}

// LoggingConfig configures logging behavior.
type LoggingConfig struct {
	Level     string `yaml:"level"`
	AccessLog bool   `yaml:"access_log"`
}

// Default returns a Config with sensible default values.
// Uses non-privileged ports by default to avoid requiring root access.
// Port forwarding (80->8080, 443->8443) can be set up via 'devproxy setup'.
func Default() *Config {
	return &Config{
		DNS: DNSConfig{
			Listen:   ":5353", // Non-privileged port (use resolver with port 5353)
			Domains:  []string{"localhost"},
			Upstream: "8.8.8.8:53",
			Enabled:  true, // Can be disabled if using external DNS (e.g., dnsmasq)
		},
		Entrypoints: map[string]EntrypointConfig{
			"http": {
				Listen: ":8080", // Non-privileged port (forwarded from 80 via pf)
			},
			"https": {
				Listen: ":8443", // Non-privileged port (forwarded from 443 via pf)
			},
			"postgres": {
				Listen:     ":15432",
				TargetPort: 5432,
			},
			"mongo": {
				Listen:     ":27017",
				TargetPort: 27017,
			},
		},
		Docker: DockerConfig{
			Enabled:     true,
			Socket:      "unix:///var/run/docker.sock",
			LabelPrefix: "devproxy",
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

	// Read existing config
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Start with defaults and overlay with file values
	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Validate the configuration
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
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal to YAML
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write file
	if err := os.WriteFile(path, data, 0600); err != nil {
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
		if ep.Listen == "" {
			return fmt.Errorf("entrypoint %q: listen address is required", name)
		}
	}

	// Validate Docker config
	if c.Docker.Enabled && c.Docker.Socket == "" {
		return fmt.Errorf("docker.socket is required when docker is enabled")
	}
	if c.Docker.LabelPrefix == "" {
		return fmt.Errorf("docker.label_prefix is required")
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
