package mitm

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// mockSSEHTTPProcessor implements HTTPProcessorInterface for SSE inspector testing
type mockSSEHTTPProcessor struct {
	t                   *testing.T
	processRequestFunc  func(data []byte, requestID string) ([]byte, *HTTPMessage, bool, error)
	processResponseFunc func(data []byte, requestID string) ([]byte, *HTTPMessage, bool, error)
}

func newMockSSEHTTPProcessor(t *testing.T) *mockSSEHTTPProcessor {
	return &mockSSEHTTPProcessor{t: t}
}

func (m *mockSSEHTTPProcessor) ProcessRequest(inputData []byte, requestID string) ([]byte, *HTTPMessage, bool, error) {
	if m.processRequestFunc != nil {
		return m.processRequestFunc(inputData, requestID)
	}
	return inputData, nil, false, nil
}

func (m *mockSSEHTTPProcessor) ProcessResponse(inputData []byte, requestID string) ([]byte, *HTTPMessage, bool, error) {
	if m.processResponseFunc != nil {
		return m.processResponseFunc(inputData, requestID)
	}
	return inputData, nil, false, nil
}

func (m *mockSSEHTTPProcessor) ClearPending(requestID string) {
	// No-op for mock
}

func (m *mockSSEHTTPProcessor) ClearPendingByPrefix(prefix string) {
	// No-op for mock
}

func (m *mockSSEHTTPProcessor) GetPendingMessage(requestID string) (*HTTPMessage, bool) {
	return nil, false
}

func TestSSEInspector_InspectRequest_Basic(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)

	// Replace with mock
	mockProc := newMockSSEHTTPProcessor(t)
	mockProc.processRequestFunc = func(data []byte, requestID string) ([]byte, *HTTPMessage, bool, error) {
		msg := &HTTPMessage{
			Hostname:    "example.com",
			Path:        "/",
			Method:      "GET",
			ContentType: "text/plain",
			Body:        []byte("Hello World"),
		}
		return data, msg, true, nil
	}
	inspector.httpProc = mockProc

	requestData := []byte("GET / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 11\r\n\r\nHello World")
	requestID := "test-1-1"

	result, err := inspector.Inspect(DirectionClientToServer, requestData, "example.com", "test-1", requestID)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if !bytes.Equal(result, requestData) {
		t.Error("Expected result to match input")
	}

	// Verify request was cached
	if _, exists := inspector.requestCache.Load(requestID); !exists {
		t.Error("Expected request to be cached")
	}
}

func TestSSEInspector_InspectResponse_Basic(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)
	requestID := "test-2-1"

	// First cache a request via mock
	mockProc := newMockSSEHTTPProcessor(t)
	mockProc.processRequestFunc = func(data []byte, rid string) ([]byte, *HTTPMessage, bool, error) {
		msg := &HTTPMessage{
			Hostname:    "example.com",
			Path:        "/",
			Method:      "GET",
			ContentType: "text/plain",
			Body:        []byte{},
		}
		return data, msg, true, nil
	}
	inspector.httpProc = mockProc

	requestData := []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
	_, _ = inspector.Inspect(DirectionClientToServer, requestData, "example.com", "test-2", requestID)

	// Now inspect response via mock
	mockProc.processResponseFunc = func(data []byte, rid string) ([]byte, *HTTPMessage, bool, error) {
		msg := &HTTPMessage{
			StatusCode:  200,
			ContentType: "text/plain",
			Body:        []byte("Hello Server"),
		}
		return data, msg, true, nil
	}

	responseData := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 12\r\n\r\nHello Server")
	result, err := inspector.Inspect(DirectionServerToClient, responseData, "example.com", "test-2", requestID)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if !bytes.Equal(result, responseData) {
		t.Error("Expected result to match input")
	}

	// Verify request was removed from cache
	if _, exists := inspector.requestCache.Load(requestID); exists {
		t.Error("Expected request to be removed from cache after response")
	}
}

func TestSSEInspector_InspectResponse_SSE(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)
	requestID := "test-sse-1"

	// First cache a request
	mockProc := newMockSSEHTTPProcessor(t)
	mockProc.processRequestFunc = func(data []byte, rid string) ([]byte, *HTTPMessage, bool, error) {
		msg := &HTTPMessage{
			Hostname: "example.com",
			Path:     "/events",
			Method:   "GET",
		}
		return data, msg, true, nil
	}
	inspector.httpProc = mockProc

	requestData := []byte("GET /events HTTP/1.1\r\nHost: example.com\r\n\r\n")
	_, _ = inspector.Inspect(DirectionClientToServer, requestData, "example.com", "test-sse-1", requestID)

	// Now inspect SSE response
	mockProc.processResponseFunc = func(data []byte, rid string) ([]byte, *HTTPMessage, bool, error) {
		msg := &HTTPMessage{
			StatusCode:  200,
			ContentType: "text/event-stream",
			Body:        []byte("data: hello\r\n\r\n"),
			IsSSE:       true,
		}
		return data, msg, false, nil
	}

	responseData := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\ndata: hello\r\n\r\n")
	result, err := inspector.Inspect(DirectionServerToClient, responseData, "example.com", "test-sse-1", requestID)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if !bytes.Equal(result, responseData) {
		t.Error("Expected result to match input")
	}
}

func TestSSEInspector_EmptyData(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)

	// Replace with mock that returns empty
	mockProc := newMockSSEHTTPProcessor(t)
	inspector.httpProc = mockProc

	result, err := inspector.Inspect(DirectionClientToServer, []byte{}, "example.com", "test-7", "test-7-1")
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("Expected empty result, got %d bytes", len(result))
	}
}

func TestSSEInspector_InvalidHTTP(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)

	// Replace with mock that returns nil message
	mockProc := newMockSSEHTTPProcessor(t)
	mockProc.processRequestFunc = func(data []byte, requestID string) ([]byte, *HTTPMessage, bool, error) {
		return data, nil, false, nil
	}
	inspector.httpProc = mockProc

	invalidData := []byte("Invalid HTTP data")
	result, err := inspector.Inspect(DirectionClientToServer, invalidData, "example.com", "test-8", "test-8-1")
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if !bytes.Equal(result, invalidData) {
		t.Error("Expected result to match input")
	}
}

func TestSSEInspector_ClearPending(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)
	requestID := "test-9-1"

	// Cache a request
	mockProc := newMockSSEHTTPProcessor(t)
	mockProc.processRequestFunc = func(data []byte, rid string) ([]byte, *HTTPMessage, bool, error) {
		msg := &HTTPMessage{
			Hostname: "example.com",
			Path:     "/",
			Method:   "GET",
		}
		return data, msg, true, nil
	}
	inspector.httpProc = mockProc

	requestData := []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
	_, _ = inspector.Inspect(DirectionClientToServer, requestData, "test-9", "test-9", requestID)

	if _, exists := inspector.requestCache.Load(requestID); !exists {
		t.Error("Expected request to be cached")
	}

	inspector.ClearPending(requestID)

	if _, exists := inspector.requestCache.Load(requestID); exists {
		t.Error("Expected request to be cleared from cache")
	}
}

func TestSSEInspector_IncrementalRequest(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)

	// Mock that returns incomplete on first call, complete on second
	callCount := 0
	mockProc := newMockSSEHTTPProcessor(t)
	mockProc.processRequestFunc = func(data []byte, requestID string) ([]byte, *HTTPMessage, bool, error) {
		callCount++
		if callCount == 1 {
			// First call - incomplete
			return data, nil, false, nil
		}
		// Second call - complete
		msg := &HTTPMessage{
			Hostname:    "example.com",
			Path:        "/",
			Method:      "GET",
			ContentType: "text/plain",
			Body:        []byte("Hello World"),
		}
		return data, msg, true, nil
	}
	inspector.httpProc = mockProc

	// First chunk
	chunk1 := []byte("GET / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 11\r\n\r\nHello")
	requestID := "test-3-1"
	result1, err := inspector.Inspect(DirectionClientToServer, chunk1, "example.com", "test-3", requestID)
	if err != nil {
		t.Fatalf("Inspect chunk1 failed: %v", err)
	}

	_ = result1

	// Second chunk - should complete
	chunk2 := []byte(" World")
	result2, err := inspector.Inspect(DirectionClientToServer, chunk2, "example.com", "test-3", requestID)
	if err != nil {
		t.Fatalf("Inspect chunk2 failed: %v", err)
	}

	// When complete, should return non-empty result
	if len(result2) == 0 {
		t.Error("Expected non-empty result when complete")
	}

	// Verify cached
	if _, exists := inspector.requestCache.Load(requestID); !exists {
		t.Error("Expected request to be cached after complete")
	}
}

func TestSSEInspector_CompressedRequest(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)

	// Replace with mock that returns compressed body
	mockProc := newMockSSEHTTPProcessor(t)
	mockProc.processRequestFunc = func(data []byte, requestID string) ([]byte, *HTTPMessage, bool, error) {
		// Create gzip compressed body
		originalBody := "Hello World"
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		gz.Write([]byte(originalBody))
		gz.Close()
		compressedBody := buf.Bytes()

		msg := &HTTPMessage{
			Hostname:    "example.com",
			Path:        "/",
			Method:      "POST",
			ContentType: "text/plain",
			Body:        compressedBody,
		}
		return data, msg, true, nil
	}
	inspector.httpProc = mockProc

	requestData := []byte("POST / HTTP/1.1\r\nHost: example.com\r\nContent-Encoding: gzip\r\nContent-Length: 11\r\n\r\n")
	requestID := "test-comp-1"

	_, err := inspector.Inspect(DirectionClientToServer, requestData, "example.com", "test-comp", requestID)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	// Verify request was cached
	if _, exists := inspector.requestCache.Load(requestID); !exists {
		t.Error("Expected request to be cached")
	}
}

// TestSSEInspector_OnConnectionClosed verifies the connection-close backstop:
// the pending SSE response buffer (which accumulates the whole stream) is
// purged when the connection closes.
func TestSSEInspector_OnConnectionClosed(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024) // keep the real HTTPProcessor
	const connID = "conn-sse-close"
	requestID := connID + "-1"

	body := `{"q":1}`
	reqData := fmt.Appendf(nil, "POST /api HTTP/1.1\r\nHost: example.com\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s", len(body), body)
	if _, err := inspector.Inspect(DirectionClientToServer, reqData, "example.com", connID, requestID); err != nil {
		t.Fatalf("request inspect failed: %v", err)
	}

	resp := "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\ndata: {\"a\":1}\n\n"
	if _, err := inspector.Inspect(DirectionServerToClient, []byte(resp), "example.com", connID, requestID); err != nil {
		t.Fatalf("response inspect failed: %v", err)
	}

	// The SSE pending response accumulates until the connection closes.
	if _, ok := inspector.httpProc.GetPendingMessage(requestID); !ok {
		t.Fatal("expected pending SSE response")
	}

	inspector.OnConnectionClosed(connID)

	if _, ok := inspector.httpProc.GetPendingMessage(requestID); ok {
		t.Error("pending SSE response not purged on connection close")
	}
	if _, ok := inspector.requestCache.Load(requestID); ok {
		t.Error("requestCache entry not purged on connection close")
	}
}

// --- SSE snapshot throttling tests (real HTTPProcessor) ---

func setTestThrottle(t *testing.T, minBytes int, interval time.Duration) {
	t.Helper()
	ob, oi := ssePublishMinBytes, ssePublishInterval
	ssePublishMinBytes, ssePublishInterval = minBytes, interval
	t.Cleanup(func() { ssePublishMinBytes, ssePublishInterval = ob, oi })
}

func newThrottleTestInspector(t *testing.T) (*SSEInspector, *Subscriber) {
	t.Helper()
	logger := slog.Default()
	eventBus := NewEventBus(logger, 100)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)
	sub := eventBus.Subscribe()
	return inspector, sub
}

func sendTestRequest(t *testing.T, inspector *SSEInspector, connID, requestID string) {
	t.Helper()
	body := `{"q":1}`
	reqData := fmt.Appendf(nil, "POST /api HTTP/1.1\r\nHost: example.com\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s", len(body), body)
	if _, err := inspector.Inspect(DirectionClientToServer, reqData, "example.com", connID, requestID); err != nil {
		t.Fatalf("request inspect failed: %v", err)
	}
}

func receiveTrafficEvent(t *testing.T, sub *Subscriber, timeout time.Duration) *PublishedEvent {
	t.Helper()
	select {
	case pe := <-sub.Channel:
		return pe
	case <-time.After(timeout):
		t.Fatal("timed out waiting for traffic event")
		return nil
	}
}

// drainTrafficEvents returns all immediately available events without blocking.
func drainTrafficEvents(sub *Subscriber) []*PublishedEvent {
	var events []*PublishedEvent
	for {
		select {
		case pe := <-sub.Channel:
			events = append(events, pe)
		default:
			return events
		}
	}
}

// TestSSEInspector_ThrottledTailFlush verifies that small chunks below the
// byte threshold are not published per chunk, but the stream's tail is
// flushed by the debounce timer with the full accumulated content.
func TestSSEInspector_ThrottledTailFlush(t *testing.T) {
	setTestThrottle(t, 64*1024, 50*time.Millisecond)
	inspector, sub := newThrottleTestInspector(t)
	const connID = "conn-throttle"
	requestID := connID + "-1"
	sendTestRequest(t, inspector, connID, requestID)

	chunks := []string{
		"HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\ndata: {\"n\":0}\n\n",
	}
	for i := 1; i < 10; i++ {
		chunks = append(chunks, fmt.Sprintf("data: {\"n\":%d}\n\n", i))
	}
	for _, c := range chunks {
		if _, err := inspector.Inspect(DirectionServerToClient, []byte(c), "example.com", connID, requestID); err != nil {
			t.Fatalf("response inspect failed: %v", err)
		}
	}

	// Only the first chunk is published immediately (below byte threshold,
	// inside the debounce interval).
	first := receiveTrafficEvent(t, sub, time.Second)
	if first.Event.Request == nil {
		t.Error("first snapshot should carry the request")
	}
	if got := drainTrafficEvents(sub); len(got) != 0 {
		t.Fatalf("expected no more events before debounce, got %d", len(got))
	}

	// The trailing flush publishes the full accumulated stream.
	tail := receiveTrafficEvent(t, sub, 2*time.Second)
	if tail.Event.Response == nil || !strings.Contains(tail.Event.Response.Body, `{"n":9}`) {
		t.Errorf("tail snapshot missing final chunk content")
	}

	// Nothing further.
	time.Sleep(150 * time.Millisecond)
	if got := drainTrafficEvents(sub); len(got) != 0 {
		t.Fatalf("expected no more events after tail flush, got %d", len(got))
	}
}

// TestSSEInspector_ByteThresholdPublishing verifies that during a fast
// stream, snapshots are published once per ssePublishMinBytes of new data
// instead of once per chunk.
func TestSSEInspector_ByteThresholdPublishing(t *testing.T) {
	setTestThrottle(t, 4096, 10*time.Second) // interval effectively disabled
	inspector, sub := newThrottleTestInspector(t)
	const connID = "conn-bytes"
	requestID := connID + "-1"
	sendTestRequest(t, inspector, connID, requestID)

	// 5 chunks x ~2KB payload: publishes on chunks 1, 3, 5.
	chunkData := strings.Repeat("y", 2048)
	hdr := "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\n"
	for i := 0; i < 5; i++ {
		data := fmt.Sprintf("data: %s\n\n", chunkData)
		if i == 0 {
			data = hdr + data
		}
		if _, err := inspector.Inspect(DirectionServerToClient, []byte(data), "example.com", connID, requestID); err != nil {
			t.Fatalf("chunk %d inspect failed: %v", i, err)
		}
	}

	events := []*PublishedEvent{
		receiveTrafficEvent(t, sub, time.Second),
		receiveTrafficEvent(t, sub, time.Second),
		receiveTrafficEvent(t, sub, time.Second),
	}
	if got := drainTrafficEvents(sub); len(got) != 0 {
		t.Fatalf("expected exactly 3 throttled events, got %d more", len(got))
	}
	last := events[len(events)-1]
	if last.Event.Response == nil {
		t.Fatal("final snapshot has no response")
	}
	wantLen := 5 * len("data: "+chunkData+"\n\n")
	if len(last.Event.Response.Body) != wantLen {
		t.Errorf("final snapshot body length = %d, want %d (full stream)", len(last.Event.Response.Body), wantLen)
	}
}

// TestSSEInspector_ConnectionCloseFlushesTail verifies that when a
// connection closes before the debounce timer fires, the unpublished tail
// snapshot is flushed and the publish state is purged.
func TestSSEInspector_ConnectionCloseFlushesTail(t *testing.T) {
	setTestThrottle(t, 64*1024, 10*time.Second) // debounce timer effectively disabled
	inspector, sub := newThrottleTestInspector(t)
	const connID = "conn-closeflush"
	requestID := connID + "-1"
	sendTestRequest(t, inspector, connID, requestID)

	chunks := []string{
		"HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\ndata: {\"n\":0}\n\n",
		"data: {\"n\":1}\n\n",
		"data: {\"n\":2}\n\n",
	}
	for _, c := range chunks {
		if _, err := inspector.Inspect(DirectionServerToClient, []byte(c), "example.com", connID, requestID); err != nil {
			t.Fatalf("response inspect failed: %v", err)
		}
	}

	_ = receiveTrafficEvent(t, sub, time.Second) // first chunk
	if got := drainTrafficEvents(sub); len(got) != 0 {
		t.Fatalf("expected no more events before close, got %d", len(got))
	}

	inspector.OnConnectionClosed(connID)

	tail := receiveTrafficEvent(t, sub, time.Second)
	if tail.Event.Response == nil || !strings.Contains(tail.Event.Response.Body, `{"n":2}`) {
		t.Errorf("close-flush snapshot missing final chunk content")
	}
	if _, ok := inspector.publishStates.Load(requestID); ok {
		t.Error("publish state not purged on connection close")
	}
}
