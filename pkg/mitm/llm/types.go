package llm

import (
	"time"
)

// LLMMessage represents a message in an LLM conversation
type LLMMessage struct {
	Role      string     `json:"role"`                 // "user", "assistant", "system", "tool"
	Content   []string   `json:"content"`              // message content
	Name      string     `json:"name,omitempty"`       // optional name for the message
	ToolCalls []ToolCall `json:"tool_calls,omitempty"` // tool calls (for assistant messages)
	System    []string   `json:"system,omitempty"`     // system prompt (extracted from request)
	Tools     []ToolDef  `json:"tools,omitempty"`      // available tools (extracted from request)
}

// ToolDef represents a tool definition
type ToolDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"input_schema"`
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
}

// TokenDelta represents incremental token updates for streaming
type TokenDelta struct {
	Text       string `json:"text"`
	Thinking   string `json:"thinking,omitempty"`
	ToolData   string `json:"tool_data,omitempty"`   // tool call JSON data (for input_json_delta)
	ToolName   string `json:"tool_name,omitempty"`   // tool name for tool_calls
	ToolID     string `json:"tool_id,omitempty"`     // tool call ID
	IsComplete bool   `json:"is_complete"`
	StopReason string `json:"stop_reason,omitempty"`
}

// LLMMessageEvent is published when a new LLM message is detected
type LLMMessageEvent struct {
	ID             string     `json:"id"`
	Timestamp      time.Time  `json:"timestamp"`
	ConversationID string     `json:"conversation_id"`
	RequestID      string     `json:"request_id"` // HTTP request ID for correlation
	Message        LLMMessage `json:"message"`
	TokenCount     int        `json:"token_count,omitempty"`  // token count for this message
	TotalTokens    int        `json:"total_tokens,omitempty"` // total tokens in conversation
	Model          string     `json:"model,omitempty"`        // model name
}

// LLMTokenEvent is published during streaming responses
type LLMTokenEvent struct {
	ID             string    `json:"id"`
	Timestamp      time.Time `json:"timestamp"`
	ConversationID string    `json:"conversation_id"`
	RequestID      string    `json:"request_id"`
	Delta          string    `json:"delta"`              // new token content
	Thinking       string    `json:"thinking,omitempty"`  // thinking content (for Claude)
	ToolName       string    `json:"tool_name,omitempty"` // tool name for tool_calls
	ToolID         string    `json:"tool_id,omitempty"`  // tool call ID
	ToolData       string    `json:"tool_data,omitempty"` // tool call arguments delta
	IsComplete     bool      `json:"is_complete"`        // true when streaming is done
	StopReason     string    `json:"stop_reason,omitempty"`
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
