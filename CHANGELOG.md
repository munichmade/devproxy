# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## 0.2.2 2026-01-30

### Changed

- macOS launchd service now uses socket activation for ports 80/443
  - Eliminates port conflicts and "address already in use" errors at boot
  - launchd owns the sockets, passing them to devproxy on startup
  - Service restarts on-demand when connections arrive (removed KeepAlive)
  - **Upgrade note:** Existing installations must reinstall the service:

    ```bash
    sudo devproxy setup --service
    ```

## 0.2.1 2026-01-19

### Changed

- Root privilege handling is improved
- Verification of Root CA certificate is more reliable

## 0.2.0 2026-01-18

### Added

- Status command now groups routes by Docker Compose project for better readability
- Routes display includes project working directory path

## [0.1.0] 2026-01-18

### Added

- MIT license
- Wildcard domain routing support (e.g., `*.app.localhost`) for dynamic subdomains
- Initial release of DevProxy
- Certificate Authority (CA) management with automatic trust store integration
- Automatic TLS certificate generation for local domains
- HTTP/HTTPS reverse proxy with virtual host routing
- TCP proxy with SNI-based routing for non-HTTP protocols
- PostgreSQL SSLRequest protocol support for SSL-preferred client connections
- Built-in DNS server for `.localhost` domain resolution
- Docker integration with automatic container discovery via labels
- Daemon mode with graceful shutdown and signal handling
- Configuration hot-reload via file watcher (automatic on config file changes)
- Hot-reloadable settings: log level, DNS domains, DNS upstream, access logging
- CLI commands: start, stop, restart, status, setup, teardown, check, logs
- macOS support with launchd integration
- Linux support with systemd integration
- Configurable entrypoints for PostgreSQL, MySQL, MongoDB, Redis, and custom TCP services

### Fixed

- Reverse proxy now preserves original Host header for backend requests
- Route registry deadlock when removing routes on container stop
- Port availability check now correctly distinguishes between "in use" and "needs sudo"
- Daemon stop command now waits for process to actually terminate

### Changed

- DNS server uses unprivileged port 15353 by default (resolver configured via setup)
- Simplified CLI by removing redundant commands (ca, config, domain, dns, reload)
- Label prefix is now hardcoded to "devproxy" (removed from config)
- Access logging is now hot-reloadable and disabled by default

### Security

- Private CA keys stored with restricted permissions (0600)
- Certificates generated with proper key usage extensions
- Support for both RSA and ECDSA certificate types

## [0.0.1] - 2025-12-21

### Added

- Initial development release
