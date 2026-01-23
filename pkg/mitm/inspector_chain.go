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

func (c *InspectorChain) Inspect(direction Direction, data []byte, hostname string) ([]byte, error) {
	var err error
	result := data

	for _, inspector := range c.inspectors {
		result, err = inspector.Inspect(direction, result, hostname)
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

type ReadWriter struct {
	rw        io.ReadWriter
	inspector *InspectorChain
	hostname  string
	direction Direction
	logger    *slog.Logger
}

func NewReadWriter(rw io.ReadWriter, inspector *InspectorChain, hostname string, direction Direction, logger *slog.Logger) *ReadWriter {
	return &ReadWriter{
		rw:        rw,
		inspector: inspector,
		hostname:  hostname,
		direction: direction,
		logger:    logger,
	}
}

func (rw *ReadWriter) Read(p []byte) (n int, err error) {
	n, err = rw.rw.Read(p)
	if n > 0 && rw.inspector.ShouldInspect(rw.hostname) {
		data := make([]byte, n)
		copy(data, p[:n])
		modified, inspectErr := rw.inspector.Inspect(rw.direction, data, rw.hostname)
		if inspectErr != nil {
			rw.logger.Debug("inspect error", "error", inspectErr)
		}
		if modified != nil {
			copy(p[:n], modified)
		}
	}
	return n, err
}

func (rw *ReadWriter) Write(p []byte) (n int, err error) {
	if rw.inspector.ShouldInspect(rw.hostname) {
		modified, inspectErr := rw.inspector.Inspect(rw.direction, p, rw.hostname)
		if inspectErr != nil {
			rw.logger.Debug("inspect error", "error", inspectErr)
		}
		if modified != nil {
			return rw.rw.Write(modified)
		}
		return len(p), nil
	}
	return rw.rw.Write(p)
}
