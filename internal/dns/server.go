// Package dns provides a DNS server for resolving local development domains.
package dns

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	"github.com/munichmade/devproxy/internal/logging"
)

const (
	// DefaultPort is the default DNS server port.
	DefaultPort = 53

	// DefaultTTL is the default TTL for DNS responses.
	DefaultTTL = 60

	// DefaultUpstream is the default upstream DNS server.
	DefaultUpstream = "8.8.8.8:53"
)

// Server is a DNS server that resolves local development domains.
type Server struct {
	// addr is the address to listen on (e.g., "127.0.0.1:53").
	addr string

	// domains is the list of domains to resolve locally (e.g., "localhost", "test").
	domains []string

	// resolveIP is the IP address to resolve local domains to.
	resolveIP net.IP

	// upstream is the upstream DNS server for non-local queries.
	upstream string

	// udpServer is the UDP DNS server.
	udpServer *dns.Server

	// tcpServer is the TCP DNS server.
	tcpServer *dns.Server

	// client is the DNS client for upstream queries.
	client *dns.Client

	// mu protects the server state.
	mu sync.RWMutex

	// running indicates if the server is running.
	running bool

	// prebound listener for privilege dropping
	preboundListener net.PacketConn
}

// Config holds DNS server configuration.
type Config struct {
	// Addr is the address to listen on (default: "127.0.0.1:53").
	Addr string

	// Domains is the list of TLDs to resolve locally (default: ["localhost"]).
	Domains []string

	// ResolveIP is the IP to resolve local domains to (default: 127.0.0.1).
	ResolveIP net.IP

	// Upstream is the upstream DNS server (default: "8.8.8.8:53").
	Upstream string
}

// DefaultConfig returns a default DNS server configuration.
func DefaultConfig() Config {
	return Config{
		Addr:      fmt.Sprintf("127.0.0.1:%d", DefaultPort),
		Domains:   []string{"localhost"},
		ResolveIP: net.ParseIP("127.0.0.1"),
		Upstream:  DefaultUpstream,
	}
}

// New creates a new DNS server with the given configuration.
func New(cfg Config) *Server {
	if cfg.Addr == "" {
		cfg.Addr = fmt.Sprintf("127.0.0.1:%d", DefaultPort)
	}
	if len(cfg.Domains) == 0 {
		cfg.Domains = []string{"localhost"}
	}
	if cfg.ResolveIP == nil {
		cfg.ResolveIP = net.ParseIP("127.0.0.1")
	}
	if cfg.Upstream == "" {
		cfg.Upstream = DefaultUpstream
	}

	return &Server{
		addr:      cfg.Addr,
		domains:   cfg.Domains,
		resolveIP: cfg.ResolveIP,
		upstream:  cfg.Upstream,
		client: &dns.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// NewWithListener creates a new DNS server using a pre-bound packet listener.
// This is used when ports are bound before dropping privileges.
func NewWithListener(cfg Config, listener net.PacketConn) *Server {
	s := New(cfg)
	s.preboundListener = listener
	return s
}

// Start starts the DNS server on both UDP and TCP.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("server already running")
	}

	// Create DNS handler
	handler := dns.HandlerFunc(s.handleDNS)

	// Start UDP server
	s.udpServer = &dns.Server{
		Addr:    s.addr,
		Net:     "udp",
		Handler: handler,
	}

	// If we have a prebound listener, use it
	if s.preboundListener != nil {
		s.udpServer.PacketConn = s.preboundListener
	}

	// Start TCP server
	s.tcpServer = &dns.Server{
		Addr:    s.addr,
		Net:     "tcp",
		Handler: handler,
	}

	// Start UDP in goroutine
	udpErrCh := make(chan error, 1)
	go func() {
		logging.Info("starting DNS server (UDP)", "addr", s.addr)
		if s.preboundListener != nil {
			udpErrCh <- s.udpServer.ActivateAndServe()
		} else {
			udpErrCh <- s.udpServer.ListenAndServe()
		}
	}()

	// Start TCP in goroutine
	tcpErrCh := make(chan error, 1)
	go func() {
		logging.Info("starting DNS server (TCP)", "addr", s.addr)
		tcpErrCh <- s.tcpServer.ListenAndServe()
	}()

	// Give servers a moment to start and check for immediate errors
	select {
	case err := <-udpErrCh:
		return fmt.Errorf("UDP server failed: %w", err)
	case err := <-tcpErrCh:
		return fmt.Errorf("TCP server failed: %w", err)
	case <-time.After(100 * time.Millisecond):
		// Servers started successfully
	}

	s.running = true
	return nil
}

// Stop stops the DNS server.
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	var errs []error

	if s.udpServer != nil {
		if err := s.udpServer.Shutdown(); err != nil {
			errs = append(errs, fmt.Errorf("UDP shutdown: %w", err))
		}
	}

	if s.tcpServer != nil {
		if err := s.tcpServer.Shutdown(); err != nil {
			errs = append(errs, fmt.Errorf("TCP shutdown: %w", err))
		}
	}

	s.running = false

	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}

	logging.Info("DNS server stopped")
	return nil
}

// Running returns true if the server is running.
func (s *Server) Running() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// Addr returns the server address.
func (s *Server) Addr() string {
	return s.addr
}

// UpdateConfig updates the DNS server configuration at runtime.
// Only domains and upstream can be changed without restart.
// The listen address cannot be changed at runtime.
func (s *Server) UpdateConfig(domains []string, upstream string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(domains) > 0 {
		s.domains = domains
		logging.Info("DNS domains updated", "domains", domains)
	}

	if upstream != "" && upstream != s.upstream {
		s.upstream = upstream
		logging.Info("DNS upstream updated", "upstream", upstream)
	}
}

// GetDomains returns the current list of domains.
func (s *Server) GetDomains() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.domains
}

// GetUpstream returns the current upstream DNS server.
func (s *Server) GetUpstream() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.upstream
}

// handleDNS handles incoming DNS queries.
func (s *Server) handleDNS(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true

	for _, q := range r.Question {
		logging.Debug("DNS query", "name", q.Name, "type", dns.TypeToString[q.Qtype])

		if s.isLocalDomain(q.Name) {
			s.handleLocalQuery(m, q)
		} else {
			s.handleUpstreamQuery(m, r)
			break // Upstream handles entire message
		}
	}

	if err := w.WriteMsg(m); err != nil {
		logging.Error("failed to write DNS response", "error", err)
	}
}

// isLocalDomain checks if the domain should be resolved locally.
func (s *Server) isLocalDomain(name string) bool {
	// Remove trailing dot
	name = strings.TrimSuffix(name, ".")
	name = strings.ToLower(name)

	for _, domain := range s.domains {
		domain = strings.ToLower(domain)
		// Match exact domain or subdomain
		if name == domain || strings.HasSuffix(name, "."+domain) {
			return true
		}
	}
	return false
}

// handleLocalQuery handles queries for local domains.
func (s *Server) handleLocalQuery(m *dns.Msg, q dns.Question) {
	switch q.Qtype {
	case dns.TypeA:
		// Return IPv4 address
		if ip4 := s.resolveIP.To4(); ip4 != nil {
			rr := &dns.A{
				Hdr: dns.RR_Header{
					Name:   q.Name,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    DefaultTTL,
				},
				A: ip4,
			}
			m.Answer = append(m.Answer, rr)
		}

	case dns.TypeAAAA:
		// Return IPv6 address (::1 for localhost)
		if s.resolveIP.Equal(net.ParseIP("127.0.0.1")) {
			rr := &dns.AAAA{
				Hdr: dns.RR_Header{
					Name:   q.Name,
					Rrtype: dns.TypeAAAA,
					Class:  dns.ClassINET,
					Ttl:    DefaultTTL,
				},
				AAAA: net.ParseIP("::1"),
			}
			m.Answer = append(m.Answer, rr)
		}

	default:
		// Return empty response for unsupported types
		m.Rcode = dns.RcodeSuccess
	}
}

// handleUpstreamQuery forwards a query to the upstream DNS server.
func (s *Server) handleUpstreamQuery(m *dns.Msg, r *dns.Msg) {
	resp, _, err := s.client.Exchange(r, s.upstream)
	if err != nil {
		logging.Error("upstream DNS query failed", "error", err)
		m.Rcode = dns.RcodeServerFailure
		return
	}

	// Copy response
	m.Answer = resp.Answer
	m.Ns = resp.Ns
	m.Extra = resp.Extra
	m.Rcode = resp.Rcode
}
