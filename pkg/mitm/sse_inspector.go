package mitm

import (
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

type SSEInspector struct {
	*BaseInspector
	eventBus     *EventBus
	logger       *slog.Logger
	httpProc     HTTPProcessorInterface
	requestCache sync.Map
}

func NewSSEInspector(logger *slog.Logger, eventBus *EventBus, hostname string, maxBodySize int64) *SSEInspector {
	if maxBodySize == 0 {
		maxBodySize = DefaultMaxBodySize
	}
	return &SSEInspector{
		BaseInspector: NewBaseInspector("sse_inspector", hostname),
		eventBus:      eventBus,
		logger:        logger,
		httpProc:      NewHTTPProcessor(logger, maxBodySize),
	}
}

func (s *SSEInspector) Inspect(direction Direction, data []byte, hostname string, connectionID, requestID string) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	if direction == DirectionClientToServer {
		return s.inspectRequest(data, requestID)
	}
	return s.inspectResponse(data, requestID)
}

func (s *SSEInspector) inspectRequest(inputData []byte, requestID string) ([]byte, error) {
	resultData, httpMsg, complete, err := s.httpProc.ProcessRequest(inputData, requestID)
	if err != nil || httpMsg == nil {
		return inputData, nil
	}

	if complete {
		s.cacheChunkedRequest(httpMsg, requestID)
		s.httpProc.ClearPending(requestID)
	}

	return resultData, nil
}

func (s *SSEInspector) inspectResponse(inputData []byte, requestID string) ([]byte, error) {
	resultData, httpMsg, complete, err := s.httpProc.ProcessResponse(inputData, requestID)
	if err != nil || httpMsg == nil {
		return inputData, nil
	}

	if httpMsg.IsSSE {
		return s.processSSEStream(httpMsg, requestID, resultData)
	}

	if complete {
		s.processCompleteResponse(httpMsg, requestID)
		s.httpProc.ClearPending(requestID)
	}

	return resultData, nil
}

func (s *SSEInspector) cacheChunkedRequest(httpMsg *HTTPMessage, requestID string) {
	s.requestCache.Store(requestID, &HTTPRequest{
		Method:        httpMsg.Method,
		URL:           httpMsg.Path,
		Host:          httpMsg.Hostname,
		Headers:       httpMsg.Headers,
		Body:          string(httpMsg.Body),
		ContentType:   httpMsg.ContentType,
		ContentLength: int64(len(httpMsg.Body)),
	})
}

func (s *SSEInspector) processCompleteResponse(httpMsg *HTTPMessage, requestID string) {
	var httpReq *HTTPRequest
	if val, exists := s.requestCache.LoadAndDelete(requestID); exists {
		httpReq = val.(*HTTPRequest)
	}

	// Body is already decompressed by HTTPProcessor
	bodyStr := string(httpMsg.Body)

	httpResp := &HTTPResponse{
		Status:        http.StatusText(httpMsg.StatusCode),
		StatusCode:    httpMsg.StatusCode,
		Headers:       httpMsg.Headers,
		Body:          bodyStr,
		ContentType:   httpMsg.ContentType,
		ContentLength: int64(len(bodyStr)),
		Latency:       0,
	}

	s.publishTrafficEvent(requestID, "", httpReq, httpResp)
}

func (s *SSEInspector) processSSEStream(httpMsg *HTTPMessage, requestID string, resultData []byte) ([]byte, error) {
	var httpReq *HTTPRequest
	if val, exists := s.requestCache.LoadAndDelete(requestID); exists {
		httpReq = val.(*HTTPRequest)
	}

	// Body is already decompressed by HTTPProcessor
	bodyStr := string(httpMsg.Body)

	httpResp := &HTTPResponse{
		Status:        http.StatusText(httpMsg.StatusCode),
		StatusCode:    httpMsg.StatusCode,
		Headers:       httpMsg.Headers,
		Body:          bodyStr,
		ContentType:   httpMsg.ContentType,
		ContentLength: int64(len(bodyStr)),
	}

	s.publishTrafficEvent(requestID, DirectionServerToClient.String(), httpReq, httpResp)
	// For SSE, return the accumulated data (resultData may be longer than bodyStr if there are multiple events)
	return resultData, nil
}

func (s *SSEInspector) publishTrafficEvent(requestID, direction string, httpReq *HTTPRequest, httpResp *HTTPResponse) {
	event := &TrafficEvent{
		ID:           requestID,
		Timestamp:    time.Now(),
		Direction:    direction,
		ConnectionID: s.extractConnectionID(requestID),
		RequestID:    requestID,
		Request:      httpReq,
		Response:     httpResp,
	}
	if httpReq != nil {
		event.Hostname = httpReq.Host
	}
	s.eventBus.Publish(event)
}

func (s *SSEInspector) extractConnectionID(requestID string) string {
	if idx := strings.LastIndex(requestID, "-"); idx > 0 {
		return requestID[:idx]
	}
	return requestID
}

func (s *SSEInspector) ClearPending(requestID string) {
	s.httpProc.ClearPending(requestID)
	s.requestCache.Delete(requestID)
}

// GetRequestCache returns the request cache for other inspectors to access
func (s *SSEInspector) GetRequestCache() *sync.Map {
	return &s.requestCache
}
