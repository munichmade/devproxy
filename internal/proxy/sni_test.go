package proxy

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"
)

// mockConn implements net.Conn for testing
type mockConn struct {
	reader *bytes.Reader
	closed bool
}

func newMockConn(data []byte) *mockConn {
	return &mockConn{
		reader: bytes.NewReader(data),
	}
}

func (m *mockConn) Read(b []byte) (int, error) {
	return m.reader.Read(b)
}

func (m *mockConn) Write(b []byte) (int, error) {
	return len(b), nil
}

func (m *mockConn) Close() error {
	m.closed = true
	return nil
}

func (m *mockConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 443}
}

func (m *mockConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 12345}
}

func (m *mockConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }

// buildClientHello constructs a minimal TLS ClientHello with SNI
func buildClientHello(hostname string) []byte {
	// Build SNI extension
	var sniExt []byte
	if hostname != "" {
		hostnameBytes := []byte(hostname)
		// SNI extension data: list length (2) + type (1) + name length (2) + name
		sniData := make([]byte, 2+1+2+len(hostnameBytes))
		// List length
		listLen := 1 + 2 + len(hostnameBytes)
		sniData[0] = byte(listLen >> 8)
		sniData[1] = byte(listLen)
		// Name type (0 = hostname)
		sniData[2] = 0
		// Name length
		sniData[3] = byte(len(hostnameBytes) >> 8)
		sniData[4] = byte(len(hostnameBytes))
		// Name
		copy(sniData[5:], hostnameBytes)

		// Extension header: type (2) + length (2) + data
		sniExt = make([]byte, 4+len(sniData))
		sniExt[0] = 0x00 // SNI extension type high byte
		sniExt[1] = 0x00 // SNI extension type low byte
		sniExt[2] = byte(len(sniData) >> 8)
		sniExt[3] = byte(len(sniData))
		copy(sniExt[4:], sniData)
	}

	// Build extensions block
	extensions := sniExt
	extensionsLen := len(extensions)

	// Build ClientHello body
	// Version (2) + Random (32) + Session ID (1) + Cipher Suites (4) + Compression (2) + Extensions
	clientHelloBody := make([]byte, 2+32+1+4+2+2+extensionsLen)
	pos := 0

	// Version: TLS 1.2
	clientHelloBody[pos] = 0x03
	clientHelloBody[pos+1] = 0x03
	pos += 2

	// Random (32 bytes of zeros)
	pos += 32

	// Session ID length (0)
	clientHelloBody[pos] = 0
	pos++

	// Cipher suites length (2) + one cipher suite
	clientHelloBody[pos] = 0x00
	clientHelloBody[pos+1] = 0x02
	clientHelloBody[pos+2] = 0x00
	clientHelloBody[pos+3] = 0x2f // TLS_RSA_WITH_AES_128_CBC_SHA
	pos += 4

	// Compression methods length (1) + null compression
	clientHelloBody[pos] = 0x01
	clientHelloBody[pos+1] = 0x00
	pos += 2

	// Extensions length
	clientHelloBody[pos] = byte(extensionsLen >> 8)
	clientHelloBody[pos+1] = byte(extensionsLen)
	pos += 2

	// Extensions
	copy(clientHelloBody[pos:], extensions)

	// Build Handshake message
	// Type (1) + Length (3) + ClientHello body
	handshake := make([]byte, 4+len(clientHelloBody))
	handshake[0] = 0x01 // ClientHello
	handshakeLen := len(clientHelloBody)
	handshake[1] = byte(handshakeLen >> 16)
	handshake[2] = byte(handshakeLen >> 8)
	handshake[3] = byte(handshakeLen)
	copy(handshake[4:], clientHelloBody)

	// Build TLS record
	// Content Type (1) + Version (2) + Length (2) + Handshake
	record := make([]byte, 5+len(handshake))
	record[0] = 0x16 // Handshake
	record[1] = 0x03 // TLS 1.0 (for compatibility)
	record[2] = 0x01
	recordLen := len(handshake)
	record[3] = byte(recordLen >> 8)
	record[4] = byte(recordLen)
	copy(record[5:], handshake)

	return record
}

func TestExtractSNI(t *testing.T) {
	t.Run("extracts SNI from valid ClientHello", func(t *testing.T) {
		clientHello := buildClientHello("example.com")
		conn := newMockConn(clientHello)

		hostname, peeked, err := ExtractSNI(conn)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if hostname != "example.com" {
			t.Errorf("expected hostname 'example.com', got '%s'", hostname)
		}

		if len(peeked) != len(clientHello) {
			t.Errorf("expected peeked length %d, got %d", len(clientHello), len(peeked))
		}

		if !bytes.Equal(peeked, clientHello) {
			t.Error("peeked bytes don't match original ClientHello")
		}
	})

	t.Run("extracts SNI with subdomain", func(t *testing.T) {
		clientHello := buildClientHello("api.example.com")
		conn := newMockConn(clientHello)

		hostname, _, err := ExtractSNI(conn)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if hostname != "api.example.com" {
			t.Errorf("expected hostname 'api.example.com', got '%s'", hostname)
		}
	})

	t.Run("returns empty string for missing SNI", func(t *testing.T) {
		clientHello := buildClientHello("")
		conn := newMockConn(clientHello)

		hostname, peeked, err := ExtractSNI(conn)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if hostname != "" {
			t.Errorf("expected empty hostname, got '%s'", hostname)
		}

		if len(peeked) == 0 {
			t.Error("expected peeked bytes even without SNI")
		}
	})

	t.Run("returns error for non-TLS data", func(t *testing.T) {
		// HTTP request instead of TLS
		httpData := []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
		conn := newMockConn(httpData)

		_, _, err := ExtractSNI(conn)
		if err != ErrNotTLS {
			t.Errorf("expected ErrNotTLS, got %v", err)
		}
	})

	t.Run("returns error for empty connection", func(t *testing.T) {
		conn := newMockConn([]byte{})

		_, _, err := ExtractSNI(conn)
		if err != ErrNotTLS {
			t.Errorf("expected ErrNotTLS, got %v", err)
		}
	})

	t.Run("returns error for truncated TLS header", func(t *testing.T) {
		// Only 3 bytes of header
		truncated := []byte{0x16, 0x03, 0x01}
		conn := newMockConn(truncated)

		_, _, err := ExtractSNI(conn)
		if err == nil {
			t.Error("expected error for truncated header")
		}
	})

	t.Run("returns error for invalid record length", func(t *testing.T) {
		// Record length too large (> 16384)
		invalid := []byte{0x16, 0x03, 0x01, 0xFF, 0xFF}
		conn := newMockConn(invalid)

		_, _, err := ExtractSNI(conn)
		if err != ErrInvalidClientHello {
			t.Errorf("expected ErrInvalidClientHello, got %v", err)
		}
	})

	t.Run("handles long hostname", func(t *testing.T) {
		longHostname := "very-long-subdomain.another-subdomain.example.com"
		clientHello := buildClientHello(longHostname)
		conn := newMockConn(clientHello)

		hostname, _, err := ExtractSNI(conn)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if hostname != longHostname {
			t.Errorf("expected hostname '%s', got '%s'", longHostname, hostname)
		}
	})
}

func TestPeekedConn(t *testing.T) {
	t.Run("returns peeked bytes first", func(t *testing.T) {
		peeked := []byte("peeked data")
		remaining := []byte("remaining data")

		innerConn := newMockConn(remaining)
		conn := NewPeekedConn(innerConn, peeked)

		// Read should return peeked bytes first
		buf := make([]byte, 20)
		n, err := conn.Read(buf)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if n != len(peeked) {
			t.Errorf("expected to read %d bytes, got %d", len(peeked), n)
		}

		if string(buf[:n]) != "peeked data" {
			t.Errorf("expected 'peeked data', got '%s'", string(buf[:n]))
		}

		// Next read should return from underlying conn
		n, err = conn.Read(buf)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if string(buf[:n]) != "remaining data" {
			t.Errorf("expected 'remaining data', got '%s'", string(buf[:n]))
		}
	})

	t.Run("handles partial reads of peeked data", func(t *testing.T) {
		peeked := []byte("peeked data")
		innerConn := newMockConn([]byte{})
		conn := NewPeekedConn(innerConn, peeked)

		// Read in small chunks
		buf := make([]byte, 4)
		var result []byte

		for {
			n, err := conn.Read(buf)
			if n > 0 {
				result = append(result, buf[:n]...)
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result) >= len(peeked) {
				break
			}
		}

		if string(result) != "peeked data" {
			t.Errorf("expected 'peeked data', got '%s'", string(result))
		}
	})

	t.Run("preserves underlying connection methods", func(t *testing.T) {
		innerConn := newMockConn([]byte{})
		conn := NewPeekedConn(innerConn, []byte("test"))

		// LocalAddr and RemoteAddr should work
		if conn.LocalAddr() == nil {
			t.Error("LocalAddr should not be nil")
		}
		if conn.RemoteAddr() == nil {
			t.Error("RemoteAddr should not be nil")
		}

		// Close should work
		err := conn.Close()
		if err != nil {
			t.Errorf("Close failed: %v", err)
		}
		if !innerConn.closed {
			t.Error("underlying connection should be closed")
		}
	})
}

func TestParseClientHello(t *testing.T) {
	t.Run("returns error for too short data", func(t *testing.T) {
		_, err := parseClientHello([]byte{0x01, 0x00})
		if err != ErrInvalidClientHello {
			t.Errorf("expected ErrInvalidClientHello, got %v", err)
		}
	})

	t.Run("returns error for wrong handshake type", func(t *testing.T) {
		// Type 2 = ServerHello instead of ClientHello
		data := []byte{0x02, 0x00, 0x00, 0x10}
		_, err := parseClientHello(data)
		if err != ErrInvalidClientHello {
			t.Errorf("expected ErrInvalidClientHello, got %v", err)
		}
	})
}
