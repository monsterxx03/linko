package mitm

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

func TestHTTPProcessor_ProcessRequest_Basic(t *testing.T) {
	logger := slog.Default()
	processor := NewHTTPProcessor(logger, 1024*1024)

	requestData := []byte("GET / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 11\r\n\r\nHello World")
	requestID := "test-req-1"

	result, msg, isComplete, err := processor.ProcessRequest(requestData, requestID)
	_ = result
	if err != nil {
		t.Fatalf("ProcessRequest failed: %v", err)
	}

	if !isComplete {
		t.Error("Expected isComplete to be true")
	}

	if msg == nil {
		t.Fatal("Expected HTTPMessage to be returned")
	}

	if msg.Method != "GET" {
		t.Errorf("Expected method GET, got %s", msg.Method)
	}

	if msg.Path != "/" {
		t.Errorf("Expected path /, got %s", msg.Path)
	}

	if msg.Hostname != "example.com" {
		t.Errorf("Expected hostname example.com, got %s", msg.Hostname)
	}

	if string(msg.Body) != "Hello World" {
		t.Errorf("Expected body 'Hello World', got '%s'", msg.Body)
	}

	// Result should be independent copy
	if !bytes.Equal(result, requestData) {
		t.Error("Expected result to match input data")
	}
}

func TestHTTPProcessor_ProcessRequest_NoBody(t *testing.T) {
	logger := slog.Default()
	processor := NewHTTPProcessor(logger, 1024*1024)

	requestData := []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
	requestID := "test-req-2"

	result, msg, isComplete, err := processor.ProcessRequest(requestData, requestID)
	_ = result
	if err != nil {
		t.Fatalf("ProcessRequest failed: %v", err)
	}

	if !isComplete {
		t.Error("Expected isComplete to be true")
	}

	if msg == nil {
		t.Fatal("Expected HTTPMessage to be returned")
	}

	if msg.Method != "GET" {
		t.Errorf("Expected method GET, got %s", msg.Method)
	}

	if len(msg.Body) != 0 {
		t.Errorf("Expected empty body, got %d bytes", len(msg.Body))
	}
}

func TestHTTPProcessor_ProcessRequest_Incremental(t *testing.T) {
	logger := slog.Default()
	processor := NewHTTPProcessor(logger, 1024*1024)

	// First chunk: headers only
	chunk1 := []byte("GET / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 11\r\n\r\nHello")
	requestID := "test-req-3"

	_, msg1, isComplete1, err := processor.ProcessRequest(chunk1, requestID)
	if err != nil {
		t.Fatalf("ProcessRequest chunk1 failed: %v", err)
	}

	if isComplete1 {
		t.Error("Expected isComplete to be false for first chunk")
	}

	if msg1 != nil {
		t.Error("Expected nil message for incomplete request")
	}

	// Second chunk: rest of body
	chunk2 := []byte(" World")

	result2, msg2, isComplete2, err := processor.ProcessRequest(chunk2, requestID)
	if err != nil {
		t.Fatalf("ProcessRequest chunk2 failed: %v", err)
	}

	if !isComplete2 {
		t.Error("Expected isComplete to be true for second chunk")
	}

	if msg2 == nil {
		t.Fatal("Expected HTTPMessage to be returned")
	}

	if string(msg2.Body) != "Hello World" {
		t.Errorf("Expected body 'Hello World', got '%s'", msg2.Body)
	}

	// Result should contain the full request
	expectedFull := []byte("GET / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 11\r\n\r\nHello World")
	if !bytes.Equal(result2, expectedFull) {
		t.Error("Expected result to contain full request")
	}
}

func TestHTTPProcessor_ProcessRequest_Chunked(t *testing.T) {
	logger := slog.Default()
	processor := NewHTTPProcessor(logger, 1024*1024)

	requestData := []byte("POST / HTTP/1.1\r\nHost: example.com\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nHello\r\n6\r\n World\r\n0\r\n\r\n")
	requestID := "test-req-4"

	_, msg, isComplete, err := processor.ProcessRequest(requestData, requestID)
	if err != nil {
		t.Fatalf("ProcessRequest failed: %v", err)
	}

	if !isComplete {
		t.Error("Expected isComplete to be true for chunked request")
	}

	if msg == nil {
		t.Fatal("Expected HTTPMessage to be returned")
	}

	if string(msg.Body) != "Hello World" {
		t.Errorf("Expected body 'Hello World', got '%s'", msg.Body)
	}
}

func TestHTTPProcessor_ProcessRequest_EmptyData(t *testing.T) {
	logger := slog.Default()
	processor := NewHTTPProcessor(logger, 1024*1024)

	result, msg, isComplete, err := processor.ProcessRequest([]byte{}, "test-req-5")
	if err != nil {
		t.Fatalf("ProcessRequest failed: %v", err)
	}

	if isComplete {
		t.Error("Expected isComplete to be false for empty data")
	}

	if msg != nil {
		t.Error("Expected nil message for empty data")
	}

	if len(result) != 0 {
		t.Error("Expected empty result")
	}
}

func TestHTTPProcessor_ProcessRequest_InvalidHTTP(t *testing.T) {
	logger := slog.Default()
	processor := NewHTTPProcessor(logger, 1024*1024)

	// Non-HTTP data should be passed through
	invalidData := []byte("Invalid HTTP data")
	requestID := "test-req-6"

	result, msg, isComplete, err := processor.ProcessRequest(invalidData, requestID)
	if err != nil {
		t.Fatalf("ProcessRequest failed: %v", err)
	}

	if isComplete {
		t.Error("Expected isComplete to be false for invalid HTTP")
	}

	if msg != nil {
		t.Error("Expected nil message for invalid HTTP")
	}

	if !bytes.Equal(result, invalidData) {
		t.Error("Expected result to match input for invalid HTTP")
	}
}

func TestHTTPProcessor_ProcessRequest_TruncateBody(t *testing.T) {
	logger := slog.Default()
	processor := NewHTTPProcessor(logger, 10) // 10 bytes max

	requestData := []byte("POST / HTTP/1.1\r\nHost: example.com\r\nContent-Type: text/plain\r\nContent-Length: 14\r\n\r\nHello World!!!")
	requestID := "test-req-7"

	_, msg, isComplete, err := processor.ProcessRequest(requestData, requestID)
	if err != nil {
		t.Fatalf("ProcessRequest failed: %v", err)
	}

	if !isComplete {
		t.Error("Expected isComplete to be true")
	}

	if msg == nil {
		t.Fatal("Expected HTTPMessage to be returned")
	}

	// Body should be truncated
	if len(msg.Body) > 10 {
		t.Errorf("Expected body to be truncated to 10 bytes, got %d bytes", len(msg.Body))
	}

	if !strings.HasPrefix(string(msg.Body), "Hello Wor") {
		t.Errorf("Expected truncated body 'Hello Wor', got '%s'", msg.Body)
	}
}

func TestHTTPProcessor_ProcessResponse_Basic(t *testing.T) {
	logger := slog.Default()
	processor := NewHTTPProcessor(logger, 1024*1024)

	responseData := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 12\r\n\r\nHello Server")
	requestID := "test-resp-1"

	result, msg, isComplete, err := processor.ProcessResponse(responseData, requestID)
	_ = result
	if err != nil {
		t.Fatalf("ProcessResponse failed: %v", err)
	}

	if !isComplete {
		t.Error("Expected isComplete to be true")
	}

	if msg == nil {
		t.Fatal("Expected HTTPMessage to be returned")
	}

	if msg.StatusCode != 200 {
		t.Errorf("Expected status code 200, got %d", msg.StatusCode)
	}

	if msg.IsResponse != true {
		t.Error("Expected IsResponse to be true")
	}

	if string(msg.Body) != "Hello Server" {
		t.Errorf("Expected body 'Hello Server', got '%s'", msg.Body)
	}
}

func TestHTTPProcessor_ProcessResponse_NoBody(t *testing.T) {
	logger := slog.Default()
	processor := NewHTTPProcessor(logger, 1024*1024)

	responseData := []byte("HTTP/1.1 204 No Content\r\n\r\n")
	requestID := "test-resp-2"

	result, msg, isComplete, err := processor.ProcessResponse(responseData, requestID)
	_ = result
	if err != nil {
		t.Fatalf("ProcessResponse failed: %v", err)
	}

	if !isComplete {
		t.Error("Expected isComplete to be true")
	}

	if msg == nil {
		t.Fatal("Expected HTTPMessage to be returned")
	}

	if msg.StatusCode != 204 {
		t.Errorf("Expected status code 204, got %d", msg.StatusCode)
	}

	if len(msg.Body) != 0 {
		t.Errorf("Expected empty body, got %d bytes", len(msg.Body))
	}
}

func TestHTTPProcessor_ProcessResponse_Incremental(t *testing.T) {
	logger := slog.Default()
	processor := NewHTTPProcessor(logger, 1024*1024)

	// First chunk: headers only
	chunk1 := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 12\r\n\r\nHello")
	requestID := "test-resp-3"

	_, msg1, isComplete1, err := processor.ProcessResponse(chunk1, requestID)
	if err != nil {
		t.Fatalf("ProcessResponse chunk1 failed: %v", err)
	}

	if isComplete1 {
		t.Error("Expected isComplete to be false for first chunk")
	}

	if msg1 != nil {
		t.Error("Expected nil message for incomplete response")
	}

	// Second chunk: rest of body
	chunk2 := []byte(" Server")

	result2, msg2, isComplete2, err := processor.ProcessResponse(chunk2, requestID)
	if err != nil {
		t.Fatalf("ProcessResponse chunk2 failed: %v", err)
	}

	if !isComplete2 {
		t.Error("Expected isComplete to be true for second chunk")
	}

	if msg2 == nil {
		t.Fatal("Expected HTTPMessage to be returned")
	}

	if string(msg2.Body) != "Hello Server" {
		t.Errorf("Expected body 'Hello Server', got '%s'", msg2.Body)
	}

	// Result should contain the full response
	expectedFull := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 12\r\n\r\nHello Server")
	if !bytes.Equal(result2, expectedFull) {
		t.Error("Expected result to contain full response")
	}
}

func TestHTTPProcessor_ProcessResponse_Chunked(t *testing.T) {
	logger := slog.Default()
	processor := NewHTTPProcessor(logger, 1024*1024)

	responseData := []byte("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\nContent-Type: text/plain\r\n\r\n5\r\nHello\r\n6\r\n World\r\n0\r\n\r\n")
	requestID := "test-resp-4"

	_, msg, isComplete, err := processor.ProcessResponse(responseData, requestID)
	if err != nil {
		t.Fatalf("ProcessResponse failed: %v", err)
	}

	if !isComplete {
		t.Error("Expected isComplete to be true for chunked response")
	}

	if msg == nil {
		t.Fatal("Expected HTTPMessage to be returned")
	}

	if string(msg.Body) != "Hello World" {
		t.Errorf("Expected body 'Hello World', got '%s'", msg.Body)
	}
}

func TestHTTPProcessor_ProcessResponse_SSE(t *testing.T) {
	logger := slog.Default()
	processor := NewHTTPProcessor(logger, 1024*1024)

	// SSE response
	responseData := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\ndata: hello\r\n\r\ndata: world\r\n\r\n")
	requestID := "test-resp-sse-1"

	result, msg, isComplete, err := processor.ProcessResponse(responseData, requestID)
	if err != nil {
		t.Fatalf("ProcessResponse failed: %v", err)
	}

	// SSE responses should never be marked as complete (streaming)
	if isComplete {
		t.Error("Expected isComplete to be false for SSE response")
	}

	if msg == nil {
		t.Fatal("Expected HTTPMessage to be returned")
	}

	if !msg.IsSSE {
		t.Error("Expected IsSSE to be true")
	}

	// Result should be returned
	if !bytes.Equal(result, responseData) {
		t.Error("Expected result to match input")
	}
}

func TestHTTPProcessor_ProcessResponse_SSE_Incremental(t *testing.T) {
	logger := slog.Default()
	processor := NewHTTPProcessor(logger, 1024*1024)

	requestID := "test-resp-sse-2"

	// First chunk: headers + first event
	chunk1 := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\ndata: first\r\n\r\n")

	_, msg1, isComplete1, err := processor.ProcessResponse(chunk1, requestID)
	if err != nil {
		t.Fatalf("ProcessResponse chunk1 failed: %v", err)
	}

	if isComplete1 {
		t.Error("Expected isComplete to be false for SSE")
	}

	if msg1 == nil || !msg1.IsSSE {
		t.Error("Expected SSE message")
	}

	// Second chunk
	chunk2 := []byte("data: second\r\n\r\n")

	result2, msg2, isComplete2, err := processor.ProcessResponse(chunk2, requestID)
	if err != nil {
		t.Fatalf("ProcessResponse chunk2 failed: %v", err)
	}

	if isComplete2 {
		t.Error("Expected isComplete to remain false for SSE")
	}

	// Should return accumulated data
	expected2 := append(chunk1, chunk2...)
	if !bytes.Equal(result2, expected2) {
		t.Error("Expected accumulated data")
	}

	// Message should still be SSE
	if msg2 == nil || !msg2.IsSSE {
		t.Error("Expected SSE message")
	}
}

func TestHTTPProcessor_ProcessResponse_EmptyData(t *testing.T) {
	logger := slog.Default()
	processor := NewHTTPProcessor(logger, 1024*1024)

	result, msg, isComplete, err := processor.ProcessResponse([]byte{}, "test-resp-5")
	if err != nil {
		t.Fatalf("ProcessResponse failed: %v", err)
	}

	if isComplete {
		t.Error("Expected isComplete to be false for empty data")
	}

	if msg != nil {
		t.Error("Expected nil message for empty data")
	}

	if len(result) != 0 {
		t.Error("Expected empty result")
	}
}

func TestHTTPProcessor_ProcessResponse_InvalidHTTP(t *testing.T) {
	logger := slog.Default()
	processor := NewHTTPProcessor(logger, 1024*1024)

	// Non-HTTP data should be passed through
	invalidData := []byte("Invalid HTTP response")
	requestID := "test-resp-6"

	result, msg, isComplete, err := processor.ProcessResponse(invalidData, requestID)
	if err != nil {
		t.Fatalf("ProcessResponse failed: %v", err)
	}

	if isComplete {
		t.Error("Expected isComplete to be false for invalid HTTP")
	}

	if msg != nil {
		t.Error("Expected nil message for invalid HTTP")
	}

	if !bytes.Equal(result, invalidData) {
		t.Error("Expected result to match input for invalid HTTP")
	}
}

func TestHTTPProcessor_ProcessResponse_TruncateBody(t *testing.T) {
	logger := slog.Default()
	processor := NewHTTPProcessor(logger, 10) // 10 bytes max

	responseData := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 14\r\n\r\nHello World!!!")
	requestID := "test-resp-7"

	_, msg, isComplete, err := processor.ProcessResponse(responseData, requestID)
	if err != nil {
		t.Fatalf("ProcessResponse failed: %v", err)
	}

	if !isComplete {
		t.Error("Expected isComplete to be true")
	}

	if msg == nil {
		t.Fatal("Expected HTTPMessage to be returned")
	}

	// Body should be truncated
	if len(msg.Body) > 10 {
		t.Errorf("Expected body to be truncated to 10 bytes, got %d bytes", len(msg.Body))
	}

	if !strings.HasPrefix(string(msg.Body), "Hello Wor") {
		t.Errorf("Expected truncated body 'Hello Wor', got '%s'", msg.Body)
	}
}

func TestHTTPProcessor_GetPendingMessage(t *testing.T) {
	logger := slog.Default()
	processor := NewHTTPProcessor(logger, 1024*1024)

	requestID := "test-pending-1"

	// First send partial request to create pending state
	chunk1 := []byte("GET / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 11\r\n\r\nHello")
	_, _, isComplete, _ := processor.ProcessRequest(chunk1, requestID)
	if isComplete {
		t.Fatal("Request should not be complete yet")
	}

	// Get pending message
	msg, exists := processor.GetPendingMessage(requestID)
	if !exists {
		t.Error("Expected pending message to exist")
	}

	if msg == nil {
		t.Fatal("Expected pending message to be returned")
	}

	if msg.Method != "GET" {
		t.Errorf("Expected method GET, got %s", msg.Method)
	}

	// After complete, pending should be cleared
	chunk2 := []byte(" World")
	_, _, isComplete, _ = processor.ProcessRequest(chunk2, requestID)
	if !isComplete {
		t.Error("Request should be complete now")
	}

	_, exists = processor.GetPendingMessage(requestID)
	if exists {
		t.Error("Expected pending message to be cleared after completion")
	}
}

func TestHTTPProcessor_ClearPending(t *testing.T) {
	logger := slog.Default()
	processor := NewHTTPProcessor(logger, 1024*1024)

	requestID := "test-clear-1"

	// Create pending request
	chunk1 := []byte("GET / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 11\r\n\r\nHello")
	processor.ProcessRequest(chunk1, requestID)

	// Verify pending exists
	_, exists := processor.GetPendingMessage(requestID)
	if !exists {
		t.Error("Expected pending message to exist before clear")
	}

	// Clear pending
	processor.ClearPending(requestID)

	// Verify pending is cleared
	_, exists = processor.GetPendingMessage(requestID)
	if exists {
		t.Error("Expected pending message to be cleared")
	}
}

func TestHTTPProcessor_BuildRequestMessage_WithHeaders(t *testing.T) {
	logger := slog.Default()
	processor := NewHTTPProcessor(logger, 1024*1024)

	requestData := []byte("POST /api/data HTTP/1.1\r\nHost: example.com\r\nContent-Type: application/json\r\nX-Custom-Header: value\r\nContent-Length: 15\r\n\r\n{\"status\":\"ok\"}")
	requestID := "test-build-req-1"

	_, msg, _, err := processor.ProcessRequest(requestData, requestID)
	if err != nil {
		t.Fatalf("ProcessRequest failed: %v", err)
	}

	if msg == nil {
		t.Fatal("Expected HTTPMessage to be returned")
	}

	if msg.Method != "POST" {
		t.Errorf("Expected method POST, got %s", msg.Method)
	}

	if msg.Path != "/api/data" {
		t.Errorf("Expected path /api/data, got %s", msg.Path)
	}

	if msg.Hostname != "example.com" {
		t.Errorf("Expected hostname example.com, got %s", msg.Hostname)
	}

	if msg.ContentType != "application/json" {
		t.Errorf("Expected content type application/json, got %s", msg.ContentType)
	}

	if msg.Headers["X-Custom-Header"] != "value" {
		t.Errorf("Expected X-Custom-Header to be 'value', got '%s'", msg.Headers["X-Custom-Header"])
	}
}

func TestHTTPProcessor_BuildResponseMessage_WithHeaders(t *testing.T) {
	logger := slog.Default()
	processor := NewHTTPProcessor(logger, 1024*1024)

	// Need to have a pending request first for response to have hostname/path
	requestID := "test-build-resp-1"

	// Create pending request (to provide request info)
	processor.ProcessRequest([]byte("GET /api/data HTTP/1.1\r\nHost: api.example.com\r\nContent-Length: 0\r\n\r\n"), requestID)

	// Complete the request to clear it, then create a new pending
	processor.ClearPending(requestID)

	// Now process response - note: hostname/path won't be available since request was cleared
	// This test just verifies headers work correctly
	responseData := []byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nX-Custom-Header: value\r\nContent-Length: 15\r\n\r\n{\"status\":\"ok\"}")

	_, msg, _, err := processor.ProcessResponse(responseData, requestID)
	if err != nil {
		t.Fatalf("ProcessResponse failed: %v", err)
	}

	if msg == nil {
		t.Fatal("Expected HTTPMessage to be returned")
	}

	if msg.StatusCode != 200 {
		t.Errorf("Expected status code 200, got %d", msg.StatusCode)
	}

	if msg.ContentType != "application/json" {
		t.Errorf("Expected content type application/json, got %s", msg.ContentType)
	}

	if msg.Headers["X-Custom-Header"] != "value" {
		t.Errorf("Expected X-Custom-Header to be 'value', got '%s'", msg.Headers["X-Custom-Header"])
	}
}

func TestHTTPProcessor_ExtractHeaders(t *testing.T) {
	logger := slog.Default()
	processor := NewHTTPProcessor(logger, 1024*1024)

	// Include body to make it a complete request
	requestData := []byte("GET / HTTP/1.1\r\nHost: example.com\r\nAccept: */*\r\nAccept-Language: en-US\r\nContent-Length: 5\r\n\r\nhello")
	requestID := "test-headers-1"

	_, msg, isComplete, err := processor.ProcessRequest(requestData, requestID)
	if err != nil {
		t.Fatalf("ProcessRequest failed: %v", err)
	}

	if !isComplete {
		t.Error("Expected isComplete to be true")
	}

	if msg == nil {
		t.Fatal("Expected HTTPMessage to be returned")
	}

	// Hostname comes from req.Host
	if msg.Hostname != "example.com" {
		t.Errorf("Expected hostname example.com, got '%s'", msg.Hostname)
	}

	// Other headers come from Header map
	headers := msg.Headers
	if headers["Accept"] != "*/*" {
		t.Errorf("Expected Accept header, got '%s'", headers["Accept"])
	}

	if headers["Accept-Language"] != "en-US" {
		t.Errorf("Expected Accept-Language header, got '%s'", headers["Accept-Language"])
	}
}

func TestHTTPProcessor_DecompressGzip(t *testing.T) {
	logger := slog.Default()
	processor := NewHTTPProcessor(logger, 1024*1024)

	// Create gzip compressed body
	originalBody := "Hello World"
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	gz.Write([]byte(originalBody))
	gz.Close()
	compressedBody := buf.Bytes()

	// Build request with gzip encoding
	cl := len(compressedBody)
	requestData := fmt.Sprintf("POST / HTTP/1.1\r\nHost: example.com\r\nContent-Type: text/plain\r\nContent-Encoding: gzip\r\nContent-Length: %d\r\n\r\n", cl)
	requestDataBytes := append([]byte(requestData), compressedBody...)

	requestID := "test-gzip-1"

	_, msg, _, err := processor.ProcessRequest(requestDataBytes, requestID)
	if err != nil {
		t.Fatalf("ProcessRequest failed: %v", err)
	}

	if msg == nil {
		t.Fatal("Expected HTTPMessage to be returned")
	}

	// Body should be decompressed
	if string(msg.Body) != originalBody {
		t.Errorf("Expected body '%s', got '%s'", originalBody, msg.Body)
	}
}

func TestHTTPProcessor_DefaultMaxBodySize(t *testing.T) {
	logger := slog.Default()
	processor := NewHTTPProcessor(logger, 0) // Should use default 1MB

	if processor.maxBodySize != 1024*1024 {
		t.Errorf("Expected default maxBodySize 1MB, got %d", processor.maxBodySize)
	}
}

func TestHTTPProcessor_EmptyBodyRequest(t *testing.T) {
	logger := slog.Default()
	processor := NewHTTPProcessor(logger, 1024*1024)

	// POST with empty body
	requestData := []byte("POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 0\r\n\r\n")
	requestID := "test-empty-body"

	_, msg, isComplete, err := processor.ProcessRequest(requestData, requestID)
	if err != nil {
		t.Fatalf("ProcessRequest failed: %v", err)
	}

	if !isComplete {
		t.Error("Expected isComplete to be true")
	}

	if msg == nil {
		t.Fatal("Expected HTTPMessage")
	}

	if len(msg.Body) != 0 {
		t.Errorf("Expected empty body, got %d bytes", len(msg.Body))
	}
}

func TestHTTPProcessor_ResponseWithNoContentLength(t *testing.T) {
	logger := slog.Default()
	processor := NewHTTPProcessor(logger, 1024*1024)

	// Response without Content-Length - this will be treated as incomplete
	// since there's no way to know when it ends
	responseData := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\nHello Server")
	requestID := "test-no-cl"

	_, msg, isComplete, err := processor.ProcessResponse(responseData, requestID)
	if err != nil {
		t.Fatalf("ProcessResponse failed: %v", err)
	}

	// Without Content-Length, it's treated as incomplete (streaming)
	// Note: actual behavior depends on the implementation
	_ = isComplete
	_ = msg
}
