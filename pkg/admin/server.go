package admin

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

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
	llmEventBus *mitm.EventBus
}

type StatsResponse struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data"`
}

func NewAdminServer(addr string, uiPath string, uiEmbed bool, dnsServer *dns.DNSServer, eventBus *mitm.EventBus, llmEventBus *mitm.EventBus) *AdminServer {
	return &AdminServer{
		addr:      addr,
		uiPath:    uiPath,
		uiEmbed:   uiEmbed,
		dnsServer: dnsServer,
		eventBus:  eventBus,
		llmEventBus: llmEventBus,
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

	// LLM conversation SSE endpoint
	mux.HandleFunc("/api/llm/conversation/sse", s.handleLLMConversationSSE)

	s.server = &http.Server{
		Handler: mux,
	}

	s.wg.Go(func() {
		slog.Info("Admin server listening", "address", s.addr)
		if err := s.server.Serve(s.listener); err != nil && err != http.ErrServerClosed {
			slog.Error("Admin server error", "error", err)
		}
	})

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

// handleEmbed serves the embedded static files
func (s *AdminServer) handleEmbed() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Remove leading slash
		path := r.URL.Path
		if path == "/" {
			path = "/admin.html"
		}

		// Try to open the file from embedded FS (embed path is dist/admin)
		data, err := ui.AdminFS.ReadFile("dist/admin" + path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				http.NotFound(w, r)
			} else {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}

		// Set content type based on extension
		contentType := "application/octet-stream"
		switch {
		case hasExt(path, ".html"):
			contentType = "text/html; charset=utf-8"
		case hasExt(path, ".js"):
			contentType = "application/javascript"
		case hasExt(path, ".css"):
			contentType = "text/css"
		case hasExt(path, ".json"):
			contentType = "application/json"
		case hasExt(path, ".png"):
			contentType = "image/png"
		case hasExt(path, ".svg"):
			contentType = "image/svg+xml"
		}

		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "no-cache")
		reader := bytes.NewReader(data)
		http.ServeContent(w, r, "admin.html", time.Time{}, reader)
	})
}

func hasExt(path string, ext string) bool {
	return len(path) >= len(ext) && path[len(path)-len(ext):] == ext
}

func (s *AdminServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

func (s *AdminServer) writeServiceUnavailable(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	json.NewEncoder(w).Encode(StatsResponse{
		Code:    503,
		Message: msg,
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

	if s.dnsServer == nil {
		s.writeServiceUnavailable(w, "DNS server not available")
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

	if s.dnsServer == nil {
		s.writeServiceUnavailable(w, "DNS server not available")
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

	if s.dnsServer == nil {
		s.writeServiceUnavailable(w, "DNS server not available")
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
	subscriber := s.eventBus.SubscribeWithName("mitm-traffic-sse")
	defer s.eventBus.Unsubscribe(subscriber)

	// Send welcome message
	welcomeMsg := `event: welcome
data: {"message":"Connected to MITM traffic stream"}

`
	w.Write([]byte(welcomeMsg))
	flusher.Flush()

	// Set up connection close handling using context
	ctx := r.Context()

	// Listen for events
	for {
		select {
		case <-ctx.Done():
			// Client closed connection or context cancelled
			return
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
		}
	}
}

// handleLLMConversationSSE handles the SSE endpoint for LLM conversation events
func (s *AdminServer) handleLLMConversationSSE(w http.ResponseWriter, r *http.Request) {
	// Check if LLM event bus is available
	if s.llmEventBus == nil {
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
	subscriber := s.llmEventBus.SubscribeWithName("llm-conversation-sse")
	defer s.llmEventBus.Unsubscribe(subscriber)

	// Send welcome message
	welcomeMsg := `event: welcome
data: {"message":"Connected to LLM conversation stream"}

`
	w.Write([]byte(welcomeMsg))
	flusher.Flush()

	// Set up connection close handling using context
	ctx := r.Context()

	// Listen for events
	for {
		select {
		case <-ctx.Done():
			// Client closed connection or context cancelled
			return
		case event, ok := <-subscriber.Channel:
			if !ok {
				return
			}
			// Extract the actual event data from Extra field
			var actualEvent interface{}
			var eventType string

			// Check if this is a TrafficEvent with Extra field
			if event.Extra != nil {
				// Use the Extra field as the actual event
				actualEvent = event.Extra
				// Determine event type based on direction
				switch event.Direction {
				case "llm_message":
					eventType = "llm_message"
				case "llm_token":
					eventType = "llm_token"
				case "conversation":
					eventType = "conversation"
				default:
					eventType = "traffic"
				}
			} else {
				// If no Extra field, use the event itself (for backward compatibility)
				actualEvent = event
				eventType = "traffic"
			}

			// Marshal the actual event to JSON
			eventData, err := json.Marshal(actualEvent)
			if err != nil {
				continue
			}

			// Format SSE message
			eventMsg := `event: ` + eventType + `
data: ` + string(eventData) + `

`
			// Write event to response
			_, err = w.Write([]byte(eventMsg))
			if err != nil {
				return
			}
			// Flush the data immediately to the client
			flusher.Flush()
		}
	}
}
