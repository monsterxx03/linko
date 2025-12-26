package config

import (
	"time"
)

// Config represents the main configuration for the proxy
type Config struct {
	// Server configuration
	Server ServerConfig `mapstructure:"server" json:"server"`

	// DNS configuration
	DNS DNSConfig `mapstructure:"dns" json:"dns"`

	// Proxy protocols
	Proxy ProxyConfig `mapstructure:"proxy" json:"proxy"`

	// Traffic statistics
	Traffic TrafficConfig `mapstructure:"traffic" json:"traffic"`
}

// ServerConfig contains server-related settings
type ServerConfig struct {
	// Listen address for the main proxy server
	ListenAddr string `mapstructure:"listen_addr" json:"listen_addr"`

	// HTTP API port for management interface
	AdminPort int `mapstructure:"admin_port" json:"admin_port"`

	// Log level (debug, info, warn, error)
	LogLevel string `mapstructure:"log_level" json:"log_level"`
}

// DNSConfig contains DNS分流 settings
type DNSConfig struct {
	// Listen address for DNS server
	ListenAddr string `mapstructure:"listen_addr" json:"listen_addr"`

	// Domestic DNS servers (China)
	DomesticDNS []string `mapstructure:"domestic_dns" json:"domestic_dns"`

	// Foreign DNS servers (International)
	ForeignDNS []string `mapstructure:"foreign_dns" json:"foreign_dns"`

	// IP database file path
	IPDBPath string `mapstructure:"ipdb_path" json:"ipdb_path"`

	// DNS cache TTL
	CacheTTL time.Duration `mapstructure:"cache_ttl" json:"cache_ttl"`

	// Enable DNS over TCP for foreign queries
	TCPForForeign bool `mapstructure:"tcp_for_foreign" json:"tcp_for_foreign"`
}

// ProxyConfig contains proxy protocol settings
type ProxyConfig struct {
	// Enable SOCKS5 proxy
	SOCKS5 bool `mapstructure:"socks5" json:"socks5"`

	// Enable HTTP CONNECT tunnel
	HTTPTunnel bool `mapstructure:"http_tunnel" json:"http_tunnel"`

	// Enable Shadowsocks
	Shadowsocks bool `mapstructure:"shadowsocks" json:"shadowsocks"`

	// Shadowsocks configuration
	ShadowsocksConfig *ShadowsocksConfig `mapstructure:"shadowsocks_config" json:"shadowsocks_config,omitempty"`
}

// ShadowsocksConfig contains Shadowsocks-specific settings
type ShadowsocksConfig struct {
	Method   string `mapstructure:"method" json:"method"`
	Password string `mapstructure:"password" json:"password"`
	Port     int    `mapstructure:"port" json:"port"`
}

// TrafficConfig contains traffic statistics settings
type TrafficConfig struct {
	// Enable real-time traffic statistics
	EnableRealtime bool `mapstructure:"enable_realtime" json:"enable_realtime"`

	// Enable historical statistics
	EnableHistory bool `mapstructure:"enable_history" json:"enable_history"`

	// Statistics update interval
	UpdateInterval time.Duration `mapstructure:"update_interval" json:"update_interval"`

	// Database file path
	DBPath string `mapstructure:"db_path" json:"db_path"`
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			ListenAddr: "127.0.0.1:7890",
			AdminPort:  8080,
			LogLevel:   "info",
		},
		DNS: DNSConfig{
			ListenAddr:     "127.0.0.1:5353",
			DomesticDNS:    []string{"114.114.114.114", "223.5.5.5"},
			ForeignDNS:     []string{"8.8.8.8", "1.1.1.1"},
			IPDBPath:       "data/geoip.mmdb",
			CacheTTL:       5 * time.Minute,
			TCPForForeign:  true,
		},
		Proxy: ProxyConfig{
			SOCKS5:      true,
			HTTPTunnel:  true,
			Shadowsocks: false,
		},
		Traffic: TrafficConfig{
			EnableRealtime: true,
			EnableHistory:  true,
			UpdateInterval: 1 * time.Second,
			DBPath:         "data/traffic.db",
		},
	}
}