# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial release of DevProxy
- Certificate Authority (CA) management with automatic trust store integration
- Automatic TLS certificate generation for local domains
- HTTP/HTTPS reverse proxy with virtual host routing
- TCP proxy with SNI-based routing for non-HTTP protocols
- Built-in DNS server for `.localhost` domain resolution
- Docker integration with automatic container discovery via labels
- Daemon mode with graceful shutdown and signal handling
- Configuration hot-reload via SIGHUP
- CLI commands: start, stop, restart, status, reload, setup, check
- macOS support with launchd integration
- Linux support with systemd integration and systemd-resolved configuration
- Configurable entrypoints for PostgreSQL, MySQL, Redis, and custom TCP services

### Security
- Private CA keys stored with restricted permissions (0600)
- Certificates generated with proper key usage extensions
- Support for both RSA and ECDSA certificate types

## [0.1.0] - 2024-12-21

### Added
- Initial development release
