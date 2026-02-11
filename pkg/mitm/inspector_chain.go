package mitm

import (
	"errors"
	"io"
	"log/slog"
	"strconv"
	"sync"
)

type InspectorChain struct {
	inspectors []Inspector
}

func NewInspectorChain() *InspectorChain {
	return &InspectorChain{
		inspectors: make([]Inspector, 0),
	}
}

func (c *InspectorChain) Add(inspector Inspector) {
	c.inspectors = append(c.inspectors, inspector)
}

func (c *InspectorChain) Inspect(direction Direction, data []byte, hostname string, connectionID, requestID string) error {
	errs := make([]error, 0)
	for _, inspector := range c.inspectors {
		_, err := inspector.Inspect(direction, data, hostname, connectionID, requestID)
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (c *InspectorChain) ShouldInspect(hostname string) bool {
	for _, inspector := range c.inspectors {
		if inspector.ShouldInspect(hostname) {
			return true
		}
	}
	return false
}

// RequestIDGenerator generates unique request IDs for HTTP request/response pairs
type RequestIDGenerator struct {
	connectionID string
	seq          uint64
	mu           sync.Mutex
}

// NewRequestIDGenerator creates a new RequestIDGenerator
func NewRequestIDGenerator(connectionID string) *RequestIDGenerator {
	return &RequestIDGenerator{
		connectionID: connectionID,
		seq:          0,
	}
}

// Next generates the next request ID
func (g *RequestIDGenerator) Next() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.seq++
	return g.connectionID + "-" + strconv.FormatUint(g.seq, 10)
}

// Current returns the current request ID without incrementing
func (g *RequestIDGenerator) Current() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.connectionID + "-" + strconv.FormatUint(g.seq, 10)
}

// ConnectionID returns the connection ID
func (g *RequestIDGenerator) ConnectionID() string {
	return g.connectionID
}

type InspectReader struct {
	r                 io.Reader
	inspector         *InspectorChain
	hostname          string
	direction         Direction
	logger            *slog.Logger
	idGenerator       *RequestIDGenerator
	pendingRequestIDs map[string]struct{} // Track request IDs for pending requests
}

func NewInspectReader(r io.Reader, inspector *InspectorChain, hostname string, direction Direction, logger *slog.Logger, idGenerator *RequestIDGenerator) *InspectReader {
	return &InspectReader{
		r:                 r,
		inspector:         inspector,
		hostname:          hostname,
		direction:         direction,
		logger:            logger,
		idGenerator:       idGenerator,
		pendingRequestIDs: make(map[string]struct{}),
	}
}

func (ir *InspectReader) Read(p []byte) (n int, err error) {
	n, err = ir.r.Read(p)
	if n > 0 && ir.inspector.ShouldInspect(ir.hostname) {
		data := make([]byte, n)
		copy(data, p[:n])

		// Determine request ID based on data direction and content
		requestID := ir.determineRequestID(data)

		err := ir.inspector.Inspect(ir.direction, data, ir.hostname, ir.idGenerator.ConnectionID(), requestID)
		if err != nil {
			ir.logger.Warn("inspect error", "error", err)
		}
	}
	return n, err
}

// determineRequestID returns the appropriate request ID for the current data
func (ir *InspectReader) determineRequestID(data []byte) string {
	// For response data (server->client), use the current request ID
	// which was generated when the corresponding request was sent
	if ir.direction == DirectionServerToClient {
		return ir.idGenerator.Current()
	}

	// For request data (client->server):
	// Only generate a new request ID when detecting a new HTTP request starts
	// Otherwise, continue using the current request ID
	if isHTTPPrefix(data) {
		return ir.idGenerator.Next()
	}
	return ir.idGenerator.Current()
}
