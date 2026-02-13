package llm

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

// OpenAI API types
type OpenAIRequest struct {
	Model       string          `json:"model"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Messages    []OpenAIMessage `json:"messages"`
	System      string          `json:"system,omitempty"`
	Stop        interface{}     `json:"stop,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
	TopP        float64         `json:"top_p,omitempty"`
	Tools       []ToolDef       `json:"tools,omitempty"`
}

type OpenAIMessage struct {
	Role      string      `json:"role"`
	Content   interface{} `json:"content"` // string or array of content parts
	Name      string      `json:"name,omitempty"`
	ToolCalls []ToolCall  `json:"tool_calls,omitempty"`
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
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []OpenAIChoice `json:"choices"`
	Usage   OpenAIUsage    `json:"usage"`
}

type OpenAIChoice struct {
	Index        int           `json:"index"`
	Message      OpenAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
	LogProbs     interface{}   `json:"logprobs,omitempty"`
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type OpenAIStreamChunk struct {
	ID      string              `json:"id"`
	Object  string              `json:"object"`
	Created int64               `json:"created"`
	Model   string              `json:"model"`
	Choices []OpenAIChunkChoice `json:"choices"`
}

type OpenAIChunkChoice struct {
	Index        int         `json:"index"`
	Delta        OpenAIDelta `json:"delta"`
	FinishReason string      `json:"finish_reason,omitempty"`
	LogProbs     interface{} `json:"logprobs,omitempty"`
}

type OpenAIDelta struct {
	Role      string     `json:"role,omitempty"`
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// openaiProvider implements Provider for OpenAI Chat API
type openaiProvider struct {
	logger *slog.Logger
}

func (o openaiProvider) Match(hostname, path string, body []byte) bool {
	return (strings.Contains(hostname, "api.openai.com") ||
		strings.Contains(hostname, "openai.azure.com") ||
		strings.Contains(path, "/chat/completions"))
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
		ToolCalls: resp.Choices[0].Message.ToolCalls,
	}, nil
}

// extractSystemPromptsFromReq extracts system prompts from a parsed OpenAIRequest
func (o openaiProvider) extractSystemPromptsFromReq(req *OpenAIRequest) []string {
	if req.System == "" {
		return nil
	}

	return []string{req.System}
}

// ParseFullRequest parses the request body once and returns all extracted info
func (o openaiProvider) ParseFullRequest(body []byte) (*RequestInfo, error) {
	var req OpenAIRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAI request: %w", err)
	}

	return &RequestInfo{
		ConversationID: fmt.Sprintf("openai-%s", generateOpenAIConversationHash(req.Messages)),
		Model:           req.Model,
		Messages:        convertOpenAIMessages(req.Messages),
		SystemPrompts:   o.extractSystemPromptsFromReq(&req),
		Tools:           o.extractToolsFromReq(&req),
	}, nil
}

// extractToolsFromReq extracts tools from a parsed OpenAIRequest
func (o openaiProvider) extractToolsFromReq(req *OpenAIRequest) []ToolDef {
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

// ParseSSEStreamFrom parses SSE stream from a specific position for incremental processing
func (o openaiProvider) ParseSSEStreamFrom(body []byte, startPos int) []TokenDelta {
	if startPos >= len(body) {
		return nil
	}

	remaining := string(body[startPos:])
	lines := strings.Split(remaining, "\n")

	var deltas []TokenDelta

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

		var chunk OpenAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			o.logger.Warn("failed to parse OpenAI SSE event", "error", err, "data", data)
			continue
		}

		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				deltas = append(deltas, TokenDelta{
					Text: choice.Delta.Content,
				})
			}
			if len(choice.Delta.ToolCalls) > 0 {
				for _, toolCall := range choice.Delta.ToolCalls {
					if toolCall.ID != "" && toolCall.Function.Name != "" {
						deltas = append(deltas, TokenDelta{
							ToolName: toolCall.Function.Name,
							ToolID:   toolCall.ID,
						})
						if toolCall.Function.Arguments != "" {
							deltas = append(deltas, TokenDelta{
								ToolData: toolCall.Function.Arguments,
								ToolID:   toolCall.ID,
							})
						}
					}
				}
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
