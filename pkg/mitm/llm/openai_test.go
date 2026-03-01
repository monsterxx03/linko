package llm

import (
	"log/slog"
	"testing"
)

func TestOpenAIMatch(t *testing.T) {
	provider := openaiProvider{logger: slog.Default()}

	tests := []struct {
		name     string
		hostname string
		path     string
		body     []byte
		want     bool
	}{
		{
			name:     "official OpenAI API",
			hostname: "api.openai.com",
			path:     "/v1/chat/completions",
			body:     []byte(`{"model": "gpt-4"}`),
			want:     true,
		},
		{
			name:     "Azure OpenAI",
			hostname: "openai.azure.com",
			path:     "/v1/chat/completions",
			body:     []byte(`{"model": "gpt-4"}`),
			want:     true,
		},
		{
			name:     "path contains chat/completions",
			hostname: "api.example.com",
			path:     "/v1/chat/completions",
			body:     []byte(`{"model": "gpt-4"}`),
			want:     true,
		},
		{
			name:     "unknown hostname",
			hostname: "api.unknown.com",
			path:     "/v1/other",
			body:     []byte(`{"model": "gpt-4"}`),
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

func TestOpenAIParseResponse(t *testing.T) {
	provider := openaiProvider{logger: slog.Default()}

	tests := []struct {
		name    string
		path    string
		body    string
		want    *LLMResponse
		wantErr bool
	}{
		{
			name: "normal text response",
			path: "/v1/chat/completions",
			body: `{
				"id": "chatcmpl-123",
				"object": "chat.completion",
				"created": 1677652288,
				"model": "gpt-3.5-turbo",
				"choices": [{
					"index": 0,
					"message": {
						"role": "assistant",
						"content": "Hello, world!"
					},
					"finish_reason": "stop"
				}],
				"usage": {
					"prompt_tokens": 10,
					"completion_tokens": 5,
					"total_tokens": 15
				}
			}`,
			want: &LLMResponse{
				Content:    "Hello, world!",
				StopReason: "stop",
				Usage: TokenUsage{
					InputTokens:  10,
					OutputTokens: 5,
				},
			},
		},
		{
			name: "response with reasoning_content (o1 model)",
			path: "/v1/chat/completions",
			body: `{
				"id": "chatcmpl-123",
				"object": "chat.completion",
				"created": 1677652288,
				"model": "o1-preview",
				"choices": [{
					"index": 0,
					"message": {
						"role": "assistant",
						"content": "The answer is 42.",
						"reasoning_content": "Let me think about this step by step..."
					},
					"finish_reason": "stop"
				}],
				"usage": {
					"prompt_tokens": 100,
					"completion_tokens": 50,
					"total_tokens": 150
				}
			}`,
			want: &LLMResponse{
				Content:    "[Reasoning]\nLet me think about this step by step...\n[/Reasoning]\nThe answer is 42.",
				StopReason: "stop",
				Usage: TokenUsage{
					InputTokens:  100,
					OutputTokens: 50,
				},
			},
		},
		{
			name: "response with tool calls",
			path: "/v1/chat/completions",
			body: `{
				"id": "chatcmpl-123",
				"object": "chat.completion",
				"created": 1677652288,
				"model": "gpt-4",
				"choices": [{
					"index": 0,
					"message": {
						"role": "assistant",
						"content": null,
						"tool_calls": [
							{
								"id": "call_123",
								"type": "function",
								"function": {
									"name": "get_weather",
									"arguments": "{\"location\": \"Beijing\"}"
								}
							}
						]
					},
					"finish_reason": "tool_calls"
				}],
				"usage": {
					"prompt_tokens": 50,
					"completion_tokens": 30,
					"total_tokens": 80
				}
			}`,
			want: &LLMResponse{
				Content:    "",
				StopReason: "tool_calls",
				Usage: TokenUsage{
					InputTokens:  50,
					OutputTokens: 30,
				},
				ToolCalls: []ToolCall{
					{
						ID:   "call_123",
						Type: "function",
						Function: FunctionCall{
							Name:      "get_weather",
							Arguments: "{\"location\": \"Beijing\"}",
						},
					},
				},
			},
		},
		{
			name: "response with content array",
			path: "/v1/chat/completions",
			body: `{
				"id": "chatcmpl-123",
				"object": "chat.completion",
				"created": 1677652288,
				"model": "gpt-4-vision",
				"choices": [{
					"index": 0,
					"message": {
						"role": "assistant",
						"content": [
							{"type": "text", "text": "This is "},
							{"type": "text", "text": "a test."}
						]
					},
					"finish_reason": "stop"
				}],
				"usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
			}`,
			want: &LLMResponse{
				Content:    "This is a test.",
				StopReason: "stop",
				Usage: TokenUsage{
					InputTokens:  10,
					OutputTokens: 5,
				},
			},
		},
		{
			name:    "invalid JSON",
			path:    "/v1/chat/completions",
			body:    `invalid json`,
			wantErr: true,
		},
		{
			name: "empty choices",
			path: "/v1/chat/completions",
			body: `{
				"id": "chatcmpl-123",
				"object": "chat.completion",
				"created": 1677652288,
				"model": "gpt-3.5-turbo",
				"choices": [],
				"usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
			}`,
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
			if len(got.ToolCalls) != len(tt.want.ToolCalls) {
				t.Errorf("ToolCalls length = %v, want %v", len(got.ToolCalls), len(tt.want.ToolCalls))
			}
		})
	}
}

func TestOpenAIParseFullRequest(t *testing.T) {
	provider := openaiProvider{logger: slog.Default()}

	tests := []struct {
		name string
		body string
		want *RequestInfo
	}{
		{
			name: "full request with all fields",
			body: `{
				"model": "gpt-4",
				"max_tokens": 1024,
				"temperature": 0.7,
				"system": "You are a helpful assistant.",
				"messages": [
					{"role": "user", "content": "Hello"}
				],
				"tools": [
					{
						"type": "function",
						"function": {
							"name": "get_weather",
							"description": "Get weather information",
							"parameters": {
								"type": "object",
								"properties": {
									"location": {"type": "string"}
								}
							}
						}
					}
				]
			}`,
			want: &RequestInfo{
				Model:        "gpt-4",
				Messages:     []LLMMessage{{Role: "user", Content: []string{"Hello"}}},
				SystemPrompts: []string{"You are a helpful assistant."},
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
			name: "request with tool call message",
			body: `{
				"model": "gpt-4",
				"messages": [
					{"role": "user", "content": "What's the weather?"},
					{"role": "assistant", "content": null, "tool_calls": [{"id": "call_123", "type": "function", "function": {"name": "get_weather", "arguments": "{\"location\": \"Beijing\"}"}}]},
					{"role": "tool", "tool_call_id": "call_123", "content": "Sunny, 25°C"}
				]
			}`,
			want: &RequestInfo{
				Model: "gpt-4",
				Messages: []LLMMessage{
					{Role: "user", Content: []string{"What's the weather?"}},
					{Role: "assistant", ToolCalls: []ToolCall{{ID: "call_123", Type: "function", Function: FunctionCall{Name: "get_weather", Arguments: "{\"location\": \"Beijing\"}"}}}},
					{Role: "tool", ToolResults: []ToolResult{{ToolUseID: "call_123", Content: "Sunny, 25°C"}}},
				},
			},
		},
		{
			name: "request without tools",
			body: `{
				"model": "gpt-3.5-turbo",
				"messages": [{"role": "user", "content": "Hi"}]
			}`,
			want: &RequestInfo{
				Model:     "gpt-3.5-turbo",
				Messages:  []LLMMessage{{Role: "user", Content: []string{"Hi"}}},
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
			if got.ConversationID == "" || got.ConversationID[:7] != "openai-" {
				t.Errorf("ConversationID = %v, want prefix openai-", got.ConversationID)
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

func TestOpenAIParseSSEStreamFrom(t *testing.T) {
	provider := openaiProvider{logger: slog.Default()}

	tests := []struct {
		name     string
		body     string
		startPos int
		wantText string
	}{
		{
			name: "text delta",
			body: `data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-3.5-turbo","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}
data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-3.5-turbo","choices":[{"index":0,"delta":{"content":" World"},"finish_reason":null}]}
data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-3.5-turbo","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}
`,
			startPos: 0,
			wantText: "Hello World",
		},
		{
			name: "reasoning_content delta (o1 model)",
			body: `data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"o1-preview","choices":[{"index":0,"delta":{"reasoning_content":"Let me think"}}]}
data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"o1-preview","choices":[{"index":0,"delta":{"reasoning_content":" about this..."}}]}
data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"o1-preview","choices":[{"index":0,"delta":{"content":"The answer is 42."},"finish_reason":"stop"}]}
`,
			startPos: 0,
			wantText: "The answer is 42.",
		},
		{
			name: "tool call delta",
			body: `data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"id":"call_123","type":"function","function":{"name":"get_weather"}}]}}]}
data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{"tool_calls":[{"id":"call_123","type":"function","function":{"arguments":"{\"location\":"}}]}}]}
data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{"tool_calls":[{"id":"call_123","type":"function","function":{"arguments":"\"Beijing\"}"}}]}}]}
data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}
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
		{
			name: "[DONE] marker",
			body: `data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-3.5-turbo","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}
data: [DONE]
`,
			startPos: 0,
			wantText: "Hello",
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

func TestOpenAIExtractSystemPrompts(t *testing.T) {
	provider := openaiProvider{logger: slog.Default()}

	tests := []struct {
		name    string
		req     *OpenAIRequest
		want    []string
	}{
		{
			name: "system as string",
			req: &OpenAIRequest{
				System: "You are a helpful assistant.",
			},
			want: []string{"You are a helpful assistant."},
		},
		{
			name: "system in messages",
			req: &OpenAIRequest{
				Messages: []OpenAIMessage{
					{Role: "system", Content: "You are a helpful assistant."},
					{Role: "user", Content: "Hello"},
				},
			},
			want: []string{"You are a helpful assistant."},
		},
		{
			name: "both top-level and message system",
			req: &OpenAIRequest{
				System: "Top level system prompt.",
				Messages: []OpenAIMessage{
					{Role: "system", Content: "Message system prompt."},
					{Role: "user", Content: "Hello"},
				},
			},
			want: []string{"Top level system prompt.", "Message system prompt."},
		},
		{
			name: "empty system",
			req:  &OpenAIRequest{},
			want: nil,
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

func TestOpenAIExtractToolsFromReq(t *testing.T) {
	provider := openaiProvider{logger: slog.Default()}

	tests := []struct {
		name string
		req  *OpenAIRequest
		want []ToolDef
	}{
		{
			name: "with tools",
			req: &OpenAIRequest{
				Tools: []OpenAITool{
					{
						Type: "function",
						Function: OpenAIFunction{
							Name:        "get_weather",
							Description: "Get weather",
							Parameters:  map[string]interface{}{"type": "object"},
						},
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
			req:  &OpenAIRequest{},
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

func TestConvertOpenAIMessages(t *testing.T) {
	tests := []struct {
		name     string
		messages []OpenAIMessage
		want     []LLMMessage
	}{
		{
			name: "simple message",
			messages: []OpenAIMessage{
				{Role: "user", Content: "Hello"},
			},
			want: []LLMMessage{
				{Role: "user", Content: []string{"Hello"}},
			},
		},
		{
			name: "messages with tool calls",
			messages: []OpenAIMessage{
				{Role: "user", Content: "What's the weather?"},
				{Role: "assistant", ToolCalls: []ToolCall{
					{ID: "call_123", Type: "function", Function: FunctionCall{Name: "get_weather", Arguments: "{}"}},
				}},
				{Role: "tool", Content: "Sunny", Name: "get_weather"}, // Note: tool messages use Name for tool name
			},
			want: []LLMMessage{
				{Role: "user", Content: []string{"What's the weather?"}},
				{Role: "assistant", ToolCalls: []ToolCall{{ID: "call_123", Type: "function", Function: FunctionCall{Name: "get_weather", Arguments: "{}"}}}},
				{Role: "tool", Content: []string{"Sunny"}, Name: "get_weather"},
			},
		},
		{
			name: "messages with content array",
			messages: []OpenAIMessage{
				{Role: "user", Content: []any{
					map[string]any{"type": "text", "text": "Hello"},
					map[string]any{"type": "text", "text": " World"},
				}},
			},
			want: []LLMMessage{
				{Role: "user", Content: []string{"Hello", " World"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertOpenAIMessages(tt.messages)
			if len(got) != len(tt.want) {
				t.Errorf("convertOpenAIMessages() length = %v, want %v", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i].Role != tt.want[i].Role {
					t.Errorf("convertOpenAIMessages()[%d].Role = %v, want %v", i, got[i].Role, tt.want[i].Role)
				}
				if len(got[i].Content) != len(tt.want[i].Content) {
					t.Errorf("convertOpenAIMessages()[%d].Content length = %v, want %v", i, len(got[i].Content), len(tt.want[i].Content))
				}
			}
		})
	}
}

func TestOpenAITokenDeltaMerging(t *testing.T) {
	provider := openaiProvider{logger: slog.Default()}

	// Multiple text deltas should be merged
	body := `data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-3.5-turbo","choices":[{"index":0,"delta":{"content":"Hello"}}]}
data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-3.5-turbo","choices":[{"index":0,"delta":{"content":" World"}}]}
data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-3.5-turbo","choices":[{"index":0,"delta":{},"finish_reason":"stop","usage":{"completion_tokens":5}}]}
`

	deltas := provider.ParseSSEStreamFrom([]byte(body), 0)

	if len(deltas) != 1 {
		t.Errorf("expected 1 merged delta, got %d", len(deltas))
	}

	if deltas[0].Text != "Hello World" {
		t.Errorf("expected merged text 'Hello World', got %q", deltas[0].Text)
	}

	if deltas[0].StopReason != "stop" {
		t.Errorf("expected stop_reason 'stop', got %q", deltas[0].StopReason)
	}

	// Check usage is propagated
	if deltas[0].Usage.OutputTokens != 5 {
		t.Errorf("expected output_tokens 5, got %d", deltas[0].Usage.OutputTokens)
	}
}

// TestOpenAISSELastChunkWithContent tests the case where the last chunk has both content and finish_reason
func TestOpenAISSELastChunkWithContent(t *testing.T) {
	provider := openaiProvider{logger: slog.Default()}

	// Simulate the case where the last chunk has both content and finish_reason
	body := `data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-3.5-turbo","choices":[{"index":0,"delta":{"content":"Hello"}}]}
data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-3.5-turbo","choices":[{"index":0,"delta":{"content":"?"},"finish_reason":"stop"}]}
`

	deltas := provider.ParseSSEStreamFrom([]byte(body), 0)

	// Should have 1 delta with merged content
	if len(deltas) != 1 {
		t.Errorf("expected 1 delta, got %d", len(deltas))
	}

	// Content should be "Hello?" (merged)
	if deltas[0].Text != "Hello?" {
		t.Errorf("expected text 'Hello?', got %q", deltas[0].Text)
	}

	// Should be marked as complete
	if !deltas[0].IsComplete {
		t.Errorf("expected IsComplete=true")
	}

	if deltas[0].StopReason != "stop" {
		t.Errorf("expected stop_reason 'stop', got %q", deltas[0].StopReason)
	}
}

// TestOpenAISSESeparateFinishReason tests separate finish_reason chunk
func TestOpenAISSESeparateFinishReason(t *testing.T) {
	provider := openaiProvider{logger: slog.Default()}

	// Simulate separate finish_reason chunk (like [DONE])
	body := `data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-3.5-turbo","choices":[{"index":0,"delta":{"content":"Hello"}}]}
data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-3.5-turbo","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}
data: [DONE]
`

	deltas := provider.ParseSSEStreamFrom([]byte(body), 0)

	if len(deltas) != 1 {
		t.Errorf("expected 1 delta, got %d", len(deltas))
	}

	if deltas[0].Text != "Hello" {
		t.Errorf("expected text 'Hello', got %q", deltas[0].Text)
	}

	if !deltas[0].IsComplete {
		t.Errorf("expected IsComplete=true")
	}
}

// BenchmarkOpenAIParseSSEStreamFrom benchmarks the SSE parsing
func BenchmarkOpenAIParseSSEStreamFrom(b *testing.B) {
	provider := openaiProvider{logger: slog.Default()}

	body := `data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-3.5-turbo","choices":[{"index":0,"delta":{"content":"Hello"}}]}
data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-3.5-turbo","choices":[{"index":0,"delta":{"content":" World"}}]}
data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-3.5-turbo","choices":[{"index":0,"delta":{},"finish_reason":"stop","usage":{"completion_tokens":5}}]}
`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		provider.ParseSSEStreamFrom([]byte(body), 0)
	}
}
