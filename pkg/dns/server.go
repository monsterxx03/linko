package dns

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// DNSServer handles DNS requests with splitting and caching
type DNSServer struct {
	addr      string
	splitter  *DNSSplitter
	cache     *DNSCache
	serverUDP *dns.Server
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewDNSServer creates a new DNS server
func NewDNSServer(addr string, splitter *DNSSplitter, cache *DNSCache) *DNSServer {
	ctx, cancel := context.WithCancel(context.Background())
	return &DNSServer{
		addr:     addr,
		splitter: splitter,
		cache:    cache,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start starts the DNS server (UDP only for transparent proxy)
func (s *DNSServer) Start() error {
	// Create UDP handler
	udpHandler := s.handleDNS
	dns.HandleFunc(".", udpHandler)

	// Create UDP server
	s.serverUDP = &dns.Server{
		Addr:    s.addr,
		Net:     "udp",
		Handler: dns.HandlerFunc(udpHandler),
	}

	// Start UDP server
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.serverUDP.ListenAndServe(); err != nil {
			slog.Error("UDP server error", "error", err)
		}
	}()

	slog.Info("DNS server started", "address", s.addr, "mode", "UDP only (transparent proxy)")
	return nil
}

// Stop stops the DNS server
func (s *DNSServer) Stop() {
	s.cancel()

	if s.serverUDP != nil {
		s.serverUDP.Shutdown()
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.wg.Wait()
	}()

	select {
	case <-done:
		slog.Info("DNS server stopped")
	case <-time.After(10 * time.Second):
		slog.Warn("DNS server stop timeout")
	}
}

// handleDNS handles DNS requests
func (s *DNSServer) handleDNS(w dns.ResponseWriter, r *dns.Msg) {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	// Check cache first
	if cached := s.cache.Get(r); cached != nil {
		cached.SetReply(r)
		w.WriteMsg(cached)
		return
	}

	// Process query through splitter
	resp, err := s.splitter.SplitQuery(ctx, r)
	if err != nil {
		slog.Error("DNS query error", "error", err)
		dns.HandleFailed(w, r)
		return
	}

	// Check if response is nil
	if resp == nil {
		slog.Warn("DNS query returned nil response")
		dns.HandleFailed(w, r)
		return
	}

	// Cache the response
	s.cache.Set(r, resp)

	// Send response
	w.WriteMsg(resp)
}

// GetAddr returns the server address
func (s *DNSServer) GetAddr() string {
	return s.addr
}

// IsRunning checks if the server is running
func (s *DNSServer) IsRunning() bool {
	return s.ctx.Err() == nil
}

// HealthCheck performs a health check on the DNS server
func (s *DNSServer) HealthCheck() error {
	if !s.IsRunning() {
		return fmt.Errorf("DNS server is not running")
	}

	// Try to query localhost
	c := new(dns.Client)
	c.Timeout = 5 * time.Second

	req := new(dns.Msg)
	req.SetQuestion("google.com.", dns.TypeA)

	resp, _, err := c.Exchange(req, net.JoinHostPort(s.addr, "53"))
	if err != nil {
		return fmt.Errorf("DNS health check failed: %v", err)
	}

	if resp == nil || resp.Rcode != dns.RcodeSuccess {
		return fmt.Errorf("DNS health check failed: invalid response")
	}

	return nil
}

// GetStats returns server statistics
func (s *DNSServer) GetStats() map[string]interface{} {
	stats := make(map[string]interface{})
	stats["address"] = s.addr
	stats["running"] = s.IsRunning()
	stats["cache_stats"] = s.cache.GetStats()
	return stats
}

// ServeDNS is the standard dns.HandleFunc interface
func (s *DNSServer) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	s.handleDNS(w, r)
}
