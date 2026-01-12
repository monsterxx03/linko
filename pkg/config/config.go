package config

import (
	"net"
	"time"
)

// Config represents the main configuration for the proxy
type Config struct {
	// Server configuration
	Server ServerConfig `mapstructure:"server"`

	// DNS configuration
	DNS DNSConfig `mapstructure:"dns"`

	// Traffic statistics
	Traffic TrafficConfig `mapstructure:"traffic"`

	// Firewall configuration
	Firewall FirewallConfig `mapstructure:"firewall"`

	// Upstream proxy configuration
	Upstream UpstreamConfig `mapstructure:"upstream"`

	// Admin server configuration
	Admin AdminConfig `mapstructure:"admin"`
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

// TrafficConfig contains traffic statistics settings
type TrafficConfig struct {
	// Enable real-time traffic statistics
	EnableRealtime bool `mapstructure:"enable_realtime" yaml:"enable_realtime"`

	// Enable historical statistics
	EnableHistory bool `mapstructure:"enable_history" yaml:"enable_history"`

	// Statistics update interval
	UpdateInterval time.Duration `mapstructure:"update_interval" yaml:"update_interval"`

	// Database file path
	DBPath string `mapstructure:"db_path" yaml:"db_path"`
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

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
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
		Traffic: TrafficConfig{
			EnableRealtime: true,
			EnableHistory:  true,
			UpdateInterval: 1 * time.Second,
			DBPath:         "data/traffic.db",
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
			UIEmbed:    false,
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
