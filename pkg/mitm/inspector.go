package mitm

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// Direction represents the direction of traffic
type Direction int

const (
	// DirectionClientToServer represents traffic from client to server
	DirectionClientToServer Direction = iota
	// DirectionServerToClient represents traffic from server to client
	DirectionServerToClient
)

func (d Direction) String() string {
	switch d {
	case DirectionClientToServer:
		return "client->server"
	case DirectionServerToClient:
		return "server->client"
	default:
		return "unknown"
	}
}

// Inspector defines the interface for traffic inspection
type Inspector interface {
	// Name returns the inspector name
	Name() string

	// Inspect processes the data and returns modified data
	// Return nil to drop the connection
	// Return the original data unchanged to pass through
	Inspect(direction Direction, data []byte) ([]byte, error)

	// ShouldInspect returns true if this inspector should process traffic for the given hostname
	ShouldInspect(hostname string) bool
}

// BaseInspector provides a base implementation of Inspector
type BaseInspector struct {
	name     string
	hostname string
}

// NewBaseInspector creates a new base inspector
func NewBaseInspector(name, hostname string) *BaseInspector {
	return &BaseInspector{
		name:     name,
		hostname: hostname,
	}
}

// Name returns the inspector name
func (b *BaseInspector) Name() string {
	return b.name
}

// ShouldInspect checks if the hostname matches
func (b *BaseInspector) ShouldInspect(hostname string) bool {
	if b.hostname == "" {
		return true
	}
	return strings.Contains(hostname, b.hostname)
}

// HTTPInspector inspects HTTP traffic
type HTTPInspector struct {
	*BaseInspector
	logger *slog.Logger
}

// NewHTTPInspector creates a new HTTP inspector
func NewHTTPInspector(logger *slog.Logger, hostname string) *HTTPInspector {
	return &HTTPInspector{
		BaseInspector: NewBaseInspector("http-inspector", hostname),
		logger:        logger,
	}
}

// Inspect processes HTTP traffic
func (h *HTTPInspector) Inspect(direction Direction, data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	// Try to parse as HTTP request (client to server)
	if direction == DirectionClientToServer {
		return h.inspectRequest(data)
	}

	// Try to parse as HTTP response (server to client)
	return h.inspectResponse(data)
}

func (h *HTTPInspector) inspectRequest(data []byte) ([]byte, error) {
	// Check if it looks like an HTTP request
	if !isHTTPPrefix(data) {
		return data, nil
	}

	// Parse HTTP request
	reader := bufio.NewReader(bytes.NewReader(data))
	req, err := http.ReadRequest(reader)
	if err != nil {
		// Not a valid HTTP request, pass through
		return data, nil
	}
	defer req.Body.Close()

	// Log the request
	h.logger.Debug("HTTP request",
		"method", req.Method,
		"url", req.URL.String(),
		"host", req.Host,
		"user-agent", req.UserAgent(),
	)

	return data, nil
}

func (h *HTTPInspector) inspectResponse(data []byte) ([]byte, error) {
	// Check if it looks like an HTTP response
	if !isHTTPResponsePrefix(data) {
		return data, nil
	}

	// Parse HTTP response
	reader := bufio.NewReader(bytes.NewReader(data))
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		// Not a valid HTTP response, pass through
		return data, nil
	}
	defer resp.Body.Close()

	// Log the response
	h.logger.Debug("HTTP response",
		"status", resp.Status,
		"content-type", resp.Header.Get("Content-Type"),
		"content-length", resp.ContentLength,
	)

	return data, nil
}

// isHTTPPrefix checks if data starts with an HTTP request method
func isHTTPPrefix(data []byte) bool {
	methods := []string{"GET ", "POST ", "HEAD ", "PUT ", "DELETE ", "PATCH ", "OPTIONS ", "CONNECT ", "TRACE "}
	for _, method := range methods {
		if bytes.HasPrefix(data, []byte(method)) {
			return true
		}
	}
	return false
}

// isHTTPResponsePrefix checks if data starts with an HTTP response status line
func isHTTPResponsePrefix(data []byte) bool {
	// HTTP/1.1 200, HTTP/1.0 404, etc.
	if len(data) < 9 {
		return false
	}
	return bytes.HasPrefix(data, []byte("HTTP/1.1 ")) ||
		bytes.HasPrefix(data, []byte("HTTP/1.0 ")) ||
		bytes.HasPrefix(data, []byte("HTTP/2 "))
}

// LogInspector logs all traffic
type LogInspector struct {
	*BaseInspector
	logger *slog.Logger
	opts   LogInspectorOptions
}

// LogInspectorOptions configures the log inspector
type LogInspectorOptions struct {
	MaxBodySize int64 // Maximum body size to log (0 = unlimited)
}

// NewLogInspector creates a new log inspector
func NewLogInspector(logger *slog.Logger, hostname string, opts LogInspectorOptions) *LogInspector {
	if opts.MaxBodySize == 0 {
		opts.MaxBodySize = 16 * 1024 // Default 16KB
	}
	return &LogInspector{
		BaseInspector: NewBaseInspector("log-inspector", hostname),
		logger:        logger,
		opts:          opts,
	}
}

// Inspect logs traffic data
func (l *LogInspector) Inspect(direction Direction, data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	// Truncate if needed
	displayData := data
	if int64(len(displayData)) > l.opts.MaxBodySize {
		displayData = displayData[:l.opts.MaxBodySize]
	}

	text := string(data)
	if text != "" {
		if len(text) > int(l.opts.MaxBodySize) {
			text = text[:l.opts.MaxBodySize]
		}
		l.logger.Debug("MITM traffic",
			"direction", direction,
			"preview", text,
		)
	}

	return data, nil
}

// extractReadableText extracts readable text from binary data
func extractReadableText(data []byte) string {
	var result strings.Builder
	for _, b := range data {
		if (b >= 32 && b <= 126) || b == 10 || b == 13 || b == 9 {
			result.WriteByte(b)
		} else {
			if result.Len() > 0 && result.Len()%100 == 0 {
				result.WriteString("...")
				break
			}
		}
	}
	return strings.TrimSpace(result.String())
}

// InspectorChain allows multiple inspectors to be chained
type InspectorChain struct {
	inspectors []Inspector
}

// NewInspectorChain creates a new inspector chain
func NewInspectorChain() *InspectorChain {
	return &InspectorChain{
		inspectors: make([]Inspector, 0),
	}
}

// Add adds an inspector to the chain
func (c *InspectorChain) Add(inspector Inspector) {
	c.inspectors = append(c.inspectors, inspector)
}

// Inspect processes data through all inspectors
func (c *InspectorChain) Inspect(direction Direction, data []byte) ([]byte, error) {
	var err error
	result := data

	for _, inspector := range c.inspectors {
		result, err = inspector.Inspect(direction, result)
		if err != nil {
			return nil, fmt.Errorf("inspector %s failed: %w", inspector.Name(), err)
		}
		if result == nil {
			return nil, nil // Connection should be dropped
		}
	}

	return result, nil
}

// ShouldInspect checks if any inspector wants to inspect this hostname
func (c *InspectorChain) ShouldInspect(hostname string) bool {
	for _, inspector := range c.inspectors {
		if inspector.ShouldInspect(hostname) {
			return true
		}
	}
	return false
}

// ReadWriter is an io.ReadWriter that can be inspected
type ReadWriter struct {
	rw        io.ReadWriter
	inspector *InspectorChain
	hostname  string
	direction Direction
	logger    *slog.Logger
}

// NewReadWriter creates a new inspectable ReadWriter
func NewReadWriter(rw io.ReadWriter, inspector *InspectorChain, hostname string, direction Direction, logger *slog.Logger) *ReadWriter {
	return &ReadWriter{
		rw:        rw,
		inspector: inspector,
		hostname:  hostname,
		direction: direction,
		logger:    logger,
	}
}

// Read reads data and optionally inspects it
func (rw *ReadWriter) Read(p []byte) (n int, err error) {
	n, err = rw.rw.Read(p)
	if n > 0 && rw.inspector.ShouldInspect(rw.hostname) {
		data := make([]byte, n)
		copy(data, p[:n])
		modified, inspectErr := rw.inspector.Inspect(rw.direction, data)
		if inspectErr != nil {
			rw.logger.Debug("inspect error", "error", inspectErr)
		}
		if modified != nil {
			copy(p[:n], modified)
		}
	}
	return n, err
}

// Write writes data and optionally inspects it
func (rw *ReadWriter) Write(p []byte) (n int, err error) {
	if rw.inspector.ShouldInspect(rw.hostname) {
		modified, inspectErr := rw.inspector.Inspect(rw.direction, p)
		if inspectErr != nil {
			rw.logger.Debug("inspect error", "error", inspectErr)
		}
		if modified != nil {
			return rw.rw.Write(modified)
		}
		return len(p), nil // Pretend we wrote it but drop the data
	}
	return rw.rw.Write(p)
}
