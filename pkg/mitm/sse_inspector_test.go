package mitm

import (
	"bytes"
	"compress/gzip"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestSSEInspector_InspectRequest(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)

	requestData := []byte("GET / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 11\r\n\r\nHello World")
	requestID := "test-1-1"

	result, err := inspector.Inspect(DirectionClientToServer, requestData, "example.com", "test-1", requestID)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if !bytes.Equal(result, requestData) {
		t.Errorf("Expected result to be same as input, got different data")
	}

	if _, exists := inspector.requestCache.Load(requestID); !exists {
		t.Error("Expected request to be cached")
	} else {
		if val, exists := inspector.requestCache.Load(requestID); exists {
			httpReq := val.(*HTTPRequest)
			if httpReq.Method != "GET" {
				t.Errorf("Expected method GET, got %s", httpReq.Method)
			}
			if httpReq.URL != "/" {
				t.Errorf("Expected URL /, got %s", httpReq.URL)
			}
			if httpReq.Host != "example.com" {
				t.Errorf("Expected host example.com, got %s", httpReq.Host)
			}
			if httpReq.Body != "Hello World" {
				t.Errorf("Expected body 'Hello World', got '%s'", httpReq.Body)
			}
		}
	}
}

func TestSSEInspector_InspectResponse(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)

	// First cache a request
	requestData := []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
	requestID := "test-2-1"
	_, _ = inspector.Inspect(DirectionClientToServer, requestData, "example.com", "test-2", requestID)

	// Then inspect response
	responseData := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 12\r\n\r\nHello Server")
	result, err := inspector.Inspect(DirectionServerToClient, responseData, "example.com", "test-2", requestID)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if !bytes.Equal(result, responseData) {
		t.Errorf("Expected result to be same as input, got different data")
	}

	if _, exists := inspector.requestCache.Load(requestID); exists {
		t.Error("Expected request to be removed from cache after response")
	}
}

func TestSSEInspector_IncrementalRequest(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)

	// Split request into two chunks
	chunk1 := []byte("GET / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 11\r\n\r\nHello")
	chunk2 := []byte(" World")
	requestID := "test-3-1"

	// First chunk
	result1, err := inspector.Inspect(DirectionClientToServer, chunk1, "example.com", "test-3", requestID)
	if err != nil {
		t.Fatalf("Inspect chunk1 failed: %v", err)
	}
	if !bytes.Equal(result1, chunk1) {
		t.Errorf("Expected result1 to be same as chunk1, got different data")
	}

	// Second chunk (should complete the request)
	result2, err := inspector.Inspect(DirectionClientToServer, chunk2, "example.com", "test-3", requestID)
	if err != nil {
		t.Fatalf("Inspect chunk2 failed: %v", err)
	}
	// When complete, returns full request, not just the chunk
	expectedFullRequest := []byte("GET / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 11\r\n\r\nHello World")
	if !bytes.Equal(result2, expectedFullRequest) {
		t.Errorf("Expected result2 to be full request, got different data")
	}

	if _, exists := inspector.requestCache.Load(requestID); !exists {
		t.Error("Expected request to be cached after complete")
	} else {
		if val, exists := inspector.requestCache.Load(requestID); exists {
			httpReq := val.(*HTTPRequest)
			if httpReq.Body != "Hello World" {
				t.Errorf("Expected body 'Hello World', got '%s'", httpReq.Body)
			}
		}
	}
}

func TestSSEInspector_IncrementalResponse(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)

	requestID := "test-4-1"

	// Cache a request first
	requestData := []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
	_, _ = inspector.Inspect(DirectionClientToServer, requestData, "example.com", "test-4", requestID)

	// Split response into two chunks
	chunk1 := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 12\r\n\r\nHello")
	chunk2 := []byte(" Server")

	// First chunk
	result1, err := inspector.Inspect(DirectionServerToClient, chunk1, "example.com", "test-4", requestID)
	if err != nil {
		t.Fatalf("Inspect chunk1 failed: %v", err)
	}
	if !bytes.Equal(result1, chunk1) {
		t.Errorf("Expected result1 to be same as chunk1, got different data")
	}

	// Second chunk (should complete the response)
	result2, err := inspector.Inspect(DirectionServerToClient, chunk2, "example.com", "test-4", requestID)
	if err != nil {
		t.Fatalf("Inspect chunk2 failed: %v", err)
	}
	// When complete, returns full response, not just the chunk
	expectedFullResponse := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 12\r\n\r\nHello Server")
	if !bytes.Equal(result2, expectedFullResponse) {
		t.Errorf("Expected result2 to be full response, got different data")
	}

	if _, exists := inspector.requestCache.Load(requestID); exists {
		t.Error("Expected request to be removed from cache after response")
	}
}

func TestSSEInspector_ChunkedRequest(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)

	// Chunked encoded request
	requestData := []byte("POST / HTTP/1.1\r\nHost: example.com\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nHello\r\n6\r\n World\r\n0\r\n\r\n")
	requestID := "test-5-1"

	result, err := inspector.Inspect(DirectionClientToServer, requestData, "example.com", "test-5", requestID)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if !bytes.Equal(result, requestData) {
		t.Errorf("Expected result to be same as input, got different data")
	}
}

func TestSSEInspector_ChunkedResponse(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)

	requestID := "test-6-1"

	// Cache a request first
	requestData := []byte("POST / HTTP/1.1\r\nHost: example.com\r\n\r\n")
	_, _ = inspector.Inspect(DirectionClientToServer, requestData, "example.com", "test-6", requestID)

	// Chunked encoded response
	responseData := []byte("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nHello\r\n6\r\n World\r\n0\r\n\r\n")
	result, err := inspector.Inspect(DirectionServerToClient, responseData, "example.com", "test-6", requestID)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if !bytes.Equal(result, responseData) {
		t.Errorf("Expected result to be same as input, got different data")
	}
}

func TestSSEInspector_EmptyData(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)

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

	invalidData := []byte("Invalid HTTP data")
	result, err := inspector.Inspect(DirectionClientToServer, invalidData, "example.com", "test-8", "test-8-1")
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if !bytes.Equal(result, invalidData) {
		t.Errorf("Expected result to be same as input, got different data")
	}
}

func TestSSEInspector_ClearPending(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)

	requestID := "test-9-1"

	// Cache a request
	requestData := []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
	_, _ = inspector.Inspect(DirectionClientToServer, requestData, "example.com", "test-9", requestID)

	if _, exists := inspector.requestCache.Load(requestID); !exists {
		t.Error("Expected request to be cached")
	}

	inspector.ClearPending(requestID)

	if _, exists := inspector.requestCache.Load(requestID); exists {
		t.Error("Expected request to be cleared from cache")
	}
}

func TestSSEInspector_MaxBodySize(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewSSEInspector(logger, eventBus, "", 10) // 10 bytes max body

	requestData := []byte("POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 11\r\n\r\nHello World")
	requestID := "test-10-1"

	_, err := inspector.Inspect(DirectionClientToServer, requestData, "example.com", "test-10", requestID)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if _, exists := inspector.requestCache.Load(requestID); exists {
		if val, exists := inspector.requestCache.Load(requestID); exists {
			httpReq := val.(*HTTPRequest)
			if len(httpReq.Body) > 10 {
				t.Errorf("Expected body to be truncated to 10 bytes, got %d bytes", len(httpReq.Body))
			}
			if !strings.HasPrefix(httpReq.Body, "Hello Wor") {
				t.Errorf("Expected truncated body 'Hello Wor', got '%s'", httpReq.Body)
			}
		}
	}
}

func TestSSEInspector_EventBusIntegration(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)

	requestID := "test-11-1"

	// Cache a request
	requestData := []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
	_, _ = inspector.Inspect(DirectionClientToServer, requestData, "example.com", "test-11", requestID)

	// Send response
	responseData := []byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n")
	_, err := inspector.Inspect(DirectionServerToClient, responseData, "example.com", "test-11", requestID)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	// Give time for event to be published
	time.Sleep(100 * time.Millisecond)
}

func TestSSEInspector_SSEResponse(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)

	requestID := "test-sse-1-1"

	// Cache a request first
	requestData := []byte("GET /events HTTP/1.1\r\nHost: example.com\r\n\r\n")
	_, _ = inspector.Inspect(DirectionClientToServer, requestData, "example.com", "test-sse-1", requestID)

	// SSE response (no Content-Length, streaming)
	responseData := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\ndata: hello\r\n\r\ndata: world\r\n\r\n")
	result, err := inspector.Inspect(DirectionServerToClient, responseData, "example.com", "test-sse-1", requestID)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if !bytes.Equal(result, responseData) {
		t.Errorf("Expected result to be same as input, got different data")
	}
}

func TestSSEInspector_SSEIncrementalEvents(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)

	requestID := "test-sse-2-1"

	// Cache a request first
	requestData := []byte("GET /events HTTP/1.1\r\nHost: example.com\r\n\r\n")
	_, _ = inspector.Inspect(DirectionClientToServer, requestData, "example.com", "test-sse-2", requestID)

	// Split SSE events into multiple chunks
	chunk1 := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\ndata: first event\r\n\r\n")
	chunk2 := []byte("data: second event\r\n\r\n")
	chunk3 := []byte("data: third event\r\n\r\n")

	result1, err := inspector.Inspect(DirectionServerToClient, chunk1, "example.com", "test-sse-2", requestID)
	if err != nil {
		t.Fatalf("Inspect chunk1 failed: %v", err)
	}
	// For SSE, we return the full accumulated data
	expected1 := make([]byte, len(chunk1))
	copy(expected1, chunk1)
	if !bytes.Equal(result1, expected1) {
		t.Errorf("Expected result1 to match expected, got different data")
	}

	result2, err := inspector.Inspect(DirectionServerToClient, chunk2, "example.com", "test-sse-2", requestID)
	if err != nil {
		t.Fatalf("Inspect chunk2 failed: %v", err)
	}
	// Second chunk returns accumulated data (chunk1 + chunk2)
	expected2 := append(chunk1, chunk2...)
	if !bytes.Equal(result2, expected2) {
		t.Errorf("Expected result2 to contain accumulated data")
	}

	result3, err := inspector.Inspect(DirectionServerToClient, chunk3, "example.com", "test-sse-2", requestID)
	if err != nil {
		t.Fatalf("Inspect chunk3 failed: %v", err)
	}
	// Third chunk returns accumulated data (chunk1 + chunk2 + chunk3)
	expected3 := append(append(chunk1, chunk2...), chunk3...)
	if !bytes.Equal(result3, expected3) {
		t.Errorf("Expected result3 to contain accumulated data")
	}
}

func TestSSEInspector_SSEMultiLineData(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)

	requestID := "test-sse-3-1"

	// Cache a request first
	requestData := []byte("GET /events HTTP/1.1\r\nHost: example.com\r\n\r\n")
	_, _ = inspector.Inspect(DirectionClientToServer, requestData, "example.com", "test-sse-3", requestID)

	// SSE with multi-line data (data: followed by more data: lines)
	responseData := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\nevent: message\r\ndata: line1\r\ndata: line2\r\ndata: line3\r\n\r\n")
	result, err := inspector.Inspect(DirectionServerToClient, responseData, "example.com", "test-sse-3", requestID)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if !bytes.Equal(result, responseData) {
		t.Errorf("Expected result to be same as input")
	}
}

func TestSSEInspector_SSEWithAllFields(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)

	requestID := "test-sse-4-1"

	// Cache a request first
	requestData := []byte("GET /events HTTP/1.1\r\nHost: example.com\r\n\r\n")
	_, _ = inspector.Inspect(DirectionClientToServer, requestData, "example.com", "test-sse-4", requestID)

	// SSE with all fields: event, id, retry, data
	responseData := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\nid: 123\r\nevent: update\r\nretry: 5000\r\ndata: {\"status\":\"ok\"}\r\n\r\n")
	result, err := inspector.Inspect(DirectionServerToClient, responseData, "example.com", "test-sse-4", requestID)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if !bytes.Equal(result, responseData) {
		t.Errorf("Expected result to be same as input")
	}
}

func TestSSEInspector_CompressedSSE(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)

	requestID := "test-sse-compressed-1"

	// Cache a request first
	requestData := []byte("GET /events HTTP/1.1\r\nHost: example.com\r\n\r\n")
	_, _ = inspector.Inspect(DirectionClientToServer, requestData, "example.com", "test-sse-compressed", requestID)

	// Compress SSE data with gzip
	originalData := "data: compressed event 1\r\n\r\ndata: compressed event 2\r\n\r\n"
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	gz.Write([]byte(originalData))
	gz.Close()
	compressedData := buf.Bytes()

	// Build response with Content-Encoding: gzip
	responseData := bytes.NewBuffer(nil)
	responseData.Write([]byte("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nContent-Encoding: gzip\r\n\r\n"))
	responseData.Write(compressedData)

	result, err := inspector.Inspect(DirectionServerToClient, responseData.Bytes(), "example.com", "test-sse-compressed", requestID)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if !bytes.Equal(result, responseData.Bytes()) {
		t.Errorf("Expected result to be same as input")
	}
}

func TestSSEInspector_SSEWithChunkedTransfer(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)

	requestID := "test-sse-chunked-1"

	// Cache a request first
	requestData := []byte("GET /events HTTP/1.1\r\nHost: example.com\r\n\r\n")
	_, _ = inspector.Inspect(DirectionClientToServer, requestData, "example.com", "test-sse-chunked", requestID)

	// Chunked encoded SSE response
	responseData := []byte("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\nContent-Type: text/event-stream\r\n\r\nd\r\ndata: hello\r\n\r\n0\r\n\r\n")
	result, err := inspector.Inspect(DirectionServerToClient, responseData, "example.com", "test-sse-chunked", requestID)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if !bytes.Equal(result, responseData) {
		t.Errorf("Expected result to be same as input")
	}
}

func TestSSEInspector_RegularHTTPNotAffected(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)

	requestID := "test-regular-1"

	// Cache a request first
	requestData := []byte("GET /api/data HTTP/1.1\r\nHost: example.com\r\n\r\n")
	_, _ = inspector.Inspect(DirectionClientToServer, requestData, "example.com", "test-regular", requestID)

	// Regular JSON response should still work
	responseData := []byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 15\r\n\r\n{\"status\":\"ok\"}")
	result, err := inspector.Inspect(DirectionServerToClient, responseData, "example.com", "test-regular", requestID)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if !bytes.Equal(result, responseData) {
		t.Errorf("Expected result to be same as input")
	}
}

func TestSSEInspector_ClearPendingWithDecompressor(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)

	requestID := "test-clear-sse-1"

	// Cache a request first
	requestData := []byte("GET /events HTTP/1.1\r\nHost: example.com\r\n\r\n")
	_, _ = inspector.Inspect(DirectionClientToServer, requestData, "example.com", "test-clear-sse", requestID)

	// SSE response with compression
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	gz.Write([]byte("data: test\r\n\r\n"))
	gz.Close()

	responseData := bytes.NewBuffer(nil)
	responseData.Write([]byte("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nContent-Encoding: gzip\r\n\r\n"))
	responseData.Write(buf.Bytes())

	_, _ = inspector.Inspect(DirectionServerToClient, responseData.Bytes(), "example.com", "test-clear-sse", requestID)

	// Clear pending - should not panic
	inspector.ClearPending(requestID)

	if _, exists := inspector.pendingResps.Load(requestID); exists {
		t.Error("Expected pending response to be cleared")
	}
}
