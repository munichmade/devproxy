// Package ca provides Certificate Authority functionality for generating
// and managing local development certificates.
package ca

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/munichmade/devproxy/internal/paths"
)

const (
	// CACertFilename is the filename for the CA certificate.
	CACertFilename = "root-ca.pem"
	// CAKeyFilename is the filename for the CA private key.
	CAKeyFilename = "root-ca-key.pem"

	// caValidityYears is how long the CA certificate is valid.
	caValidityYears = 1

	// caOrganization is the organization name in the CA certificate.
	caOrganization = "DevProxy Local CA"
	// caCommonName is the common name in the CA certificate.
	caCommonName = "DevProxy Local CA"
)

// CA represents a Certificate Authority with its certificate and private key.
type CA struct {
	Certificate *x509.Certificate
	PrivateKey  *ecdsa.PrivateKey

	// Raw PEM-encoded data for convenience
	CertPEM []byte
	KeyPEM  []byte
}

// Exists checks if the CA certificate and key files exist.
func Exists() bool {
	certPath := filepath.Join(paths.CADir(), CACertFilename)
	keyPath := filepath.Join(paths.CADir(), CAKeyFilename)

	_, certErr := os.Stat(certPath)
	_, keyErr := os.Stat(keyPath)

	return certErr == nil && keyErr == nil
}

// Generate creates a new CA certificate and private key.
// It saves the files to the CA directory with appropriate permissions.
func Generate() (*CA, error) {
	// Ensure CA directory exists
	if err := os.MkdirAll(paths.CADir(), 0o700); err != nil {
		return nil, fmt.Errorf("failed to create CA directory: %w", err)
	}

	// Generate ECDSA P-384 private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Generate serial number
	serialNumber, err := generateSerialNumber()
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	// Create certificate template
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{caOrganization},
			CommonName:   caCommonName,
		},
		NotBefore:             now,
		NotAfter:              now.AddDate(caValidityYears, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}

	// Self-sign the certificate
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	// Parse the certificate back to get the x509.Certificate object
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Encode certificate to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	// Encode private key to PEM
	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyDER,
	})

	// Save certificate (world-readable)
	certPath := filepath.Join(paths.CADir(), CACertFilename)
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return nil, fmt.Errorf("failed to write certificate: %w", err)
	}

	// Save private key (owner-only)
	keyPath := filepath.Join(paths.CADir(), CAKeyFilename)
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		// Clean up certificate if key write fails
		os.Remove(certPath)
		return nil, fmt.Errorf("failed to write private key: %w", err)
	}

	return &CA{
		Certificate: cert,
		PrivateKey:  privateKey,
		CertPEM:     certPEM,
		KeyPEM:      keyPEM,
	}, nil
}

// Load reads an existing CA from the CA directory.
func Load() (*CA, error) {
	certPath := filepath.Join(paths.CADir(), CACertFilename)
	keyPath := filepath.Join(paths.CADir(), CAKeyFilename)

	// Read certificate
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read certificate: %w", err)
	}

	// Read private key
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %w", err)
	}

	// Parse certificate
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return nil, errors.New("failed to decode certificate PEM")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Parse private key
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, errors.New("failed to decode private key PEM")
	}
	privateKey, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	return &CA{
		Certificate: cert,
		PrivateKey:  privateKey,
		CertPEM:     certPEM,
		KeyPEM:      keyPEM,
	}, nil
}

// LoadOrGenerate loads an existing CA or generates a new one if none exists.
func LoadOrGenerate() (*CA, error) {
	if Exists() {
		return Load()
	}
	return Generate()
}

// CertPath returns the full path to the CA certificate file.
func CertPath() string {
	return filepath.Join(paths.CADir(), CACertFilename)
}

// KeyPath returns the full path to the CA private key file.
func KeyPath() string {
	return filepath.Join(paths.CADir(), CAKeyFilename)
}

// generateSerialNumber creates a random serial number for certificates.
func generateSerialNumber() (*big.Int, error) {
	// Serial number should be at most 20 octets (160 bits)
	// We use 128 bits for safety margin
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, serialNumberLimit)
}
