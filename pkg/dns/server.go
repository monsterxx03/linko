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
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.serverUDP.ListenAndServe(); err != nil {
			slog.Error("UDP server error", "error", err)
		}
	}()

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

	if cached := s.cache.Get(r); cached != nil {
		cached.SetReply(r)
		queryRecord.Success = true
		w.WriteMsg(cached)
		return
	}

	resp, err := s.splitter.SplitQuery(ctx, r)
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

	s.cache.Set(r, resp)
	queryRecord.Success = true
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

// ServeDNS is the standard dns.HandleFunc interface
func (s *DNSServer) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	s.handleDNS(w, r)
}

// GetCacheStats returns cache statistics
func (s *DNSServer) GetCacheStats() map[string]interface{} {
	cacheStats := s.cache.GetStats()
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
