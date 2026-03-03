package llm

import (
	"encoding/json"
	"log/slog"
	"testing"
)

func TestGeminiMatch(t *testing.T) {
	provider := geminiProvider{}

	tests := []struct {
		name     string
		hostname string
		path     string
		body     []byte
		want     bool
	}{
		{
			name:     "Google Generative Language API",
			hostname: "generativelanguage.googleapis.com",
			path:     "/v1beta/models/gemini-1.5-flash:generateContent",
			body:     nil,
			want:     true,
		},
		{
			name:     "Non-matching hostname",
			hostname: "api.openai.com",
			path:     "/v1/chat/completions",
			body:     nil,
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

func TestGeminiParseResponse(t *testing.T) {
	provider := geminiProvider{logger: testLogger()}

	response := GeminiResponse{
		Candidates: []GeminiCandidate{
			{
				Content: GeminiContent{
					Parts: []GeminiPart{
						{Text: "Hello, world!"},
					},
				},
				FinishReason: "STOP",
				Index:        0,
			},
		},
		UsageMetadata: &GeminiUsageMetadata{
			PromptTokenCount:     10,
			CandidatesTokenCount: 5,
			TotalTokenCount:      15,
		},
	}

	body, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("failed to marshal response: %v", err)
	}

	result, err := provider.ParseResponse("/v1/models/gemini-1.5-flash:generateContent", body)
	if err != nil {
		t.Fatalf("ParseResponse() error = %v", err)
	}

	if result.Content != "Hello, world!" {
		t.Errorf("Content = %v, want 'Hello, world!'", result.Content)
	}

	if result.StopReason != "stop" {
		t.Errorf("StopReason = %v, want 'stop'", result.StopReason)
	}

	if result.Usage.InputTokens != 10 {
		t.Errorf("InputTokens = %v, want 10", result.Usage.InputTokens)
	}

	if result.Usage.OutputTokens != 5 {
		t.Errorf("OutputTokens = %v, want 5", result.Usage.OutputTokens)
	}
}

func TestGeminiParseFullRequest(t *testing.T) {
	provider := geminiProvider{logger: testLogger()}

	request := GeminiRequest{
		Model: "gemini-1.5-flash",
		Contents: []GeminiContent{
			{
				Role: "user",
				Parts: []GeminiPart{
					{Text: "Hello"},
				},
			},
		},
		SystemInstruction: &GeminiContent{
			Parts: []GeminiPart{
				{Text: "You are a helpful assistant."},
			},
		},
	}

	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	info, err := provider.ParseFullRequest("", nil, body)
	if err != nil {
		t.Fatalf("ParseFullRequest() error = %v", err)
	}

	if info.Model != "gemini-1.5-flash" {
		t.Errorf("Model = %v, want 'gemini-1.5-flash'", info.Model)
	}

	if len(info.Messages) != 1 {
		t.Errorf("Messages length = %v, want 1", len(info.Messages))
	}

	if info.Messages[0].Role != "user" {
		t.Errorf("Message role = %v, want 'user'", info.Messages[0].Role)
	}

	if len(info.SystemPrompts) != 1 {
		t.Errorf("SystemPrompts length = %v, want 1", len(info.SystemPrompts))
	}

	if info.SystemPrompts[0] != "You are a helpful assistant." {
		t.Errorf("SystemPrompt = %v, want 'You are a helpful assistant.'", info.SystemPrompts[0])
	}
}

func TestGeminiParseSSEStream(t *testing.T) {
	provider := geminiProvider{logger: testLogger()}

	// Simulate SSE stream data
	sseData := `data: {"candidates": [{"content": {"parts": [{"text": "Hello"}],"role": "model"},"finishReason": "STOP","index": 0}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}
`

	deltas := provider.ParseSSEStreamFrom([]byte(sseData), 0)
	if len(deltas) == 0 {
		t.Fatal("expected deltas, got none")
	}

	// First delta should be text
	if deltas[0].Text != "Hello" {
		t.Errorf("First delta text = %v, want 'Hello'", deltas[0].Text)
	}

	// Last delta should be complete
	lastDelta := deltas[len(deltas)-1]
	if !lastDelta.IsComplete {
		t.Error("Expected last delta to be complete")
	}

	if lastDelta.StopReason != "stop" {
		t.Errorf("StopReason = %v, want 'stop'", lastDelta.StopReason)
	}
}

func testLogger() *slog.Logger {
	return slog.Default()
}
