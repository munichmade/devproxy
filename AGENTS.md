# AGENTS.md - AI Coding Agent Guidelines for DevProxy

This document provides guidelines for AI coding agents working in the devproxy codebase.

## Project Overview

DevProxy is a local development reverse proxy written in Go. It automatically routes traffic to Docker containers based on labels, provides automatic TLS certificates, and includes a built-in DNS server.

- **Language:** Go 1.25.5
- **Module:** `github.com/munichmade/devproxy`
- **Entry Point:** `cmd/devproxy/main.go`
- **CLI Framework:** Cobra (`github.com/spf13/cobra`)

## Build Commands

```bash
# Build binary for current platform
make build

# Build for all platforms (darwin/linux, amd64/arm64)
make build-all

# Run in foreground
make run

# Development with live reload (requires entr)
make dev

# Install to /usr/local/bin
make install

# Clean build artifacts
make clean
```

## Testing

```bash
# Run all tests
make test
# Or directly:
go test -v ./...

# Run a single test file
go test -v ./internal/docker/client_test.go

# Run a specific test function
go test -v ./internal/docker -run TestNewClient

# Run a specific subtest
go test -v ./internal/docker -run "TestNewClient/creates_client_with_default_options"

# Run tests with race detector (used in CI)
go test -v -race ./...

# Run tests with coverage
make test-coverage

# Run integration tests (requires Docker)
go test -v -tags=integration ./...
```

## Linting and Formatting

```bash
# Run linter (golangci-lint)
make lint

# Format code
make fmt
# Or directly:
go fmt ./...
goimports -local github.com/munichmade/devproxy -w .
```

### Enabled Linters

From `.golangci.yml`: errcheck, gosimple, govet, ineffassign, staticcheck, unused, gofmt, goimports, misspell, unconvert

## Code Style Guidelines

### Package Structure

```
cmd/devproxy/           # CLI application entry point
  cmd/                  # Cobra commands (root.go, start.go, stop.go, etc.)
internal/               # Private packages
  ca/                   # Certificate Authority management
  cert/                 # TLS certificate generation
  config/               # Configuration loading/watching
  daemon/               # Daemon lifecycle management
  dns/                  # Built-in DNS server
  docker/               # Docker integration/watcher
  logging/              # Logging utilities
  paths/                # File path utilities
  privilege/            # Privilege management
  proxy/                # HTTP/HTTPS/TCP proxy
  resolver/             # System DNS resolver config
  service/              # System service integration
```

### Import Organization

Imports must be grouped and ordered (enforced by goimports):

```go
import (
    // Standard library
    "context"
    "fmt"
    "sync"

    // External dependencies
    "github.com/docker/docker/api/types"
    "github.com/spf13/cobra"

    // Local packages (prefix: github.com/munichmade/devproxy)
    "github.com/munichmade/devproxy/internal/config"
    "github.com/munichmade/devproxy/internal/proxy"
)
```

### Package Comments

Every package must have a doc comment:

```go
// Package config provides configuration loading and management for devproxy.
package config
```

### Type and Function Documentation

```go
// Config represents the complete devproxy configuration.
type Config struct {
    DNS DNSConfig `yaml:"dns"`
}

// Default returns a Config with sensible default values.
// HTTP/HTTPS use privileged ports 80/443 (requires running as root).
func Default() *Config {
    // ...
}
```

### Error Handling

- Use `errors.New()` for sentinel errors
- Use `fmt.Errorf()` with `%w` for error wrapping
- Define package-level error variables for common errors

```go
var (
    ErrRouteExists   = errors.New("route already exists")
    ErrRouteNotFound = errors.New("route not found")
)

func doSomething() error {
    if err := operation(); err != nil {
        return fmt.Errorf("operation failed: %w", err)
    }
    return nil
}
```

### Struct Tags

Use yaml/json tags for serialization:

```go
type DNSConfig struct {
    Enabled  bool     `yaml:"enabled"`
    Listen   string   `yaml:"listen"`
    Domains  []string `yaml:"domains"`
}
```

### Testing Conventions

- Use table-driven tests with `t.Run()` for subtests
- Test file naming: `*_test.go`
- Skip tests that require external dependencies (Docker) when unavailable

```go
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
}
```

### Concurrency

- Use `sync.RWMutex` for thread-safe data structures
- Use `context.Context` for cancellation and timeouts

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
```

### Naming Conventions

- **Packages:** lowercase, single word (e.g., `proxy`, `config`)
- **Exported types:** PascalCase (e.g., `RouteTable`, `Config`)
- **Unexported functions:** camelCase (e.g., `isWildcardHost`)
- **Constants:** PascalCase for exported, camelCase for unexported
- **Interfaces:** often end in `-er` (e.g., `Resolver`, `Watcher`)

### Spelling

US English spelling is enforced by the misspell linter.

## CI Requirements

Before submitting code, ensure:

1. `make lint` passes
2. `make test` passes
3. Code is formatted with `make fmt`

CI runs on both Ubuntu and macOS with race detector enabled.

## Key Dependencies

- `github.com/docker/docker` - Docker API client
- `github.com/miekg/dns` - DNS server/client
- `github.com/spf13/cobra` - CLI framework
- `gopkg.in/yaml.v3` - YAML parsing
- `github.com/gorilla/websocket` - WebSocket support
