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
	hostname      string // track current hostname for response parsing
}

func (g geminiProvider) Match(hostname, path string, body []byte) bool {
	// Match Google Generative Language API
	if strings.Contains(hostname, "generativelanguage.googleapis.com") {
		return true
	}

	// Match Google Cloud Code Prediction API
	if strings.Contains(hostname, "cloudcode-pa.googleapis.com") {
		if strings.Contains(path, "generateContent") || strings.Contains(path, "streamGenerateContent") {
			return true
		}
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
	// First try standard Gemini format
	var resp GeminiResponse
	if err := json.Unmarshal(body, &resp); err == nil && len(resp.Candidates) > 0 {
		return g.parseGeminiResponse(&resp)
	}

	// Try CloudCode format (candidates nested under "response")
	var cloudResp CloudCodeResponse
	if err := json.Unmarshal(body, &cloudResp); err != nil {
		return nil, fmt.Errorf("failed to parse Gemini response: %w", err)
	}

	if len(cloudResp.Response.Candidates) == 0 {
		return nil, fmt.Errorf("no candidates in response")
	}

	return g.parseGeminiCandidates(cloudResp.Response.Candidates, cloudResp.Response.UsageMetadata)
}

func (g geminiProvider) parseGeminiResponse(resp *GeminiResponse) (*LLMResponse, error) {
	if len(resp.Candidates) == 0 {
		return nil, fmt.Errorf("no candidates in response")
	}
	return g.parseGeminiCandidates(resp.Candidates, resp.UsageMetadata)
}

func (g geminiProvider) parseGeminiCandidates(candidates []GeminiCandidate, usageMeta *GeminiUsageMetadata) (*LLMResponse, error) {
	content := ""
	for _, part := range candidates[0].Content.Parts {
		if part.Text != "" {
			content += part.Text
		}
	}

	stopReason := candidates[0].FinishReason
	if stopReason == "STOP" {
		stopReason = "stop"
	}

	usage := TokenUsage{}
	if usageMeta != nil {
		usage.InputTokens = usageMeta.PromptTokenCount
		usage.OutputTokens = usageMeta.CandidatesTokenCount
	}

	return &LLMResponse{
		Content:    content,
		StopReason: stopReason,
		Usage:      usage,
	}, nil
}

func (g geminiProvider) ParseFullRequest(hostname string, headers map[string]string, body []byte) (*RequestInfo, error) {
	// First try standard Gemini format
	var req GeminiRequest
	if err := json.Unmarshal(body, &req); err == nil && len(req.Contents) > 0 {
		// 当 hostname 是 opencode.ai 时，优先从 header X-Opencode-Session 获取会话 ID
		conversationID := "gemini-default"
		if hostname == "opencode.ai" {
			if sessionID, ok := headers["X-Opencode-Session"]; ok && sessionID != "" {
				conversationID = fmt.Sprintf("opencode-%s", sessionID)
			}
		}
		return &RequestInfo{
			ConversationID: conversationID,
			Model:          req.Model,
			Messages:       convertGeminiMessages(req.Contents),
			SystemPrompts:  g.extractSystemPromptsFromReq(&req),
			Tools:          g.extractToolsFromReq(&req),
		}, nil
	}

	// Try CloudCode format (nested under "request")
	var cloudReq CloudCodeRequest
	if err := json.Unmarshal(body, &cloudReq); err != nil {
		return nil, fmt.Errorf("failed to parse Gemini request: %w", err)
	}

	// Build ConversationID from project and user_prompt_id
	conversationID := "gemini-default"
	if cloudReq.Project != "" {
		conversationID = fmt.Sprintf("cloudcode-%s", cloudReq.Project)
	}

	// Extract tools from CloudCode format
	extractCloudCodeTools := func(cloudReq *CloudCodeRequest) []ToolDef {
		if cloudReq.Request.Tools == nil {
			return nil
		}
		var tools []ToolDef
		for _, tool := range cloudReq.Request.Tools {
			for _, t := range tool.FunctionDeclarations {
				tools = append(tools, ToolDef{
					Name:        t.Name,
					Description: t.Description,
					InputSchema: t.ParametersJsonSchema,
				})
			}
		}
		return tools
	}

	// Extract system prompts from CloudCode format
	extractCloudCodeSystemPrompts := func(cloudReq *CloudCodeRequest) []string {
		if cloudReq.Request.SystemInstruction == nil {
			return nil
		}
		var prompts []string
		for _, part := range cloudReq.Request.SystemInstruction.Parts {
			if part.Text != "" {
				prompts = append(prompts, part.Text)
			}
		}
		if len(prompts) == 0 {
			return nil
		}
		return prompts
	}

	return &RequestInfo{
		ConversationID: conversationID,
		Model:          cloudReq.Model,
		Messages:       convertGeminiMessages(cloudReq.Request.Contents),
		SystemPrompts:  extractCloudCodeSystemPrompts(&cloudReq),
		Tools:          extractCloudCodeTools(&cloudReq),
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

	// Helper to process candidates
	processCandidates := func(candidates []GeminiCandidate, usageMeta *GeminiUsageMetadata) {
		if usageMeta != nil {
			cumulativeUsage.InputTokens = usageMeta.PromptTokenCount
			cumulativeUsage.OutputTokens = usageMeta.CandidatesTokenCount
		}

		for _, candidate := range candidates {
			for _, part := range candidate.Content.Parts {
				// Handle thinking content (thought: true from CloudCode)
				if part.Thought && part.Text != "" {
					tryMergeDelta(TokenDelta{
						Thinking: part.Text,
					})
				}

				// Handle text content (skip if thought: true was handled above)
				if !part.Thought && part.Text != "" {
					tryMergeDelta(TokenDelta{
						Text: part.Text,
					})
				}

				// Handle function call
				if part.FunctionCall != nil {
					// Args can be string or object (CloudCode uses object)
					toolData := ""
					switch a := part.FunctionCall.Args.(type) {
					case string:
						toolData = a
					case map[string]any:
						argsJSON, _ := json.Marshal(a)
						toolData = string(argsJSON)
					}
					tryMergeDelta(TokenDelta{
						ToolName: part.FunctionCall.Name,
						ToolID:   part.FunctionCall.ID,
						ToolData: toolData,
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

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "" {
			continue
		}

		// First try standard Gemini format
		var chunk GeminiStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err == nil && len(chunk.Candidates) > 0 {
			processCandidates(chunk.Candidates, chunk.UsageMetadata)
			continue
		}

		// Try CloudCode format
		var cloudChunk CloudCodeStreamChunk
		if err := json.Unmarshal([]byte(data), &cloudChunk); err == nil && len(cloudChunk.Response.Candidates) > 0 {
			processCandidates(cloudChunk.Response.Candidates, cloudChunk.Response.UsageMetadata)
			continue
		}

	}

	return deltas
}
