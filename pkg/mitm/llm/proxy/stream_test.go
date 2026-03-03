package proxy

import (
	"log/slog"
	"strings"
	"testing"
)

func TestStreamTransformer_TransformChunk(t *testing.T) {
	logger := slog.Default()
	st := NewStreamTransformer(logger)

	// Test basic text chunk
	data := []byte(`data: {"id":"chatcmpl-123","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}
`)
	result := st.TransformChunk(data)

	if !strings.Contains(result, "message_start") {
		t.Error("expected message_start event")
	}
	if !strings.Contains(result, "content_block_delta") {
		t.Error("expected content_block_delta event")
	}
	if !strings.Contains(result, "Hello") {
		t.Error("expected Hello in output")
	}
}

func TestStreamTransformer_TransformChunk_Reasoning(t *testing.T) {
	logger := slog.Default()
	st := NewStreamTransformer(logger)

	// Test reasoning content (o1 model)
	data := []byte(`data: {"id":"chatcmpl-123","choices":[{"index":0,"delta":{"reasoning_content":"Let me think..."},"finish_reason":null}]}
`)
	result := st.TransformChunk(data)

	if !strings.Contains(result, "thinking_delta") {
		t.Error("expected thinking_delta event")
	}
	if !strings.Contains(result, "Let me think...") {
		t.Error("expected thinking content in output")
	}
}

func TestStreamTransformer_TransformChunk_ToolCall(t *testing.T) {
	logger := slog.Default()
	st := NewStreamTransformer(logger)

	// First chunk: tool call start
	data1 := []byte(`data: {"id":"chatcmpl-123","choices":[{"index":0,"delta":{"tool_calls":[{"id":"call_123","type":"function","function":{"name":"get_weather"}}]}}]}
`)
	result1 := st.TransformChunk(data1)

	if !strings.Contains(result1, "content_block_start") {
		t.Error("expected content_block_start event")
	}
	if !strings.Contains(result1, "tool_use") {
		t.Error("expected tool_use in content_block_start")
	}

	// Second chunk: tool arguments
	data2 := []byte(`data: {"id":"chatcmpl-123","choices":[{"index":0,"delta":{"tool_calls":[{"function":{"arguments":"{"}}]}}]}
`)
	result2 := st.TransformChunk(data2)

	if !strings.Contains(result2, "input_json_delta") {
		t.Error("expected input_json_delta event")
	}
}

func TestStreamTransformer_TransformChunk_Completion(t *testing.T) {
	logger := slog.Default()
	st := NewStreamTransformer(logger)

	// First send some content to start the message
	st.TransformChunk([]byte(`data: {"id":"chatcmpl-123","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}
`))

	// Then send completion
	data := []byte(`data: {"id":"chatcmpl-123","choices":[{"index":0,"delta":{},"finish_reason":"stop","usage":{"prompt_tokens":10,"completion_tokens":5}}]}
`)
	result := st.TransformChunk(data)

	if !strings.Contains(result, "message_delta") {
		t.Error("expected message_delta event")
	}
	if !strings.Contains(result, "end_turn") {
		t.Error("expected end_turn in message_delta")
	}
	if !strings.Contains(result, "message_stop") {
		t.Error("expected message_stop event")
	}
}

func TestStreamTransformer_TransformChunk_Done(t *testing.T) {
	logger := slog.Default()
	st := NewStreamTransformer(logger)

	// First send some content
	st.TransformChunk([]byte(`data: {"id":"chatcmpl-123","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}
`))

	// Then send [DONE]
	data := []byte(`data: [DONE]`)
	result := st.TransformChunk(data)

	if !strings.Contains(result, "message_stop") {
		t.Error("expected message_stop event when [DONE]")
	}
}

func TestStreamTransformer_MultipleChunks(t *testing.T) {
	logger := slog.Default()
	st := NewStreamTransformer(logger)

	// Send multiple chunks and verify they're transformed correctly
	chunk1 := []byte(`data: {"id":"chatcmpl-123","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}
`)
	chunk2 := []byte(`data: {"id":"chatcmpl-123","choices":[{"index":0,"delta":{"content":" World"},"finish_reason":null}]}
`)

	result1 := st.TransformChunk(chunk1)
	result2 := st.TransformChunk(chunk2)

	// Both should have content_block_delta
	if !strings.Contains(result1, "Hello") {
		t.Error("expected Hello in first chunk")
	}
	if !strings.Contains(result2, "World") {
		t.Error("expected World in second chunk")
	}
}
