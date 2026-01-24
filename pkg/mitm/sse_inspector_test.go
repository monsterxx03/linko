package mitm

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestSSEInspector_InspectRequest(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)

	requestData := []byte("GET / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 11\r\n\r\nHello World")
	connectionID := "test-1"

	result, err := inspector.Inspect(DirectionClientToServer, requestData, "example.com", connectionID)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if !bytes.Equal(result, requestData) {
		t.Errorf("Expected result to be same as input, got different data")
	}

	if _, exists := inspector.requestCache.Load(connectionID); !exists {
		t.Error("Expected request to be cached")
	} else {
		if val, exists := inspector.requestCache.Load(connectionID); exists {
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
	eventBus := NewEventBus(logger)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)

	// First cache a request
	requestData := []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
	connectionID := "test-2"
	_, _ = inspector.Inspect(DirectionClientToServer, requestData, "example.com", connectionID)

	// Then inspect response
	responseData := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 12\r\n\r\nHello Server")
	result, err := inspector.Inspect(DirectionServerToClient, responseData, "example.com", connectionID)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if !bytes.Equal(result, responseData) {
		t.Errorf("Expected result to be same as input, got different data")
	}

	if _, exists := inspector.requestCache.Load(connectionID); exists {
		t.Error("Expected request to be removed from cache after response")
	}
}

func TestSSEInspector_IncrementalRequest(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)

	// Split request into two chunks
	chunk1 := []byte("GET / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 11\r\n\r\nHello")
	chunk2 := []byte(" World")
	connectionID := "test-3"

	// First chunk
	result1, err := inspector.Inspect(DirectionClientToServer, chunk1, "example.com", connectionID)
	if err != nil {
		t.Fatalf("Inspect chunk1 failed: %v", err)
	}
	if !bytes.Equal(result1, chunk1) {
		t.Errorf("Expected result1 to be same as chunk1, got different data")
	}

	// Second chunk (should complete the request)
	result2, err := inspector.Inspect(DirectionClientToServer, chunk2, "example.com", connectionID)
	if err != nil {
		t.Fatalf("Inspect chunk2 failed: %v", err)
	}
	// When complete, returns full request, not just the chunk
	expectedFullRequest := []byte("GET / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 11\r\n\r\nHello World")
	if !bytes.Equal(result2, expectedFullRequest) {
		t.Errorf("Expected result2 to be full request, got different data")
	}

	if _, exists := inspector.requestCache.Load(connectionID); !exists {
		t.Error("Expected request to be cached after complete")
	} else {
		if val, exists := inspector.requestCache.Load(connectionID); exists {
			httpReq := val.(*HTTPRequest)
			if httpReq.Body != "Hello World" {
				t.Errorf("Expected body 'Hello World', got '%s'", httpReq.Body)
			}
		}
	}
}

func TestSSEInspector_IncrementalResponse(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)

	connectionID := "test-4"

	// Cache a request first
	requestData := []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
	_, _ = inspector.Inspect(DirectionClientToServer, requestData, "example.com", connectionID)

	// Split response into two chunks
	chunk1 := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 12\r\n\r\nHello")
	chunk2 := []byte(" Server")

	// First chunk
	result1, err := inspector.Inspect(DirectionServerToClient, chunk1, "example.com", connectionID)
	if err != nil {
		t.Fatalf("Inspect chunk1 failed: %v", err)
	}
	if !bytes.Equal(result1, chunk1) {
		t.Errorf("Expected result1 to be same as chunk1, got different data")
	}

	// Second chunk (should complete the response)
	result2, err := inspector.Inspect(DirectionServerToClient, chunk2, "example.com", connectionID)
	if err != nil {
		t.Fatalf("Inspect chunk2 failed: %v", err)
	}
	// When complete, returns full response, not just the chunk
	expectedFullResponse := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 12\r\n\r\nHello Server")
	if !bytes.Equal(result2, expectedFullResponse) {
		t.Errorf("Expected result2 to be full response, got different data")
	}

	if _, exists := inspector.requestCache.Load(connectionID); exists {
		t.Error("Expected request to be removed from cache after response")
	}
}

func TestSSEInspector_ChunkedRequest(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)

	// Chunked encoded request
	requestData := []byte("POST / HTTP/1.1\r\nHost: example.com\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nHello\r\n6\r\n World\r\n0\r\n\r\n")
	connectionID := "test-5"

	result, err := inspector.Inspect(DirectionClientToServer, requestData, "example.com", connectionID)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if !bytes.Equal(result, requestData) {
		t.Errorf("Expected result to be same as input, got different data")
	}
}

func TestSSEInspector_ChunkedResponse(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)

	connectionID := "test-6"

	// Cache a request first
	requestData := []byte("POST / HTTP/1.1\r\nHost: example.com\r\n\r\n")
	_, _ = inspector.Inspect(DirectionClientToServer, requestData, "example.com", connectionID)

	// Chunked encoded response
	responseData := []byte("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nHello\r\n6\r\n World\r\n0\r\n\r\n")
	result, err := inspector.Inspect(DirectionServerToClient, responseData, "example.com", connectionID)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if !bytes.Equal(result, responseData) {
		t.Errorf("Expected result to be same as input, got different data")
	}
}

func TestSSEInspector_EmptyData(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)

	result, err := inspector.Inspect(DirectionClientToServer, []byte{}, "example.com", "test-7")
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("Expected empty result, got %d bytes", len(result))
	}
}

func TestSSEInspector_InvalidHTTP(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)

	invalidData := []byte("Invalid HTTP data")
	result, err := inspector.Inspect(DirectionClientToServer, invalidData, "example.com", "test-8")
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if !bytes.Equal(result, invalidData) {
		t.Errorf("Expected result to be same as input, got different data")
	}
}

func TestSSEInspector_ClearPending(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)

	connectionID := "test-9"

	// Cache a request
	requestData := []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
	_, _ = inspector.Inspect(DirectionClientToServer, requestData, "example.com", connectionID)

	if _, exists := inspector.requestCache.Load(connectionID); !exists {
		t.Error("Expected request to be cached")
	}

	inspector.ClearPending(connectionID)

	if _, exists := inspector.requestCache.Load(connectionID); exists {
		t.Error("Expected request to be cleared from cache")
	}
}

func TestSSEInspector_MaxBodySize(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger)
	inspector := NewSSEInspector(logger, eventBus, "", 10) // 10 bytes max body

	requestData := []byte("POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 11\r\n\r\nHello World")
	connectionID := "test-10"

	_, err := inspector.Inspect(DirectionClientToServer, requestData, "example.com", connectionID)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if _, exists := inspector.requestCache.Load(connectionID); exists {
		if val, exists := inspector.requestCache.Load(connectionID); exists {
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
	eventBus := NewEventBus(logger)
	inspector := NewSSEInspector(logger, eventBus, "", 1024*1024)

	connectionID := "test-11"

	// Cache a request
	requestData := []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
	_, _ = inspector.Inspect(DirectionClientToServer, requestData, "example.com", connectionID)

	// Send response
	responseData := []byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n")
	_, err := inspector.Inspect(DirectionServerToClient, responseData, "example.com", connectionID)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	// Give time for event to be published
	time.Sleep(100 * time.Millisecond)
}
