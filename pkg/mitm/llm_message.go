package mitm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// LLMMessage represents a message in an LLM conversation
type LLMMessage struct {
	Role      string     `json:"role"`                 // "user", "assistant", "system", "tool"
	Content   string     `json:"content"`              // message content
	Name      string     `json:"name,omitempty"`       // optional name for the message
	ToolCalls []ToolCall `json:"tool_calls,omitempty"` // tool calls (for assistant messages)
}

// ToolCall represents a tool call in an LLM message
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall represents a function call within a tool call
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// TokenUsage represents token usage statistics
type TokenUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// LLMResponse represents a response from an LLM
type LLMResponse struct {
	Content    string     `json:"content"`
	StopReason string     `json:"stop_reason"`
	Usage      TokenUsage `json:"usage"`
}

// TokenDelta represents incremental token updates for streaming
type TokenDelta struct {
	Text       string `json:"text"`
	IsComplete bool   `json:"is_complete"`
	StopReason string `json:"stop_reason,omitempty"`
}

// LLMMessageEvent is published when a new LLM message is detected
type LLMMessageEvent struct {
	ID             string     `json:"id"`
	Timestamp      time.Time  `json:"timestamp"`
	ConversationID string     `json:"conversation_id"`
	RequestID      string     `json:"request_id"` // HTTP request ID for correlation
	Message        LLMMessage `json:"message"`
	TokenCount     int        `json:"token_count,omitempty"`  // token count for this message
	TotalTokens    int        `json:"total_tokens,omitempty"` // total tokens in conversation
	Model          string     `json:"model,omitempty"`        // model name
}

// LLMTokenEvent is published during streaming responses
type LLMTokenEvent struct {
	ID             string    `json:"id"`
	Timestamp      time.Time `json:"timestamp"`
	ConversationID string    `json:"conversation_id"`
	RequestID      string    `json:"request_id"`
	Delta          string    `json:"delta"`       // new token content
	IsComplete     bool      `json:"is_complete"` // true when streaming is done
	StopReason     string    `json:"stop_reason,omitempty"`
}

// ConversationUpdateEvent is published when conversation status changes
type ConversationUpdateEvent struct {
	ID             string    `json:"id"`
	Timestamp      time.Time `json:"timestamp"`
	ConversationID string    `json:"conversation_id"`
	Status         string    `json:"status"` // "streaming", "complete", "error"
	MessageCount   int       `json:"message_count"`
	TotalTokens    int       `json:"total_tokens"`
	Duration       int64     `json:"duration_ms"` // duration in milliseconds
	Model          string    `json:"model,omitempty"`
}

// Provider interface defines the contract for LLM API parsers
type Provider interface {
	Match(hostname, path string, body []byte) bool
	ExtractConversationID(body []byte) string
	ParseRequest(body []byte) ([]LLMMessage, error)
	ParseResponse(body []byte) (*LLMResponse, error)
	ParseSSEStream(body []byte) []TokenDelta
}

// Anthropic API types
type anthropicRequest struct {
	Model         string         `json:"model"`
	MaxTokens     int            `json:"max_tokens"`
	Messages      []anthropicMsg `json:"messages"`
	System        any            `json:"system,omitempty"` // Can be string or array of strings
	StopSequences []string       `json:"stop_sequences,omitempty"`
	Temperature   float64        `json:"temperature,omitempty"`
	TopK          int            `json:"top_k,omitempty"`
	TopP          float64        `json:"top_p,omitempty"`
}

type anthropicMsg struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string or []anthropicContent
}

type anthropicContent struct {
	Type   string       `json:"type"`
	Text   string       `json:"text,omitempty"`
	Source *ImageSource `json:"source,omitempty"`
}

type ImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type anthropicResponse struct {
	ID           string             `json:"id"`
	Type         string             `json:"type"`
	Role         string             `json:"role"`
	Content      []anthropicContent `json:"content"`
	Model        string             `json:"model"`
	StopReason   string             `json:"stop_reason"`
	StopSequence string             `json:"stop_sequence,omitempty"`
	Usage        anthropicUsage     `json:"usage"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicStreamEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index,omitempty"`
	Delta struct {
		Type string `json:"type"`
		Text string `json:"text,omitempty"`
	} `json:"delta,omitempty"`
	Message struct {
		ID         string             `json:"id"`
		Type       string             `json:"type"`
		Role       string             `json:"role"`
		Content    []anthropicContent `json:"content"`
		StopReason string             `json:"stop_reason,omitempty"`
		Usage      anthropicUsage     `json:"usage,omitempty"`
	} `json:"message,omitempty"`
}

// OpenAI API types (for future extension)
type openaiRequest struct {
	Model       string      `json:"model"`
	MaxTokens   int         `json:"max_tokens,omitempty"`
	Messages    []openaiMsg `json:"messages"`
	System      string      `json:"system,omitempty"`
	Stop        interface{} `json:"stop,omitempty"`
	Temperature float64     `json:"temperature,omitempty"`
	TopP        float64     `json:"top_p,omitempty"`
}

type openaiMsg struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string or array of content parts
	Name    string      `json:"name,omitempty"`
}

type openaiContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL *struct {
		URL    string `json:"url"`
		Detail string `json:"detail,omitempty"`
	} `json:"image_url,omitempty"`
}

type openaiResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []openaiChoice `json:"choices"`
	Usage   openaiUsage    `json:"usage"`
}

type openaiChoice struct {
	Index        int         `json:"index"`
	Message      openaiMsg   `json:"message"`
	FinishReason string      `json:"finish_reason"`
	LogProbs     interface{} `json:"logprobs,omitempty"`
}

type openaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type openaiStreamChunk struct {
	ID      string              `json:"id"`
	Object  string              `json:"object"`
	Created int64               `json:"created"`
	Model   string              `json:"model"`
	Choices []openaiChunkChoice `json:"choices"`
}

type openaiChunkChoice struct {
	Index        int         `json:"index"`
	Delta        openaiDelta `json:"delta"`
	FinishReason string      `json:"finish_reason,omitempty"`
	LogProbs     interface{} `json:"logprobs,omitempty"`
}

type openaiDelta struct {
	Role      string     `json:"role,omitempty"`
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// anthropicProvider implements Provider for Anthropic Claude API
type anthropicProvider struct{}

// generateAnthropicMessagesHash generates a hash from all messages in the request
// Used for conversation grouping - same conversation context produces same hash
func generateAnthropicMessagesHash(messages []anthropicMsg) string {
	var data string
	for _, m := range messages {
		// Include role to distinguish user/assistant/system messages
		data += m.Role + ":"
		switch c := m.Content.(type) {
		case string:
			data += c
		case []anthropicContent:
			for _, item := range c {
				data += item.Text
			}
		}
		data += "|"
	}
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:8])
}

func (a anthropicProvider) Match(hostname, path string, body []byte) bool {
	// Anthropic official API
	if strings.Contains(hostname, "api.anthropic.com") && strings.HasPrefix(path, "/v1/messages") {
		return true
	}
	// MiniMax API (compatible with Anthropic format)
	if strings.Contains(hostname, "api.minimaxi.com") && strings.HasPrefix(path, "/anthropic/v1/messages") {
		return true
	}
	return false
}

func (a anthropicProvider) ExtractConversationID(body []byte) string {
	var req anthropicRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return ""
	}

	// 提取所有消息的文本内容生成 hash
	// 这样同一轮对话（相同的历史消息）会产生相同的 ID
	hash := generateAnthropicMessagesHash(req.Messages)

	return fmt.Sprintf("anthropic-%s", hash)
}

func (a anthropicProvider) ParseRequest(body []byte) ([]LLMMessage, error) {
	var req anthropicRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("failed to parse Anthropic request: %w", err)
	}
	return convertAnthropicMessages(req.Messages), nil
}

func (a anthropicProvider) ParseResponse(body []byte) (*LLMResponse, error) {
	var resp anthropicResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse Anthropic response: %w", err)
	}

	content := ""
	for _, c := range resp.Content {
		if c.Type == "text" {
			content = c.Text
			break
		}
	}

	return &LLMResponse{
		Content:    content,
		StopReason: resp.StopReason,
		Usage: TokenUsage{
			InputTokens:  resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
		},
	}, nil
}

func (a anthropicProvider) ParseSSEStream(body []byte) []TokenDelta {
	var deltas []TokenDelta
	lines := strings.Split(string(body), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "" || data == "[DONE]" {
			continue
		}

		var event anthropicStreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		if event.Type == "content_block_delta" && event.Delta.Type == "text_delta" {
			deltas = append(deltas, TokenDelta{
				Text: event.Delta.Text,
			})
		} else if event.Type == "message_delta" {
			deltas = append(deltas, TokenDelta{
				Text:       "",
				IsComplete: true,
				StopReason: event.Message.StopReason,
			})
		}
	}

	return deltas
}

// openaiProvider implements Provider for OpenAI Chat API
type openaiProvider struct{}

func (o openaiProvider) Match(hostname, path string, body []byte) bool {
	return (strings.Contains(hostname, "api.openai.com") ||
		strings.Contains(hostname, "openai.azure.com") ||
		strings.Contains(path, "/chat/completions"))
}

func (o openaiProvider) ExtractConversationID(body []byte) string {
	var req openaiRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return ""
	}
	hash := generateOpenAIConversationHash(req.Messages)
	return fmt.Sprintf("openai-%s", hash)
}

func (o openaiProvider) ParseRequest(body []byte) ([]LLMMessage, error) {
	var req openaiRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAI request: %w", err)
	}
	return convertOpenAIMessages(req.Messages), nil
}

func (o openaiProvider) ParseResponse(body []byte) (*LLMResponse, error) {
	var resp openaiResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAI response: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	// Extract content with type assertion
	content := ""
	switch c := resp.Choices[0].Message.Content.(type) {
	case string:
		content = c
	case []interface{}:
		for _, part := range c {
			if partMap, ok := part.(map[string]interface{}); ok {
				if text, ok := partMap["text"].(string); ok {
					content += text
				}
			}
		}
	}

	return &LLMResponse{
		Content:    content,
		StopReason: resp.Choices[0].FinishReason,
		Usage: TokenUsage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		},
	}, nil
}

func (o openaiProvider) ParseSSEStream(body []byte) []TokenDelta {
	var deltas []TokenDelta
	lines := strings.Split(string(body), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			deltas = append(deltas, TokenDelta{
				Text:       "",
				IsComplete: true,
			})
			continue
		}

		var chunk openaiStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				deltas = append(deltas, TokenDelta{
					Text: choice.Delta.Content,
				})
			}
			if choice.FinishReason != "" {
				deltas = append(deltas, TokenDelta{
					Text:       "",
					IsComplete: true,
					StopReason: choice.FinishReason,
				})
			}
		}
	}

	return deltas
}

// Helper functions

func generateConversationHash(messages []anthropicMsg) string {
	// Generate a simple hash from message content
	data := ""
	for _, m := range messages {
		switch c := m.Content.(type) {
		case string:
			data += c
		case []anthropicContent:
			for _, item := range c {
				data += item.Text
			}
		}
	}
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:8])
}

func generateOpenAIConversationHash(messages []openaiMsg) string {
	data := ""
	for _, m := range messages {
		// Include role to distinguish user/assistant/system messages
		data += m.Role + ":"
		if content, ok := m.Content.(string); ok {
			data += content
		}
		data += "|"
	}
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:8])
}

func convertAnthropicMessages(messages []anthropicMsg) []LLMMessage {
	var result []LLMMessage
	for _, m := range messages {
		content := ""
		var toolCalls []ToolCall

		switch c := m.Content.(type) {
		case string:
			content = c
		case []anthropicContent:
			for _, item := range c {
				if item.Type == "text" {
					content += item.Text
				}
			}
		case []interface{}:
			for _, item := range c {
				if itemMap, ok := item.(map[string]interface{}); ok {
					if itemType, ok := itemMap["type"].(string); ok && itemType == "text" {
						if text, ok := itemMap["text"].(string); ok {
							content += text
						}
					}
				}
			}
		}

		result = append(result, LLMMessage{
			Role:      m.Role,
			Content:   content,
			ToolCalls: toolCalls,
		})
	}
	return result
}

func convertOpenAIMessages(messages []openaiMsg) []LLMMessage {
	var result []LLMMessage
	for _, m := range messages {
		content := ""
		switch c := m.Content.(type) {
		case string:
			content = c
		case []interface{}:
			for _, part := range c {
				if partMap, ok := part.(map[string]interface{}); ok {
					if text, ok := partMap["text"].(string); ok {
						content += text
					}
				}
			}
		}
		result = append(result, LLMMessage{
			Role:    m.Role,
			Content: content,
			Name:    m.Name,
		})
	}
	return result
}

// FindProvider returns the appropriate provider for the given request
func FindProvider(hostname, path string, body []byte) Provider {
	providers := []Provider{
		anthropicProvider{},
		openaiProvider{},
	}

	for _, p := range providers {
		if p.Match(hostname, path, body) {
			return p
		}
	}
	return nil
}
