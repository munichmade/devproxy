// Package cert provides wildcard certificate generation and management
// for on-demand TLS certificate issuance.
package cert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/munichmade/devproxy/internal/ca"
	"github.com/munichmade/devproxy/internal/paths"
)

const (
	// certValidityDays is how long generated certificates are valid.
	certValidityDays = 30

	// renewBeforeDays is how many days before expiry to renew.
	renewBeforeDays = 7

	// certFileSuffix is the file extension for certificate files.
	certFileSuffix = ".pem"

	// keyFileSuffix is the file extension for key files.
	keyFileSuffix = "-key.pem"
)

var (
	// ErrNoCA is returned when the CA is not available.
	ErrNoCA = errors.New("CA not available - run 'devproxy ca generate' first")

	// ErrInvalidDomain is returned when a domain name is invalid.
	ErrInvalidDomain = errors.New("invalid domain name")
)

// Manager handles certificate generation and caching.
type Manager struct {
	ca    *ca.CA
	mu    sync.RWMutex
	cache map[string]*tls.Certificate
}

// NewManager creates a new certificate manager.
// It loads the CA from disk; returns an error if the CA doesn't exist.
func NewManager() (*Manager, error) {
	rootCA, err := ca.Load()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoCA, err)
	}

	// Ensure certs directory exists
	if err := os.MkdirAll(paths.CertsDir(), 0700); err != nil {
		return nil, fmt.Errorf("failed to create certs directory: %w", err)
	}

	return &Manager{
		ca:    rootCA,
		cache: make(map[string]*tls.Certificate),
	}, nil
}

// GetCertificate returns a certificate for the given domain.
// This is designed to be used as tls.Config.GetCertificate.
// It generates wildcard certificates for subdomains (e.g., *.example.localhost).
func (m *Manager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	domain := hello.ServerName
	if domain == "" {
		return nil, ErrInvalidDomain
	}

	// Normalize domain and determine wildcard base
	domain = strings.ToLower(domain)
	wildcardDomain := toWildcard(domain)

	// Check memory cache first
	m.mu.RLock()
	if cert, ok := m.cache[wildcardDomain]; ok {
		m.mu.RUnlock()
		// Check if still valid
		if isValid(cert) {
			return cert, nil
		}
	} else {
		m.mu.RUnlock()
	}

	// Try to load from disk
	cert, err := m.loadFromDisk(wildcardDomain)
	if err == nil && isValid(cert) {
		m.mu.Lock()
		m.cache[wildcardDomain] = cert
		m.mu.Unlock()
		return cert, nil
	}

	// Generate new certificate
	cert, err = m.generate(wildcardDomain, domain)
	if err != nil {
		return nil, err
	}

	// Cache in memory
	m.mu.Lock()
	m.cache[wildcardDomain] = cert
	m.mu.Unlock()

	return cert, nil
}

// generate creates a new certificate for the given domain.
func (m *Manager) generate(wildcardDomain, originalDomain string) (*tls.Certificate, error) {
	// Generate ECDSA P-256 private key (faster than P-384 for leaf certs)
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Generate serial number
	serialNumber, err := generateSerialNumber()
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	// Build DNS names for SAN
	dnsNames := buildDNSNames(wildcardDomain, originalDomain)

	// Create certificate template
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"DevProxy"},
			CommonName:   wildcardDomain,
		},
		NotBefore:             now,
		NotAfter:              now.AddDate(0, 0, certValidityDays),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
	}

	// Sign with CA
	certDER, err := x509.CreateCertificate(
		rand.Reader,
		template,
		m.ca.Certificate,
		&privateKey.PublicKey,
		m.ca.PrivateKey,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	// Encode to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyDER,
	})

	// Save to disk
	if err := m.saveToDisk(wildcardDomain, certPEM, keyPEM); err != nil {
		// Log but don't fail - we can still use the cert in memory
		fmt.Fprintf(os.Stderr, "warning: failed to cache certificate: %v\n", err)
	}

	// Create tls.Certificate
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to create TLS certificate: %w", err)
	}

	return &tlsCert, nil
}

// loadFromDisk attempts to load a certificate from the disk cache.
func (m *Manager) loadFromDisk(wildcardDomain string) (*tls.Certificate, error) {
	filename := domainToFilename(wildcardDomain)
	certPath := filepath.Join(paths.CertsDir(), filename+certFileSuffix)
	keyPath := filepath.Join(paths.CertsDir(), filename+keyFileSuffix)

	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}

	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}

	return &tlsCert, nil
}

// saveToDisk saves a certificate to the disk cache.
func (m *Manager) saveToDisk(wildcardDomain string, certPEM, keyPEM []byte) error {
	filename := domainToFilename(wildcardDomain)
	certPath := filepath.Join(paths.CertsDir(), filename+certFileSuffix)
	keyPath := filepath.Join(paths.CertsDir(), filename+keyFileSuffix)

	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		return fmt.Errorf("failed to write certificate: %w", err)
	}

	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		os.Remove(certPath) // Clean up
		return fmt.Errorf("failed to write private key: %w", err)
	}

	return nil
}

// toWildcard converts a domain to its wildcard form.
// e.g., "api.example.localhost" -> "*.example.localhost"
// e.g., "example.localhost" -> "example.localhost" (no wildcard for TLD+1)
func toWildcard(domain string) string {
	parts := strings.Split(domain, ".")
	if len(parts) <= 2 {
		// e.g., "example.localhost" - no wildcard
		return domain
	}
	// e.g., "api.example.localhost" -> "*.example.localhost"
	return "*." + strings.Join(parts[1:], ".")
}

// buildDNSNames creates the list of DNS names for the certificate SAN.
func buildDNSNames(wildcardDomain, originalDomain string) []string {
	names := make(map[string]bool)

	// Add the wildcard domain
	names[wildcardDomain] = true

	// Add the base domain (without wildcard prefix)
	if strings.HasPrefix(wildcardDomain, "*.") {
		baseDomain := wildcardDomain[2:]
		names[baseDomain] = true
	}

	// Add the original requested domain
	names[originalDomain] = true

	// Convert to slice
	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	return result
}

// domainToFilename converts a domain to a safe filename.
func domainToFilename(domain string) string {
	// Replace * with _wildcard_ and use hash for long domains
	safe := strings.ReplaceAll(domain, "*", "_wildcard_")
	safe = strings.ReplaceAll(safe, ":", "_")

	if len(safe) > 200 {
		// Use hash for very long domains
		hash := sha256.Sum256([]byte(domain))
		safe = hex.EncodeToString(hash[:16])
	}

	return safe
}

// isValid checks if a certificate is still valid and not expiring soon.
func isValid(cert *tls.Certificate) bool {
	if cert == nil || len(cert.Certificate) == 0 {
		return false
	}

	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return false
	}

	// Check if expired or expiring soon
	renewTime := x509Cert.NotAfter.AddDate(0, 0, -renewBeforeDays)
	return time.Now().Before(renewTime)
}

// generateSerialNumber creates a random serial number for certificates.
func generateSerialNumber() (*big.Int, error) {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, serialNumberLimit)
}

// ClearCache removes all cached certificates from memory and disk.
func (m *Manager) ClearCache() error {
	m.mu.Lock()
	m.cache = make(map[string]*tls.Certificate)
	m.mu.Unlock()

	// Remove all files in certs directory
	entries, err := os.ReadDir(paths.CertsDir())
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			os.Remove(filepath.Join(paths.CertsDir(), entry.Name()))
		}
	}

	return nil
}
