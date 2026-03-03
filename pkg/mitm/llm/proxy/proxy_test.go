package proxy

import (
	"log/slog"
	"testing"

	"github.com/monsterxx03/linko/pkg/mitm/llm"
)

func TestProxy_TransformRequest(t *testing.T) {
	cfg := &ProxyConfig{
		UpstreamURL: "http://localhost:11434",
		ModelMapping: map[string]string{
			"claude-3-sonnet-20240229": "llama3",
		},
		Timeout: 120,
	}
	logger := testLogger()
	p := NewProxy(cfg, logger)

	// Test basic request transformation
	anthropicReq := &llm.AnthropicRequest{
		Model:     "claude-3-sonnet-20240229",
		MaxTokens: 1024,
		System:    "You are a helpful assistant.",
		Messages: []llm.AnthropicMessage{
			{
				Role:    "user",
				Content: "Hello",
			},
		},
		Temperature: 0.7,
		TopP:       0.9,
	}

	openaiReq, err := p.TransformRequest(anthropicReq)
	if err != nil {
		t.Fatalf("failed to transform request: %v", err)
	}

	// Check model mapping
	if openaiReq.Model != "llama3" {
		t.Errorf("expected model 'llama3', got '%s'", openaiReq.Model)
	}

	// Check system prompt was converted to messages
	if len(openaiReq.Messages) == 0 {
		t.Fatal("expected messages, got none")
	}
	if openaiReq.Messages[0].Role != "system" {
		t.Errorf("expected first message to be system, got '%s'", openaiReq.Messages[0].Role)
	}

	// Check user message
	if len(openaiReq.Messages) < 2 {
		t.Fatal("expected at least 2 messages")
	}
	if openaiReq.Messages[1].Role != "user" {
		t.Errorf("expected second message to be user, got '%s'", openaiReq.Messages[1].Role)
	}

	// Check other fields
	if openaiReq.MaxTokens != 1024 {
		t.Errorf("expected max_tokens 1024, got %d", openaiReq.MaxTokens)
	}
	if openaiReq.Temperature != 0.7 {
		t.Errorf("expected temperature 0.7, got %f", openaiReq.Temperature)
	}
	if openaiReq.TopP != 0.9 {
		t.Errorf("expected top_p 0.9, got %f", openaiReq.TopP)
	}
}

func TestProxy_TransformRequest_WithTools(t *testing.T) {
	cfg := &ProxyConfig{
		UpstreamURL: "http://localhost:11434",
		Timeout:    120,
	}
	logger := testLogger()
	p := NewProxy(cfg, logger)

	anthropicReq := &llm.AnthropicRequest{
		Model:     "claude-3-sonnet-20240229",
		MaxTokens: 1024,
		Messages: []llm.AnthropicMessage{
			{
				Role:    "user",
				Content: "What's the weather?",
			},
		},
		Tools: []llm.AnthropicTool{
			{
				Name:        "get_weather",
				Description: "Get weather information",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"city": map[string]interface{}{
							"type": "string",
						},
					},
					"required": []string{"city"},
				},
			},
		},
	}

	openaiReq, err := p.TransformRequest(anthropicReq)
	if err != nil {
		t.Fatalf("failed to transform request: %v", err)
	}

	if len(openaiReq.Tools) == 0 {
		t.Fatal("expected tools in transformed request")
	}

	tool := openaiReq.Tools[0]
	if tool.Type != "function" {
		t.Errorf("expected tool type 'function', got '%s'", tool.Type)
	}
	if tool.Function.Name != "get_weather" {
		t.Errorf("expected tool name 'get_weather', got '%s'", tool.Function.Name)
	}
}

func TestProxy_TransformRequest_WithToolCalls(t *testing.T) {
	cfg := &ProxyConfig{
		UpstreamURL: "http://localhost:11434",
		Timeout:    120,
	}
	logger := testLogger()
	p := NewProxy(cfg, logger)

	anthropicReq := &llm.AnthropicRequest{
		Model:     "claude-3-sonnet-20240229",
		MaxTokens: 1024,
		Messages: []llm.AnthropicMessage{
			{
				Role: "assistant",
				Content: []any{
					map[string]any{
						"type": "tool_use",
						"id":   "toolu_1",
						"name": "get_weather",
						"input": map[string]any{
							"city": "Beijing",
						},
					},
				},
			},
			{
				Role: "user",
				Content: []any{
					map[string]any{
						"type":       "tool_result",
						"tool_use_id": "toolu_1",
						"content":    "Sunny, 25°C",
					},
				},
			},
		},
	}

	openaiReq, err := p.TransformRequest(anthropicReq)
	if err != nil {
		t.Fatalf("failed to transform request: %v", err)
	}

	// Check assistant message with tool call
	if len(openaiReq.Messages) < 1 {
		t.Fatal("expected messages")
	}

	assistantMsg := openaiReq.Messages[0]
	if assistantMsg.Role != "assistant" {
		t.Errorf("expected role 'assistant', got '%s'", assistantMsg.Role)
	}
	if len(assistantMsg.ToolCalls) == 0 {
		t.Error("expected tool calls in assistant message")
	}
}

func TestProxy_TransformResponse(t *testing.T) {
	cfg := &ProxyConfig{
		UpstreamURL: "http://localhost:11434",
		Timeout:    120,
	}
	logger := testLogger()
	p := NewProxy(cfg, logger)

	openaiResp := &llm.OpenAIResponse{
		ID:      "chatcmpl-123",
		Model:   "llama3",
		Choices: []llm.OpenAIChoice{
			{
				Message: llm.OpenAIMessage{
					Role:    "assistant",
					Content: "Hello! How can I help you?",
				},
				FinishReason: "stop",
			},
		},
		Usage: llm.OpenAIUsage{
			PromptTokens:     10,
			CompletionTokens: 20,
			TotalTokens:      30,
		},
	}

	anthropicResp := p.TransformResponse(openaiResp, "claude-3-sonnet-20240229")

	if anthropicResp.Type != "message" {
		t.Errorf("expected type 'message', got '%s'", anthropicResp.Type)
	}
	if anthropicResp.Role != "assistant" {
		t.Errorf("expected role 'assistant', got '%s'", anthropicResp.Role)
	}
	if anthropicResp.StopReason != "end_turn" {
		t.Errorf("expected stop_reason 'end_turn', got '%s'", anthropicResp.StopReason)
	}
	if len(anthropicResp.Content) == 0 {
		t.Fatal("expected content")
	}
	if anthropicResp.Content[0].Type != "text" {
		t.Errorf("expected content type 'text', got '%s'", anthropicResp.Content[0].Type)
	}
	if anthropicResp.Content[0].Text != "Hello! How can I help you?" {
		t.Errorf("expected text 'Hello! How can I help you?', got '%s'", anthropicResp.Content[0].Text)
	}
	if anthropicResp.Usage.InputTokens != 10 {
		t.Errorf("expected input_tokens 10, got %d", anthropicResp.Usage.InputTokens)
	}
	if anthropicResp.Usage.OutputTokens != 20 {
		t.Errorf("expected output_tokens 20, got %d", anthropicResp.Usage.OutputTokens)
	}
}

func TestProxy_TransformResponse_WithReasoning(t *testing.T) {
	cfg := &ProxyConfig{
		UpstreamURL: "http://localhost:11434",
		Timeout:    120,
	}
	logger := testLogger()
	p := NewProxy(cfg, logger)

	openaiResp := &llm.OpenAIResponse{
		ID:      "chatcmpl-123",
		Model:   "gpt-4o",
		Choices: []llm.OpenAIChoice{
			{
				Message: llm.OpenAIMessage{
					Role:             "assistant",
					Content:          "Final answer",
					ReasoningContent: "Reasoning process...",
				},
				FinishReason: "stop",
			},
		},
		Usage: llm.OpenAIUsage{
			PromptTokens:     10,
			CompletionTokens: 20,
			TotalTokens:      30,
		},
	}

	anthropicResp := p.TransformResponse(openaiResp, "claude-3-sonnet-20240229")

	// Check that reasoning content was converted to thinking
	if len(anthropicResp.Content) < 2 {
		t.Fatalf("expected at least 2 content blocks, got %d", len(anthropicResp.Content))
	}
	if anthropicResp.Content[0].Type != "thinking" {
		t.Errorf("expected first content type 'thinking', got '%s'", anthropicResp.Content[0].Type)
	}
	if anthropicResp.Content[0].Thinking != "Reasoning process..." {
		t.Errorf("expected thinking 'Reasoning process...', got '%s'", anthropicResp.Content[0].Thinking)
	}
}

func TestProxy_TransformResponse_ToolCalls(t *testing.T) {
	cfg := &ProxyConfig{
		UpstreamURL: "http://localhost:11434",
		Timeout:    120,
	}
	logger := testLogger()
	p := NewProxy(cfg, logger)

	openaiResp := &llm.OpenAIResponse{
		ID:      "chatcmpl-123",
		Model:   "llama3",
		Choices: []llm.OpenAIChoice{
			{
				Message: llm.OpenAIMessage{
					Role:    "assistant",
					Content: nil,
					ToolCalls: []llm.ToolCall{
						{
							ID:   "call_123",
							Type: "function",
							Function: llm.FunctionCall{
								Name:      "get_weather",
								Arguments: `{"city":"Beijing"}`,
							},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
	}

	anthropicResp := p.TransformResponse(openaiResp, "claude-3-sonnet-20240229")

	if anthropicResp.StopReason != "tool_use" {
		t.Errorf("expected stop_reason 'tool_use', got '%s'", anthropicResp.StopReason)
	}
}

func TestProxy_mapModel(t *testing.T) {
	cfg := &ProxyConfig{
		ModelMapping: map[string]string{
			"claude-3-sonnet-20240229": "llama3",
			"claude-3-opus-20240229":   "llama3:70b",
		},
	}
	logger := testLogger()
	p := NewProxy(cfg, logger)

	tests := []struct {
		input    string
		expected string
	}{
		{"claude-3-sonnet-20240229", "llama3"},
		{"claude-3-opus-20240229", "llama3:70b"},
		{"unknown-model", "unknown-model"}, // unmapped model should pass through
	}

	for _, tt := range tests {
		result := p.mapModel(tt.input)
		if result != tt.expected {
			t.Errorf("mapModel(%s) = %s; want %s", tt.input, result, tt.expected)
		}
	}
}

func TestMapStopReason(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"stop", "end_turn"},
		{"length", "max_tokens"},
		{"tool_calls", "tool_use"},
		{"content_filter", "stopping_reason"},
		{"unknown", "end_turn"},
	}

	for _, tt := range tests {
		result := mapStopReason(tt.input)
		if result != tt.expected {
			t.Errorf("mapStopReason(%s) = %s; want %s", tt.input, result, tt.expected)
		}
	}
}

// testLogger creates a test logger
func testLogger() *slog.Logger {
	return slog.Default()
}
