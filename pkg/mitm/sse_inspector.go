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

type PendingMessage struct {
	Data          bytes.Buffer
	ContentLength int64
	Headers       []byte
	IsSSE         bool // 是否为 SSE 响应
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
		return s.inspectRequestIncremental(data, hostname, connectionID)
	}

	return s.inspectResponseIncremental(data, hostname, connectionID)
}

func (s *SSEInspector) inspectRequestIncremental(data []byte, hostname string, connectionID string) ([]byte, error) {
	var pending *PendingMessage
	if val, exists := s.pendingReqs.Load(connectionID); exists {
		pending = val.(*PendingMessage)
	}

	if pending == nil {
		if !isHTTPPrefix(data) {
			return data, nil
		}

		pending = &PendingMessage{
			Data:          bytes.Buffer{},
			ContentLength: -2,
		}
		pending.Data.Write(data)

		idx := bytes.Index(data, []byte("\r\n\r\n"))
		if idx >= 0 {
			headerData := make([]byte, idx+4)
			copy(headerData, data[:idx+4])
			pending.Headers = headerData

			reader := bytes.NewReader(headerData)
			req, err := http.ReadRequest(bufio.NewReader(reader))
			if err != nil {
				return data, nil
			}

			if req.ContentLength > 0 {
				pending.ContentLength = req.ContentLength
			} else if req.Header.Get("Transfer-Encoding") == "chunked" {
				pending.ContentLength = -1
			} else {
				pending.ContentLength = 0
			}

			req.Body.Close()
		}

		s.pendingReqs.Store(connectionID, pending)
	} else {
		pending.Data.Write(data)
	}

	if pending.ContentLength == 0 && pending.Headers != nil {
		s.pendingReqs.Delete(connectionID)
		return s.processCompleteRequest(pending.Data.Bytes(), hostname, connectionID)
	}

	if pending.ContentLength > 0 && pending.Headers != nil {
		needed := int(pending.ContentLength) + len(pending.Headers)
		if pending.Data.Len() >= needed {
			fullData := make([]byte, needed)
			copy(fullData, pending.Data.Bytes()[:needed])

			s.pendingReqs.Delete(connectionID)
			return s.processCompleteRequest(fullData, hostname, connectionID)
		}
	}

	if pending.ContentLength < 0 && pending.Headers != nil {
		fullData := pending.Data.Bytes()
		// find chunked body end
		bodyEnd := bytes.Index(fullData, []byte("\r\n0\r\n"))
		if bodyEnd >= 0 {
			fullData = make([]byte, bodyEnd+5)
			copy(fullData, pending.Data.Bytes()[:bodyEnd+5])

			s.pendingReqs.Delete(connectionID)
			return s.processCompleteRequest(fullData, hostname, connectionID)
		}
	}

	return data, nil
}

func (s *SSEInspector) inspectResponseIncremental(data []byte, hostname string, connectionID string) ([]byte, error) {
	var pending *PendingMessage
	if val, exists := s.pendingResps.Load(connectionID); exists {
		pending = val.(*PendingMessage)
	}

	if pending == nil {
		if !isHTTPResponsePrefix(data) {
			return data, nil
		}

		pending = &PendingMessage{
			Data:          bytes.Buffer{},
			ContentLength: -2,
		}
		pending.Data.Write(data)

		idx := bytes.Index(data, []byte("\r\n\r\n"))
		if idx >= 0 {
			headerData := make([]byte, idx+4)
			copy(headerData, data[:idx+4])
			pending.Headers = headerData

			reader := bytes.NewReader(headerData)
			resp, err := http.ReadResponse(bufio.NewReader(reader), nil)
			if err != nil {
				return data, nil
			}

			// 检测 SSE 响应
			if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
				pending.IsSSE = true
				pending.ContentLength = -1 // SSE 是流式响应，无明确结束
			} else if resp.ContentLength > 0 {
				pending.ContentLength = resp.ContentLength
			} else if resp.Header.Get("Transfer-Encoding") == "chunked" {
				pending.ContentLength = -1
			} else {
				pending.ContentLength = 0
			}

			resp.Body.Close()
		}

		s.pendingResps.Store(connectionID, pending)
	} else {
		pending.Data.Write(data)
	}

	if pending.ContentLength == 0 && pending.Headers != nil {
		s.pendingResps.Delete(connectionID)
		return s.processCompleteResponse(pending.Data.Bytes(), hostname, connectionID)
	}

	if pending.ContentLength > 0 && pending.Headers != nil {
		needed := int(pending.ContentLength) + len(pending.Headers)
		if pending.Data.Len() >= needed {
			fullData := make([]byte, needed)
			copy(fullData, pending.Data.Bytes()[:needed])

			s.pendingResps.Delete(connectionID)
			return s.processCompleteResponse(fullData, hostname, connectionID)
		}
	}

	if pending.ContentLength < 0 && pending.Headers != nil {
		fullData := pending.Data.Bytes()
		// 对于非 SSE 的 chunked 响应，检查是否结束
		if !pending.IsSSE {
			bodyEnd := bytes.Index(fullData, []byte("\r\n0\r\n"))
			if bodyEnd >= 0 {
				fullData = make([]byte, bodyEnd+5)
				copy(fullData, pending.Data.Bytes()[:bodyEnd+5])

				s.pendingResps.Delete(connectionID)
				return s.processCompleteResponse(fullData, hostname, connectionID)
			}
		} else {
			// SSE 响应：增量处理事件
			return s.processSSEStream(fullData, hostname, connectionID)
		}
	}

	return data, nil
}

func (s *SSEInspector) processCompleteRequest(data []byte, hostname string, connectionID string) ([]byte, error) {
	reader := bufio.NewReader(bytes.NewReader(data))
	req, err := http.ReadRequest(reader)
	if err != nil {
		return data, nil
	}
	defer req.Body.Close()

	bodyBytes, _ := io.ReadAll(req.Body)
	decompressedBody := decompressBody(bodyBytes, req.Header.Get("Content-Encoding"), req.Header.Get("Content-Type"), s.HTTPInspector.Logger)
	bodyStr := string(decompressedBody)
	if s.maxBodySize > 0 && len(bodyStr) > int(s.maxBodySize) {
		bodyStr = bodyStr[:s.maxBodySize]
	}

	headers := make(map[string]string)
	for k, v := range req.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
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

	s.requestCache.Store(connectionID, httpReq)

	return data, nil
}

func (s *SSEInspector) processCompleteResponse(data []byte, hostname string, connectionID string) ([]byte, error) {
	reader := bufio.NewReader(bytes.NewReader(data))
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		return data, nil
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	decompressedBody := decompressBody(bodyBytes, resp.Header.Get("Content-Encoding"), resp.Header.Get("Content-Type"), s.HTTPInspector.Logger)
	bodyStr := string(decompressedBody)
	if s.maxBodySize > 0 && len(bodyStr) > int(s.maxBodySize) {
		bodyStr = bodyStr[:s.maxBodySize]
	}

	headers := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
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

	var httpReq *HTTPRequest

	if val, exists := s.requestCache.Load(connectionID); exists {
		httpReq = val.(*HTTPRequest)
		s.requestCache.Delete(connectionID)
	}

	if httpReq != nil {
		event := &TrafficEvent{
			ID:           connectionID,
			Hostname:     hostname,
			Timestamp:    time.Now(),
			Direction:    "complete",
			ConnectionID: connectionID,
			Request:      httpReq,
			Response:     httpResp,
		}
		s.eventBus.Publish(event)
	} else {
		event := &TrafficEvent{
			ID:           connectionID,
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

// processSSEStream 处理 SSE 流式响应，直接返回累积数据并发布，不解压不解析
func (s *SSEInspector) processSSEStream(fullData []byte, hostname string, connectionID string) ([]byte, error) {
	// 发布原始 HTTP 响应到 eventBus（不解压不解析）
	s.publishOriginalResponse(fullData, hostname, connectionID)

	// 返回完整响应数据（不解压）
	return fullData, nil
}

// publishOriginalResponse 发布原始 HTTP 响应到 eventBus
func (s *SSEInspector) publishOriginalResponse(fullData []byte, hostname string, connectionID string) {
	reader := bufio.NewReader(bytes.NewReader(fullData))
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	decompressedBody := decompressBody(bodyBytes, resp.Header.Get("Content-Encoding"), resp.Header.Get("Content-Type"), s.HTTPInspector.Logger)
	bodyStr := string(decompressedBody)
	if s.maxBodySize > 0 && len(bodyStr) > int(s.maxBodySize) {
		bodyStr = bodyStr[:s.maxBodySize]
	}

	headers := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	httpResp := &HTTPResponse{
		Status:        resp.Status,
		StatusCode:    resp.StatusCode,
		Headers:       headers,
		Body:          bodyStr,
		ContentType:   resp.Header.Get("Content-Type"),
		ContentLength: resp.ContentLength,
	}

	var httpReq *HTTPRequest
	if val, exists := s.requestCache.Load(connectionID); exists {
		httpReq = val.(*HTTPRequest)
		s.requestCache.Delete(connectionID)
	}

	if httpReq != nil {
		event := &TrafficEvent{
			ID:           connectionID,
			Hostname:     hostname,
			Timestamp:    time.Now(),
			Direction:    "complete",
			ConnectionID: connectionID,
			Request:      httpReq,
			Response:     httpResp,
		}
		s.eventBus.Publish(event)
	} else {
		event := &TrafficEvent{
			ID:           connectionID,
			Hostname:     hostname,
			Timestamp:    time.Now(),
			Direction:    DirectionServerToClient.String(),
			ConnectionID: connectionID,
			Response:     httpResp,
		}
		s.eventBus.Publish(event)
	}
}

func (s *SSEInspector) ClearPending(connectionID string) {
	s.pendingReqs.Delete(connectionID)
	s.pendingResps.Delete(connectionID)
	s.requestCache.Delete(connectionID)
}
