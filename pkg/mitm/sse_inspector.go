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
	eventBus      *EventBus
	logger        *slog.Logger
	httpProc      HTTPProcessorInterface
	requestCache  sync.Map
	publishStates sync.Map // requestID -> *ssePublishState (throttled snapshot publishing)
}

// Throttle settings for SSE snapshot publishing. Every published event is
// still a full snapshot (frontend merge semantics unchanged), just emitted
// less often: at most once per ssePublishMinBytes new bytes, or once per
// ssePublishInterval, whichever comes first. Vars (not consts) so tests can
// override them.
var (
	ssePublishMinBytes = 64 * 1024
	ssePublishInterval = 100 * time.Millisecond
)

// ssePublishState tracks throttled snapshot publishing for one SSE stream.
type ssePublishState struct {
	mu          sync.Mutex
	hostname    string
	lastLen     int           // body length at last publish
	lastAt      time.Time     // last publish time (zero = never published)
	pendingResp *HTTPResponse // latest snapshot, possibly unpublished
	pendingLen  int           // length of pendingResp.Body
	timer       *time.Timer   // trailing debounce timer, if any
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
	return s.inspectResponse(data, hostname, requestID)
}

func (s *SSEInspector) inspectRequest(inputData []byte, requestID string) ([]byte, error) {
	// Detect HTTP/2 requests (they use binary framing, not text-based method names)
	if isHTTP2(inputData) {
		// ALPN is restricted to HTTP/1.1 on both sides of the MITM connection,
		// so seeing h2 framing here means something leaked through (e.g. h2
		// with prior knowledge). Inspection is skipped for this stream.
		s.logger.Warn("detected HTTP/2 request despite ALPN restriction, skipping inspection",
			"request_id", requestID)
		return inputData, nil
	}

	resultData, httpMsg, complete, err := s.httpProc.ProcessRequest(inputData, requestID)
	if err != nil || httpMsg == nil {
		if err != nil {
			s.logger.Warn("invalid http msg", "err", err)
		}
		return inputData, nil
	}

	if complete {
		s.cacheChunkedRequest(httpMsg, requestID)
		s.httpProc.ClearPending(requestID)
	}

	return resultData, nil
}

func (s *SSEInspector) inspectResponse(inputData []byte, hostname string, requestID string) ([]byte, error) {
	resultData, httpMsg, complete, err := s.httpProc.ProcessResponse(inputData, requestID)
	if err != nil || httpMsg == nil {
		return inputData, nil
	}

	if httpMsg.IsSSE {
		return s.processSSEStream(httpMsg, hostname, requestID, resultData)
	}

	if complete {
		s.processCompleteResponse(httpMsg, hostname, requestID)
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

func (s *SSEInspector) processCompleteResponse(httpMsg *HTTPMessage, hostname string, requestID string) {
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

	s.publishTrafficEvent(hostname, requestID, "", httpReq, httpResp)
}

func (s *SSEInspector) processSSEStream(httpMsg *HTTPMessage, hostname string, requestID string, resultData []byte) ([]byte, error) {
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

	st := s.loadPublishState(requestID, hostname)

	st.mu.Lock()
	st.pendingResp = httpResp
	st.pendingLen = len(bodyStr)

	now := time.Now()
	due := st.lastAt.IsZero() || // first chunk always publishes (carries the request)
		len(bodyStr)-st.lastLen >= ssePublishMinBytes ||
		now.Sub(st.lastAt) >= ssePublishInterval
	if due {
		st.lastLen = len(bodyStr)
		st.lastAt = now
		if st.timer != nil {
			st.timer.Stop()
			st.timer = nil
		}
		st.mu.Unlock()
		s.publishTrafficEvent(hostname, requestID, DirectionServerToClient.String(), httpReq, httpResp)
		return resultData, nil
	}

	// Not due: the snapshot stays stashed; schedule a trailing flush so the
	// stream's tail is published even if no more chunks arrive (e.g. on
	// keep-alive connections that stay open after the stream ends).
	if st.timer == nil {
		st.timer = time.AfterFunc(ssePublishInterval, func() {
			s.trailingPublish(requestID)
		})
	}
	st.mu.Unlock()
	return resultData, nil
}

// loadPublishState returns the publish state for requestID, creating it if
// needed, and refreshes the hostname.
func (s *SSEInspector) loadPublishState(requestID, hostname string) *ssePublishState {
	if val, exists := s.publishStates.Load(requestID); exists {
		st := val.(*ssePublishState)
		st.mu.Lock()
		st.hostname = hostname
		st.mu.Unlock()
		return st
	}
	st := &ssePublishState{hostname: hostname}
	s.publishStates.Store(requestID, st)
	return st
}

// trailingPublish publishes the latest stashed snapshot of a stream whose
// updates fell below the throttle thresholds (debounced tail flush).
func (s *SSEInspector) trailingPublish(requestID string) {
	val, exists := s.publishStates.Load(requestID)
	if !exists {
		return
	}
	st := val.(*ssePublishState)

	st.mu.Lock()
	if st.pendingResp == nil || st.pendingLen <= st.lastLen {
		// Nothing new to publish.
		st.timer = nil
		st.mu.Unlock()
		return
	}
	resp := st.pendingResp
	hostname := st.hostname
	st.lastLen = st.pendingLen
	st.lastAt = time.Now()
	st.timer = nil
	st.mu.Unlock()

	s.publishTrafficEvent(hostname, requestID, DirectionServerToClient.String(), nil, resp)
}

func (s *SSEInspector) publishTrafficEvent(hostname, requestID, direction string, httpReq *HTTPRequest, httpResp *HTTPResponse) {
	event := &TrafficEvent{
		ID:           requestID,
		Timestamp:    time.Now(),
		Direction:    direction,
		ConnectionID: s.extractConnectionID(requestID),
		RequestID:    requestID,
		Request:      httpReq,
		Response:     httpResp,
		Hostname:     hostname,
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

// OnConnectionClosed purges all per-connection state when a connection
// closes. This is the backstop for SSE streams that never terminated
// cleanly (their pending response buffers accumulate the whole stream).
// Unpublished tail snapshots are flushed first: a server may close the
// connection right after the final chunk, before the debounce timer fires.
func (s *SSEInspector) OnConnectionClosed(connectionID string) {
	prefix := connectionID + "-"

	// Flush unpublished tail snapshots, stop timers, and purge publish states.
	s.publishStates.Range(func(key, val any) bool {
		id, ok := key.(string)
		if !ok || !strings.HasPrefix(id, prefix) {
			return true
		}
		st := val.(*ssePublishState)
		st.mu.Lock()
		resp := st.pendingResp
		hasUnpublished := resp != nil && st.pendingLen > st.lastLen
		hostname := st.hostname
		if st.timer != nil {
			st.timer.Stop()
			st.timer = nil
		}
		st.mu.Unlock()

		if hasUnpublished {
			s.publishTrafficEvent(hostname, id, DirectionServerToClient.String(), nil, resp)
		}
		s.publishStates.Delete(key)
		return true
	})

	deleteKeysByPrefix(&s.requestCache, prefix)
	s.httpProc.ClearPendingByPrefix(prefix)
}

// GetRequestCache returns the request cache for other inspectors to access
func (s *SSEInspector) GetRequestCache() *sync.Map {
	return &s.requestCache
}
