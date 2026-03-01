package llm

import (
	"time"
)

// LLMMessage represents a message in an LLM conversation
type LLMMessage struct {
	Role        string       `json:"role"`                   // "user", "assistant", "system", "tool"
	Content     []string     `json:"content"`                // message content
	Name        string       `json:"name,omitempty"`         // optional name for the message
	ToolCalls   []ToolCall   `json:"tool_calls,omitempty"`   // tool calls (for assistant messages)
	ToolResults []ToolResult `json:"tool_results,omitempty"` // tool results (for user messages)
	System      []string     `json:"system,omitempty"`       // system prompt (extracted from request)
	Tools       []ToolDef    `json:"tools,omitempty"`        // available tools (extracted from request)
}

// ToolDef represents a tool definition
type ToolDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// ToolResult represents a tool execution result
type ToolResult struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
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

func (tu TokenUsage) TotalTokens() int {
	return tu.InputTokens + tu.OutputTokens
}

// APIError represents an API error response
type APIError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// LLMResponse represents a response from an LLM
type LLMResponse struct {
	Content    string     `json:"content"`
	StopReason string     `json:"stop_reason"`
	Usage      TokenUsage `json:"usage"`
	Error      *APIError  `json:"error,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

// TokenDelta represents incremental token updates for streaming
type TokenDelta struct {
	Text       string     `json:"text"`
	Thinking   string     `json:"thinking,omitempty"`
	ToolData   string     `json:"tool_data,omitempty"` // tool call JSON data (for input_json_delta)
	ToolName   string     `json:"tool_name,omitempty"` // tool name for tool_calls
	ToolID     string     `json:"tool_id,omitempty"`   // tool call ID
	IsComplete bool       `json:"is_complete"`
	StopReason string     `json:"stop_reason,omitempty"`
	Usage      TokenUsage `json:"usage,omitempty"` // cumulative token usage
}

// LLMMessageEvent is published when a new LLM message is detected
type LLMMessageEvent struct {
	ID             string     `json:"id"`
	Timestamp      time.Time  `json:"timestamp"`
	ConversationID string     `json:"conversation_id"`
	Message        LLMMessage `json:"message"`
	TokenCount     int        `json:"token_count,omitempty"`  // token count for this message
	TotalTokens    int        `json:"total_tokens,omitempty"` // total tokens in conversation
	Model          string     `json:"model,omitempty"`        // model name
}

// LLMTokenEvent is published during streaming responses
type LLMTokenEvent struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversation_id"`
	Delta          string `json:"delta"`               // new token content
	Thinking       string `json:"thinking,omitempty"`  // thinking content (for Claude)
	ToolName       string `json:"tool_name,omitempty"` // tool name for tool_calls
	ToolID         string `json:"tool_id,omitempty"`   // tool call ID
	ToolData       string `json:"tool_data,omitempty"` // tool call arguments delta
	IsComplete     bool   `json:"is_complete"`         // true when streaming is done
	StopReason     string `json:"stop_reason,omitempty"`
	TokenCount     int    `json:"token_count,omitempty"`  // token count for this message
	TotalTokens    int    `json:"total_tokens,omitempty"` // total tokens in conversation
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

// RequestInfo contains parsed information from an LLM request body
// Used to avoid multiple JSON unmarshaling of the same request
type RequestInfo struct {
	ConversationID string
	Model          string
	Messages       []LLMMessage
	SystemPrompts  []string
	Tools          []ToolDef
}

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
	Text              string              `json:"text,omitempty"`
	Thought           bool                `json:"thought,omitempty"`       // thought: true from CloudCode
	ThoughtSignature string              `json:"thoughtSignature,omitempty"` // thinking content from Gemini
	FunctionCall      *GeminiFunctionCall `json:"functionCall,omitempty"`
	FunctionResponse  *struct {
		Name     string `json:"name"`
		Response any    `json:"response"`
	} `json:"functionResponse,omitempty"`
}

type GeminiFunctionCall struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
	Args any   `json:"args"` // Can be string or object (for CloudCode)
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

// CloudCodeResponse represents response from cloudcode-pa.googleapis.com
// The candidates field is nested under "response" key
type CloudCodeResponse struct {
	Response struct {
		Candidates     []GeminiCandidate `json:"candidates"`
		UsageMetadata *GeminiUsageMetadata `json:"usageMetadata,omitempty"`
	} `json:"response"`
}

type CloudCodeStreamChunk struct {
	Response struct {
		Candidates     []GeminiCandidate `json:"candidates"`
		UsageMetadata *GeminiUsageMetadata `json:"usageMetadata,omitempty"`
	} `json:"response"`
}

// CloudCodeRequest represents request to cloudcode-pa.googleapis.com
// The contents are nested under "request" key, but model is at top level
type CloudCodeRequest struct {
	Model        string `json:"model"`
	Project      string `json:"project"`
	UserPromptID string `json:"user_prompt_id"`
	Request      struct {
		Contents          []GeminiContent        `json:"contents"`
		SystemInstruction *GeminiContent         `json:"systemInstruction,omitempty"`
		Tools             []CloudCodeTools      `json:"tools,omitempty"`
		GenerationConfig *GeminiConfig          `json:"generationConfig,omitempty"`
	} `json:"request"`
}

// CloudCodeTools represents the tools structure in CloudCode API
// Format: {"functionDeclarations": [{name, description, parametersJsonSchema}]}
type CloudCodeTools struct {
	FunctionDeclarations []CloudCodeFunction `json:"functionDeclarations"`
}

type CloudCodeFunction struct {
	Name                string                 `json:"name"`
	Description         string                 `json:"description,omitempty"`
	ParametersJsonSchema map[string]interface{} `json:"parametersJsonSchema"`
}
