package mitm

import (
	"bufio"
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

type pendingMessage struct {
	data          []byte
	contentLength int64
	headers       []byte
	isSSE         bool
}

type SSEInspector struct {
	*HTTPInspector
	eventBus     *EventBus
	maxBodySize  int64
	pendingReqs  sync.Map
	pendingResps sync.Map
	requestCache sync.Map
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

func (s *SSEInspector) Inspect(direction Direction, data []byte, hostname string, connectionID string) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	if direction == DirectionClientToServer {
		return s.inspectRequestIncremental(data, connectionID)
	}
	return s.inspectResponseIncremental(data, connectionID)
}

func (s *SSEInspector) inspectRequestIncremental(inputData []byte, connectionID string) ([]byte, error) {
	pending := s.loadOrCreatePending(&s.pendingReqs, inputData, connectionID, false)
	if pending == nil {
		return inputData, nil
	}

	pending.data = append(pending.data, inputData...)

	if pending.headers == nil {
		return inputData, nil
	}

	return s.checkRequestComplete(pending, inputData, connectionID)
}

func (s *SSEInspector) inspectResponseIncremental(inputData []byte, connectionID string) ([]byte, error) {
	pending := s.loadOrCreatePending(&s.pendingResps, inputData, connectionID, true)
	if pending == nil {
		return inputData, nil
	}

	pending.data = append(pending.data, inputData...)

	if pending.headers == nil {
		return inputData, nil
	}

	if pending.isSSE {
		return s.processSSEStream(pending.data, connectionID)
	}

	return s.checkResponseComplete(pending, inputData, connectionID)
}

func (s *SSEInspector) loadOrCreatePending(storage *sync.Map, data []byte, connectionID string, isResponse bool) *pendingMessage {
	if val, exists := storage.Load(connectionID); exists {
		return val.(*pendingMessage)
	}

	var isPrefix func([]byte) bool
	if isResponse {
		isPrefix = isHTTPResponsePrefix
	} else {
		isPrefix = isHTTPPrefix
	}

	if !isPrefix(data) {
		return nil
	}

	pending := &pendingMessage{
		contentLength: -2,
	}

	idx := bytes.Index(data, []byte("\r\n\r\n"))
	if idx >= 0 {
		pending.headers = make([]byte, idx+4)
		copy(pending.headers, data[:idx+4])
		if isResponse {
			pending.isSSE = s.detectSSE(pending.headers)
		}
		pending.contentLength = s.parseContentLength(pending.headers, isResponse)
	}

	storage.Store(connectionID, pending)
	return pending
}

func (s *SSEInspector) parseContentLength(headerData []byte, isResponse bool) int64 {
	reader := bytes.NewReader(headerData)

	if isResponse {
		resp, err := http.ReadResponse(bufio.NewReader(reader), nil)
		if err != nil {
			return 0
		}
		defer resp.Body.Close()

		if resp.Header.Get("Transfer-Encoding") == "chunked" {
			return -1
		}
		return resp.ContentLength
	}

	req, err := http.ReadRequest(bufio.NewReader(reader))
	if err != nil {
		return 0
	}
	defer req.Body.Close()

	if req.Header.Get("Transfer-Encoding") == "chunked" {
		return -1
	}
	return req.ContentLength
}

func (s *SSEInspector) detectSSE(headerData []byte) bool {
	reader := bytes.NewReader(headerData)
	resp, err := http.ReadResponse(bufio.NewReader(reader), nil)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream")
}

func (s *SSEInspector) cacheChunkedRequest(data []byte, connectionID string) {
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(data)))
	if err != nil {
		return
	}
	defer req.Body.Close()

	s.requestCache.Store(connectionID, &HTTPRequest{
		Method:        req.Method,
		URL:           req.URL.String(),
		Host:          req.Host,
		Headers:       extractHeaders(req.Header),
		Body:          "",
		ContentType:   req.Header.Get("Content-Type"),
		ContentLength: req.ContentLength,
	})
}

func (s *SSEInspector) checkRequestComplete(pending *pendingMessage, inputData []byte, connectionID string) ([]byte, error) {
	switch pending.contentLength {
	case 0:
		s.pendingReqs.Delete(connectionID)
		return s.processCompleteRequest(pending.data, connectionID)
	case -1:
		bodyEndIdx := bytes.Index(pending.data, []byte("\r\n0\r\n\r\n"))
		if bodyEndIdx >= 0 {
			endIdx := bodyEndIdx + 7
			s.pendingReqs.Delete(connectionID)
			fullData := make([]byte, endIdx)
			copy(fullData, pending.data[:endIdx])
			s.cacheChunkedRequest(fullData, connectionID)
			return fullData, nil
		}
	default:
		needed := int(pending.contentLength) + len(pending.headers)
		if len(pending.data) >= needed {
			s.pendingReqs.Delete(connectionID)
			fullData := make([]byte, needed)
			copy(fullData, pending.data[:needed])
			return s.processCompleteRequest(fullData, connectionID)
		}
	}
	return inputData, nil
}

func (s *SSEInspector) checkResponseComplete(pending *pendingMessage, inputData []byte, connectionID string) ([]byte, error) {
	switch pending.contentLength {
	case 0:
		s.pendingResps.Delete(connectionID)
		return s.processCompleteResponse(pending.data, connectionID)
	case -1:
		bodyEndIdx := bytes.Index(pending.data, []byte("\r\n0\r\n\r\n"))
		if bodyEndIdx >= 0 {
			endIdx := bodyEndIdx + 7
			s.pendingResps.Delete(connectionID)
			fullData := make([]byte, endIdx)
			copy(fullData, pending.data[:endIdx])
			return s.processCompleteResponse(fullData, connectionID)
		}
	default:
		needed := int(pending.contentLength) + len(pending.headers)
		if len(pending.data) >= needed {
			s.pendingResps.Delete(connectionID)
			fullData := make([]byte, needed)
			copy(fullData, pending.data[:needed])
			return s.processCompleteResponse(fullData, connectionID)
		}
	}
	return inputData, nil
}

func (s *SSEInspector) processCompleteRequest(data []byte, connectionID string) ([]byte, error) {
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(data)))
	if err != nil {
		return data, nil
	}
	defer req.Body.Close()

	bodyBytes, _ := io.ReadAll(req.Body)
	bodyStr := s.truncateBody(bodyBytes, req.Header.Get("Content-Encoding"), req.Header.Get("Content-Type"))

	s.requestCache.Store(connectionID, &HTTPRequest{
		Method:        req.Method,
		URL:           req.URL.String(),
		Host:          req.Host,
		Headers:       extractHeaders(req.Header),
		Body:          bodyStr,
		ContentType:   req.Header.Get("Content-Type"),
		ContentLength: req.ContentLength,
	})

	return data, nil
}

func (s *SSEInspector) processCompleteResponse(data []byte, connectionID string) ([]byte, error) {
	resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(data)), nil)
	if err != nil {
		return data, nil
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyStr := s.truncateBody(bodyBytes, resp.Header.Get("Content-Encoding"), resp.Header.Get("Content-Type"))

	var httpReq *HTTPRequest
	if val, exists := s.requestCache.LoadAndDelete(connectionID); exists {
		httpReq = val.(*HTTPRequest)
	}

	httpResp := &HTTPResponse{
		Status:        resp.Status,
		StatusCode:    resp.StatusCode,
		Headers:       extractHeaders(resp.Header),
		Body:          bodyStr,
		ContentType:   resp.Header.Get("Content-Type"),
		ContentLength: resp.ContentLength,
		Latency:       0,
	}

	s.publishTrafficEvent(connectionID, "", httpReq, httpResp)
	return data, nil
}

func (s *SSEInspector) processSSEStream(fullData []byte, connectionID string) ([]byte, error) {
	resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(fullData)), nil)
	if err != nil {
		return fullData, nil
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyStr := s.truncateBody(bodyBytes, resp.Header.Get("Content-Encoding"), resp.Header.Get("Content-Type"))

	var httpReq *HTTPRequest
	if val, exists := s.requestCache.LoadAndDelete(connectionID); exists {
		httpReq = val.(*HTTPRequest)
	}

	httpResp := &HTTPResponse{
		Status:        resp.Status,
		StatusCode:    resp.StatusCode,
		Headers:       extractHeaders(resp.Header),
		Body:          bodyStr,
		ContentType:   resp.Header.Get("Content-Type"),
		ContentLength: resp.ContentLength,
	}

	s.publishTrafficEvent(connectionID, DirectionServerToClient.String(), httpReq, httpResp)
	return fullData, nil
}

func (s *SSEInspector) truncateBody(body []byte, contentEncoding, contentType string) string {
	decompressed := decompressBody(body, contentEncoding, contentType, s.HTTPInspector.Logger)
	bodyStr := string(decompressed)
	if s.maxBodySize > 0 && len(bodyStr) > int(s.maxBodySize) {
		return bodyStr[:s.maxBodySize]
	}
	return bodyStr
}

func extractHeaders(header http.Header) map[string]string {
	headers := make(map[string]string)
	for k, v := range header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}
	return headers
}

func (s *SSEInspector) publishTrafficEvent(connectionID, direction string, httpReq *HTTPRequest, httpResp *HTTPResponse) {
	event := &TrafficEvent{
		ID:           connectionID,
		Timestamp:    time.Now(),
		Direction:    direction,
		ConnectionID: connectionID,
		Request:      httpReq,
		Response:     httpResp,
	}
	if httpReq != nil {
		event.Hostname = httpReq.Host
	}
	s.eventBus.Publish(event)
}

func (s *SSEInspector) ClearPending(connectionID string) {
	s.pendingReqs.Delete(connectionID)
	s.pendingResps.Delete(connectionID)
	s.requestCache.Delete(connectionID)
}
