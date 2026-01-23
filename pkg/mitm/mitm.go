package mitm

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Manager is the main MITM manager
type Manager struct {
	certManager     *CertManager
	siteCertManager *SiteCertManager
	logger          *slog.Logger
	enabled         bool
	inspector       *InspectorChain
	eventBus        *EventBus
	mu              sync.RWMutex
}

// ManagerConfig contains MITM manager configuration
type ManagerConfig struct {
	CACertPath       string
	CAKeyPath        string
	CertCacheDir     string
	SiteCertValidity time.Duration
	CACertValidity   time.Duration
	Enabled          bool
	MaxBodySize      int64
}

// NewManager creates a new MITM manager
func NewManager(config ManagerConfig, logger *slog.Logger) (*Manager, error) {
	// Default site certificate validity is 7 days
	siteValidity := 168 * time.Hour
	if config.SiteCertValidity > 0 {
		siteValidity = config.SiteCertValidity
	}

	// Default CA certificate validity is 365 days
	caValidity := 365 * 24 * time.Hour
	if config.CACertValidity > 0 {
		caValidity = config.CACertValidity
	}

	// Create certificate manager (loads or creates CA)
	certManager, err := NewCertManager(config.CACertPath, config.CAKeyPath, caValidity)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate manager: %w", err)
	}

	// Create site certificate manager using CA's private key for signing
	// The CA key (RSA or ECDSA) is obtained from certManager
	siteCertManager, err := NewSiteCertManager(
		certManager.GetCACertificate(),
		certManager.GetCAPrivateKey(), // Use CA's actual private key
		config.CertCacheDir,
		siteValidity,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create site certificate manager: %w", err)
	}

	m := &Manager{
		certManager:     certManager,
		siteCertManager: siteCertManager,
		logger:          logger,
		enabled:         config.Enabled,
		inspector:       NewInspectorChain(),
		eventBus:        NewEventBus(1000), // Create event bus with buffer size 1000
	}

	m.inspector.Add(NewSSEInspector(logger, m.eventBus, "", config.MaxBodySize))

	return m, nil
}

// GetCertManager returns the certificate manager
func (m *Manager) GetCertManager() *CertManager {
	return m.certManager
}

// GetSiteCertManager returns the site certificate manager
func (m *Manager) GetSiteCertManager() *SiteCertManager {
	return m.siteCertManager
}

// IsEnabled returns whether MITM is enabled
func (m *Manager) IsEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.enabled
}

// SetEnabled enables or disables MITM
func (m *Manager) SetEnabled(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enabled = enabled
}

// GetCACertificatePath returns the CA certificate path
func (m *Manager) GetCACertificatePath() string {
	return m.certManager.GetCACertPath()
}

// ConnectionHandler returns a new connection handler
func (m *Manager) ConnectionHandler(upstream UpstreamClient) *ConnectionHandler {
	return NewConnectionHandler(m.siteCertManager, m.logger, upstream, m.inspector, nil)
}

// ConnectionHandlerWithPeekReader returns a connection handler that uses the provided PeekReader
func (m *Manager) ConnectionHandlerWithPeekReader(upstream UpstreamClient, peekReader *PeekReader) *ConnectionHandler {
	return NewConnectionHandler(m.siteCertManager, m.logger, upstream, m.inspector, peekReader)
}

// Statistics holds MITM statistics
type Statistics struct {
	TotalConnections  uint64
	ActiveConnections uint64
	InspectedBytes    uint64
	CertsGenerated    uint64
}

// GetStatistics returns MITM statistics
func (m *Manager) GetStatistics() Statistics {
	return Statistics{
		CertsGenerated: 0, // TODO: Add atomic counter
	}
}

// GetEventBus returns the event bus for traffic events
func (m *Manager) GetEventBus() *EventBus {
	return m.eventBus
}
