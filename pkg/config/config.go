package config

import (
	"net"
	"os"
	"path/filepath"
	"time"
)

// GetConfigDir returns the default configuration directory (~/.config/linko)
func GetConfigDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "config"
	}
	return filepath.Join(homeDir, ".config", "linko")
}

// Config represents the main configuration for the proxy
type Config struct {
	// Server configuration
	Server ServerConfig `mapstructure:"server"`

	// DNS configuration
	DNS DNSConfig `mapstructure:"dns"`

	// Firewall configuration
	Firewall FirewallConfig `mapstructure:"firewall"`

	// Upstream proxy configuration
	Upstream UpstreamConfig `mapstructure:"upstream"`

	// Admin server configuration
	Admin AdminConfig `mapstructure:"admin"`

	// MITM configuration
	MITM MITMConfig `mapstructure:"mitm"`
}

// ServerConfig contains server-related settings
type ServerConfig struct {
	// Listen address for the main proxy server
	ListenAddr string `mapstructure:"listen_addr" yaml:"listen_addr"`

	// Log level (debug, info, warn, error)
	LogLevel string `mapstructure:"log_level" yaml:"log_level"`
}

// DNSConfig contains DNS分流 settings
type DNSConfig struct {
	// Listen address for DNS server
	ListenAddr string `mapstructure:"listen_addr" yaml:"listen_addr"`

	// Domestic DNS servers (China)
	DomesticDNS []string `mapstructure:"domestic_dns" yaml:"domestic_dns"`

	// Foreign DNS servers (International)
	ForeignDNS []string `mapstructure:"foreign_dns" yaml:"foreign_dns"`

	// DNS cache TTL
	CacheTTL time.Duration `mapstructure:"cache_ttl" yaml:"cache_ttl"`

	// Enable DNS over TCP for foreign queries
	TCPForForeign bool `mapstructure:"tcp_for_foreign" yaml:"tcp_for_foreign"`
}

// FirewallConfig contains firewall-related settings
type FirewallConfig struct {
	// Enable automatic firewall rule management
	EnableAuto bool `mapstructure:"enable_auto" yaml:"enable_auto"`

	// Enable DNS redirect (UDP 53 -> local DNS server)
	RedirectDNS bool `mapstructure:"redirect_dns" yaml:"redirect_dns"`

	// Enable HTTP redirect
	RedirectHTTP bool `mapstructure:"redirect_http" yaml:"redirect_http"`

	// Enable HTTPS redirect
	RedirectHTTPS bool `mapstructure:"redirect_https" yaml:"redirect_https"`

	// Enable SSH redirect (TCP 22 -> proxy)
	RedirectSSH bool `mapstructure:"redirect_ssh" yaml:"redirect_ssh"`

	// ForceProxyHosts is a list of domains or IPs that should always be proxied
	// These hosts will not be added to the reserved list and will always be redirected
	ForceProxyHosts []string `mapstructure:"force_proxy_hosts" yaml:"force_proxy_hosts"`
}

// UpstreamConfig contains upstream proxy settings
type UpstreamConfig struct {
	// Enable upstream proxy
	Enable bool `mapstructure:"enable" yaml:"enable"`

	// Upstream proxy type (socks5, http)
	Type string `mapstructure:"type" yaml:"type"`

	// Upstream proxy address (host:port)
	Addr string `mapstructure:"addr" yaml:"addr"`

	// Username for upstream proxy (optional)
	Username string `mapstructure:"username" yaml:"username"`

	// Password for upstream proxy (optional)
	Password string `mapstructure:"password" yaml:"password"`
}

// AdminConfig contains admin server settings
type AdminConfig struct {
	// Enable admin server
	Enable bool `mapstructure:"enable" yaml:"enable"`

	// Admin server listen address
	ListenAddr string `mapstructure:"listen_addr" yaml:"listen_addr"`

	// UI directory path for static files
	UIPath string `mapstructure:"ui_path" yaml:"ui_path"`

	// UI embed mode - serve embedded HTML directly
	UIEmbed bool `mapstructure:"ui_embed" yaml:"ui_embed"`
}

// MITMConfig contains MITM proxy settings
type MITMConfig struct {
	// Enable MITM functionality
	Enable bool `mapstructure:"enable" yaml:"enable"`

	// Setgid after start
	GID int `mapstructure:"gid" yaml:"gid"`

	// CA certificate path
	CACertPath string `mapstructure:"ca_cert_path" yaml:"ca_cert_path"`

	// CA private key path
	CAKeyPath string `mapstructure:"ca_key_path" yaml:"ca_key_path"`

	// Site certificate cache directory
	CertCacheDir string `mapstructure:"cert_cache_dir" yaml:"cert_cache_dir"`

	// Site certificate validity duration
	SiteCertValidity time.Duration `mapstructure:"site_cert_validity" yaml:"site_cert_validity"`

	// CA certificate validity duration (default: 365 days)
	CACertValidity time.Duration `mapstructure:"ca_cert_validity" yaml:"ca_cert_validity"`

	// Whitelist of domains to perform MITM on
	// If empty, MITM is performed on all HTTPS traffic
	// If specified, only traffic to these domains will be MITM'd
	Whitelist []string `mapstructure:"whitelist" yaml:"whitelist"`

	// MaxBodySize is the maximum body size to capture for inspection (0 = unlimited)
	MaxBodySize int64 `mapstructure:"max_body_size" yaml:"max_body_size"`

	// EventHistorySize is the number of events to keep in history for replay (default: 10)
	EventHistorySize int `mapstructure:"event_history_size" yaml:"event_history_size"`

	// LLMEventHistorySize is the number of LLM events to keep in history for replay (default: 10)
	LLMEventHistorySize int `mapstructure:"llm_event_history_size" yaml:"llm_event_history_size"`

	// CustomAnthropicMatches is a list of custom hostname/path patterns for Anthropic API matching
	// Format: "hostname/path" (e.g., "api.example.com/v1/messages")
	// These patterns will be matched in addition to the built-in Anthropic-compatible APIs
	CustomAnthropicMatches []string `mapstructure:"custom_anthropic_matches" yaml:"custom_anthropic_matches"`

	// CustomOpenAIMatches is a list of custom hostname/path patterns for OpenAI API matching
	// Format: "hostname/path" (e.g., "api.myai.com/v1/chat/completions")
	// These patterns will be matched in addition to the built-in OpenAI-compatible APIs
	CustomOpenAIMatches []string `mapstructure:"custom_openai_matches" yaml:"custom_openai_matches"`
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	configDir := GetConfigDir()
	certsDir := filepath.Join(configDir, "certs")

	return &Config{
		Server: ServerConfig{
			ListenAddr: "127.0.0.1:9890",
			LogLevel:   "info",
		},
		DNS: DNSConfig{
			ListenAddr:    "127.0.0.1:6363",
			DomesticDNS:   []string{"223.5.5.5", "114.114.114.114"},
			ForeignDNS:    []string{"8.8.8.8", "1.1.1.1"},
			CacheTTL:      5 * time.Minute,
			TCPForForeign: true,
		},
		Firewall: FirewallConfig{
			EnableAuto:    true,
			RedirectDNS:   true,
			RedirectHTTP:  true,
			RedirectHTTPS: true,
			RedirectSSH:   false,
		},
		Upstream: UpstreamConfig{
			Enable:   true,
			Type:     "socks5",
			Addr:     "127.0.0.1:7891",
			Username: "",
			Password: "",
		},
		Admin: AdminConfig{
			Enable:     true,
			ListenAddr: "0.0.0.0:9810",
			UIPath:     "pkg/ui",
			UIEmbed:    true,
		},
		MITM: MITMConfig{
			Enable:              false,
			GID:                 8001,
			CACertPath:          filepath.Join(certsDir, "ca.crt"),
			CAKeyPath:           filepath.Join(certsDir, "ca.key"),
			CertCacheDir:        filepath.Join(certsDir, "sites"),
			SiteCertValidity:    168 * time.Hour,      // 7 days
			CACertValidity:      365 * 24 * time.Hour, // 365 days
			MaxBodySize:         2097152,              // 2M default
			EventHistorySize:    10,                   // Default 10 historical events
			LLMEventHistorySize: 10,                   // Default 10 LLM historical events
		},
	}
}

func extractPort(listenAddr string) string {
	_, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return ""
	}
	return port
}

func (c *Config) ProxyPort() string {
	return extractPort(c.Server.ListenAddr)
}

func (c *Config) DNSServerPort() string {
	return extractPort(c.DNS.ListenAddr)
}
