package mitm

import (
	"log/slog"
	"testing"

	"github.com/monsterxx03/linko/pkg/mitm/llm"
)

// mockHTTPProcessor implements HTTPProcessorInterface for testing
type mockHTTPProcessor struct {
	t                  *testing.T
	pendingReqs       map[string]*HTTPMessage
	pendingResps      map[string]*HTTPMessage
	processRequestFunc func(data []byte, requestID string) ([]byte, *HTTPMessage, bool, error)
	processResponseFunc func(data []byte, requestID string) ([]byte, *HTTPMessage, bool, error)
}

func newMockHTTPProcessor(t *testing.T) *mockHTTPProcessor {
	return &mockHTTPProcessor{
		t:             t,
		pendingReqs:   make(map[string]*HTTPMessage),
		pendingResps:  make(map[string]*HTTPMessage),
	}
}

func (m *mockHTTPProcessor) ProcessRequest(inputData []byte, requestID string) ([]byte, *HTTPMessage, bool, error) {
	if m.processRequestFunc != nil {
		return m.processRequestFunc(inputData, requestID)
	}
	// Default: return the input data as-is, with no message
	return inputData, nil, false, nil
}

func (m *mockHTTPProcessor) ProcessResponse(inputData []byte, requestID string) ([]byte, *HTTPMessage, bool, error) {
	if m.processResponseFunc != nil {
		return m.processResponseFunc(inputData, requestID)
	}
	// Default: return the input data as-is, with no message
	return inputData, nil, false, nil
}

func (m *mockHTTPProcessor) ClearPending(requestID string) {
	delete(m.pendingReqs, requestID)
	delete(m.pendingResps, requestID)
}

func (m *mockHTTPProcessor) GetPendingMessage(requestID string) (*HTTPMessage, bool) {
	if msg, ok := m.pendingReqs[requestID]; ok {
		return msg, true
	}
	if msg, ok := m.pendingResps[requestID]; ok {
		return msg, true
	}
	return nil, false
}

// mockProvider implements llm.Provider for testing
type mockProvider struct {
	t             *testing.T
	matched       bool
	matchFunc     func(hostname, path string, body []byte) bool
	resp          *llm.LLMResponse
	respErr       error
	reqInfo       *llm.RequestInfo
	reqInfoErr    error
	deltas        []llm.TokenDelta
}

func newMockProvider(t *testing.T) *mockProvider {
	return &mockProvider{t: t}
}

func (m *mockProvider) Match(hostname, path string, body []byte) bool {
	if m.matchFunc != nil {
		return m.matchFunc(hostname, path, body)
	}
	return m.matched
}

func (m *mockProvider) ParseResponse(path string, body []byte) (*llm.LLMResponse, error) {
	return m.resp, m.respErr
}

func (m *mockProvider) ParseSSEStreamFrom(body []byte, startPos int) []llm.TokenDelta {
	return m.deltas
}

func (m *mockProvider) ParseFullRequest(body []byte) (*llm.RequestInfo, error) {
	return m.reqInfo, m.reqInfoErr
}

func TestLLMInspector_InspectRequest_Basic(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewLLMInspector(logger, eventBus, "api.openai.com")

	// Setup mock HTTP processor
	mockProc := newMockHTTPProcessor(t)
	mockProc.processRequestFunc = func(data []byte, requestID string) ([]byte, *HTTPMessage, bool, error) {
		msg := &HTTPMessage{
			Hostname:    "api.openai.com",
			Path:        "/v1/chat/completions",
			Method:      "POST",
			ContentType: "application/json",
			Body:        []byte(`{"messages":[{"role":"user","content":"hello"}]}`),
		}
		return data, msg, true, nil
	}
	inspector.httpProc = mockProc

	// Setup mock provider
	mockProvider := newMockProvider(t)
	mockProvider.matched = true
	mockProvider.reqInfo = &llm.RequestInfo{
		ConversationID: "conv-123",
		Model:          "gpt-4",
		Messages: []llm.LLMMessage{
			{Role: "user", Content: []string{"hello"}},
		},
	}
	// Override FindProvider to return our mock
	// Note: We can't easily override FindProvider, so we test the flow indirectly

	// Process request
	requestData := []byte("POST /v1/chat/completions HTTP/1.1\r\nHost: api.openai.com\r\nContent-Length: 50\r\n\r\n{\"messages\":[{\"role\":\"user\",\"content\":\"hello\"}]}")
	result, err := inspector.Inspect(DirectionClientToServer, requestData, "api.openai.com", "conn-1", "req-1")

	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	// Result should be unchanged
	if !bytesEqual(result, requestData) {
		t.Error("Expected result to match input")
	}
}

func TestLLMInspector_InspectRequest_NoProvider(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewLLMInspector(logger, eventBus, "unknown.com")

	// Setup mock HTTP processor that returns a valid message
	mockProc := newMockHTTPProcessor(t)
	mockProc.processRequestFunc = func(data []byte, requestID string) ([]byte, *HTTPMessage, bool, error) {
		msg := &HTTPMessage{
			Hostname:    "unknown.com",
			Path:        "/unknown",
			Method:      "POST",
			ContentType: "application/json",
			Body:        []byte(`{}`),
		}
		return data, msg, true, nil
	}
	inspector.httpProc = mockProc

	// Process request - should not panic even without a matching provider
	requestData := []byte("POST /unknown HTTP/1.1\r\nHost: unknown.com\r\nContent-Length: 2\r\n\r\n{}")
	result, err := inspector.Inspect(DirectionClientToServer, requestData, "unknown.com", "conn-1", "req-2")

	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	_ = result // Should be unchanged
}

func TestLLMInspector_InspectRequest_EmptyBody(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewLLMInspector(logger, eventBus, "api.openai.com")

	// Setup mock HTTP processor that returns empty body
	mockProc := newMockHTTPProcessor(t)
	mockProc.processRequestFunc = func(data []byte, requestID string) ([]byte, *HTTPMessage, bool, error) {
		msg := &HTTPMessage{
			Hostname:    "api.openai.com",
			Path:        "/v1/chat/completions",
			Method:      "POST",
			ContentType: "application/json",
			Body:        []byte{},
		}
		return data, msg, true, nil
	}
	inspector.httpProc = mockProc

	// Process request with empty body - should not panic
	requestData := []byte("POST /v1/chat/completions HTTP/1.1\r\nHost: api.openai.com\r\nContent-Length: 0\r\n\r\n")
	result, err := inspector.Inspect(DirectionClientToServer, requestData, "api.openai.com", "conn-1", "req-3")

	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	_ = result
}

func TestLLMInspector_InspectResponse_Basic(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewLLMInspector(logger, eventBus, "api.openai.com")
	requestID := "req-4"

	// First, process a request to set up the conversation
	mockProc := newMockHTTPProcessor(t)
	mockProc.processRequestFunc = func(data []byte, rid string) ([]byte, *HTTPMessage, bool, error) {
		msg := &HTTPMessage{
			Hostname:    "api.openai.com",
			Path:        "/v1/chat/completions",
			Method:      "POST",
			ContentType: "application/json",
			Body:        []byte(`{"messages":[{"role":"user","content":"hello"}]}`),
		}
		return data, msg, true, nil
	}
	inspector.httpProc = mockProc

	requestData := []byte("POST /v1/chat/completions HTTP/1.1\r\nHost: api.openai.com\r\nContent-Length: 50\r\n\r\n{\"messages\":[{\"role\":\"user\",\"content\":\"hello\"}]}")
	_, _ = inspector.Inspect(DirectionClientToServer, requestData, "api.openai.com", "conn-1", requestID)

	// Now process response - setup mock to return a complete response
	mockProc.processResponseFunc = func(data []byte, rid string) ([]byte, *HTTPMessage, bool, error) {
		msg := &HTTPMessage{
			Hostname:    "api.openai.com",
			Path:        "/v1/chat/completions",
			StatusCode: 200,
			ContentType: "application/json",
			Body:        []byte(`{"choices":[{"message":{"role":"assistant","content":"hi"}}]}`),
		}
		return data, msg, true, nil
	}

	responseData := []byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 60\r\n\r\n{\"choices\":[{\"message\":{\"role\":\"assistant\",\"content\":\"hi\"}}]}")
	result, err := inspector.Inspect(DirectionServerToClient, responseData, "api.openai.com", "conn-1", requestID)

	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	_ = result
}

func TestLLMInspector_InspectResponse_SSE(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewLLMInspector(logger, eventBus, "api.openai.com")
	requestID := "req-5"

	// First process a request
	mockProc := newMockHTTPProcessor(t)
	mockProc.processRequestFunc = func(data []byte, rid string) ([]byte, *HTTPMessage, bool, error) {
		msg := &HTTPMessage{
			Hostname:    "api.openai.com",
			Path:        "/v1/chat/completions",
			Method:      "POST",
			ContentType: "application/json",
			Body:        []byte(`{"messages":[{"role":"user","content":"hello"}]}`),
		}
		return data, msg, true, nil
	}
	inspector.httpProc = mockProc

	requestData := []byte("POST /v1/chat/completions HTTP/1.1\r\nHost: api.openai.com\r\nContent-Length: 50\r\n\r\n{\"messages\":[{\"role\":\"user\",\"content\":\"hello\"}]}")
	_, _ = inspector.Inspect(DirectionClientToServer, requestData, "api.openai.com", "conn-1", requestID)

	// Now process SSE response
	mockProc.processResponseFunc = func(data []byte, rid string) ([]byte, *HTTPMessage, bool, error) {
		msg := &HTTPMessage{
			Hostname:    "api.openai.com",
			Path:        "/v1/chat/completions",
			StatusCode: 200,
			ContentType: "text/event-stream",
			Body:        []byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"),
			IsSSE:       true,
		}
		return data, msg, false, nil
	}

	responseData := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\ndata: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n")
	result, err := inspector.Inspect(DirectionServerToClient, responseData, "api.openai.com", "conn-1", requestID)

	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	_ = result
}

func TestLLMInspector_InspectResponse_EmptyData(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewLLMInspector(logger, eventBus, "api.openai.com")

	// Process empty data - should not panic
	result, err := inspector.Inspect(DirectionServerToClient, []byte{}, "api.openai.com", "conn-1", "req-6")

	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if len(result) != 0 {
		t.Error("Expected empty result for empty input")
	}
}

func TestLLMInspector_InspectRequest_Incremental(t *testing.T) {
	logger := slog.Default()
	eventBus := NewEventBus(logger, 10)
	inspector := NewLLMInspector(logger, eventBus, "api.openai.com")
	requestID := "req-7"

	// First chunk - incomplete request
	mockProc := newMockHTTPProcessor(t)
	mockProc.processRequestFunc = func(data []byte, rid string) ([]byte, *HTTPMessage, bool, error) {
		// Return incomplete (no message)
		return data, nil, false, nil
	}
	inspector.httpProc = mockProc

	chunk1 := []byte("POST /v1/chat/completions HTTP/1.1\r\nHost: api.openai.com\r\nContent-Length: 50\r\n\r\n{\"mess")
	result1, err := inspector.Inspect(DirectionClientToServer, chunk1, "api.openai.com", "conn-1", requestID)

	if err != nil {
		t.Fatalf("Inspect chunk1 failed: %v", err)
	}

	_ = result1

	// Second chunk - complete request
	mockProc.processRequestFunc = func(data []byte, rid string) ([]byte, *HTTPMessage, bool, error) {
		msg := &HTTPMessage{
			Hostname:    "api.openai.com",
			Path:        "/v1/chat/completions",
			Method:      "POST",
			ContentType: "application/json",
			Body:        []byte(`{"messages":[{"role":"user","content":"hello"}]}`),
		}
		return data, msg, true, nil
	}

	chunk2 := []byte("ages\":[{\"role\":\"user\",\"content\":\"hello\"}]}")
	result2, err := inspector.Inspect(DirectionClientToServer, chunk2, "api.openai.com", "conn-1", requestID)

	if err != nil {
		t.Fatalf("Inspect chunk2 failed: %v", err)
	}

	_ = result2
}

// bytesEqual compares two byte slices
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
