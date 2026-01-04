package admin

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"sync"

	"github.com/monsterxx03/linko/pkg/dns"
	"github.com/monsterxx03/linko/pkg/traffic"
	"github.com/monsterxx03/linko/pkg/ui"
)

type AdminServer struct {
	addr        string
	uiPath      string
	uiEmbed     bool
	server      *http.Server
	listener    net.Listener
	wg          sync.WaitGroup
	dnsServer   *dns.DNSServer
	trafficStats *traffic.TrafficStatsCollector
}

type StatsResponse struct {
	Code    int                    `json:"code"`
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data"`
}

func NewAdminServer(addr string, uiPath string, uiEmbed bool, dnsServer *dns.DNSServer, trafficStats *traffic.TrafficStatsCollector) *AdminServer {
	return &AdminServer{
		addr:        addr,
		uiPath:      uiPath,
		uiEmbed:     uiEmbed,
		dnsServer:   dnsServer,
		trafficStats: trafficStats,
	}
}

func (s *AdminServer) Start() error {
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.listener = listener

	mux := http.NewServeMux()

	// Serve UI files or embedded HTML
	if s.uiEmbed {
		mux.Handle("/", s.handleEmbed())
	} else if s.uiPath != "" {
		mux.Handle("/", s.handleStatic())
	}

	// API endpoints
	mux.HandleFunc("/stats/dns", s.handleDNSStats)
	mux.HandleFunc("/stats/dns/clear", s.handleDNSStatsClear)
	mux.HandleFunc("/cache/dns/clear", s.handleDNSCacheClear)
	mux.HandleFunc("/stats/traffic", s.handleTrafficStats)
	mux.HandleFunc("/stats/traffic/clear", s.handleTrafficStatsClear)
	mux.HandleFunc("/health", s.handleHealth)

	s.server = &http.Server{
		Handler: mux,
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

// handleStatic serves static files from the UI directory
func (s *AdminServer) handleStatic() http.Handler {
	return http.FileServer(http.Dir(s.uiPath))
}

// handleEmbed serves the embedded HTML
func (s *AdminServer) handleEmbed() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(ui.AdminHTML))
	})
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

	cacheStats := s.dnsServer.GetCacheStats()

	response := StatsResponse{
		Code:    0,
		Message: "success",
		Data:    cacheStats,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func (s *AdminServer) handleDNSStatsClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(StatsResponse{
			Code:    405,
			Message: "Method not allowed",
		})
		return
	}

	s.dnsServer.ClearStats()

	response := StatsResponse{
		Code:    0,
		Message: "success",
		Data:    nil,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func (s *AdminServer) handleDNSCacheClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(StatsResponse{
			Code:    405,
			Message: "Method not allowed",
		})
		return
	}

	s.dnsServer.ClearCache()

	response := StatsResponse{
		Code:    0,
		Message: "success",
		Data:    nil,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func (s *AdminServer) handleTrafficStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(StatsResponse{
			Code:    405,
			Message: "Method not allowed",
		})
		return
	}

	if s.trafficStats == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(StatsResponse{
			Code:    0,
			Message: "success",
			Data: map[string]interface{}{
				"summary":      traffic.FormatSummary(traffic.TrafficSummary{}),
				"top_traffic":  []interface{}{},
				"all_stats":    map[string]interface{}{},
			},
		})
		return
	}

	summary := s.trafficStats.GetSummary()
	topTraffic := s.trafficStats.GetTopTraffic(20)
	allStats := s.trafficStats.GetStats()

	// Format top traffic
	topList := make([]map[string]interface{}, 0, len(topTraffic))
	for _, stats := range topTraffic {
		topList = append(topList, traffic.FormatTrafficStats(stats))
	}

	// Format all stats
	allList := make([]map[string]interface{}, 0, len(allStats))
	for _, stats := range allStats {
		allList = append(allList, traffic.FormatTrafficStats(stats))
	}

	response := StatsResponse{
		Code:    0,
		Message: "success",
		Data: map[string]interface{}{
			"summary":     traffic.FormatSummary(summary),
			"top_traffic": topList,
			"all_stats":   allList,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func (s *AdminServer) handleTrafficStatsClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(StatsResponse{
			Code:    405,
			Message: "Method not allowed",
		})
		return
	}

	if s.trafficStats != nil {
		s.trafficStats.ClearStats()
	}

	response := StatsResponse{
		Code:    0,
		Message: "success",
		Data:    nil,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
