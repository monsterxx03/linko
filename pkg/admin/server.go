package admin

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"sync"

	"github.com/monsterxx03/linko/pkg/dns"
	"github.com/monsterxx03/linko/pkg/mitm"
	"github.com/monsterxx03/linko/pkg/ui"
)

type AdminServer struct {
	addr      string
	uiPath    string
	uiEmbed   bool
	server    *http.Server
	listener  net.Listener
	wg        sync.WaitGroup
	dnsServer *dns.DNSServer
	eventBus  *mitm.EventBus
}

type StatsResponse struct {
	Code    int                    `json:"code"`
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data"`
}

func NewAdminServer(addr string, uiPath string, uiEmbed bool, dnsServer *dns.DNSServer, eventBus *mitm.EventBus) *AdminServer {
	return &AdminServer{
		addr:      addr,
		uiPath:    uiPath,
		uiEmbed:   uiEmbed,
		dnsServer: dnsServer,
		eventBus:  eventBus,
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
	mux.HandleFunc("/health", s.handleHealth)

	// MITM traffic SSE endpoint
	mux.HandleFunc("/api/mitm/traffic/sse", s.handleMITMTrafficSSE)

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

// handleMITMTrafficSSE handles the SSE endpoint for MITM traffic
func (s *AdminServer) handleMITMTrafficSSE(w http.ResponseWriter, r *http.Request) {
	// Check if event bus is available
	if s.eventBus == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Get flusher for SSE
	flusher, ok := w.(http.Flusher)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Create subscriber
	subscriber := s.eventBus.Subscribe()
	defer s.eventBus.Unsubscribe(subscriber)

	// Send welcome message
	welcomeMsg := `event: welcome
data: {"message":"Connected to MITM traffic stream"}

`
	w.Write([]byte(welcomeMsg))
	flusher.Flush()

	// Set up connection close handling
	notify := w.(http.CloseNotifier).CloseNotify()

	// Listen for events
	for {
		select {
		case event, ok := <-subscriber.Channel:
			if !ok {
				return
			}
			// Marshal event to JSON
			eventData, err := json.Marshal(event)
			if err != nil {
				continue
			}
			// Format SSE message
			eventMsg := `event: traffic
data: ` + string(eventData) + `

`
			// Write event to response
			_, err = w.Write([]byte(eventMsg))
			if err != nil {
				return
			}
			// Flush the data immediately to the client
			flusher.Flush()
		case <-notify:
			// Client closed connection
			return
		}
	}
}
