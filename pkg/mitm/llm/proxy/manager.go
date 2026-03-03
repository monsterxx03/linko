package proxy

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/monsterxx03/linko/pkg/mitm/llm"
)

// ProxyConfig is the Anthropic to OpenAI proxy configuration
type ProxyConfig struct {
	Enabled      bool
	UpstreamURL  string
	APIKey       string
	ModelMapping map[string]string
	Timeout      time.Duration
}

// Manager manages the Anthropic to OpenAI proxy
type Manager struct {
	config *ProxyConfig
	proxy  *Proxy
	logger *slog.Logger
	once   sync.Once
}

// NewManager creates a new proxy manager
func NewManager(cfg *ProxyConfig, logger *slog.Logger) *Manager {
	return &Manager{
		config: cfg,
		logger: logger,
	}
}

// GetProxy returns the proxy instance (lazy initialization)
func (m *Manager) GetProxy() *Proxy {
	m.once.Do(func() {
		if m.config != nil && m.config.Enabled {
			m.proxy = NewProxy(m.config, m.logger)
			m.logger.Info("Anthropic to OpenAI proxy enabled", "upstream", m.config.UpstreamURL)
		}
	})
	return m.proxy
}

// IsEnabled returns whether the proxy is enabled
func (m *Manager) IsEnabled() bool {
	return m.config != nil && m.config.Enabled
}

// RoundTrip performs a full request-response transformation
func (m *Manager) RoundTrip(ctx context.Context, anthropicReq *llm.AnthropicRequest) (*llm.AnthropicResponse, error) {
	proxy := m.GetProxy()
	if proxy == nil {
		return nil, nil // Proxy not enabled
	}

	return proxy.RoundTrip(ctx, anthropicReq)
}

// RoundTripStream performs streaming request-response transformation
func (m *Manager) RoundTripStream(ctx context.Context, anthropicReq *llm.AnthropicRequest) (<-chan []byte, error) {
	proxy := m.GetProxy()
	if proxy == nil {
		return nil, nil // Proxy not enabled
	}

	return proxy.RoundTripStream(ctx, anthropicReq)
}
