package admin

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"sync"

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
