package admin

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/monsterxx03/linko/pkg/dns"
)

type AdminServer struct {
	addr      string
	server    *http.Server
	listener  net.Listener
	wg        sync.WaitGroup
	dnsServer *dns.DNSServer
}

type StatsResponse struct {
	Code    int                    `json:"code"`
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data"`
}

func NewAdminServer(addr string, dnsServer *dns.DNSServer) *AdminServer {
	return &AdminServer{
		addr:      addr,
		dnsServer: dnsServer,
	}
}

func (s *AdminServer) Start() error {
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.listener = listener

	mux := http.NewServeMux()
	mux.HandleFunc("/stats/dns", s.handleDNSStats)
	mux.HandleFunc("/health", s.handleHealth)

	s.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		slog.Info("Admin server listening", "address", s.addr)
		if err := s.server.Serve(s.listener); err != nil && err != http.ErrServerClosed {
			slog.Error("Admin server error", "error", err)
		}
	}()

	return nil
}

func (s *AdminServer) Stop() {
	if s.server != nil {
		s.server.Close()
	}
	s.wg.Wait()
}

func (s *AdminServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

func (s *AdminServer) handleDNSStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(StatsResponse{
			Code:    405,
			Message: "Method not allowed",
		})
		return
	}

	stats := dns.GetGlobalStatsCollector().GetStatsSummary()
	topDomains := dns.GetGlobalStatsCollector().GetTopDomains(20, "queries")
	cacheStats := s.dnsServer.GetCacheStats()

	summary := map[string]interface{}{
		"total_domains":     stats.TotalDomains,
		"total_queries":     stats.TotalQueries,
		"total_success":     stats.TotalSuccess,
		"total_failed":      stats.TotalFailed,
		"success_rate":      stats.SuccessRate,
		"avg_response_time": stats.AvgResponseTime.String(),
	}

	domains := make([]map[string]interface{}, 0, len(topDomains))
	for _, d := range topDomains {
		domains = append(domains, formatDomainStats(d))
	}

	response := StatsResponse{
		Code:    0,
		Message: "success",
		Data: map[string]interface{}{
			"summary":     summary,
			"top_domains": domains,
			"cache":       cacheStats,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func formatDomainStats(d *dns.DomainStats) map[string]interface{} {
	avgResponseTime := time.Duration(0)
	if d.TotalQueries > 0 {
		avgResponseTime = time.Duration(d.TotalResponseNs / d.TotalQueries)
	}

	queryTypes := make(map[string]interface{})
	for qt, ts := range d.QueryTypes {
		avgNs := time.Duration(0)
		if ts.Count > 0 {
			avgNs = time.Duration(ts.TotalNs / ts.Count)
		}
		queryTypes[string(qt)] = map[string]interface{}{
			"count":         ts.Count,
			"success_count": ts.SuccessCount,
			"failed_count":  ts.FailedCount,
			"avg_ns":        avgNs.String(),
		}
	}

	return map[string]interface{}{
		"domain":            d.Domain,
		"total_queries":     d.TotalQueries,
		"success_queries":   d.SuccessQueries,
		"failed_queries":    d.FailedQueries,
		"avg_response_time": avgResponseTime.String(),
		"first_query":       d.FirstQueryTime,
		"last_query":        d.LastQueryTime,
		"query_types":       queryTypes,
	}
}
