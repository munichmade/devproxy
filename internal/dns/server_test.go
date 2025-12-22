package dns

import (
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestNew(t *testing.T) {
	s := New(DefaultConfig())

	if s.addr != "127.0.0.1:53" {
		t.Errorf("expected addr 127.0.0.1:53, got %s", s.addr)
	}

	if len(s.domains) != 1 || s.domains[0] != "localhost" {
		t.Errorf("expected domains [localhost], got %v", s.domains)
	}

	if !s.resolveIP.Equal(net.ParseIP("127.0.0.1")) {
		t.Errorf("expected resolveIP 127.0.0.1, got %v", s.resolveIP)
	}
}

func TestNewWithCustomConfig(t *testing.T) {
	cfg := Config{
		Addr:      "127.0.0.1:5353",
		Domains:   []string{"test", "local"},
		ResolveIP: net.ParseIP("10.0.0.1"),
		Upstream:  "1.1.1.1:53",
	}

	s := New(cfg)

	if s.addr != "127.0.0.1:5353" {
		t.Errorf("expected addr 127.0.0.1:5353, got %s", s.addr)
	}

	if len(s.domains) != 2 {
		t.Errorf("expected 2 domains, got %d", len(s.domains))
	}

	if !s.resolveIP.Equal(net.ParseIP("10.0.0.1")) {
		t.Errorf("expected resolveIP 10.0.0.1, got %v", s.resolveIP)
	}

	if s.upstream != "1.1.1.1:53" {
		t.Errorf("expected upstream 1.1.1.1:53, got %s", s.upstream)
	}
}

func TestIsLocalDomain(t *testing.T) {
	s := New(Config{
		Domains: []string{"localhost", "test"},
	})

	tests := []struct {
		name     string
		expected bool
	}{
		{"localhost.", true},
		{"app.localhost.", true},
		{"sub.app.localhost.", true},
		{"test.", true},
		{"myapp.test.", true},
		{"example.com.", false},
		{"google.com.", false},
		{"localhost.com.", false}, // Not a subdomain of localhost
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := s.isLocalDomain(tt.name); got != tt.expected {
				t.Errorf("isLocalDomain(%s) = %v, want %v", tt.name, got, tt.expected)
			}
		})
	}
}

func TestServerStartStop(t *testing.T) {
	// Use a random high port to avoid conflicts with running devproxy
	cfg := Config{
		Addr:    "127.0.0.1:25353",
		Domains: []string{"localhost"},
	}

	s := New(cfg)

	// Server should not be running initially
	if s.Running() {
		t.Error("server should not be running initially")
	}

	// Start server
	if err := s.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}

	// Server should be running
	if !s.Running() {
		t.Error("server should be running after Start()")
	}

	// Starting again should fail
	if err := s.Start(); err == nil {
		t.Error("starting already running server should fail")
	}

	// Stop server
	if err := s.Stop(); err != nil {
		t.Fatalf("failed to stop server: %v", err)
	}

	// Server should not be running
	if s.Running() {
		t.Error("server should not be running after Stop()")
	}
}

func TestDNSQuery(t *testing.T) {
	// Use a high port to avoid permission issues
	cfg := Config{
		Addr:      "127.0.0.1:15354",
		Domains:   []string{"localhost"},
		ResolveIP: net.ParseIP("127.0.0.1"),
	}

	s := New(cfg)

	if err := s.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer s.Stop()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Create DNS client
	c := new(dns.Client)
	c.Timeout = 2 * time.Second

	// Test A record query for localhost domain
	m := new(dns.Msg)
	m.SetQuestion("app.localhost.", dns.TypeA)

	r, _, err := c.Exchange(m, "127.0.0.1:15354")
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}

	if len(r.Answer) == 0 {
		t.Fatal("expected at least one answer")
	}

	a, ok := r.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("expected A record, got %T", r.Answer[0])
	}

	if !a.A.Equal(net.ParseIP("127.0.0.1")) {
		t.Errorf("expected 127.0.0.1, got %v", a.A)
	}
}

func TestDNSQueryAAAA(t *testing.T) {
	cfg := Config{
		Addr:      "127.0.0.1:15355",
		Domains:   []string{"localhost"},
		ResolveIP: net.ParseIP("127.0.0.1"),
	}

	s := New(cfg)

	if err := s.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer s.Stop()

	time.Sleep(50 * time.Millisecond)

	c := new(dns.Client)
	c.Timeout = 2 * time.Second

	// Test AAAA record query
	m := new(dns.Msg)
	m.SetQuestion("app.localhost.", dns.TypeAAAA)

	r, _, err := c.Exchange(m, "127.0.0.1:15355")
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}

	if len(r.Answer) == 0 {
		t.Fatal("expected at least one answer")
	}

	aaaa, ok := r.Answer[0].(*dns.AAAA)
	if !ok {
		t.Fatalf("expected AAAA record, got %T", r.Answer[0])
	}

	if !aaaa.AAAA.Equal(net.ParseIP("::1")) {
		t.Errorf("expected ::1, got %v", aaaa.AAAA)
	}
}

func TestDNSQueryTCP(t *testing.T) {
	cfg := Config{
		Addr:      "127.0.0.1:15356",
		Domains:   []string{"localhost"},
		ResolveIP: net.ParseIP("127.0.0.1"),
	}

	s := New(cfg)

	if err := s.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer s.Stop()

	time.Sleep(50 * time.Millisecond)

	// Use TCP
	c := new(dns.Client)
	c.Net = "tcp"
	c.Timeout = 2 * time.Second

	m := new(dns.Msg)
	m.SetQuestion("app.localhost.", dns.TypeA)

	r, _, err := c.Exchange(m, "127.0.0.1:15356")
	if err != nil {
		t.Fatalf("TCP DNS query failed: %v", err)
	}

	if len(r.Answer) == 0 {
		t.Fatal("expected at least one answer")
	}

	a, ok := r.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("expected A record, got %T", r.Answer[0])
	}

	if !a.A.Equal(net.ParseIP("127.0.0.1")) {
		t.Errorf("expected 127.0.0.1, got %v", a.A)
	}
}

func TestMultipleDomains(t *testing.T) {
	cfg := Config{
		Addr:      "127.0.0.1:15357",
		Domains:   []string{"localhost", "test", "dev"},
		ResolveIP: net.ParseIP("127.0.0.1"),
	}

	s := New(cfg)

	if err := s.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer s.Stop()

	time.Sleep(50 * time.Millisecond)

	c := new(dns.Client)
	c.Timeout = 2 * time.Second

	domains := []string{"app.localhost.", "myapp.test.", "api.dev."}

	for _, domain := range domains {
		t.Run(domain, func(t *testing.T) {
			m := new(dns.Msg)
			m.SetQuestion(domain, dns.TypeA)

			r, _, err := c.Exchange(m, "127.0.0.1:15357")
			if err != nil {
				t.Fatalf("DNS query for %s failed: %v", domain, err)
			}

			if len(r.Answer) == 0 {
				t.Fatalf("expected answer for %s", domain)
			}
		})
	}
}
