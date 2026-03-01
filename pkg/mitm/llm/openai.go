package llm

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

// OpenAI API types
type OpenAIRequest struct {
	Model       string           `json:"model"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Messages    []OpenAIMessage  `json:"messages"`
	System      string           `json:"system,omitempty"`
	Stop        any              `json:"stop,omitempty"`
	Temperature float64          `json:"temperature,omitempty"`
	TopP        float64          `json:"top_p,omitempty"`
	Tools       []OpenAITool     `json:"tools,omitempty"`
}

// OpenAITool represents a tool definition in OpenAI API format
type OpenAITool struct {
	Type     string         `json:"type"`
	Function OpenAIFunction `json:"function"`
}

// OpenAIFunction represents the function definition within a tool
type OpenAIFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type OpenAIMessage struct {
	Role             string      `json:"role"`
	Content          interface{} `json:"content"` // string or array of content parts
	Name             string      `json:"name,omitempty"`
	ToolCalls        []ToolCall  `json:"tool_calls,omitempty"`
	ReasoningContent string      `json:"reasoning_content,omitempty"` // o1 model's reasoning
}

type OpenAIContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL *struct {
		URL    string `json:"url"`
		Detail string `json:"detail,omitempty"`
	} `json:"image_url,omitempty"`
}

type OpenAIResponse struct {
	ID                string         `json:"id"`
	Object            string         `json:"object"`
	Created           int64          `json:"created"`
	Model             string         `json:"model"`
	Choices           []OpenAIChoice `json:"choices"`
	Usage             OpenAIUsage    `json:"usage"`
	ReasoningContent  string         `json:"reasoning_content,omitempty"` // o1 model's reasoning
	SystemFingerprint string         `json:"system_fingerprint,omitempty"`
}

type OpenAIChoice struct {
	Index            int           `json:"index"`
	Message          OpenAIMessage `json:"message"`
	FinishReason     string        `json:"finish_reason"`
	LogProbs         interface{}   `json:"logprobs,omitempty"`
	ReasoningContent string        `json:"reasoning_content,omitempty"` // o1 model's reasoning in choice
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type OpenAIStreamChunk struct {
	ID                string              `json:"id"`
	Object            string              `json:"object"`
	Created           int64               `json:"created"`
	Model             string              `json:"model"`
	Choices           []OpenAIChunkChoice `json:"choices"`
	Usage             OpenAIUsage         `json:"usage,omitempty"` // for streaming final usage
	SystemFingerprint string              `json:"system_fingerprint,omitempty"`
}

type OpenAIChunkChoice struct {
	Index            int         `json:"index"`
	Delta            OpenAIDelta `json:"delta"`
	FinishReason     string      `json:"finish_reason,omitempty"`
	LogProbs         any         `json:"logprobs,omitempty"`
	ReasoningContent string      `json:"reasoning_content,omitempty"` // for non-streaming o1 responses
	Usage            OpenAIUsage `json:"usage,omitempty"`             // for streaming final usage
}

type OpenAIDelta struct {
	Role             string     `json:"role,omitempty"`
	Content          string     `json:"content,omitempty"`
	ReasoningContent string     `json:"reasoning_content,omitempty"` // o1 model's reasoning delta
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
}

// openaiProvider implements Provider for OpenAI Chat API
type openaiProvider struct {
	logger        *slog.Logger
	customMatches *ProviderMatcher
}

func (o openaiProvider) Match(hostname, path string, body []byte) bool {
	if strings.Contains(hostname, "api.openai.com") ||
		strings.Contains(hostname, "openai.azure.com") ||
		strings.Contains(path, "/chat/completions") {
		return true
	}

	// Check custom matches
	if o.customMatches != nil {
		for _, match := range o.customMatches.CustomOpenAIMatches {
			if matchCustomPattern(hostname, path, match) {
				return true
			}
		}
	}

	return false
}

func (o openaiProvider) ParseResponse(path string, body []byte) (*LLMResponse, error) {
	var resp OpenAIResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAI response: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	// Extract content with type assertion
	content := ""
	reasoningContent := ""

	// Check reasoning_content at response level (for o1 model)
	if resp.ReasoningContent != "" {
		reasoningContent = resp.ReasoningContent
	}

	// Check reasoning_content at choice level (for o1 model)
	if resp.Choices[0].ReasoningContent != "" {
		reasoningContent = resp.Choices[0].ReasoningContent
	}

	// Extract content from message
	switch c := resp.Choices[0].Message.Content.(type) {
	case string:
		content = c
	case []any:
		for _, part := range c {
			if partMap, ok := part.(map[string]any); ok {
				if text, ok := partMap["text"].(string); ok {
					content += text
				}
			}
		}
	}

	// Check message level reasoning_content (for o1 model)
	if resp.Choices[0].Message.ReasoningContent != "" {
		reasoningContent = resp.Choices[0].Message.ReasoningContent
	}

	// Include reasoning content in the content if present
	if reasoningContent != "" {
		content = "[Reasoning]\n" + reasoningContent + "\n[/Reasoning]\n" + content
	}

	return &LLMResponse{
		Content:    content,
		StopReason: resp.Choices[0].FinishReason,
		Usage: TokenUsage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		},
		ToolCalls: resp.Choices[0].Message.ToolCalls,
	}, nil
}

// extractSystemPromptsFromReq extracts system prompts from a parsed OpenAIRequest
// It extracts from both the top-level "system" field and messages with role="system"
func (o openaiProvider) extractSystemPromptsFromReq(req *OpenAIRequest) []string {
	var systemPrompts []string

	// Extract from top-level system field
	if req.System != "" {
		systemPrompts = append(systemPrompts, req.System)
	}

	// Extract from messages with role="system"
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			if content, ok := msg.Content.(string); ok && content != "" {
				systemPrompts = append(systemPrompts, content)
			}
		}
	}

	if len(systemPrompts) == 0 {
		return nil
	}

	return systemPrompts
}

// ParseFullRequest parses the request body once and returns all extracted info
func (o openaiProvider) ParseFullRequest(body []byte) (*RequestInfo, error) {
	var req OpenAIRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAI request: %w", err)
	}

	return &RequestInfo{
		ConversationID: "openai-default",
		Model:          req.Model,
		Messages:       convertOpenAIMessages(req.Messages),
		SystemPrompts:  o.extractSystemPromptsFromReq(&req),
		Tools:          o.extractToolsFromReq(&req),
	}, nil
}

// extractToolsFromReq extracts tools from a parsed OpenAIRequest
func (o openaiProvider) extractToolsFromReq(req *OpenAIRequest) []ToolDef {
	if len(req.Tools) == 0 {
		return nil
	}

	var tools []ToolDef
	for _, t := range req.Tools {
		// OpenAI tools have nested "function" object
		tools = append(tools, ToolDef{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: t.Function.Parameters,
		})
	}
	return tools
}

// ParseSSEStreamFrom parses SSE stream from a specific position for incremental processing
func (o openaiProvider) ParseSSEStreamFrom(body []byte, startPos int) []TokenDelta {
	if startPos >= len(body) {
		return nil
	}

	remaining := string(body[startPos:])
	lines := strings.Split(remaining, "\n")

	// Track cumulative token usage
	var cumulativeUsage TokenUsage

	// Track tool call info by index
	toolInfoByIndex := make(map[int]struct {
		toolID   string
		toolName string
	})

	var deltas []TokenDelta

	// Helper function to merge deltas of the same type (similar to anthropic.go)
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

		// 2. Merge reasoning content (for o1 model)
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
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			tryMergeDelta(TokenDelta{
				Text:       "",
				IsComplete: true,
			})
			continue
		}

		var chunk OpenAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			o.logger.Warn("failed to parse OpenAI SSE event", "error", err, "data", data)
			continue
		}

		// Update cumulative usage if present
		if chunk.Usage.CompletionTokens > 0 || chunk.Usage.PromptTokens > 0 {
			cumulativeUsage.InputTokens = chunk.Usage.PromptTokens
			cumulativeUsage.OutputTokens = chunk.Usage.CompletionTokens
		}

		for _, choice := range chunk.Choices {
			// Check for usage in choice (some API versions include it here)
			if choice.Usage.CompletionTokens > 0 || choice.Usage.PromptTokens > 0 {
				cumulativeUsage.InputTokens = choice.Usage.PromptTokens
				cumulativeUsage.OutputTokens = choice.Usage.CompletionTokens
			}

			// Handle reasoning_content (for o1 model streaming)
			if choice.Delta.ReasoningContent != "" {
				tryMergeDelta(TokenDelta{
					Thinking: choice.Delta.ReasoningContent,
				})
			}

			// Handle content
			if choice.Delta.Content != "" {
				tryMergeDelta(TokenDelta{
					Text: choice.Delta.Content,
				})
			}

			// Handle tool calls
			if len(choice.Delta.ToolCalls) > 0 {
				for i, toolCall := range choice.Delta.ToolCalls {
					if toolCall.ID != "" && toolCall.Function.Name != "" {
						// Store tool info for later use
						toolInfoByIndex[i] = struct {
							toolID   string
							toolName string
						}{
							toolID:   toolCall.ID,
							toolName: toolCall.Function.Name,
						}
						tryMergeDelta(TokenDelta{
							ToolName: toolCall.Function.Name,
							ToolID:   toolCall.ID,
						})
					}
					if toolCall.Function.Arguments != "" {
						var toolID string
						if info, exists := toolInfoByIndex[i]; exists {
							toolID = info.toolID
						}
						tryMergeDelta(TokenDelta{
							ToolData: toolCall.Function.Arguments,
							ToolID:   toolID,
						})
					}
				}
			}

			// Handle completion
			if choice.FinishReason != "" {
				tryMergeDelta(TokenDelta{
					Text:       "",
					IsComplete: true,
					StopReason: choice.FinishReason,
					Usage:      cumulativeUsage,
				})
			}
		}
	}

	return deltas
}
