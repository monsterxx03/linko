package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func generateConversationHash(messages []AnthropicMessage) string {
	data := ""
	for _, m := range messages {
		switch c := m.Content.(type) {
		case string:
			data += c
		case []AnthropicContent:
			for _, item := range c {
				data += item.Text
			}
		}
	}
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:8])
}

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
		switch c := m.Content.(type) {
		case string:
			contentParts = []string{c}
		case []any:
			for _, part := range c {
				if partMap, ok := part.(map[string]any); ok {
					if text, ok := partMap["text"].(string); ok {
						contentParts = append(contentParts, text)
					}
				}
			}
		}
		result = append(result, LLMMessage{
			Role:    m.Role,
			Content: contentParts,
			Name:    m.Name,
		})
	}
	return result
}
