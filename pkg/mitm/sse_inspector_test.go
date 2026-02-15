package mitm

import (
	"bytes"
	"compress/gzip"
	"log/slog"
	"testing"
)

// mockSSEHTTPProcessor implements HTTPProcessorInterface for SSE inspector testing
type mockSSEHTTPProcessor struct {
	t                  *testing.T
	processRequestFunc func(data []byte, requestID string) ([]byte, *HTTPMessage, bool, error)
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
