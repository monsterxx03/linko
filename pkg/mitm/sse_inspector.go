package mitm

import (
	"bufio"
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type SSEInspector struct {
	*HTTPInspector
	eventBus    *EventBus
	maxBodySize int64
}

func NewSSEInspector(logger *slog.Logger, eventBus *EventBus, hostname string, maxBodySize int64) *SSEInspector {
	if maxBodySize == 0 {
		maxBodySize = DefaultMaxBodySize
	}
	return &SSEInspector{
		HTTPInspector: NewHTTPInspector(logger, hostname),
		eventBus:      eventBus,
		maxBodySize:   maxBodySize,
	}
}

func (s *SSEInspector) Inspect(direction Direction, data []byte, hostname string) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	if direction == DirectionClientToServer {
		return s.inspectRequest(data)
	}

	return s.inspectResponse(data, hostname)
}

func (s *SSEInspector) inspectRequest(data []byte) ([]byte, error) {
	if !isHTTPPrefix(data) {
		return data, nil
	}

	reader := bufio.NewReader(bytes.NewReader(data))
	req, err := http.ReadRequest(reader)
	if err != nil {
		return data, nil
	}
	defer req.Body.Close()

	headers := make(map[string]string)
	for k, v := range req.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	bodyBytes, _ := io.ReadAll(req.Body)
	bodyStr := string(bodyBytes)
	bodyStr = decompressBody(bodyStr, req.Header.Get("Content-Encoding"))
	if s.maxBodySize > 0 && len(bodyStr) > int(s.maxBodySize) {
		bodyStr = bodyStr[:s.maxBodySize]
	}

	httpReq := &HTTPRequest{
		Method:        req.Method,
		URL:           req.URL.String(),
		Host:          req.Host,
		Headers:       headers,
		Body:          bodyStr,
		ContentType:   req.Header.Get("Content-Type"),
		ContentLength: req.ContentLength,
	}

	event := &TrafficEvent{
		ID:           time.Now().Format("20060102150405.000000") + "-req-" + req.Host,
		Hostname:     req.Host,
		Timestamp:    time.Now(),
		Direction:    DirectionClientToServer.String(),
		ConnectionID: "",
		Request:      httpReq,
	}

	s.eventBus.Publish(event)

	return data, nil
}

func (s *SSEInspector) inspectResponse(data []byte, hostname string) ([]byte, error) {
	if !isHTTPResponsePrefix(data) {
		return data, nil
	}

	reader := bufio.NewReader(bytes.NewReader(data))
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		return data, nil
	}
	defer resp.Body.Close()

	headers := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyStr := string(bodyBytes)
	bodyStr = decompressBody(bodyStr, resp.Header.Get("Content-Encoding"))
	if s.maxBodySize > 0 && len(bodyStr) > int(s.maxBodySize) {
		bodyStr = bodyStr[:s.maxBodySize]
	}

	httpResp := &HTTPResponse{
		Status:        resp.Status,
		StatusCode:    resp.StatusCode,
		Headers:       headers,
		Body:          bodyStr,
		ContentType:   resp.Header.Get("Content-Type"),
		ContentLength: resp.ContentLength,
		Latency:       0,
	}

	// Use the provided hostname
	if hostname == "" {
		hostname = "unknown"
	}

	event := &TrafficEvent{
		ID:           time.Now().Format("20060102150405.000000") + "-resp-" + hostname,
		Hostname:     hostname,
		Timestamp:    time.Now(),
		Direction:    DirectionServerToClient.String(),
		ConnectionID: "",
		Response:     httpResp,
	}

	s.eventBus.Publish(event)

	return data, nil
}
