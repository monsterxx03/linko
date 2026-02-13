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
	addr           string
	splitter       *DNSSplitter
	cache          *DNSCache
	serverUDP      *dns.Server
	wg             sync.WaitGroup
	ctx            context.Context
	cancel         context.CancelFunc
	statsCollector *DNSStatsCollector
}

// NewDNSServer creates a new DNS server
func NewDNSServer(addr string, splitter *DNSSplitter, cache *DNSCache) *DNSServer {
	ctx, cancel := context.WithCancel(context.Background())
	return &DNSServer{
		addr:           addr,
		splitter:       splitter,
		cache:          cache,
		ctx:            ctx,
		cancel:         cancel,
		statsCollector: NewDNSStatsCollector(),
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
	s.wg.Go(func() {
		if err := s.serverUDP.ListenAndServe(); err != nil {
			slog.Error("UDP server error", "error", err)
		}
	})

	clearDNSCache()
	slog.Info("DNS server started", "address", s.addr, "mode", "UDP only (transparent proxy)")
	return nil
}

// Stop stops the DNS server
func (s *DNSServer) Stop() {
	s.cancel()

	if s.serverUDP != nil {
		s.serverUDP.Shutdown()
	}

	if s.statsCollector != nil {
		s.statsCollector.Shutdown()
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
	startTime := time.Now()

	var queryRecord *QueryRecord
	defer func() {
		if queryRecord != nil {
			queryRecord.ResponseTime = time.Since(startTime)
			s.statsCollector.RecordQuery(queryRecord)
		}
	}()

	if len(r.Question) == 0 {
		return
	}

	domain := r.Question[0].Name
	queryType := QueryType(dns.TypeToString[r.Question[0].Qtype])

	queryRecord = &QueryRecord{
		Domain:    domain,
		QueryType: queryType,
		Timestamp: startTime,
	}

	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	// Check cache if cache is not nil
	if s.cache != nil {
		if cached := s.cache.Get(r); cached != nil {
			cached.SetReply(r)
			queryRecord.Success = true
			w.WriteMsg(cached)
			return
		}
	}

	var resp *dns.Msg
	var err error

	if s.splitter != nil {
		// Use DNSSplitter for intelligent DNS splitting
		resp, err = s.splitter.SplitQuery(ctx, r)
	} else {
		// Use system default DNS resolver
		resp, err = s.resolveWithSystemDNS(ctx, r)
	}

	if err != nil {
		slog.Error("DNS query error", "domain", domain, "error", err)
		queryRecord.Success = false
		dns.HandleFailed(w, r)
		return
	}

	if resp == nil {
		slog.Warn("DNS query returned nil response", "domain", domain)
		queryRecord.Success = false
		dns.HandleFailed(w, r)
		return
	}

	// Cache the response if cache is not nil
	if s.cache != nil {
		s.cache.Set(r, resp)
	}
	queryRecord.Success = true
	w.WriteMsg(resp)
}

// resolveWithSystemDNS resolves DNS using the system's default resolver
func (s *DNSServer) resolveWithSystemDNS(_ context.Context, r *dns.Msg) (*dns.Msg, error) {
	// Extract domain from the request
	if len(r.Question) == 0 {
		return nil, fmt.Errorf("empty question")
	}
	domain := r.Question[0].Name

	// Use system DNS resolver
	addrs, err := net.DefaultResolver.LookupIP(context.Background(), "ip", domain)
	if err != nil {
		return nil, err
	}

	// Build response
	resp := new(dns.Msg)
	resp.SetReply(r)
	resp.Authoritative = true

	for _, ip := range addrs {
		if ipv4 := ip.To4(); ipv4 != nil {
			resp.Answer = append(resp.Answer, &dns.A{
				Hdr: dns.RR_Header{
					Name:   domain,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    300,
				},
				A: ipv4,
			})
		}
	}

	return resp, nil
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

// ServeDNS is the standard dns.HandleFunc interface
func (s *DNSServer) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	s.handleDNS(w, r)
}

// GetCacheStats returns cache statistics
func (s *DNSServer) GetCacheStats() map[string]interface{} {
	var cacheStats map[string]any
	if s.cache != nil {
		cacheStats = s.cache.GetStats()
	} else {
		cacheStats = map[string]any{
			"size":     0,
			"max_size": 0,
			"ttl":      "0s",
			"hits":     0,
			"misses":   0,
			"hit_rate": 0.0,
		}
	}
	statsStats := s.statsCollector.GetStatsSummary()
	topDomains := s.statsCollector.GetTopDomains(20, "queries")

	domains := make([]map[string]interface{}, 0, len(topDomains))
	for _, d := range topDomains {
		domains = append(domains, FormatDomainStats(d))
	}

	return map[string]interface{}{
		"cache": cacheStats,
		"dns": map[string]interface{}{
			"total_domains":     statsStats.TotalDomains,
			"total_queries":     statsStats.TotalQueries,
			"total_success":     statsStats.TotalSuccess,
			"total_failed":      statsStats.TotalFailed,
			"success_rate":      statsStats.SuccessRate,
			"avg_response_time": statsStats.AvgResponseTime.String(),
			"top_domains":       domains,
		},
	}
}

// ClearStats clears all DNS statistics
func (s *DNSServer) ClearStats() {
	s.statsCollector.ClearStats()
}

// ClearCache clears the DNS cache
func (s *DNSServer) ClearCache() {
	if s.cache != nil {
		s.cache.Clear()
	}
}
