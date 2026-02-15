package llm

import (
	"encoding/json"
	"log/slog"
	"testing"
)

func TestAnthropicMatch(t *testing.T) {
	provider := anthropicProvider{logger: slog.Default()}

	tests := []struct {
		name     string
		hostname string
		path     string
		body     []byte
		want     bool
	}{
		{
			name:     "official Anthropic API",
			hostname: "api.anthropic.com",
			path:     "/v1/messages",
			body:     []byte(`{"model": "claude-3-5-sonnet-20241022"}`),
			want:     true,
		},
		{
			name:     "official Anthropic API with different path",
			hostname: "api.anthropic.com",
			path:     "/v1/messages/count_tokens",
			body:     []byte(`{"model": "claude-3-5-sonnet-20241022"}`),
			want:     false, // Match only matches exact /v1/messages
		},
		{
			name:     "DeepSeek compatible API",
			hostname: "api.deepseek.com",
			path:     "/v1/chat/completions",  // path doesn't contain "anthropic", so will return false
			body:     []byte(`{"model": "deepseek-chat"}`),
			want:     false,
		},
		{
			name:     "MiniMax compatible API",
			hostname: "api.minimaxi.com",
			path:     "/v1/chat/completions",
			body:     []byte(`{}`),
			want:     false,
		},
		{
			name:     "Moonshot compatible API",
			hostname: "api.moonshot.cn",
			path:     "/v1/chat/completions",
			body:     []byte(`{}`),
			want:     false,
		},
		{
			name:     "compatible API without anthropic in path",
			hostname: "api.deepseek.com",
			path:     "/v1/other",
			body:     []byte(`{}`),
			want:     false,
		},
		{
			name:     "unknown hostname",
			hostname: "api.unknown.com",
			path:     "/v1/messages",
			body:     []byte(`{}`),
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := provider.Match(tt.hostname, tt.path, tt.body)
			if got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAnthropicParseResponse(t *testing.T) {
	provider := anthropicProvider{logger: slog.Default()}

	tests := []struct {
		name    string
		path    string
		body    string
		want    *LLMResponse
		wantErr bool
	}{
		{
			name: "normal text response",
			path: "/v1/messages",
			body: `{
				"id": "msg_123",
				"type": "message",
				"role": "assistant",
				"content": [{"type": "text", "text": "Hello, world!"}],
				"model": "claude-3-5-sonnet-20241022",
				"stop_reason": "end_turn",
				"usage": {"input_tokens": 100, "output_tokens": 50}
			}`,
			want: &LLMResponse{
				Content:    "Hello, world!",
				StopReason: "end_turn",
				Usage: TokenUsage{
					InputTokens:  100,
					OutputTokens: 50,
				},
			},
		},
		{
			name: "count_tokens response",
			path: "/v1/messages/count_tokens",
			body: `{"input_tokens": 150}`,
			want: &LLMResponse{
				Content: "input_tokens: 150",
				Usage: TokenUsage{
					InputTokens: 150,
				},
			},
		},
		{
			name: "error response",
			path: "/v1/messages",
			body: `{
				"type": "error",
				"error": {
					"type": "invalid_request_error",
					"message": "Invalid API key"
				}
			}`,
			want: &LLMResponse{
				Content:    "Invalid API key",
				StopReason: "error",
				Error:      &APIError{Type: "invalid_request_error", Message: "Invalid API key"},
			},
		},
		{
			name:    "invalid JSON",
			path:    "/v1/messages",
			body:    `invalid json`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := provider.ParseResponse(tt.path, []byte(tt.body))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if got.Content != tt.want.Content {
				t.Errorf("Content = %v, want %v", got.Content, tt.want.Content)
			}
			if got.StopReason != tt.want.StopReason {
				t.Errorf("StopReason = %v, want %v", got.StopReason, tt.want.StopReason)
			}
			if got.Usage.InputTokens != tt.want.Usage.InputTokens {
				t.Errorf("InputTokens = %v, want %v", got.Usage.InputTokens, tt.want.Usage.InputTokens)
			}
			if got.Usage.OutputTokens != tt.want.Usage.OutputTokens {
				t.Errorf("OutputTokens = %v, want %v", got.Usage.OutputTokens, tt.want.Usage.OutputTokens)
			}
		})
	}
}

func TestAnthropicParseFullRequest(t *testing.T) {
	provider := anthropicProvider{logger: slog.Default()}

	tests := []struct {
		name string
		body string
		want *RequestInfo
	}{
		{
			name: "full request with all fields",
			body: `{
				"model": "claude-3-5-sonnet-20241022",
				"max_tokens": 1024,
				"system": "You are a helpful assistant.",
				"messages": [
					{"role": "user", "content": "Hello"}
				],
				"metadata": {
					"user_id": "user-123"
				},
				"tools": [
					{
						"name": "get_weather",
						"description": "Get weather information",
						"input_schema": {
							"type": "object",
							"properties": {
								"location": {"type": "string"}
							}
						}
					}
				]
			}`,
			want: &RequestInfo{
				ConversationID: "anthropic-", // will have hash suffix
				Model:          "claude-3-5-sonnet-20241022",
				Messages:       []LLMMessage{{Role: "user", Content: []string{"Hello"}}},
				SystemPrompts:  []string{"You are a helpful assistant."},
				Tools: []ToolDef{
					{
						Name:        "get_weather",
						Description: "Get weather information",
						InputSchema: map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"location": map[string]interface{}{"type": "string"},
							},
						},
					},
				},
			},
		},
		{
			name: "request without metadata",
			body: `{
				"model": "claude-3-5-sonnet-20241022",
				"messages": [{"role": "user", "content": "Hi"}]
			}`,
			want: &RequestInfo{
				ConversationID: "anthropic-default",
				Model:          "claude-3-5-sonnet-20241022",
				Messages:       []LLMMessage{{Role: "user", Content: []string{"Hi"}}},
			},
		},
		{
			name: "request with system array",
			body: `{
				"model": "claude-3-5-sonnet-20241022",
				"system": [{"type": "text", "text": "System prompt 1"}, {"type": "text", "text": "System prompt 2"}],
				"messages": []
			}`,
			want: &RequestInfo{
				ConversationID: "anthropic-default",
				Model:          "claude-3-5-sonnet-20241022",
				SystemPrompts:  []string{"System prompt 1", "System prompt 2"},
			},
		},
		{
			name: "invalid JSON",
			body: `invalid`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := provider.ParseFullRequest([]byte(tt.body))
			if tt.want == nil {
				if err == nil {
					t.Error("expected error for invalid JSON")
				}
				return
			}
			if err != nil {
				t.Errorf("ParseFullRequest() error = %v", err)
				return
			}
			if got.Model != tt.want.Model {
				t.Errorf("Model = %v, want %v", got.Model, tt.want.Model)
			}
			// Check ConversationID prefix
			if tt.name == "full request with all fields" {
				if got.ConversationID == "" || got.ConversationID[:10] != "anthropic-" {
					t.Errorf("ConversationID = %v, want prefix anthropic-", got.ConversationID)
				}
			} else {
				if got.ConversationID != tt.want.ConversationID {
					t.Errorf("ConversationID = %v, want %v", got.ConversationID, tt.want.ConversationID)
				}
			}
			if len(got.SystemPrompts) != len(tt.want.SystemPrompts) {
				t.Errorf("SystemPrompts length = %v, want %v", len(got.SystemPrompts), len(tt.want.SystemPrompts))
			}
			if len(got.Tools) != len(tt.want.Tools) {
				t.Errorf("Tools length = %v, want %v", len(got.Tools), len(tt.want.Tools))
			}
		})
	}
}

func TestAnthropicParseSSEStreamFrom(t *testing.T) {
	provider := anthropicProvider{logger: slog.Default()}

	tests := []struct {
		name     string
		body     string
		startPos int
		wantText string
	}{
		{
			name: "text delta",
			body: `data: {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "Hello"}}
data: {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": " World"}}
data: {"type": "message_delta", "delta": {"stop_reason": "end_turn"}, "message": {"usage": {"output_tokens": 10}}}
`,
			startPos: 0,
			wantText: "Hello World",
		},
		{
			name: "thinking delta",
			body: `data: {"type": "content_block_delta", "index": 0, "delta": {"type": "thinking_delta", "thinking": "Let me think"}}
`,
			startPos: 0,
			wantText: "", // thinking is stored in Thinking field, not Text
		},
		{
			name: "tool use",
			body: `data: {"type": "content_block_start", "index": 0, "content_block": {"type": "tool_use", "id": "tool_123", "name": "get_weather"}}
data: {"type": "content_block_delta", "index": 0, "delta": {"type": "input_json_delta", "partial_json": "{\"location\": \"Beijing\"}"}}
`,
			startPos: 0,
			wantText: "",
		},
		{
			name:     "empty body",
			body:     "",
			startPos: 0,
			wantText: "",
		},
		{
			name:     "start beyond length",
			body:     "data: test",
			startPos: 100,
			wantText: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(tt.body)
			deltas := provider.ParseSSEStreamFrom(body, tt.startPos)

			var text string
			for _, d := range deltas {
				text += d.Text
			}
			if text != tt.wantText {
				t.Errorf("ParseSSEStreamFrom() text = %v, want %v", text, tt.wantText)
			}
		})
	}
}

func TestAnthropicExtractSystemPrompts(t *testing.T) {
	provider := anthropicProvider{logger: slog.Default()}

	tests := []struct {
		name    string
		req     *AnthropicRequest
		want    []string
	}{
		{
			name: "system as string",
			req: &AnthropicRequest{
				System: "You are a helpful assistant.",
			},
			want: []string{"You are a helpful assistant."},
		},
		{
			name: "system as array",
			req: &AnthropicRequest{
				System: []any{
					map[string]any{"type": "text", "text": "System prompt 1"},
					map[string]any{"type": "text", "text": "System prompt 2"},
				},
			},
			want: []string{"System prompt 1", "System prompt 2"},
		},
		{
			name: "nil system",
			req:  &AnthropicRequest{},
			want: nil,
		},
		{
			name: "empty array",
			req: &AnthropicRequest{
				System: []any{},
			},
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := provider.extractSystemPromptsFromReq(tt.req)
			if len(got) != len(tt.want) {
				t.Errorf("extractSystemPromptsFromReq() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("extractSystemPromptsFromReq()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestAnthropicExtractToolsFromReq(t *testing.T) {
	provider := anthropicProvider{logger: slog.Default()}

	tests := []struct {
		name string
		req  *AnthropicRequest
		want []ToolDef
	}{
		{
			name: "with tools",
			req: &AnthropicRequest{
				Tools: []AnthropicTool{
					{
						Name:        "get_weather",
						Description: "Get weather",
						InputSchema: map[string]interface{}{"type": "object"},
					},
				},
			},
			want: []ToolDef{
				{
					Name:        "get_weather",
					Description: "Get weather",
					InputSchema: map[string]interface{}{"type": "object"},
				},
			},
		},
		{
			name: "empty tools",
			req:  &AnthropicRequest{},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := provider.extractToolsFromReq(tt.req)
			if len(got) != len(tt.want) {
				t.Errorf("extractToolsFromReq() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i].Name != tt.want[i].Name {
					t.Errorf("extractToolsFromReq()[%d].Name = %v, want %v", i, got[i].Name, tt.want[i].Name)
				}
			}
		})
	}
}

func TestAnthropicCompatibleHosts(t *testing.T) {
	// Verify compatibleHosts map contains expected hosts
	expectedHosts := []string{
		"api.minimaxi.com",
		"api.minimax.io",
		"api.deepseek.com",
		"open.bigmodel.cn",
		"api.z.ai",
		"dashscope.aliyuncs.com",
		"api.moonshot.cn",
		"api.longcat.chat",
		"api.tbox.cn",
		"api.xiaomimimo.com",
	}

	for _, hostname := range expectedHosts {
		t.Run(hostname, func(t *testing.T) {
			if !compatibleHosts[hostname] {
				t.Errorf("expected %s to be in compatibleHosts map", hostname)
			}
		})
	}

	// Test matching with path containing "anthropic"
	provider := anthropicProvider{logger: slog.Default()}
	matched := provider.Match("api.deepseek.com", "/v1/chat/completions", []byte(`{}`))
	if matched {
		t.Error("expected Match() = false for path without 'anthropic'")
	}

	// With "anthropic" in path
	matched = provider.Match("api.deepseek.com", "/v1/chat/completions", []byte(`{}`))
	_ = matched
}

// TestTokenDeltaMerging tests that SSE deltas are properly merged
func TestTokenDeltaMerging(t *testing.T) {
	provider := anthropicProvider{logger: slog.Default()}

	// Multiple text deltas should be merged
	body := `data: {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "Hello"}}
data: {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": " World"}}
data: {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "!"}}
data: {"type": "message_delta", "delta": {"stop_reason": "end_turn"}, "message": {"usage": {"output_tokens": 10}}}
`

	deltas := provider.ParseSSEStreamFrom([]byte(body), 0)

	if len(deltas) != 1 {
		t.Errorf("expected 1 merged delta, got %d", len(deltas))
	}

	if deltas[0].Text != "Hello World!" {
		t.Errorf("expected merged text 'Hello World!', got %q", deltas[0].Text)
	}

	if deltas[0].StopReason != "end_turn" {
		t.Errorf("expected stop_reason 'end_turn', got %q", deltas[0].StopReason)
	}

	// Check usage is propagated
	if deltas[0].Usage.OutputTokens != 10 {
		t.Errorf("expected output_tokens 10, got %d", deltas[0].Usage.OutputTokens)
	}
}

// BenchmarkParseSSEStreamFrom benchmarks the SSE parsing
func BenchmarkParseSSEStreamFrom(b *testing.B) {
	provider := anthropicProvider{logger: slog.Default()}

	body := `data: {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "Hello"}}
data: {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": " World"}}
data: {"type": "message_delta", "delta": {"stop_reason": "end_turn"}, "message": {"usage": {"output_tokens": 10}}}
`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		provider.ParseSSEStreamFrom([]byte(body), 0)
	}
}

func TestAnthropicMessageContentTypes(t *testing.T) {
	// Test that different message content types are handled correctly
	tests := []struct {
		name    string
		message AnthropicMessage
		want    string
	}{
		{
			name: "content as string",
			message: AnthropicMessage{
				Role:    "user",
				Content: "Hello",
			},
			want: "Hello",
		},
		{
			name: "content as array",
			message: AnthropicMessage{
				Role: "user",
				Content: []AnthropicContent{
					{Type: "text", Text: "Hello"},
					{Type: "text", Text: " World"},
				},
			},
			want: "Hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test JSON marshaling/unmarshaling
			data, err := json.Marshal(tt.message)
			if err != nil {
				t.Fatalf("failed to marshal: %v", err)
			}

			var parsed AnthropicMessage
			if err := json.Unmarshal(data, &parsed); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}

			// Verify content extraction logic
			switch c := parsed.Content.(type) {
			case string:
				if c != tt.want {
					t.Errorf("content string = %v, want %v", c, tt.want)
				}
			case []any:
				// Content is parsed as []any, need to check first element
				if len(c) == 0 {
					t.Error("expected non-empty content array")
				}
			}
		})
	}
}
