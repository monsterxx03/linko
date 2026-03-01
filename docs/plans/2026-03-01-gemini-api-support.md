# Gemini API 解析模块实现计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 在 pkg/mitm/llm/ 中增加 Google Gemini API 的解析支持，能够解析请求、响应和流式 SSE 数据。

**Architecture:** 参照现有的 openaiProvider 和 anthropicProvider 架构，创建 geminiProvider 实现 Provider 接口。需要处理 Google 独特的 JSON 格式（contents/parts/role）和工具调用格式（functionCall）。

**Tech Stack:** Go, JSON 解析, SSE 流式处理

---

### Task 1: 定义 Gemini API 类型

**Files:**
- Modify: `pkg/mitm/llm/types.go`

**Step 1: 添加 GeminiRequest 类型定义**

在 types.go 文件末尾添加以下类型：

```go
// GeminiRequest represents a Gemini API request
type GeminiRequest struct {
	Model             string          `json:"model"`
	Contents          []GeminiContent `json:"contents"`
	SystemInstruction *GeminiContent  `json:"systemInstruction,omitempty"`
	Tools             []GeminiTool    `json:"tools,omitempty"`
	GenerationConfig *GeminiConfig   `json:"generationConfig,omitempty"`
}

type GeminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []GeminiPart `json:"parts"`
}

type GeminiPart struct {
	Text          string              `json:"text,omitempty"`
	FunctionCall  *GeminiFunctionCall `json:"functionCall,omitempty"`
	FunctionResponse *struct {
		Name     string `json:"name"`
		Response any    `json:"response"`
	} `json:"functionResponse,omitempty"`
}

type GeminiFunctionCall struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
	Args string `json:"args"`
}

type GeminiTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type GeminiConfig struct {
	Temperature     float64  `json:"temperature,omitempty"`
	TopP            float64  `json:"topP,omitempty"`
	TopK            int      `json:"topK,omitempty"`
	MaxOutputTokens int      `json:"maxOutputTokens,omitempty"`
	StopSequences   []string `json:"stopSequences,omitempty"`
}
```

**Step 2: 添加 GeminiResponse 类型定义**

```go
type GeminiResponse struct {
	Candidates     []GeminiCandidate `json:"candidates"`
	PromptFeedback *struct {
		BlockReason string `json:"blockReason,omitempty"`
	} `json:"promptFeedback,omitempty"`
	UsageMetadata *GeminiUsageMetadata `json:"usageMetadata,omitempty"`
}

type GeminiCandidate struct {
	Content        GeminiContent `json:"content"`
	FinishReason   string        `json:"finishReason,omitempty"`
	Index          int           `json:"index"`
	SafetyRatings  []any          `json:"safetyRatings,omitempty"`
}

type GeminiUsageMetadata struct {
	PromptTokenCount               int `json:"promptTokenCount"`
	CandidatesTokenCount           int `json:"candidatesTokenCount"`
	TotalTokenCount                int `json:"totalTokenCount"`
	PromptTokenDetails             any `json:"promptTokenDetails,omitempty"`
	CandidatesTokenDetails         any `json:"candidatesTokenDetails,omitempty"`
}

type GeminiStreamChunk struct {
	Candidates     []GeminiCandidate `json:"candidates"`
	UsageMetadata   *GeminiUsageMetadata `json:"usageMetadata,omitempty"`
	ModelVersion    string `json:"modelVersion,omitempty"`
}
```

**Step 3: 提交代码**

```bash
git add pkg/mitm/llm/types.go
git commit -m "feat: add Gemini API types"
```

---

### Task 2: 创建 gemini.go Provider 实现

**Files:**
- Create: `pkg/mitm/llm/gemini.go`

**Step 1: 编写 geminiProvider 基本框架**

```go
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
```

**Step 2: 实现 ParseResponse 方法**

```go
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
```

**Step 3: 实现 ParseFullRequest 方法**

```go
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
```

**Step 4: 实现 ParseSSEStreamFrom 方法**

```go
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
```

**Step 5: 提交代码**

```bash
git add pkg/mitm/llm/gemini.go
git commit -m "feat: add Gemini provider implementation"
```

---

### Task 3: 添加消息转换函数

**Files:**
- Modify: `pkg/mitm/llm/helpers.go`

**Step 1: 添加 convertGeminiMessages 函数**

在 helpers.go 文件末尾添加：

```go
func convertGeminiMessages(contents []GeminiContent) []LLMMessage {
	var result []LLMMessage
	for _, c := range contents {
		var contentParts []string
		var toolCalls []ToolCall
		var toolResults []ToolResult

		for _, part := range c.Parts {
			if part.Text != "" {
				contentParts = append(contentParts, part.Text)
			}

			// Handle function call (model requesting tool use)
			if part.FunctionCall != nil {
				toolCalls = append(toolCalls, ToolCall{
					ID:   part.FunctionCall.ID,
					Type: "function",
					Function: FunctionCall{
						Name:      part.FunctionCall.Name,
						Arguments: part.FunctionCall.Args,
					},
				})
			}

			// Handle function response (tool result)
			if part.FunctionResponse != nil {
				respJSON, _ := json.Marshal(part.FunctionResponse.Response)
				toolResults = append(toolResults, ToolResult{
					ToolUseID: "", // Gemini doesn't provide tool_use_id in functionResponse
					Content:   string(respJSON),
				})
			}
		}

		role := c.Role
		if role == "" {
			role = "model"
		}

		result = append(result, LLMMessage{
			Role:        role,
			Content:     contentParts,
			ToolCalls:   toolCalls,
			ToolResults: toolResults,
		})
	}
	return result
}
```

**Step 2: 提交代码**

```bash
git add pkg/mitm/llm/helpers.go
git commit -m "feat: add Gemini message conversion"
```

---

### Task 4: 注册 Gemini Provider

**Files:**
- Modify: `pkg/mitm/llm/provider.go`

**Step 1: 更新 ProviderMatcher 添加 CustomGeminiMatches**

```go
type ProviderMatcher struct {
	CustomAnthropicMatches []string
	CustomOpenAIMatches   []string
	CustomGeminiMatches   []string
}
```

**Step 2: 更新 FindProviderWithMatcher 添加 geminiProvider**

```go
func FindProviderWithMatcher(hostname, path string, body []byte, logger *slog.Logger, matcher *ProviderMatcher) Provider {
	providers := []Provider{
		anthropicProvider{logger: logger, customMatches: matcher},
		openaiProvider{logger: logger, customMatches: matcher},
		geminiProvider{logger: logger, customMatches: matcher},
	}

	for _, p := range providers {
		if p.Match(hostname, path, body) {
			return p
		}
	}
	return nil
}
```

**Step 3: 提交代码**

```bash
git add pkg/mitm/llm/provider.go
git commit -m "feat: register Gemini provider"
```

---

### Task 5: 编写测试

**Files:**
- Create: `pkg/mitm/llm/gemini_test.go`

**Step 1: 编写测试文件**

```go
package llm

import (
	"encoding/json"
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
			path:    "/v1beta/models/gemini-1.5-flash:generateContent",
			body:    nil,
			want:    true,
		},
		{
			name:     "Non-matching hostname",
			hostname: "api.openai.com",
			path:    "/v1/chat/completions",
			body:    nil,
			want:    false,
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

	info, err := provider.ParseFullRequest(body)
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
```

**Step 2: 运行测试验证实现**

```bash
go test -v ./pkg/mitm/llm/... -run TestGemini
```

**Step 3: 提交代码**

```bash
git add pkg/mitm/llm/gemini_test.go
git commit -m "test: add Gemini provider tests"
```

---

### Task 6: 运行全部测试

**Step 1: 运行所有 LLM 相关测试**

```bash
go test -v ./pkg/mitm/llm/...
```

**Step 2: 提交最终代码**

```bash
git add .
git commit -m "feat: add Gemini API parsing support"
```

---

**Plan complete and saved to `docs/plans/2026-03-01-gemini-api-support.md`. Two execution options:**

**1. Subagent-Driven (this session)** - I dispatch fresh subagent per task, review between tasks, fast iteration

**2. Parallel Session (separate)** - Open new session with executing-plans, batch execution with checkpoints

**Which approach?**
