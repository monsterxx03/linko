package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

// Anthropic API types
type AnthropicRequest struct {
	Model         string             `json:"model"`
	MaxTokens     int                `json:"max_tokens"`
	Messages      []AnthropicMessage `json:"messages"`
	System        any                `json:"system,omitempty"` // Can be string or array of strings
	StopSequences []string           `json:"stop_sequences,omitempty"`
	Temperature   float64            `json:"temperature,omitempty"`
	TopK          int                `json:"top_k,omitempty"`
	TopP          float64            `json:"top_p,omitempty"`
	Metadata      *AnthropicMetadata `json:"metadata,omitempty"`
	Tools         []AnthropicTool    `json:"tools,omitempty"`
}

type AnthropicMetadata struct {
	UserID string `json:"user_id,omitempty"`
}

type AnthropicTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

type AnthropicMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string or []AnthropicContent
}

type AnthropicContent struct {
	Type     string       `json:"type"`
	Text     string       `json:"text,omitempty"`
	Thinking string       `json:"thinking,omitempty"`
	Source   *ImageSource `json:"source,omitempty"`
}

type ImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type AnthropicResponse struct {
	ID           string             `json:"id"`
	Type         string             `json:"type"`
	Role         string             `json:"role"`
	Content      []AnthropicContent `json:"content"`
	Model        string             `json:"model"`
	StopReason   string             `json:"stop_reason"`
	StopSequence string             `json:"stop_sequence,omitempty"`
	Usage        AnthropicUsage     `json:"usage"`
	Error        struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type AnthropicStreamEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index,omitempty"`
	Delta struct {
		Type         string `json:"type"`
		Text         string `json:"text,omitempty"`
		Thinking     string `json:"thinking,omitempty"`
		PartialJSON  string `json:"partial_json,omitempty"`
	} `json:"delta,omitempty"`
	Message struct {
		ID         string             `json:"id"`
		Type       string             `json:"type"`
		Role       string             `json:"role"`
		Content    []AnthropicContent `json:"content"`
		StopReason string             `json:"stop_reason,omitempty"`
		Usage      AnthropicUsage     `json:"usage,omitempty"`
	} `json:"message,omitempty"`
	ContentBlock struct {
		Type     string       `json:"type"`
		Text     string       `json:"text,omitempty"`
		Thinking string       `json:"thinking,omitempty"`
	} `json:"content_block,omitempty"`
}

// anthropicProvider implements Provider for Anthropic Claude API
type anthropicProvider struct {
	logger *slog.Logger
}

// generateAnthropicMessagesHash generates a hash from all messages in the request
// Used for conversation grouping - same conversation context produces same hash
func generateAnthropicMessagesHash(messages []AnthropicMessage) string {
	var data string
	for _, m := range messages {
		// Include role to distinguish user/assistant/system messages
		data += m.Role + ":"
		switch c := m.Content.(type) {
		case string:
			data += c
		case []AnthropicContent:
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
	var req AnthropicRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return ""
	}

	// 优先从 metadata.user_id 获取会话 ID
	if req.Metadata != nil && req.Metadata.UserID != "" {
		return fmt.Sprintf("anthropic-%s", req.Metadata.UserID)
	}

	// 回退：提取所有消息的文本内容生成 hash
	// 这样同一轮对话（相同的历史消息）会产生相同的 ID
	hash := generateAnthropicMessagesHash(req.Messages)

	return fmt.Sprintf("anthropic-%s", hash)
}

func (a anthropicProvider) ParseRequest(body []byte) ([]LLMMessage, error) {
	var req AnthropicRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("failed to parse Anthropic request: %w", err)
	}
	return convertAnthropicMessages(req.Messages), nil
}

func (a anthropicProvider) ParseResponse(path string, body []byte) (*LLMResponse, error) {
	// Handle count_tokens endpoint
	if strings.Contains(path, "/v1/messages/count_tokens") {
		var countResp struct {
			InputTokens int `json:"input_tokens"`
		}
		if err := json.Unmarshal(body, &countResp); err != nil {
			return nil, fmt.Errorf("failed to parse count_tokens response: %w", err)
		}
		return &LLMResponse{
			Content: fmt.Sprintf("input_tokens: %d", countResp.InputTokens),
			Usage: TokenUsage{
				InputTokens: countResp.InputTokens,
			},
		}, nil
	}

	var resp AnthropicResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse Anthropic response: %w", err)
	}

	// Check for API error response
	if resp.Type == "error" {
		return &LLMResponse{
			Content:    resp.Error.Message,
			StopReason: "error",
			Error:      &APIError{Type: resp.Error.Type, Message: resp.Error.Message},
		}, nil
	}

	content := ""
	for _, c := range resp.Content {
		if c.Type == "text" {
			content = c.Text
			break
		}
	}
	if content == "" {
		fmt.Println(string(body))
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

		var event AnthropicStreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			a.logger.Warn("failed to parse Anthropic SSE event", "error", err, "data", data)
			continue
		}

		switch event.Type {
		case "content_block_delta":
			switch event.Delta.Type {
			case "text_delta":
				deltas = append(deltas, TokenDelta{
					Text: event.Delta.Text,
				})
			case "thinking_delta":
				deltas = append(deltas, TokenDelta{
					Thinking: event.Delta.Thinking,
				})
			case "input_json_delta":
				// Ignore JSON structure deltas for now
				a.logger.Debug("ignoring input_json_delta", "data", data)
			default:
				a.logger.Debug("unhandled delta type", "delta_type", event.Delta.Type, "data", data)
			}
		case "content_block_start":
			// Content block started - may contain thinking block
			if event.ContentBlock.Type == "thinking" {
				a.logger.Debug("thinking block started", "index", event.Index)
			}
		case "content_block_stop":
			// Content block ended
			a.logger.Debug("content block stopped", "index", event.Index)
		case "message_delta":
			deltas = append(deltas, TokenDelta{
				Text:       "",
				IsComplete: true,
				StopReason: event.Message.StopReason,
			})
		case "message_start":
			// Message started - contains role info, not useful for deltas
			a.logger.Debug("message started", "role", event.Message.Role)
		case "message_stop":
			// Message ended - final event, handled by message_delta
			a.logger.Debug("message stopped")
		case "error":
			// Error event - parse error details
			var errData struct {
				Error struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if json.Unmarshal([]byte(data), &errData) == nil {
				a.logger.Warn("Anthropic API error", "type", errData.Error.Type, "message", errData.Error.Message)
				deltas = append(deltas, TokenDelta{
					Text:       fmt.Sprintf("[Error: %s] %s", errData.Error.Type, errData.Error.Message),
					IsComplete: true,
					StopReason: "error",
				})
			}
		default:
			a.logger.Debug("unhandled Anthropic event", "type", event.Type, "data", data)
		}
	}

	return deltas
}

func (a anthropicProvider) ExtractSystemPrompt(body []byte) []string {
	var req AnthropicRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil
	}

	if req.System == nil {
		return nil
	}

	var prompts []string
	switch s := req.System.(type) {
	case string:
		prompts = append(prompts, s)
	case []interface{}:
		for _, item := range s {
			if itemMap, ok := item.(map[string]interface{}); ok {
				if text, ok := itemMap["text"].(string); ok {
					prompts = append(prompts, text)
				}
			}
		}
	}

	return prompts
}

func (a anthropicProvider) ExtractTools(body []byte) []ToolDef {
	var req AnthropicRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil
	}

	if len(req.Tools) == 0 {
		return nil
	}

	var tools []ToolDef
	for _, t := range req.Tools {
		tools = append(tools, ToolDef{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}
	return tools
}
