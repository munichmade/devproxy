// Package proxy provides HTTP and TCP proxy functionality.
package proxy

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
)

// TLS record types
const (
	tlsRecordTypeHandshake = 22
)

// TLS handshake types
const (
	tlsHandshakeTypeClientHello = 1
)

// TLS extension types
const (
	tlsExtensionSNI = 0x0000
)

// SNI name types
const (
	sniNameTypeHostname = 0
)

var (
	// ErrNotTLS is returned when the connection doesn't appear to be TLS.
	ErrNotTLS = errors.New("not a TLS connection")

	// ErrInvalidClientHello is returned when the ClientHello is malformed.
	ErrInvalidClientHello = errors.New("invalid ClientHello message")
)

// PeekedConn wraps a net.Conn and prepends peeked bytes to reads.
// This allows replaying the peeked bytes to the backend.
type PeekedConn struct {
	net.Conn
	peeked []byte
	offset int
}

// NewPeekedConn creates a new PeekedConn that will first return the peeked
// bytes before reading from the underlying connection.
func NewPeekedConn(conn net.Conn, peeked []byte) *PeekedConn {
	return &PeekedConn{
		Conn:   conn,
		peeked: peeked,
		offset: 0,
	}
}

// Read implements io.Reader, first returning peeked bytes then reading from conn.
func (p *PeekedConn) Read(b []byte) (int, error) {
	if p.offset < len(p.peeked) {
		n := copy(b, p.peeked[p.offset:])
		p.offset += n
		return n, nil
	}
	return p.Conn.Read(b)
}

// ExtractSNI reads the TLS ClientHello from conn and extracts the SNI hostname.
// It returns the hostname (empty string if no SNI), the peeked bytes that were
// read (for replay to backend), and any error.
//
// The peeked bytes should be prepended to any data sent to the backend using
// NewPeekedConn.
func ExtractSNI(conn net.Conn) (hostname string, peeked []byte, err error) {
	reader := bufio.NewReader(conn)

	// Read TLS record header (5 bytes)
	// Content Type (1) + Version (2) + Length (2)
	header, err := reader.Peek(5)
	if err != nil {
		if err == io.EOF {
			return "", nil, ErrNotTLS
		}
		return "", nil, fmt.Errorf("reading TLS header: %w", err)
	}

	// Verify it's a TLS handshake record
	if header[0] != tlsRecordTypeHandshake {
		return "", nil, ErrNotTLS
	}

	// Get record length
	recordLen := int(binary.BigEndian.Uint16(header[3:5]))
	if recordLen < 4 || recordLen > 16384 {
		return "", nil, ErrInvalidClientHello
	}

	// Peek the entire TLS record
	totalLen := 5 + recordLen
	peeked, err = reader.Peek(totalLen)
	if err != nil {
		return "", nil, fmt.Errorf("reading TLS record: %w", err)
	}

	// Now consume the bytes we peeked so they're not read again
	consumed := make([]byte, totalLen)
	_, err = io.ReadFull(reader, consumed)
	if err != nil {
		return "", peeked, fmt.Errorf("consuming peeked bytes: %w", err)
	}

	// Parse the handshake message
	hostname, err = parseClientHello(peeked[5:])
	if err != nil {
		// Return peeked bytes even on parse error for passthrough
		return "", peeked, err
	}

	return hostname, peeked, nil
}

// parseClientHello parses a TLS ClientHello message and extracts the SNI.
func parseClientHello(data []byte) (string, error) {
	if len(data) < 4 {
		return "", ErrInvalidClientHello
	}

	// Handshake type (1) + Length (3)
	if data[0] != tlsHandshakeTypeClientHello {
		return "", ErrInvalidClientHello
	}

	handshakeLen := int(data[1])<<16 | int(data[2])<<8 | int(data[3])
	if len(data) < 4+handshakeLen {
		return "", ErrInvalidClientHello
	}

	// Move past handshake header
	pos := 4

	// ClientHello structure:
	// Version (2) + Random (32) + Session ID (1 + variable) +
	// Cipher Suites (2 + variable) + Compression Methods (1 + variable) +
	// Extensions (2 + variable)

	// Skip version (2 bytes)
	if pos+2 > len(data) {
		return "", ErrInvalidClientHello
	}
	pos += 2

	// Skip random (32 bytes)
	if pos+32 > len(data) {
		return "", ErrInvalidClientHello
	}
	pos += 32

	// Skip session ID
	if pos+1 > len(data) {
		return "", ErrInvalidClientHello
	}
	sessionIDLen := int(data[pos])
	pos++
	if pos+sessionIDLen > len(data) {
		return "", ErrInvalidClientHello
	}
	pos += sessionIDLen

	// Skip cipher suites
	if pos+2 > len(data) {
		return "", ErrInvalidClientHello
	}
	cipherSuitesLen := int(binary.BigEndian.Uint16(data[pos : pos+2]))
	pos += 2
	if pos+cipherSuitesLen > len(data) {
		return "", ErrInvalidClientHello
	}
	pos += cipherSuitesLen

	// Skip compression methods
	if pos+1 > len(data) {
		return "", ErrInvalidClientHello
	}
	compressionLen := int(data[pos])
	pos++
	if pos+compressionLen > len(data) {
		return "", ErrInvalidClientHello
	}
	pos += compressionLen

	// Check if we have extensions
	if pos+2 > len(data) {
		// No extensions, no SNI
		return "", nil
	}

	extensionsLen := int(binary.BigEndian.Uint16(data[pos : pos+2]))
	pos += 2
	if pos+extensionsLen > len(data) {
		return "", ErrInvalidClientHello
	}

	// Parse extensions
	extensionsEnd := pos + extensionsLen
	for pos+4 <= extensionsEnd {
		extType := binary.BigEndian.Uint16(data[pos : pos+2])
		extLen := int(binary.BigEndian.Uint16(data[pos+2 : pos+4]))
		pos += 4

		if pos+extLen > extensionsEnd {
			return "", ErrInvalidClientHello
		}

		if extType == tlsExtensionSNI {
			return parseSNIExtension(data[pos : pos+extLen])
		}

		pos += extLen
	}

	// No SNI extension found
	return "", nil
}

// parseSNIExtension extracts the hostname from an SNI extension.
func parseSNIExtension(data []byte) (string, error) {
	if len(data) < 2 {
		return "", ErrInvalidClientHello
	}

	// SNI list length
	listLen := int(binary.BigEndian.Uint16(data[0:2]))
	if len(data) < 2+listLen {
		return "", ErrInvalidClientHello
	}

	pos := 2
	listEnd := 2 + listLen

	for pos+3 <= listEnd {
		nameType := data[pos]
		nameLen := int(binary.BigEndian.Uint16(data[pos+1 : pos+3]))
		pos += 3

		if pos+nameLen > listEnd {
			return "", ErrInvalidClientHello
		}

		if nameType == sniNameTypeHostname {
			return string(data[pos : pos+nameLen]), nil
		}

		pos += nameLen
	}

	return "", nil
}
