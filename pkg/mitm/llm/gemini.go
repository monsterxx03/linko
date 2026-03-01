package llm

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

// geminiProvider implements Provider for Google Gemini API
type geminiProvider struct {
	logger        *slog.Logger
	customMatches *ProviderMatcher
}

func (g geminiProvider) Match(hostname, path string, body []byte) bool {
	// Match Google Generative Language API
	if strings.Contains(hostname, "generativelanguage.googleapis.com") {
		return true
	}

	// Check custom matches
	if g.customMatches != nil {
		for _, match := range g.customMatches.CustomGeminiMatches {
			if matchCustomPattern(hostname, path, match) {
				return true
			}
		}
	}

	return false
}

func (g geminiProvider) ParseResponse(path string, body []byte) (*LLMResponse, error) {
	var resp GeminiResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse Gemini response: %w", err)
	}

	if len(resp.Candidates) == 0 {
		return nil, fmt.Errorf("no candidates in response")
	}

	content := ""
	for _, part := range resp.Candidates[0].Content.Parts {
		if part.Text != "" {
			content += part.Text
		}
	}

	stopReason := resp.Candidates[0].FinishReason
	if stopReason == "STOP" {
		stopReason = "stop"
	}

	usage := TokenUsage{}
	if resp.UsageMetadata != nil {
		usage.InputTokens = resp.UsageMetadata.PromptTokenCount
		usage.OutputTokens = resp.UsageMetadata.CandidatesTokenCount
	}

	return &LLMResponse{
		Content:    content,
		StopReason: stopReason,
		Usage:      usage,
	}, nil
}

func (g geminiProvider) ParseFullRequest(body []byte) (*RequestInfo, error) {
	var req GeminiRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("failed to parse Gemini request: %w", err)
	}

	return &RequestInfo{
		ConversationID: "gemini-default",
		Model:          req.Model,
		Messages:       convertGeminiMessages(req.Contents),
		SystemPrompts:  g.extractSystemPromptsFromReq(&req),
		Tools:          g.extractToolsFromReq(&req),
	}, nil
}

func (g geminiProvider) extractSystemPromptsFromReq(req *GeminiRequest) []string {
	if req.SystemInstruction == nil {
		return nil
	}

	var prompts []string
	for _, part := range req.SystemInstruction.Parts {
		if part.Text != "" {
			prompts = append(prompts, part.Text)
		}
	}

	if len(prompts) == 0 {
		return nil
	}
	return prompts
}

func (g geminiProvider) extractToolsFromReq(req *GeminiRequest) []ToolDef {
	if len(req.Tools) == 0 {
		return nil
	}

	var tools []ToolDef
	for _, t := range req.Tools {
		tools = append(tools, ToolDef{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Parameters,
		})
	}
	return tools
}

func (g geminiProvider) ParseSSEStreamFrom(body []byte, startPos int) []TokenDelta {
	if startPos >= len(body) {
		return nil
	}

	remaining := string(body[startPos:])
	lines := strings.Split(remaining, "\n")

	var deltas []TokenDelta
	var cumulativeUsage TokenUsage

	tryMergeDelta := func(newDelta TokenDelta) {
		if newDelta.Usage == (TokenUsage{}) {
			newDelta.Usage = cumulativeUsage
		}

		if len(deltas) == 0 {
			deltas = append(deltas, newDelta)
			return
		}

		last := &deltas[len(deltas)-1]

		// Merge text content
		if newDelta.Text != "" {
			last.Text += newDelta.Text
			return
		}

		// Merge completion info
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

		// Merge tool data
		if newDelta.ToolData != "" && !last.IsComplete {
			sameTool := (newDelta.ToolID == "" || newDelta.ToolID == last.ToolID) &&
				(newDelta.ToolName == "" || newDelta.ToolName == last.ToolName)
			if sameTool && !strings.HasSuffix(last.ToolData, newDelta.ToolData) {
				last.ToolData += newDelta.ToolData
				return
			}
		}

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

		var chunk GeminiStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			g.logger.Warn("failed to parse Gemini SSE event", "error", err, "data", data)
			continue
		}

		// Update cumulative usage
		if chunk.UsageMetadata != nil {
			cumulativeUsage.InputTokens = chunk.UsageMetadata.PromptTokenCount
			cumulativeUsage.OutputTokens = chunk.UsageMetadata.CandidatesTokenCount
		}

		// Process content
		for _, candidate := range chunk.Candidates {
			for _, part := range candidate.Content.Parts {
				if part.Text != "" {
					tryMergeDelta(TokenDelta{
						Text: part.Text,
					})
				}

				// Handle function call
				if part.FunctionCall != nil {
					tryMergeDelta(TokenDelta{
						ToolName: part.FunctionCall.Name,
						ToolID:   part.FunctionCall.ID,
						ToolData: part.FunctionCall.Args,
					})
				}
			}

			// Handle completion
			if candidate.FinishReason != "" && candidate.FinishReason != "FINISH_REASON_UNSPECIFIED" {
				stopReason := candidate.FinishReason
				if stopReason == "STOP" {
					stopReason = "stop"
				}
				tryMergeDelta(TokenDelta{
					Text:       "",
					IsComplete: true,
					StopReason: stopReason,
					Usage:      cumulativeUsage,
				})
			}
		}
	}

	return deltas
}
