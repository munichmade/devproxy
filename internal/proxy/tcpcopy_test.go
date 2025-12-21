package proxy

import (
	"bytes"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

func TestProxyTCP(t *testing.T) {
	t.Run("copies data bidirectionally", func(t *testing.T) {
		// Use real TCP connections to test properly
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to create listener: %v", err)
		}
		defer listener.Close()

		backendListener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to create backend listener: %v", err)
		}
		defer backendListener.Close()

		// Accept client connection
		var clientServerConn net.Conn
		var backendConn net.Conn
		var acceptWg sync.WaitGroup
		acceptWg.Add(2)

		go func() {
			defer acceptWg.Done()
			clientServerConn, _ = listener.Accept()
		}()

		go func() {
			defer acceptWg.Done()
			backendConn, _ = backendListener.Accept()
		}()

		// Client connects
		clientConn, err := net.Dial("tcp", listener.Addr().String())
		if err != nil {
			t.Fatalf("client failed to connect: %v", err)
		}
		defer clientConn.Close()

		// Proxy connects to backend
		backendClientConn, err := net.Dial("tcp", backendListener.Addr().String())
		if err != nil {
			t.Fatalf("failed to connect to backend: %v", err)
		}
		defer backendClientConn.Close()

		acceptWg.Wait()
		defer clientServerConn.Close()
		defer backendConn.Close()

		// Start proxy
		var proxyErr error
		proxyDone := make(chan struct{})
		go func() {
			proxyErr = ProxyTCP(clientServerConn, backendClientConn)
			close(proxyDone)
		}()

		// Client sends data
		clientData := []byte("hello from client")
		_, err = clientConn.Write(clientData)
		if err != nil {
			t.Fatalf("client write failed: %v", err)
		}

		// Backend receives data
		received := make([]byte, 100)
		n, err := backendConn.Read(received)
		if err != nil {
			t.Fatalf("backend read failed: %v", err)
		}
		if !bytes.Equal(received[:n], clientData) {
			t.Errorf("backend received %q, want %q", received[:n], clientData)
		}

		// Backend sends response
		backendData := []byte("hello from backend")
		_, err = backendConn.Write(backendData)
		if err != nil {
			t.Fatalf("backend write failed: %v", err)
		}

		// Client receives response
		n, err = clientConn.Read(received)
		if err != nil {
			t.Fatalf("client read failed: %v", err)
		}
		if !bytes.Equal(received[:n], backendData) {
			t.Errorf("client received %q, want %q", received[:n], backendData)
		}

		// Close connections to end proxy
		clientConn.Close()
		backendConn.Close()

		// Wait for proxy to complete
		select {
		case <-proxyDone:
			// OK
		case <-time.After(2 * time.Second):
			t.Fatal("proxy did not complete in time")
		}

		if proxyErr != nil {
			t.Errorf("ProxyTCP returned error: %v", proxyErr)
		}
	})

	t.Run("handles empty transfer", func(t *testing.T) {
		clientConn, clientBackend := net.Pipe()
		backendConn, backendFrontend := net.Pipe()

		proxyDone := make(chan error, 1)
		go func() {
			proxyDone <- ProxyTCP(clientBackend, backendFrontend)
		}()

		// Close both ends immediately
		clientConn.Close()
		backendConn.Close()

		select {
		case err := <-proxyDone:
			if err != nil {
				t.Errorf("ProxyTCP returned error: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("proxy did not complete in time")
		}
	})
}

func TestProxyTCPWithStats(t *testing.T) {
	t.Run("returns byte counts", func(t *testing.T) {
		// Create two TCP listener/connection pairs to simulate full proxy
		clientListener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer clientListener.Close()

		backendListener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer backendListener.Close()

		// Accept connections in background
		clientServerCh := make(chan net.Conn, 1)
		backendServerCh := make(chan net.Conn, 1)

		go func() {
			conn, _ := clientListener.Accept()
			clientServerCh <- conn
		}()
		go func() {
			conn, _ := backendListener.Accept()
			backendServerCh <- conn
		}()

		// Connect client and backend
		clientConn, err := net.Dial("tcp", clientListener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		defer clientConn.Close()

		backendConn, err := net.Dial("tcp", backendListener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		defer backendConn.Close()

		clientServer := <-clientServerCh
		backendServer := <-backendServerCh
		defer clientServer.Close()
		defer backendServer.Close()

		resultCh := make(chan ProxyResult, 1)
		go func() {
			resultCh <- ProxyTCPWithStats(clientServer, backendConn)
		}()

		// Client sends 100 bytes
		go func() {
			clientData := make([]byte, 100)
			clientConn.Write(clientData)
			clientConn.(*net.TCPConn).CloseWrite()
			io.Copy(io.Discard, clientConn)
		}()

		// Backend reads all, sends 200 bytes, then closes
		go func() {
			io.Copy(io.Discard, backendServer)
			backendData := make([]byte, 200)
			backendServer.Write(backendData)
			backendServer.(*net.TCPConn).CloseWrite()
		}()

		select {
		case result := <-resultCh:
			if result.ClientToBackend != 100 {
				t.Errorf("ClientToBackend = %d, want 100", result.ClientToBackend)
			}
			if result.BackendToClient != 200 {
				t.Errorf("BackendToClient = %d, want 200", result.BackendToClient)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("proxy did not complete in time")
		}
	})
}

func TestProxyTCPWithTimeout(t *testing.T) {
	t.Run("completes within timeout", func(t *testing.T) {
		// Create two TCP listener/connection pairs
		clientListener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer clientListener.Close()

		backendListener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer backendListener.Close()

		clientServerCh := make(chan net.Conn, 1)
		backendServerCh := make(chan net.Conn, 1)

		go func() {
			conn, _ := clientListener.Accept()
			clientServerCh <- conn
		}()
		go func() {
			conn, _ := backendListener.Accept()
			backendServerCh <- conn
		}()

		clientConn, err := net.Dial("tcp", clientListener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		defer clientConn.Close()

		backendConn, err := net.Dial("tcp", backendListener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		defer backendConn.Close()

		clientServer := <-clientServerCh
		backendServer := <-backendServerCh
		defer clientServer.Close()
		defer backendServer.Close()

		proxyDone := make(chan error, 1)
		go func() {
			proxyDone <- ProxyTCPWithTimeout(clientServer, backendConn, 5*time.Second)
		}()

		// Quick transfer then close
		go func() {
			clientConn.Write([]byte("hello"))
			clientConn.(*net.TCPConn).CloseWrite()
			io.Copy(io.Discard, clientConn)
		}()

		go func() {
			io.Copy(io.Discard, backendServer)
			backendServer.(*net.TCPConn).CloseWrite()
		}()

		select {
		case err := <-proxyDone:
			if err != nil {
				t.Errorf("ProxyTCPWithTimeout returned error: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("proxy did not complete in time")
		}
	})

	t.Run("times out on idle connection", func(t *testing.T) {
		clientConn, clientBackend := net.Pipe()
		backendConn, backendFrontend := net.Pipe()

		done := make(chan struct{})
		go func() {
			ProxyTCPWithTimeout(clientBackend, backendFrontend, 50*time.Millisecond)
			close(done)
		}()

		select {
		case <-done:
			// Expected - timed out
		case <-time.After(1 * time.Second):
			t.Error("expected timeout but proxy is still running")
		}

		clientConn.Close()
		backendConn.Close()
	})
}

func TestIsNormalClose(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, true},
		{"EOF", io.EOF, true},
		{"other error", io.ErrUnexpectedEOF, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isNormalClose(tt.err); got != tt.expected {
				t.Errorf("isNormalClose(%v) = %v, want %v", tt.err, got, tt.expected)
			}
		})
	}
}

func TestCloseWrite(t *testing.T) {
	t.Run("handles TCP connection", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to create listener: %v", err)
		}
		defer listener.Close()

		var serverConn net.Conn
		accepted := make(chan struct{})
		go func() {
			serverConn, _ = listener.Accept()
			close(accepted)
		}()

		clientConn, err := net.Dial("tcp", listener.Addr().String())
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}
		defer clientConn.Close()

		<-accepted
		defer serverConn.Close()

		// closeWrite should not panic
		closeWrite(clientConn)
	})

	t.Run("handles pipe connection", func(t *testing.T) {
		conn1, conn2 := net.Pipe()
		defer conn1.Close()
		defer conn2.Close()

		// closeWrite should not panic on pipe
		closeWrite(conn1)
	})
}
