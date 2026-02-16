package llm

import (
	"log/slog"
	"strings"
)

// ProviderMatcher defines custom matching rules for LLM providers
type ProviderMatcher struct {
	CustomAnthropicMatches []string
	CustomOpenAIMatches   []string
}

// Provider interface defines the contract for LLM API parsers
type Provider interface {
	Match(hostname, path string, body []byte) bool
	ParseResponse(path string, body []byte) (*LLMResponse, error)
	// ParseSSEStreamFrom parses SSE stream from a specific position (for incremental processing)
	ParseSSEStreamFrom(body []byte, startPos int) []TokenDelta
	// ParseFullRequest parses the request body once and returns all extracted info
	// This avoids multiple JSON unmarshaling of the same request
	ParseFullRequest(body []byte) (*RequestInfo, error)
}

// FindProvider returns the appropriate provider for the given request
func FindProvider(hostname, path string, body []byte, logger *slog.Logger) Provider {
	return FindProviderWithMatcher(hostname, path, body, logger, nil)
}

// FindProviderWithMatcher returns the appropriate provider for the given request with custom matching rules
func FindProviderWithMatcher(hostname, path string, body []byte, logger *slog.Logger, matcher *ProviderMatcher) Provider {
	providers := []Provider{
		anthropicProvider{logger: logger, customMatches: matcher},
		openaiProvider{logger: logger, customMatches: matcher},
	}

	for _, p := range providers {
		if p.Match(hostname, path, body) {
			return p
		}
	}
	return nil
}

// matchCustomPattern checks if hostname/path matches a custom pattern (format: "hostname/path")
func matchCustomPattern(hostname, path, pattern string) bool {
	// Split pattern into hostname and path parts
	parts := strings.SplitN(pattern, "/", 2)
	if len(parts) < 2 {
		// If no path specified, match any path on this hostname
		return hostname == parts[0]
	}
	patternHostname := parts[0]
	patternPath := "/" + parts[1]

	return hostname == patternHostname && strings.HasPrefix(path, patternPath)
}
