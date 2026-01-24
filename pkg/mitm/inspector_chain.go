package mitm

import (
	"fmt"
	"io"
	"log/slog"
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

func (c *InspectorChain) Inspect(direction Direction, data []byte, hostname string, connectionID string) ([]byte, error) {
	var err error
	result := data

	for _, inspector := range c.inspectors {
		result, err = inspector.Inspect(direction, result, hostname, connectionID)
		if err != nil {
			return nil, fmt.Errorf("inspector %s failed: %w", inspector.Name(), err)
		}
		if result == nil {
			return nil, nil
		}
	}

	return result, nil
}

func (c *InspectorChain) ShouldInspect(hostname string) bool {
	for _, inspector := range c.inspectors {
		if inspector.ShouldInspect(hostname) {
			return true
		}
	}
	return false
}

type InspectReader struct {
	r            io.Reader
	inspector    *InspectorChain
	hostname     string
	direction    Direction
	logger       *slog.Logger
	connectionID string
}

func NewInspectReader(r io.Reader, inspector *InspectorChain, hostname string, direction Direction, logger *slog.Logger, connectionID string) *InspectReader {
	return &InspectReader{
		r:            r,
		inspector:    inspector,
		hostname:     hostname,
		direction:    direction,
		logger:       logger,
		connectionID: connectionID,
	}
}

func (ir *InspectReader) Read(p []byte) (n int, err error) {
	n, err = ir.r.Read(p)
	if n > 0 && ir.inspector.ShouldInspect(ir.hostname) {
		data := make([]byte, n)
		copy(data, p[:n])
		_, err := ir.inspector.Inspect(ir.direction, data, ir.hostname, ir.connectionID)
		if err != nil {
			ir.logger.Debug("inspect error", "error", err)
		}
	}
	return n, err
}
