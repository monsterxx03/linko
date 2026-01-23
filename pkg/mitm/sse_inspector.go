package mitm

import (
	"bufio"
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

type SSEInspector struct {
	*HTTPInspector
	eventBus     *EventBus
	maxBodySize  int64
	requestCache map[string]*HTTPRequest // 缓存待处理的请求
	mutex        sync.Mutex              // 保护 requestCache 的并发访问
	connCounter  uint64                  // 连接计数器，用于生成唯一 ID
}

func NewSSEInspector(logger *slog.Logger, eventBus *EventBus, hostname string, maxBodySize int64) *SSEInspector {
	if maxBodySize == 0 {
		maxBodySize = DefaultMaxBodySize
	}
	return &SSEInspector{
		HTTPInspector: NewHTTPInspector(logger, hostname),
		eventBus:      eventBus,
		maxBodySize:   maxBodySize,
		requestCache:  make(map[string]*HTTPRequest),
		mutex:         sync.Mutex{},
		connCounter:   0,
	}
}

func (s *SSEInspector) Inspect(direction Direction, data []byte, hostname string, connectionID string) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	if direction == DirectionClientToServer {
		return s.inspectRequest(data, hostname, connectionID)
	}

	return s.inspectResponse(data, hostname, connectionID)
}

func (s *SSEInspector) inspectRequest(data []byte, hostname string, connectionID string) ([]byte, error) {
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

	// Store request in cache using connection ID as key
	s.mutex.Lock()
	s.requestCache[connectionID] = httpReq
	s.mutex.Unlock()

	// Don't send separate request event - wait for response to send combined event

	return data, nil
}

func (s *SSEInspector) inspectResponse(data []byte, hostname string, connectionID string) ([]byte, error) {
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

	// Try to find corresponding request in cache using connection ID
	var httpReq *HTTPRequest
	var requestID string

	s.mutex.Lock()
	if req, exists := s.requestCache[connectionID]; exists {
		httpReq = req
		// Generate request ID using connection ID and timestamp
		requestID = connectionID + "-" + time.Now().Format("20060102150405.000000") + "-" + req.Method
		// Remove from cache
		delete(s.requestCache, connectionID)
	}
	s.mutex.Unlock()

	if httpReq != nil {
		// Create combined event with both request and response
		event := &TrafficEvent{
			ID:           requestID,
			Hostname:     hostname,
			Timestamp:    time.Now(),
			Direction:    "complete", // Indicate complete request-response cycle
			ConnectionID: connectionID,
			Request:      httpReq,
			Response:     httpResp,
		}

		s.eventBus.Publish(event)
	} else {
		// Fallback: send separate response event if no matching request found
		event := &TrafficEvent{
			ID:           connectionID + "-" + time.Now().Format("20060102150405.000000") + "-resp",
			Hostname:     hostname,
			Timestamp:    time.Now(),
			Direction:    DirectionServerToClient.String(),
			ConnectionID: connectionID,
			Response:     httpResp,
		}

		s.eventBus.Publish(event)
	}

	return data, nil
}
