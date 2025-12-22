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

# Initial setup (creates CA, configures DNS resolver - requires sudo)
sudo devproxy setup

# Start the daemon
devproxy start

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
# Start as daemon
devproxy start

# Stop daemon
devproxy stop

# Restart daemon
devproxy restart

# Check status
devproxy status

# View logs
devproxy logs -f
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

For non-HTTP services like databases, use the `entrypoint` label to route through a TCP entrypoint:

```yaml
labels:
  - "devproxy.enable=true"
  - "devproxy.host=db.localhost"
  - "devproxy.port=5432"
  - "devproxy.entrypoint=postgres"
```

The entrypoint name must match one defined in your config (e.g., `postgres`, `mysql`, `mongo`).

**Note:** For SSL mode "Preferred" PostgreSQL clients, devproxy automatically handles the PostgreSQL SSLRequest protocol to enable SNI-based routing.

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
      - "devproxy.host=db.localhost"
      - "devproxy.port=5432"
      - "devproxy.entrypoint=postgres"
```

## Configuration File

The configuration file is located at `~/.config/devproxy/config.yaml`.

### Complete Configuration Reference

```yaml
# DNS server configuration
dns:
  # Enable/disable the built-in DNS server
  # Set to false if using external DNS (e.g., dnsmasq)
  enabled: true
  
  # Address and port to listen on
  # Uses unprivileged port by default (resolver configured via setup)
  listen: ":15353"
  
  # Domains to handle (will resolve to 127.0.0.1)
  # All subdomains are automatically included (e.g., *.localhost)
  domains:
    - localhost
    - test
  
  # Upstream DNS server for non-matching queries
  upstream: "8.8.8.8:53"

# Entrypoints define the ports devproxy listens on
# Reserved names: "http" and "https" are handled specially
entrypoints:
  # HTTP entrypoint (redirects to HTTPS by default)
  http:
    listen: ":80"
  
  # HTTPS entrypoint (TLS termination with auto-generated certs)
  https:
    listen: ":443"
  
  # TCP entrypoints for databases and other services
  # The name is used in container labels: devproxy.entrypoint=postgres
  postgres:
    listen: ":15432"      # Port devproxy listens on
    target_port: 5432     # Default backend port (optional)
  
  mysql:
    listen: ":13306"
    target_port: 3306
  
  mongo:
    listen: ":27017"
    target_port: 27017
  
  redis:
    listen: ":16379"
    target_port: 6379

# Docker integration settings
docker:
  # Enable/disable Docker container discovery
  enabled: true
  
  # Docker socket path
  socket: "unix:///var/run/docker.sock"
  
  # Prefix for container labels (e.g., devproxy.enable, devproxy.host)
  label_prefix: "devproxy"

# Logging configuration
logging:
  # Log level: debug, info, warn, error
  level: "info"
  
  # Enable HTTP access logging
  access_log: false
```

### Default Values

If no configuration file exists, devproxy creates one with these defaults:

| Setting | Default Value |
|---------|---------------|
| `dns.enabled` | `true` |
| `dns.listen` | `:15353` |
| `dns.domains` | `["localhost"]` |
| `dns.upstream` | `8.8.8.8:53` |
| `entrypoints.http.listen` | `:80` |
| `entrypoints.https.listen` | `:443` |
| `entrypoints.postgres.listen` | `:15432` |
| `entrypoints.postgres.target_port` | `5432` |
| `entrypoints.mongo.listen` | `:27017` |
| `entrypoints.mongo.target_port` | `27017` |
| `docker.enabled` | `true` |
| `docker.socket` | `unix:///var/run/docker.sock` |
| `docker.label_prefix` | `devproxy` |
| `logging.level` | `info` |
| `logging.access_log` | `false` |

### File Locations

| Platform | Config File | Data Directory |
|----------|-------------|----------------|
| **macOS** | `~/.config/devproxy/config.yaml` | `~/Library/Application Support/devproxy/` |
| **Linux** | `~/.config/devproxy/config.yaml` | `~/.local/share/devproxy/` |

Data directory contains:
- `ca/` - Root CA certificate and private key
- `certs/` - Generated TLS certificates
- `devproxy.log` - Daemon log file
- `devproxy.pid` - PID file
- `routes.json` - Active route registry

Environment variables `XDG_CONFIG_HOME` and `XDG_DATA_HOME` are respected.

### Hot Reload

Devproxy supports hot reloading of configuration changes. Changes are applied automatically when:
- The config file is modified (file watcher)
- Docker containers start/stop (automatic discovery)

**Hot-reloadable settings (no restart required):**
| Setting | Description |
|---------|-------------|
| `logging.level` | Log level changes apply immediately |
| `dns.domains` | Add/remove handled domains |
| `dns.upstream` | Change upstream DNS server |

**Settings requiring restart:**
| Setting | Description |
|---------|-------------|
| `dns.listen` | DNS server listen address/port |
| `entrypoints.*.listen` | Entrypoint listen addresses |
| `docker.label_prefix` | Docker label prefix |
| `docker.socket` | Docker socket path |

When a setting that requires restart is changed, devproxy logs a warning message indicating a restart is needed.

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

Re-run the setup command:
```bash
sudo devproxy setup
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

Check what's using the ports:
```bash
sudo lsof -i :80
sudo lsof -i :443
sudo lsof -i :15353  # DNS server
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
│  │ :15353  │  │  :80    │  │  :443   │  │ :15432,:13306 │  │
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
