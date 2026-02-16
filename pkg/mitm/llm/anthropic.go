package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

var compatibleHosts = map[string]bool{
	"api.minimaxi.com":       true,
	"api.minimax.io":         true,
	"api.deepseek.com":       true,
	"open.bigmodel.cn":       true,
	"api.z.ai":               true,
	"dashscope.aliyuncs.com": true,
	"api.moonshot.cn":        true,
	"api.longcat.chat":       true,
	"api.tbox.cn":            true,
	"api.xiaomimimo.com":     true,
}

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
		StopReason  string `json:"stop_reason,omitempty"`
	} `json:"delta,omitempty"`
	Message struct {
		ID         string             `json:"id"`
		Type       string             `json:"type"`
		Role       string             `json:"role"`
		Content    []AnthropicContent `json:"content"`
		StopReason string             `json:"stop_reason,omitempty"`
		Usage      AnthropicUsage     `json:"usage,omitempty"`
	} `json:"message,omitempty"`
	// Usage 用于 message_delta 事件的根级别 usage（不在 message 对象内）
	Usage AnthropicUsage `json:"usage,omitempty"`
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
	logger        *slog.Logger
	customMatches *ProviderMatcher
}

func (a anthropicProvider) Match(hostname, path string, body []byte) bool {
	// Anthropic official API
	if hostname == "api.anthropic.com" && path == "/v1/messages" {
		return true
	}

	// Compatible APIs: check if hostname in list and path contains "anthropic"
	if compatibleHosts[hostname] && strings.Contains(path, "anthropic") {
		return true
	}

	// Check custom matches
	if a.customMatches != nil {
		for _, match := range a.customMatches.CustomAnthropicMatches {
			if matchCustomPattern(hostname, path, match) {
				return true
			}
		}
	}

	return false
}

// matchCustomPattern checks if hostname/path matches a custom pattern (format: "hostname/path")
// extractConversationIDFromReq extracts conversation ID from a parsed AnthropicRequest
func (a anthropicProvider) extractConversationIDFromReq(req *AnthropicRequest) string {
	// 从 metadata.user_id 获取会话 ID
	if req.Metadata != nil && req.Metadata.UserID != "" {
		// 对 UserID 进行哈希处理，取前6位
		hash := sha256.Sum256([]byte(req.Metadata.UserID))
		hashStr := hex.EncodeToString(hash[:])
		if len(hashStr) > 6 {
			hashStr = hashStr[:6]
		}
		return fmt.Sprintf("anthropic-%s", hashStr)
	}

	// 如果没有 UserID，返回固定 ID
	return "anthropic-default"
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

// extractSystemPromptsFromReq extracts system prompts from a parsed AnthropicRequest
func (a anthropicProvider) extractSystemPromptsFromReq(req *AnthropicRequest) []string {
	if req.System == nil {
		return nil
	}

	var prompts []string
	switch s := req.System.(type) {
	case string:
		prompts = append(prompts, s)
	case []any:
		for _, item := range s {
			if itemMap, ok := item.(map[string]any); ok {
				if text, ok := itemMap["text"].(string); ok {
					prompts = append(prompts, text)
				}
			}
		}
	}

	return prompts
}

// extractToolsFromReq extracts tools from a parsed AnthropicRequest
func (a anthropicProvider) extractToolsFromReq(req *AnthropicRequest) []ToolDef {
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

// ParseFullRequest parses the request body once and returns all extracted info
func (a anthropicProvider) ParseFullRequest(body []byte) (*RequestInfo, error) {
	var req AnthropicRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("failed to parse Anthropic request: %w", err)
	}

	return &RequestInfo{
		ConversationID: a.extractConversationIDFromReq(&req),
		Model:          req.Model,
		Messages:       convertAnthropicMessages(req.Messages),
		SystemPrompts:  a.extractSystemPromptsFromReq(&req),
		Tools:          a.extractToolsFromReq(&req),
	}, nil
}

// ParseSSEStreamFrom parses SSE stream from a specific position for incremental processing
func (a anthropicProvider) ParseSSEStreamFrom(body []byte, startPos int) []TokenDelta {
	if startPos >= len(body) {
		a.logger.Warn("sse startPos > len(body)", "startPos", startPos)
		return nil
	}

	// Extract new lines from start position
	remaining := string(body[startPos:])
	lines := strings.Split(remaining, "\n")

	// Track tool information by content block index
	toolInfoByIndex := make(map[int]struct {
		toolID   string
		toolName string
	})

	// Track cumulative token usage
	var cumulativeUsage TokenUsage

	var deltas []TokenDelta

	// Helper function to merge deltas of the same type
	tryMergeDelta := func(newDelta TokenDelta) {
		// Apply cumulative usage if new delta has no usage info
		if newDelta.Usage == (TokenUsage{}) {
			newDelta.Usage = cumulativeUsage
		}

		// First delta always gets appended
		if len(deltas) == 0 {
			deltas = append(deltas, newDelta)
			return
		}

		last := &deltas[len(deltas)-1]

		// 1. Merge text content
		if newDelta.Text != "" {
			last.Text += newDelta.Text
			return
		}

		// 2. Merge thinking content
		if newDelta.Thinking != "" {
			last.Thinking += newDelta.Thinking
			return
		}

		// 3. Merge completion info (stop_reason, usage) - updates last delta in place
		if newDelta.IsComplete || newDelta.StopReason != "" {
			last.IsComplete = newDelta.IsComplete
			if newDelta.StopReason != "" {
				last.StopReason = newDelta.StopReason
			}
			if newDelta.Usage != (TokenUsage{}) {
				last.Usage = newDelta.Usage
			}
			return
		}

		// 4. Merge tool data
		if newDelta.ToolData != "" && !last.IsComplete {
			sameTool := (newDelta.ToolID == "" || newDelta.ToolID == last.ToolID) &&
				(newDelta.ToolName == "" || newDelta.ToolName == last.ToolName)
			if sameTool && !strings.HasSuffix(last.ToolData, newDelta.ToolData) {
				last.ToolData += newDelta.ToolData
				return
			}
		}

		// No merge possible, append new delta
		deltas = append(deltas, newDelta)
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "event: ") {
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			a.logger.Warn("skiping sse line", "line", line)
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
				toolDelta := TokenDelta{
					ToolData: event.Delta.PartialJSON,
				}
				if toolInfo, exists := toolInfoByIndex[event.Index]; exists {
					toolDelta.ToolID = toolInfo.toolID
					toolDelta.ToolName = toolInfo.toolName
				}
				tryMergeDelta(toolDelta)
			default:
				a.logger.Debug("unhandled delta type", "delta_type", event.Delta.Type, "data", data)
			}
		case "content_block_start":
			switch event.ContentBlock.Type {
			case "thinking":
				a.logger.Debug("thinking block started", "index", event.Index)
			case "tool_use":
				toolInfoByIndex[event.Index] = struct {
					toolID   string
					toolName string
				}{
					toolID:   event.ContentBlock.ID,
					toolName: event.ContentBlock.Name,
				}
				tryMergeDelta(TokenDelta{
					ToolName: event.ContentBlock.Name,
					ToolID:   event.ContentBlock.ID,
				})
			}
		case "content_block_stop":
			a.logger.Debug("content block stopped", "index", event.Index)
			tryMergeDelta(TokenDelta{
				ToolData:   "",
				IsComplete: false,
			})
		case "message_delta":
			// 使用根级别的 usage 字段（Anthropic API 在 message_delta 中返回的 usage 在根级别）
			if event.Usage.OutputTokens > 0 {
				cumulativeUsage.OutputTokens = event.Usage.OutputTokens
			}
			if event.Usage.InputTokens > 0 {
				cumulativeUsage.InputTokens = event.Usage.InputTokens
			}
			tryMergeDelta(TokenDelta{
				Text:       "",
				IsComplete: true,
				StopReason: event.Delta.StopReason,
			})
		case "message_start":
			a.logger.Debug("message started", "role", event.Message.Role)
			// Update cumulative input tokens
			if event.Message.Usage.InputTokens > 0 {
				cumulativeUsage.InputTokens = event.Message.Usage.InputTokens
			}
		case "message_stop":
			a.logger.Debug("message stopped")
		case "error":
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
