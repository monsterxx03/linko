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
		Type        string `json:"type"`
		Text        string `json:"text,omitempty"`
		Thinking    string `json:"thinking,omitempty"`
		PartialJSON string `json:"partial_json,omitempty"`
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
		Type     string `json:"type"`
		Text     string `json:"text,omitempty"`
		Thinking string `json:"thinking,omitempty"`
		ID       string `json:"id,omitempty"`   // for tool_use
		Name     string `json:"name,omitempty"` // for tool_use
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
	if hostname == "api.anthropic.com" && path == "/v1/messages" {
		return true
	}
	// compatible with Anthropic format
	if hostname == "api.minimaxi.com" && path == "/anthropic/v1/messages" {
		return true
	}
	if hostname == "api.deepseek.com" && path == "/anthropic/v1/messages" {
		return true
	}
	if hostname == "open.bigmodel.cn" && path == "/api/anthropic/v1/messages" {
		return true
	}
	return false
}

func (a anthropicProvider) ExtractConversationID(body []byte) string {
	var req AnthropicRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return ""
	}

	// 从 metadata.user_id 获取会话 ID
	if req.Metadata != nil && req.Metadata.UserID != "" {
		return fmt.Sprintf("anthropic-%s", req.Metadata.UserID)
	}

	// 如果没有 UserID，返回固定 ID
	return "anthropic-default"
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

	// Track tool information by content block index
	toolInfoByIndex := make(map[int]struct {
		toolID   string
		toolName string
	})

	// Helper function to merge deltas of the same type
	tryMergeDelta := func(newDelta TokenDelta) {
		if len(deltas) == 0 {
			deltas = append(deltas, newDelta)
			return
		}

		lastDelta := &deltas[len(deltas)-1]

		// Merge text deltas
		if newDelta.Text != "" && lastDelta.Text != "" &&
		   newDelta.Thinking == "" && lastDelta.Thinking == "" &&
		   newDelta.ToolData == "" && lastDelta.ToolData == "" &&
		   newDelta.ToolName == "" && lastDelta.ToolName == "" &&
		   newDelta.ToolID == "" && lastDelta.ToolID == "" &&
		   !newDelta.IsComplete && !lastDelta.IsComplete {
			// Merge consecutive text deltas
			// Check if new text is already a suffix of current text (avoid duplicates)
			currentText := lastDelta.Text
			newText := newDelta.Text
			if !strings.HasSuffix(currentText, newText) {
				lastDelta.Text = currentText + newText
			}
			return
		}

		// Merge thinking deltas
		if newDelta.Thinking != "" && lastDelta.Thinking != "" &&
		   newDelta.Text == "" && lastDelta.Text == "" &&
		   newDelta.ToolData == "" && lastDelta.ToolData == "" &&
		   newDelta.ToolName == "" && lastDelta.ToolName == "" &&
		   newDelta.ToolID == "" && lastDelta.ToolID == "" &&
		   !newDelta.IsComplete && !lastDelta.IsComplete {
			// Merge consecutive thinking deltas
			// Check if new thinking is already a suffix of current thinking (avoid duplicates)
			currentThinking := lastDelta.Thinking
			newThinking := newDelta.Thinking
			if !strings.HasSuffix(currentThinking, newThinking) {
				lastDelta.Thinking = currentThinking + newThinking
			}
			return
		}

		// Merge tool data deltas (must have same ToolID if specified)
		if newDelta.ToolData != "" && lastDelta.ToolData != "" &&
		   newDelta.Text == "" && lastDelta.Text == "" &&
		   newDelta.Thinking == "" && lastDelta.Thinking == "" &&
		   !newDelta.IsComplete && !lastDelta.IsComplete {
			// Check if ToolIDs match (both empty or same)
			// Allow newDelta.ToolID to be empty even if lastDelta.ToolID is set
			toolIDsMatch := (newDelta.ToolID == "" && lastDelta.ToolID == "") ||
			                (newDelta.ToolID != "" && lastDelta.ToolID != "" && newDelta.ToolID == lastDelta.ToolID) ||
			                (newDelta.ToolID == "" && lastDelta.ToolID != "")

			// Allow newDelta.ToolName to be empty even if lastDelta.ToolName is set
			toolNamesMatch := (newDelta.ToolName == "" && lastDelta.ToolName == "") ||
			                  (newDelta.ToolName != "" && lastDelta.ToolName != "" && newDelta.ToolName == lastDelta.ToolName) ||
			                  (newDelta.ToolName == "" && lastDelta.ToolName != "")

			if toolIDsMatch && toolNamesMatch {
				// Merge consecutive tool data deltas
				// Check if new tool data is already a suffix of current tool data (avoid duplicates)
				currentToolData := lastDelta.ToolData
				newToolData := newDelta.ToolData
				if !strings.HasSuffix(currentToolData, newToolData) {
					lastDelta.ToolData = currentToolData + newToolData
				}
				// Preserve ToolID/ToolName from last delta (in case new delta has empty values)
				if newDelta.ToolID == "" && lastDelta.ToolID != "" {
					// ToolID already set in lastDelta, keep it
				}
				if newDelta.ToolName == "" && lastDelta.ToolName != "" {
					// ToolName already set in lastDelta, keep it
				}
				return
			}
		}

		// Cannot merge, add as new delta
		deltas = append(deltas, newDelta)
	}

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
				tryMergeDelta(TokenDelta{
					Text: event.Delta.Text,
				})
			case "thinking_delta":
				tryMergeDelta(TokenDelta{
					Thinking: event.Delta.Thinking,
				})
			case "input_json_delta":
				// Tool call arguments are streaming in as partial JSON
				// This is part of a tool_use content block
				toolDelta := TokenDelta{
					ToolData: event.Delta.PartialJSON,
				}
				// Look up tool information for this index
				if toolInfo, exists := toolInfoByIndex[event.Index]; exists {
					toolDelta.ToolID = toolInfo.toolID
					toolDelta.ToolName = toolInfo.toolName
				}
				tryMergeDelta(toolDelta)
			default:
				a.logger.Debug("unhandled delta type", "delta_type", event.Delta.Type, "data", data)
			}
		case "content_block_start":
			// Content block started - may contain thinking block or tool_use
			switch event.ContentBlock.Type {
			case "thinking":
				a.logger.Debug("thinking block started", "index", event.Index)
			case "tool_use":
				// Store tool information for this index
				toolInfoByIndex[event.Index] = struct {
					toolID   string
					toolName string
				}{
					toolID:   event.ContentBlock.ID,
					toolName: event.ContentBlock.Name,
				}
				// Tool use block started - emit the tool name and ID
				tryMergeDelta(TokenDelta{
					ToolName: event.ContentBlock.Name,
					ToolID:   event.ContentBlock.ID,
				})
			}
		case "content_block_stop":
			// Content block ended - mark as complete for tool calls
			a.logger.Debug("content block stopped", "index", event.Index)
			tryMergeDelta(TokenDelta{
				ToolData:   "",
				IsComplete: false,
			})
		case "message_delta":
			tryMergeDelta(TokenDelta{
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
				tryMergeDelta(TokenDelta{
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
