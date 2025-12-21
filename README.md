# devproxy

A lightweight, zero-config development proxy for local Docker containers. Automatically routes traffic to your containers using custom domains with full HTTPS support.

## Features

- **Automatic Docker Discovery** - Detects containers via labels, no manual configuration needed
- **Custom Local Domains** - Use `.localhost` or any custom TLD for your services
- **Automatic HTTPS** - Generates trusted certificates on-the-fly via built-in CA
- **DNS Server** - Resolves custom domains without `/etc/hosts` modifications
- **TCP/SNI Routing** - Route any TCP traffic including databases
- **Hot Reload** - Configuration changes apply without restart

## Quick Start

```bash
# Install (macOS)
brew install munichmade/tap/devproxy

# Or build from source
git clone https://github.com/munichmade/devproxy
cd devproxy
make install

# Initial setup (creates CA, configures DNS resolver)
sudo devproxy setup

# Start the daemon
sudo devproxy start

# Add labels to your container
docker run -d \
  --label "devproxy.enable=true" \
  --label "devproxy.host=myapp.localhost" \
  nginx

# Visit https://myapp.localhost
```

## Installation

### macOS (Homebrew)

```bash
brew install munichmade/tap/devproxy
```

### Linux

```bash
# Download the latest release
curl -L https://github.com/munichmade/devproxy/releases/latest/download/devproxy-linux-amd64 -o devproxy
chmod +x devproxy
sudo mv devproxy /usr/local/bin/

# Install systemd service
sudo cp init/devproxy.service /etc/systemd/system/
sudo systemctl daemon-reload
```

### Build from Source

```bash
git clone https://github.com/munichmade/devproxy
cd devproxy
make build
sudo make install
```

## Setup

Run the setup command to initialize devproxy:

```bash
sudo devproxy setup
```

This will:
1. Create a local Certificate Authority (CA)
2. Trust the CA in the system keychain
3. Configure DNS resolver for `.localhost` domains
4. Create the configuration directory

## Usage

### Starting/Stopping

```bash
# Start as foreground process
sudo devproxy run

# Start as daemon
sudo devproxy start

# Stop daemon
sudo devproxy stop

# Restart daemon
sudo devproxy restart

# Reload configuration
sudo devproxy reload

# Check status
devproxy status
```

### Managing Domains

```bash
# List all configured domains
devproxy domain list

# Add a static domain
devproxy domain add myapp.localhost 127.0.0.1:8080
```

### Certificate Authority

```bash
# Show CA information
devproxy ca info

# Trust the CA (run during setup)
sudo devproxy ca trust

# Untrust the CA
sudo devproxy ca untrust
```

### Configuration

```bash
# Show current configuration
devproxy config show

# Edit configuration
devproxy config edit

# Validate configuration
devproxy config validate
```

## Docker Integration

Add labels to your containers to enable automatic routing:

### Basic Labels

| Label | Description | Example |
|-------|-------------|---------|
| `devproxy.enable` | Enable routing for container | `true` |
| `devproxy.host` | Domain name(s) to route | `myapp.localhost` |
| `devproxy.port` | Container port to route to | `8080` |
| `devproxy.tls` | Enable HTTPS | `true` (default) |

### Multiple Hosts

```yaml
labels:
  - "devproxy.enable=true"
  - "devproxy.host=app.localhost,api.localhost"
  - "devproxy.port=3000"
```

### TCP Routing

For non-HTTP services like databases:

```yaml
labels:
  - "devproxy.enable=true"
  - "devproxy.tcp.postgres=5432"
```

### Docker Compose Example

```yaml
# docker-compose.yml
services:
  web:
    image: nginx
    labels:
      - "devproxy.enable=true"
      - "devproxy.host=web.localhost"
      - "devproxy.port=80"

  api:
    image: node:20
    labels:
      - "devproxy.enable=true"
      - "devproxy.host=api.localhost"
      - "devproxy.port=3000"

  db:
    image: postgres:16
    labels:
      - "devproxy.enable=true"
      - "devproxy.tcp.postgres=5432"
```

## Configuration File

The configuration file is located at `~/.config/devproxy/config.yaml` (or `/etc/devproxy/config.yaml` for system-wide).

```yaml
# Log level: debug, info, warn, error
log_level: info

# DNS server configuration
dns:
  listen: ":53"
  domains:
    - localhost

# HTTP entrypoint
http:
  listen: ":80"

# HTTPS entrypoint  
https:
  listen: ":443"

# Additional TCP entrypoints
entrypoints:
  postgres:
    listen: ":15432"
  mysql:
    listen: ":13306"
  redis:
    listen: ":16379"

# Docker integration
docker:
  enabled: true
  host: "unix:///var/run/docker.sock"

# Certificate settings
certificates:
  dir: "~/.config/devproxy/certs"
  ca:
    cert: "~/.config/devproxy/ca/cert.pem"
    key: "~/.config/devproxy/ca/key.pem"
```

## Troubleshooting

### DNS not resolving

Check if the DNS server is running:
```bash
dig @127.0.0.1 myapp.localhost
```

Verify resolver configuration:
```bash
# macOS
cat /etc/resolver/localhost

# Linux (systemd-resolved)
resolvectl status
```

### Certificate not trusted

Re-run the CA trust command:
```bash
sudo devproxy ca trust
```

For browsers, you may need to restart them after trusting the CA.

### Container not discovered

Ensure labels are correctly set:
```bash
docker inspect <container> | grep -A 20 Labels
```

Check devproxy logs:
```bash
devproxy logs -f
```

### Port already in use

Check what's using port 53/80/443:
```bash
sudo lsof -i :53
sudo lsof -i :80
sudo lsof -i :443
```

### Permission denied

devproxy needs root privileges to:
- Bind to ports below 1024 (53, 80, 443)
- Modify DNS resolver configuration
- Trust CA in system keychain

Run with `sudo` or configure capabilities:
```bash
sudo setcap 'cap_net_bind_service=+ep' /usr/local/bin/devproxy
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        devproxy                             │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌───────────────┐  │
│  │   DNS   │  │  HTTP   │  │  HTTPS  │  │  TCP Router   │  │
│  │  :53    │  │  :80    │  │  :443   │  │ :15432,:13306 │  │
│  └────┬────┘  └────┬────┘  └────┬────┘  └───────┬───────┘  │
│       │            │            │               │          │
│       v            v            v               v          │
│  ┌─────────────────────────────────────────────────────┐   │
│  │                   Route Manager                      │   │
│  └─────────────────────────────────────────────────────┘   │
│       │                                                     │
│       v                                                     │
│  ┌─────────────────────────────────────────────────────┐   │
│  │                  Docker Watcher                      │   │
│  │            (container discovery via labels)          │   │
│  └─────────────────────────────────────────────────────┘   │
│       │                                                     │
│       v                                                     │
│  ┌─────────────────────────────────────────────────────┐   │
│  │                Certificate Manager                   │   │
│  │              (on-demand cert generation)             │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
                              │
                              v
                    ┌─────────────────┐
                    │     Docker      │
                    │   Containers    │
                    └─────────────────┘
```

## Contributing

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/my-feature`
3. Make your changes
4. Run tests: `make test`
5. Run linter: `make lint`
6. Commit: `git commit -m "Add my feature"`
7. Push: `git push origin feature/my-feature`
8. Open a Pull Request

### Development Setup

```bash
# Clone
git clone https://github.com/munichmade/devproxy
cd devproxy

# Install dependencies
go mod download

# Build
make build

# Run tests
make test

# Run with race detector
make test-race
```

## License

MIT License - see [LICENSE](LICENSE) for details.
