package traffic

import (
	"encoding/json"
	"time"
)

// Constants for TUI configuration
const (
	MaxEvents         = 100              // Maximum number of events to keep in memory
	MaxBodySize       = 10 * 1024        // Maximum body size to store (10KB)
	ReconnectDelay    = 3 * time.Second  // Delay before attempting to reconnect
	MaxReconnectDelay = 30 * time.Second // Maximum reconnect delay
)

// TrafficEvent represents a single MITM traffic event
type TrafficEvent struct {
	ID           string        `json:"id"`
	Hostname     string        `json:"hostname"`
	Timestamp    time.Time     `json:"timestamp"`
	Direction    string        `json:"direction"`
	ConnectionID string        `json:"connection_id"`
	RequestID    string        `json:"request_id"`
	Request      *HTTPRequest  `json:"request,omitempty"`
	Response     *HTTPResponse `json:"response,omitempty"`
	Extra        interface{}   `json:"extra,omitempty"`
}

// HTTPRequest represents an HTTP request
type HTTPRequest struct {
	Method        string            `json:"method"`
	URL           string            `json:"url"`
	Host          string            `json:"host"`
	Headers       map[string]string `json:"headers"`
	Body          string            `json:"body"`
	ContentType   string            `json:"content_type"`
	ContentLength int64             `json:"content_length"`
}

// HTTPResponse represents an HTTP response
type HTTPResponse struct {
	Status        string            `json:"status"`
	StatusCode    int               `json:"status_code"`
	Headers       map[string]string `json:"headers"`
	Body          string            `json:"body"`
	ContentType   string            `json:"content_type"`
	ContentLength int64             `json:"content_length"`
	Latency       int64             `json:"latency"`
}

// DirectionFilter represents traffic direction filter
type DirectionFilter string

const (
	DirectionAll          DirectionFilter = "all"
	DirectionClientServer DirectionFilter = "client->server"
	DirectionServerClient DirectionFilter = "server->client"
)

// ConnectionStatus represents the SSE connection status
type ConnectionStatus int

const (
	StatusConnecting ConnectionStatus = iota
	StatusConnected
	StatusDisconnected
	StatusError
)

// String returns the string representation of ConnectionStatus
func (s ConnectionStatus) String() string {
	switch s {
	case StatusConnecting:
		return "Connecting"
	case StatusConnected:
		return "Live"
	case StatusDisconnected:
		return "Disconnected"
	case StatusError:
		return "Error"
	default:
		return "Unknown"
	}
}

// ParseTrafficEvent parses a TrafficEvent from SSE data
func ParseTrafficEvent(data []byte) (*TrafficEvent, error) {
	var event TrafficEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, err
	}
	return &event, nil
}

// Message types for Bubble Tea

type trafficEventMsg struct {
	event TrafficEvent
}

type connectionStatusMsg struct {
	status ConnectionStatus
}

type errorMsg struct {
	err error
}

type reconnectMsg struct{}

type reconnectTickMsg struct{}

type scrollToBottomMsg struct{}
