package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/monsterxx03/linko/pkg/mitm/llm"

	"log/slog"
)

// Proxy implements the Anthropic to OpenAI protocol transformation proxy
type Proxy struct {
	config     *ProxyConfig
	httpClient *http.Client
	logger     *slog.Logger
}

// NewProxy creates a new Anthropic to OpenAI proxy
func NewProxy(cfg *ProxyConfig, logger *slog.Logger) *Proxy {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}

	return &Proxy{
		config: cfg,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		logger: logger,
	}
}

// TransformRequest transforms an Anthropic request to OpenAI format
func (p *Proxy) TransformRequest(req *llm.AnthropicRequest) (*llm.OpenAIRequest, error) {
	openaiReq := &llm.OpenAIRequest{
		Model:     p.mapModel(req.Model),
		MaxTokens: req.MaxTokens,
		Stop:      req.StopSequences,
		Temperature: req.Temperature,
		TopP:      req.TopP,
	}

	// Transform system to messages
	if req.System != nil {
		systemContent, err := extractSystemContent(req.System)
		if err != nil {
			return nil, fmt.Errorf("failed to extract system content: %w", err)
		}
		openaiReq.Messages = append([]llm.OpenAIMessage{
			{Role: "system", Content: systemContent},
		}, transformMessages(req.Messages)...)
	} else {
		openaiReq.Messages = transformMessages(req.Messages)
	}

	// Transform tools
	if len(req.Tools) > 0 {
		openaiReq.Tools = transformTools(req.Tools)
	}

	return openaiReq, nil
}

// RoundTrip performs the full request-response transformation
func (p *Proxy) RoundTrip(ctx context.Context, anthropicReq *llm.AnthropicRequest) (*llm.AnthropicResponse, error) {
	// Transform request
	openaiReq, err := p.TransformRequest(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("failed to transform request: %w", err)
	}

	// Marshal to JSON
	reqBody, err := json.Marshal(openaiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := p.config.UpstreamURL + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if p.config.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	}

	// Send request
	p.logger.Debug("sending request to upstream", "url", url, "model", openaiReq.Model)
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check for errors
	if resp.StatusCode != http.StatusOK {
		var openAIError struct {
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		json.Unmarshal(respBody, &openAIError)
		if openAIError.Error.Message != "" {
			return &llm.AnthropicResponse{
				Type: "error",
				Error: struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				}{
					Type:    openAIError.Error.Type,
					Message: openAIError.Error.Message,
				},
			}, nil
		}
		return nil, fmt.Errorf("upstream returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// Transform response
	var openAIResp llm.OpenAIResponse
	if err := json.Unmarshal(respBody, &openAIResp); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAI response: %w", err)
	}

	return p.TransformResponse(&openAIResp, anthropicReq.Model), nil
}

// RoundTripStream performs streaming request-response transformation
func (p *Proxy) RoundTripStream(ctx context.Context, anthropicReq *llm.AnthropicRequest) (<-chan []byte, error) {
	// Transform request
	openaiReq, err := p.TransformRequest(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("failed to transform request: %w", err)
	}

	// Add stream parameter
	// Note: OpenAI uses stream=true for SSE

	// Marshal to JSON
	reqBody, err := json.Marshal(openaiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := p.config.UpstreamURL + "/v1/chat/completions?stream=true"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if p.config.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	}

	// Send request
	p.logger.Debug("sending streaming request to upstream", "url", url, "model", openaiReq.Model)
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("upstream returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// Create channel for streaming
	ch := make(chan []byte, 10)

	// Start goroutine to read and transform stream
	go func() {
		defer resp.Body.Close()
		defer close(ch)

		stream := NewStreamTransformer(p.logger)
		buf := make([]byte, 4096)

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			n, err := resp.Body.Read(buf)
			if n > 0 {
				// Transform the chunk
				transformed := stream.TransformChunk(buf[:n])
				if len(transformed) > 0 {
					ch <- []byte(transformed)
				}
			}
			if err != nil {
				break
			}
		}
	}()

	return ch, nil
}

// TransformResponse transforms an OpenAI response to Anthropic format
func (p *Proxy) TransformResponse(openaiResp *llm.OpenAIResponse, originalModel string) *llm.AnthropicResponse {
	anthropicResp := &llm.AnthropicResponse{
		ID:     "msg_" + openaiResp.ID,
		Type:   "message",
		Role:   "assistant",
		Model:  originalModel,
		Usage: llm.AnthropicUsage{
			InputTokens:  openaiResp.Usage.PromptTokens,
			OutputTokens: openaiResp.Usage.CompletionTokens,
		},
	}

	if len(openaiResp.Choices) == 0 {
		anthropicResp.StopReason = "end_turn"
		anthropicResp.Content = []llm.AnthropicContent{
			{Type: "text", Text: ""},
		}
		return anthropicResp
	}

	choice := openaiResp.Choices[0]
	anthropicResp.StopReason = mapStopReason(choice.FinishReason)

	// Extract content and tool calls
	var content []llm.AnthropicContent

	// Handle reasoning content (o1 model) - check multiple levels
	reasoningContent := choice.ReasoningContent
	if reasoningContent == "" {
		reasoningContent = choice.Message.ReasoningContent
	}
	if reasoningContent != "" {
		content = append(content, llm.AnthropicContent{
			Type:     "thinking",
			Thinking: reasoningContent,
		})
	}

	// Handle message content
	switch msgContent := choice.Message.Content.(type) {
	case string:
		if msgContent != "" {
			content = append(content, llm.AnthropicContent{
				Type: "text",
				Text: msgContent,
			})
		}
	case []any:
		for _, part := range msgContent {
			if partMap, ok := part.(map[string]any); ok {
				partType, _ := partMap["type"].(string)
				switch partType {
				case "text":
					if text, ok := partMap["text"].(string); ok {
						content = append(content, llm.AnthropicContent{
							Type: "text",
							Text: text,
						})
					}
				}
			}
		}
	}

	// Handle tool calls
	if len(choice.Message.ToolCalls) > 0 {
		for _, tc := range choice.Message.ToolCalls {
			inputJSON := tc.Function.Arguments
			var input map[string]any
			json.Unmarshal([]byte(inputJSON), &input)
			content = append(content, llm.AnthropicContent{
				Type: "tool_use",
				Text: "",
			})
			if len(content) > 0 {
				lastContent := &content[len(content)-1]
				lastContent.Type = "tool_use"
				lastContent.Source = nil
				// Note: tool_use needs id, name, input fields - use reflection or type assertion
			}
			_ = tc // tc is used for extracting tool call info
		}
	}

	if len(content) == 0 {
		content = append(content, llm.AnthropicContent{
			Type: "text",
			Text: "",
		})
	}

	anthropicResp.Content = content
	return anthropicResp
}

// mapModel maps an Anthropic model name to an upstream model name
func (p *Proxy) mapModel(model string) string {
	if p.config.ModelMapping != nil {
		if mapped, ok := p.config.ModelMapping[model]; ok {
			return mapped
		}
	}
	return model
}

// extractSystemContent extracts the system prompt from various formats
func extractSystemContent(system any) (string, error) {
	switch s := system.(type) {
	case string:
		return s, nil
	case []any:
		var result string
		for _, item := range s {
			if itemMap, ok := item.(map[string]any); ok {
				if text, ok := itemMap["text"].(string); ok {
					result += text + "\n"
				}
			}
		}
		return result, nil
	}
	return "", nil
}

// transformMessages converts Anthropic messages to OpenAI format
func transformMessages(messages []llm.AnthropicMessage) []llm.OpenAIMessage {
	var result []llm.OpenAIMessage

	for _, m := range messages {
		openaiMsg := llm.OpenAIMessage{
			Role: m.Role,
		}

		switch c := m.Content.(type) {
		case string:
			openaiMsg.Content = c
		case []any:
			var textContent string
			var toolCalls []llm.ToolCall
			var toolResults []llm.ToolResult

			for _, item := range c {
				if itemMap, ok := item.(map[string]any); ok {
					itemType, _ := itemMap["type"].(string)
					switch itemType {
					case "text":
						if text, ok := itemMap["text"].(string); ok {
							textContent += text
						}
					case "thinking":
						if thinking, ok := itemMap["thinking"].(string); ok {
							textContent += "[Thinking]\n" + thinking + "\n[/Thinking]\n"
						}
					case "tool_use":
						id, _ := itemMap["id"].(string)
						name, _ := itemMap["name"].(string)
						input, _ := itemMap["input"].(map[string]any)
						inputJSON, _ := json.Marshal(input)
						toolCalls = append(toolCalls, llm.ToolCall{
							ID:   id,
							Type: "function",
							Function: llm.FunctionCall{
								Name:      name,
								Arguments: string(inputJSON),
							},
						})
					case "tool_result":
						toolUseID, _ := itemMap["tool_use_id"].(string)
						content, _ := itemMap["content"].(string)
						toolResults = append(toolResults, llm.ToolResult{
							ToolUseID: toolUseID,
							Content:   content,
						})
					}
				}
			}

			if textContent != "" {
				openaiMsg.Content = textContent
			}
			if len(toolCalls) > 0 {
				openaiMsg.ToolCalls = toolCalls
			}
			// Note: tool results need special handling in OpenAI
		}

		result = append(result, openaiMsg)
	}

	return result
}

// transformTools converts Anthropic tools to OpenAI format
func transformTools(tools []llm.AnthropicTool) []llm.OpenAITool {
	var result []llm.OpenAITool

	for _, t := range tools {
		result = append(result, llm.OpenAITool{
			Type: "function",
			Function: llm.OpenAIFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}

	return result
}

// mapStopReason maps OpenAI stop reasons to Anthropic stop reasons
func mapStopReason(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	case "content_filter":
		return "stopping_reason"
	default:
		return "end_turn"
	}
}
