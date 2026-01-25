package mitm

import (
	"bufio"
	"bytes"
	"log/slog"
	"net/http"
)

type HTTPInspector struct {
	*BaseInspector
	Logger *slog.Logger
}

func NewHTTPInspector(logger *slog.Logger, hostname string) *HTTPInspector {
	return &HTTPInspector{
		BaseInspector: NewBaseInspector("http-inspector", hostname),
		Logger:        logger,
	}
}

func (h *HTTPInspector) Inspect(direction Direction, data []byte, hostname string, connectionID, requestID string) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	if direction == DirectionClientToServer {
		return h.inspectRequest(data)
	}

	return h.inspectResponse(data, hostname)
}

func (h *HTTPInspector) inspectRequest(data []byte) ([]byte, error) {
	if !isHTTPPrefix(data) {
		return data, nil
	}

	reader := bufio.NewReader(bytes.NewReader(data))
	req, err := http.ReadRequest(reader)
	if err != nil {
		return data, nil
	}
	defer req.Body.Close()

	h.Logger.Debug("HTTP request",
		"method", req.Method,
		"url", req.URL.String(),
		"host", req.Host,
		"user-agent", req.UserAgent(),
	)

	return data, nil
}

func (h *HTTPInspector) inspectResponse(data []byte, hostname string) ([]byte, error) {
	if !isHTTPResponsePrefix(data) {
		return data, nil
	}

	reader := bufio.NewReader(bytes.NewReader(data))
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		return data, nil
	}
	defer resp.Body.Close()

	h.Logger.Debug("HTTP response",
		"status", resp.Status,
		"content-type", resp.Header.Get("Content-Type"),
		"content-length", resp.ContentLength,
		"hostname", hostname,
	)

	return data, nil
}
