package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	// DNS defaults
	if cfg.DNS.Listen != ":15353" {
		t.Errorf("DNS.Listen = %q, want %q", cfg.DNS.Listen, ":15353")
	}
	if len(cfg.DNS.Domains) != 1 || cfg.DNS.Domains[0] != "localhost" {
		t.Errorf("DNS.Domains = %v, want [localhost]", cfg.DNS.Domains)
	}
	if cfg.DNS.Upstream != "8.8.8.8:53" {
		t.Errorf("DNS.Upstream = %q, want %q", cfg.DNS.Upstream, "8.8.8.8:53")
	}

	// Entrypoint defaults
	if len(cfg.Entrypoints) != 4 {
		t.Errorf("len(Entrypoints) = %d, want 4", len(cfg.Entrypoints))
	}

	http, ok := cfg.Entrypoints["http"]
	if !ok || !slices.Equal(http.Listen, ListenAddresses{"127.0.0.1:80", "[::1]:80"}) {
		t.Errorf("Entrypoints[http] = %+v, want dual-stack loopback listeners", http)
	}

	https, ok := cfg.Entrypoints["https"]
	if !ok || !slices.Equal(https.Listen, ListenAddresses{"127.0.0.1:443", "[::1]:443"}) {
		t.Errorf("Entrypoints[https] = %+v, want dual-stack loopback listeners", https)
	}

	postgres, ok := cfg.Entrypoints["postgres"]
	if !ok || !slices.Equal(postgres.Listen, ListenAddresses{"127.0.0.1:15432", "[::1]:15432"}) || postgres.TargetPort != 5432 {
		t.Errorf("Entrypoints[postgres] = %+v, want dual-stack loopback listeners and TargetPort=5432", postgres)
	}

	mongo, ok := cfg.Entrypoints["mongo"]
	if !ok || !slices.Equal(mongo.Listen, ListenAddresses{"127.0.0.1:27017", "[::1]:27017"}) || mongo.TargetPort != 27017 {
		t.Errorf("Entrypoints[mongo] = %+v, want dual-stack loopback listeners and TargetPort=27017", mongo)
	}

	// Docker defaults
	if !cfg.Docker.Enabled {
		t.Error("Docker.Enabled = false, want true")
	}
	if cfg.Docker.Socket != "unix:///var/run/docker.sock" {
		t.Errorf("Docker.Socket = %q, want %q", cfg.Docker.Socket, "unix:///var/run/docker.sock")
	}

	// Logging defaults
	if cfg.Logging.Level != "info" {
		t.Errorf("Logging.Level = %q, want %q", cfg.Logging.Level, "info")
	}
	if cfg.Logging.AccessLog {
		t.Error("Logging.AccessLog = true, want false")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr bool
	}{
		{
			name:    "valid default config",
			modify:  func(c *Config) {},
			wantErr: false,
		},
		{
			name:    "empty DNS listen",
			modify:  func(c *Config) { c.DNS.Listen = "" },
			wantErr: true,
		},
		{
			name:    "empty DNS domains",
			modify:  func(c *Config) { c.DNS.Domains = nil },
			wantErr: true,
		},
		{
			name:    "no entrypoints",
			modify:  func(c *Config) { c.Entrypoints = nil },
			wantErr: true,
		},
		{
			name:    "entrypoint without listen",
			modify:  func(c *Config) { c.Entrypoints["test"] = EntrypointConfig{} },
			wantErr: true,
		},
		{
			name: "entrypoint on LAN address",
			modify: func(c *Config) {
				c.Entrypoints["http"] = EntrypointConfig{Listen: ListenAddresses{"192.168.1.10:80"}}
			},
			wantErr: true,
		},
		{
			name: "entrypoint with mixed ports",
			modify: func(c *Config) {
				c.Entrypoints["https"] = EntrypointConfig{Listen: ListenAddresses{"127.0.0.1:443", "[::1]:8443"}}
			},
			wantErr: true,
		},
		{
			name:    "docker enabled without socket",
			modify:  func(c *Config) { c.Docker.Enabled = true; c.Docker.Socket = "" },
			wantErr: true,
		},
		{
			name:    "docker disabled without socket is ok",
			modify:  func(c *Config) { c.Docker.Enabled = false; c.Docker.Socket = "" },
			wantErr: false,
		},
		{
			name:    "invalid log level",
			modify:  func(c *Config) { c.Logging.Level = "invalid" },
			wantErr: true,
		},
		{
			name:    "valid log level debug",
			modify:  func(c *Config) { c.Logging.Level = "debug" },
			wantErr: false,
		},
		{
			name:    "valid log level warn",
			modify:  func(c *Config) { c.Logging.Level = "warn" },
			wantErr: false,
		},
		{
			name:    "valid log level error",
			modify:  func(c *Config) { c.Logging.Level = "error" },
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			tt.modify(cfg)
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSaveAndLoad(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "devproxy-config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create and save config
	cfg := Default()
	cfg.DNS.Upstream = "1.1.1.1:53"
	cfg.Logging.Level = "debug"
	cfg.Docker.Enabled = false

	if err := cfg.SaveToFile(configPath); err != nil {
		t.Fatalf("SaveToFile() error = %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("Config file was not created")
	}

	// Load config back
	loaded, err := LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("LoadFromFile() error = %v", err)
	}

	// Verify values
	if loaded.DNS.Upstream != "1.1.1.1:53" {
		t.Errorf("DNS.Upstream = %q, want %q", loaded.DNS.Upstream, "1.1.1.1:53")
	}
	if loaded.Logging.Level != "debug" {
		t.Errorf("Logging.Level = %q, want %q", loaded.Logging.Level, "debug")
	}
	if loaded.Docker.Enabled {
		t.Error("Docker.Enabled = true, want false")
	}
}

func TestLoadFromFile_CreatesDefault(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "devproxy-config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "subdir", "config.yaml")

	// Load from non-existent file should create default
	cfg, err := LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("LoadFromFile() error = %v", err)
	}

	// Should have default values
	if cfg.DNS.Listen != ":15353" {
		t.Errorf("DNS.Listen = %q, want %q", cfg.DNS.Listen, ":15353")
	}

	// File should now exist
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("Config file was not created")
	}
}

func TestLoadFromFile_ListenFormats(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want ListenAddresses
	}{
		{name: "single string", yaml: `"127.0.0.1:8080"`, want: ListenAddresses{"127.0.0.1:8080"}},
		{name: "localhost", yaml: `"LOCALHOST.:8080"`, want: ListenAddresses{"127.0.0.1:8080", "[::1]:8080"}},
		{name: "legacy wildcard", yaml: `":8080"`, want: ListenAddresses{"127.0.0.1:8080", "[::1]:8080"}},
		{name: "address list", yaml: `["127.0.0.1:8080", "[::1]:8080"]`, want: ListenAddresses{"127.0.0.1:8080", "[::1]:8080"}},
		{name: "duplicate wildcards", yaml: `["0.0.0.0:8080", "[::]:8080"]`, want: ListenAddresses{"127.0.0.1:8080", "[::1]:8080"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "config.yaml")
			data := []byte("entrypoints:\n  http:\n    listen: " + tt.yaml + "\n")
			if err := os.WriteFile(configPath, data, 0o600); err != nil {
				t.Fatal(err)
			}

			cfg, err := LoadFromFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			if got := cfg.Entrypoints["http"].Listen; !slices.Equal(got, tt.want) {
				t.Errorf("Listen = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadFromFile_InvalidYAML(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "devproxy-config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write invalid YAML
	if err := os.WriteFile(configPath, []byte("invalid: yaml: content:"), 0600); err != nil {
		t.Fatalf("Failed to write invalid config: %v", err)
	}

	// Load should fail
	_, err = LoadFromFile(configPath)
	if err == nil {
		t.Error("LoadFromFile() expected error for invalid YAML, got nil")
	}
}

func TestLoadFromFile_InvalidConfig(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "devproxy-config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write valid YAML but invalid config (invalid log level)
	invalidConfig := `
logging:
  level: "invalid_level"
`
	if err := os.WriteFile(configPath, []byte(invalidConfig), 0600); err != nil {
		t.Fatalf("Failed to write invalid config: %v", err)
	}

	// Load should fail validation
	_, err = LoadFromFile(configPath)
	if err == nil {
		t.Error("LoadFromFile() expected validation error, got nil")
	}
}

func TestGetEntrypoint(t *testing.T) {
	cfg := Default()

	// Existing entrypoint
	ep, ok := cfg.GetEntrypoint("postgres")
	if !ok {
		t.Error("GetEntrypoint(postgres) returned false, want true")
	}
	want := ListenAddresses{"127.0.0.1:15432", "[::1]:15432"}
	if !slices.Equal(ep.Listen, want) {
		t.Errorf("GetEntrypoint(postgres).Listen = %v, want %v", ep.Listen, want)
	}

	// Non-existent entrypoint
	_, ok = cfg.GetEntrypoint("nonexistent")
	if ok {
		t.Error("GetEntrypoint(nonexistent) returned true, want false")
	}
}
