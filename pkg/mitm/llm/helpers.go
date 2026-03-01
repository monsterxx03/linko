package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func generateOpenAIConversationHash(messages []OpenAIMessage) string {
	data := ""
	for _, m := range messages {
		data += m.Role + ":"
		if content, ok := m.Content.(string); ok {
			data += content
		}
		data += "|"
	}
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:8])
}

func convertAnthropicMessages(messages []AnthropicMessage) []LLMMessage {
	var result []LLMMessage
	for _, m := range messages {
		var contentParts []string
		var toolCalls []ToolCall
		var toolResults []ToolResult

		switch c := m.Content.(type) {
		case string:
			contentParts = []string{c}
		case []any:
			for _, item := range c {
				if itemMap, ok := item.(map[string]any); ok {
					itemType, _ := itemMap["type"].(string)
					switch itemType {
					case "text":
						if text, ok := itemMap["text"].(string); ok {
							contentParts = append(contentParts, text)
						}
					case "thinking":
						if thinking, ok := itemMap["thinking"].(string); ok {
							contentParts = append(contentParts, "[Thinking]\n"+thinking+"[/Thinking]")
						}
					case "redacted_thinking":
						if thinking, ok := itemMap["thinking"].(string); ok {
							contentParts = append(contentParts, "[Redacted Thinking]\n"+thinking+"[/Redacted Thinking]")
						}
					case "tool_use":
						id, _ := itemMap["id"].(string)
						name, _ := itemMap["name"].(string)
						input, _ := itemMap["input"].(map[string]any)
						inputJSON, _ := json.Marshal(input)
						toolCalls = append(toolCalls, ToolCall{
							ID:   id,
							Type: "function",
							Function: FunctionCall{
								Name:      name,
								Arguments: string(inputJSON),
							},
						})
					case "tool_result":
						toolUseID, _ := itemMap["tool_use_id"].(string)
						content, _ := itemMap["content"].(string)
						// Extract to ToolResults field instead of contentParts
						toolResults = append(toolResults, ToolResult{
							ToolUseID: toolUseID,
							Content:   content,
						})
					}
				}
			}
		}

		result = append(result, LLMMessage{
			Role:        m.Role,
			Content:     contentParts,
			ToolCalls:   toolCalls,
			ToolResults: toolResults,
		})
	}
	return result
}

func convertOpenAIMessages(messages []OpenAIMessage) []LLMMessage {
	var result []LLMMessage
	for _, m := range messages {
		var contentParts []string
		var toolCalls []ToolCall
		var toolResults []ToolResult

		switch c := m.Content.(type) {
		case string:
			contentParts = []string{c}
		case []any:
			for _, part := range c {
				if partMap, ok := part.(map[string]any); ok {
					partType, _ := partMap["type"].(string)
					switch partType {
					case "text":
						if text, ok := partMap["text"].(string); ok {
							contentParts = append(contentParts, text)
						}
					case "image_url":
						// Handle image content: extract URL or base64 data
						if imageURL, ok := partMap["image_url"].(map[string]any); ok {
							if url, ok := imageURL["url"].(string); ok {
								contentParts = append(contentParts, "[Image: "+url+"]")
							}
						}
					case "tool_use":
						// This is typically in assistant messages with tool_calls
						id, _ := partMap["id"].(string)
						name, _ := partMap["name"].(string)
						input, _ := partMap["input"].(map[string]any)
						inputJSON, _ := json.Marshal(input)
						toolCalls = append(toolCalls, ToolCall{
							ID:   id,
							Type: "function",
							Function: FunctionCall{
								Name:      name,
								Arguments: string(inputJSON),
							},
						})
					case "tool_result":
						toolUseID, _ := partMap["tool_use_id"].(string)
						content, _ := partMap["content"].(string)
						toolResults = append(toolResults, ToolResult{
							ToolUseID: toolUseID,
							Content:   content,
						})
					}
				}
			}
		}

		// Handle ToolCalls at message level (from assistant messages)
		for _, tc := range m.ToolCalls {
			toolCalls = append(toolCalls, ToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: FunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			})
		}

		result = append(result, LLMMessage{
			Role:        m.Role,
			Content:     contentParts,
			Name:        m.Name,
			ToolCalls:   toolCalls,
			ToolResults: toolResults,
		})
	}
	return result
}
