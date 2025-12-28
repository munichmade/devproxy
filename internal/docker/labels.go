// Package docker provides Docker integration for the dev proxy.
package docker

import (
	"fmt"
	"strconv"
	"strings"
)

// LabelPrefix is the prefix used for all devproxy Docker labels.
const LabelPrefix = "devproxy"

// ServiceConfig represents a parsed service configuration from Docker labels.
type ServiceConfig struct {
	// Name is the service name (for multi-service configs) or empty for single service.
	Name string

	// Host is the hostname to route to this service (required).
	Host string

	// Port is the container port to forward to (defaults to 80 for HTTP).
	Port int

	// Entrypoint specifies which TCP entrypoint to use (empty for HTTP).
	// Examples: "postgres", "mongo", "redis"
	Entrypoint string
}

// LabelParser parses Docker container labels into service configurations.
type LabelParser struct {
	prefix string
}

// NewLabelParser creates a new label parser with the devproxy prefix.
func NewLabelParser() *LabelParser {
	return &LabelParser{prefix: LabelPrefix}
}

// ParseLabels parses container labels and returns service configurations.
// Returns nil if devproxy is not enabled for this container.
func (p *LabelParser) ParseLabels(labels map[string]string) ([]ServiceConfig, error) {
	// Check if devproxy is enabled
	enableKey := p.prefix + ".enable"
	if labels[enableKey] != "true" {
		return nil, nil
	}

	// Check for multi-service configuration
	servicesPrefix := p.prefix + ".services."
	hasMultiService := false
	for key := range labels {
		if strings.HasPrefix(key, servicesPrefix) {
			hasMultiService = true
			break
		}
	}

	if hasMultiService {
		return p.parseMultiService(labels)
	}

	return p.parseSingleService(labels)
}

// parseSingleService parses simple single-service labels.
func (p *LabelParser) parseSingleService(labels map[string]string) ([]ServiceConfig, error) {
	hostKey := p.prefix + ".host"
	portKey := p.prefix + ".port"
	entrypointKey := p.prefix + ".entrypoint"

	host := labels[hostKey]
	if host == "" {
		return nil, fmt.Errorf("missing required label: %s", hostKey)
	}

	// Validate host (may be comma-separated for multiple hosts)
	for _, h := range strings.Split(host, ",") {
		h = strings.TrimSpace(h)
		if err := validateHost(h); err != nil {
			return nil, fmt.Errorf("invalid host in label %s: %w", hostKey, err)
		}
	}

	config := ServiceConfig{
		Host:       host,
		Port:       80, // Default port
		Entrypoint: labels[entrypointKey],
	}

	// Parse port if specified
	if portStr := labels[portKey]; portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return nil, fmt.Errorf("invalid port value %q: %w", portStr, err)
		}
		if port < 1 || port > 65535 {
			return nil, fmt.Errorf("port %d out of valid range (1-65535)", port)
		}
		config.Port = port
	}

	return []ServiceConfig{config}, nil
}

// parseMultiService parses multi-service labels.
func (p *LabelParser) parseMultiService(labels map[string]string) ([]ServiceConfig, error) {
	servicesPrefix := p.prefix + ".services."

	// Collect all service names
	services := make(map[string]map[string]string)

	for key, value := range labels {
		if !strings.HasPrefix(key, servicesPrefix) {
			continue
		}

		// Parse: devproxy.services.<name>.<field>
		rest := strings.TrimPrefix(key, servicesPrefix)
		parts := strings.SplitN(rest, ".", 2)
		if len(parts) != 2 {
			continue
		}

		serviceName := parts[0]
		field := parts[1]

		if services[serviceName] == nil {
			services[serviceName] = make(map[string]string)
		}
		services[serviceName][field] = value
	}

	if len(services) == 0 {
		return nil, fmt.Errorf("no valid service configurations found")
	}

	// Parse each service
	var configs []ServiceConfig
	for name, fields := range services {
		host := fields["host"]
		if host == "" {
			return nil, fmt.Errorf("service %q missing required field: host", name)
		}

		// Validate host (may be comma-separated for multiple hosts)
		for _, h := range strings.Split(host, ",") {
			h = strings.TrimSpace(h)
			if err := validateHost(h); err != nil {
				return nil, fmt.Errorf("service %q has invalid host: %w", name, err)
			}
		}

		config := ServiceConfig{
			Name:       name,
			Host:       host,
			Port:       80,
			Entrypoint: fields["entrypoint"],
		}

		// Parse port if specified
		if portStr := fields["port"]; portStr != "" {
			port, err := strconv.Atoi(portStr)
			if err != nil {
				return nil, fmt.Errorf("service %q has invalid port %q: %w", name, portStr, err)
			}
			if port < 1 || port > 65535 {
				return nil, fmt.Errorf("service %q port %d out of valid range (1-65535)", name, port)
			}
			config.Port = port
		}

		configs = append(configs, config)
	}

	return configs, nil
}

// IsEnabled checks if devproxy is enabled for the given labels.
func (p *LabelParser) IsEnabled(labels map[string]string) bool {
	enableKey := p.prefix + ".enable"
	return labels[enableKey] == "true"
}

// isValidWildcard validates wildcard host syntax (e.g., "*.app.localhost").
func isValidWildcard(host string) bool {
	if !strings.HasPrefix(host, "*.") {
		return false
	}
	pattern := strings.TrimPrefix(host, "*.")
	// Must have at least one character after "*." and not start with another dot
	return len(pattern) > 0 && !strings.HasPrefix(pattern, ".")
}

// isValidHost validates a host string (exact or wildcard).
func isValidHost(host string) bool {
	if host == "" {
		return false
	}

	// Check for wildcard pattern
	if strings.HasPrefix(host, "*") {
		return isValidWildcard(host)
	}

	// Basic validation for exact hosts: not empty, doesn't start/end with dot
	return !strings.HasPrefix(host, ".") && !strings.HasSuffix(host, ".")
}

// validateHost validates a host and returns an error if invalid.
func validateHost(host string) error {
	if host == "" {
		return fmt.Errorf("host cannot be empty")
	}

	if strings.HasPrefix(host, "*") {
		if !isValidWildcard(host) {
			return fmt.Errorf("invalid wildcard host %q: must be in format *.domain.tld", host)
		}
		return nil
	}

	if !isValidHost(host) {
		return fmt.Errorf("invalid host %q", host)
	}

	return nil
}
