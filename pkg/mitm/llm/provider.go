package llm

import "log/slog"

// Provider interface defines the contract for LLM API parsers
type Provider interface {
	Match(hostname, path string, body []byte) bool
	ExtractConversationID(body []byte) string
	ParseRequest(body []byte) ([]LLMMessage, error)
	ParseResponse(path string, body []byte) (*LLMResponse, error)
	// ParseSSEStreamFrom parses SSE stream from a specific position (for incremental processing)
	ParseSSEStreamFrom(body []byte, startPos int) []TokenDelta
	ExtractSystemPrompt(body []byte) []string
	ExtractTools(body []byte) []ToolDef
}

// FindProvider returns the appropriate provider for the given request
func FindProvider(hostname, path string, body []byte, logger *slog.Logger) Provider {
	providers := []Provider{
		anthropicProvider{logger: logger},
		openaiProvider{logger: logger},
	}

	for _, p := range providers {
		if p.Match(hostname, path, body) {
			return p
		}
	}
	return nil
}
