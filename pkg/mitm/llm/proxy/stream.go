package proxy

import (
	"encoding/json"
	"strings"

	"github.com/monsterxx03/linko/pkg/mitm/llm"

	"log/slog"
)

// StreamTransformer handles SSE stream transformation from OpenAI to Anthropic format
type StreamTransformer struct {
	logger *slog.Logger

	// State tracking
	currentIndex     int
	thinkingBuffer   string
	toolBuffer       string
	currentToolID    string
	currentToolName  string
	messageStarted   bool
}

// NewStreamTransformer creates a new stream transformer
func NewStreamTransformer(logger *slog.Logger) *StreamTransformer {
	return &StreamTransformer{
		logger: logger,
	}
}

// TransformChunk transforms an OpenAI SSE chunk to Anthropic SSE format
func (t *StreamTransformer) TransformChunk(data []byte) string {
	lines := strings.Split(string(data), "\n")
	var result []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		dataStr := strings.TrimPrefix(line, "data: ")
		if dataStr == "" || dataStr == "[DONE]" {
			if dataStr == "[DONE]" {
				// Send message_stop event
				result = append(result, t.createMessageStopEvent())
			}
			continue
		}

		// Parse OpenAI stream chunk
		var chunk llm.OpenAIStreamChunk
		if err := json.Unmarshal([]byte(dataStr), &chunk); err != nil {
			t.logger.Warn("failed to parse OpenAI stream chunk", "error", err)
			continue
		}

		// Transform to Anthropic events
		events := t.transformChunk(&chunk)
		result = append(result, events...)
	}

	return strings.Join(result, "\n")
}

func (t *StreamTransformer) transformChunk(chunk *llm.OpenAIStreamChunk) []string {
	var events []string

	if !t.messageStarted {
		// Send message_start event
		events = append(events, t.createMessageStartEvent(chunk))
		t.messageStarted = true
	}

	for _, choice := range chunk.Choices {
		// Handle reasoning content (for o1 model)
		if choice.Delta.ReasoningContent != "" {
			t.thinkingBuffer += choice.Delta.ReasoningContent
			events = append(events, t.createThinkingDeltaEvent(choice.Delta.ReasoningContent))
		}

		// Handle content
		if choice.Delta.Content != "" {
			events = append(events, t.createContentDeltaEvent(choice.Delta.Content))
		}

		// Handle tool calls
		if len(choice.Delta.ToolCalls) > 0 {
			for _, tc := range choice.Delta.ToolCalls {
				if tc.ID != "" && tc.Function.Name != "" {
					// Start of a new tool call
					t.currentToolID = tc.ID
					t.currentToolName = tc.Function.Name
					t.toolBuffer = ""
					events = append(events, t.createToolUseStartEvent(tc.ID, tc.Function.Name))
				}
				if tc.Function.Arguments != "" {
					t.toolBuffer += tc.Function.Arguments
					events = append(events, t.createInputJSONDeltaEvent(t.toolBuffer))
				}
			}
		}

		// Handle completion
		if choice.FinishReason != "" {
			if t.currentToolID != "" {
				// Close tool use block
				events = append(events, t.createContentBlockStopEvent())
				t.currentToolID = ""
				t.currentToolName = ""
				t.toolBuffer = ""
			}

			// Send message_delta
			events = append(events, t.createMessageDeltaEvent(choice.FinishReason, chunk.Usage))

			// Send message_stop when stream ends
			events = append(events, t.createMessageStopEvent())
		}
	}

	return events
}

func (t *StreamTransformer) createMessageStartEvent(chunk *llm.OpenAIStreamChunk) string {
	event := map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":      "msg_" + chunk.ID,
			"type":    "message",
			"role":    "assistant",
			"content": []any{},
		},
	}
	data, _ := json.Marshal(event)
	return "data: " + string(data)
}

func (t *StreamTransformer) createMessageStopEvent() string {
	event := map[string]any{
		"type": "message_stop",
	}
	data, _ := json.Marshal(event)
	return "data: " + string(data)
}

func (t *StreamTransformer) createContentDeltaEvent(text string) string {
	// Ensure we have a content block started
	event := map[string]any{
		"type":  "content_block_delta",
		"index": t.currentIndex,
		"delta": map[string]any{
			"type": "text_delta",
			"text": text,
		},
	}
	data, _ := json.Marshal(event)
	return "data: " + string(data)
}

func (t *StreamTransformer) createThinkingDeltaEvent(thinking string) string {
	event := map[string]any{
		"type":  "content_block_delta",
		"index": t.currentIndex,
		"delta": map[string]any{
			"type":    "thinking_delta",
			"thinking": thinking,
		},
	}
	data, _ := json.Marshal(event)
	return "data: " + string(data)
}

func (t *StreamTransformer) createToolUseStartEvent(id, name string) string {
	// First, start a content block for the tool
	event := map[string]any{
		"type":  "content_block_start",
		"index": t.currentIndex,
		"content_block": map[string]any{
			"type": "tool_use",
			"id":   id,
			"name": name,
		},
	}
	data, _ := json.Marshal(event)
	return "data: " + string(data)
}

func (t *StreamTransformer) createInputJSONDeltaEvent(partialJSON string) string {
	event := map[string]any{
		"type":  "content_block_delta",
		"index": t.currentIndex,
		"delta": map[string]any{
			"type":        "input_json_delta",
			"partial_json": partialJSON,
		},
	}
	data, _ := json.Marshal(event)
	return "data: " + string(data)
}

func (t *StreamTransformer) createContentBlockStopEvent() string {
	event := map[string]any{
		"type":  "content_block_stop",
		"index": t.currentIndex,
	}
	data, _ := json.Marshal(event)
	return "data: " + string(data)
}

func (t *StreamTransformer) createMessageDeltaEvent(stopReason string, usage llm.OpenAIUsage) string {
	var usageMap map[string]any
	if usage.CompletionTokens > 0 || usage.PromptTokens > 0 {
		usageMap = map[string]any{
			"output_tokens": usage.CompletionTokens,
		}
	}

	event := map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason": mapStopReason(stopReason),
		},
	}
	if usageMap != nil {
		event["usage"] = usageMap
	}

	data, _ := json.Marshal(event)
	return "data: " + string(data)
}
