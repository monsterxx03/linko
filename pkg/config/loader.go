package config

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// LoadConfig loads configuration from file
func LoadConfig(configPath string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")
	v.SetDefault("server.listen_addr", "127.0.0.1:7890")
	v.SetDefault("server.admin_port", 8080)
	v.SetDefault("server.log_level", "info")
	v.SetDefault("dns.listen_addr", "127.0.0.1:5353")
	v.SetDefault("dns.domestic_dns", []string{"114.114.114.114", "223.5.5.5"})
	v.SetDefault("dns.foreign_dns", []string{"8.8.8.8", "1.1.1.1"})
	v.SetDefault("dns.cache_ttl", "5m")
	v.SetDefault("dns.tcp_for_foreign", true)
	v.SetDefault("proxy.socks5", true)
	v.SetDefault("proxy.http_tunnel", true)
	v.SetDefault("proxy.shadowsocks", false)
	v.SetDefault("traffic.enable_realtime", true)
	v.SetDefault("traffic.enable_history", true)
	v.SetDefault("traffic.update_interval", "1s")
	v.SetDefault("firewall.enable_auto", false)
	v.SetDefault("firewall.proxy_port", "7890")
	v.SetDefault("firewall.redirect_http", true)
	v.SetDefault("firewall.redirect_https", true)
	v.SetDefault("upstream.enable", false)
	v.SetDefault("upstream.type", "socks5")
	v.SetDefault("upstream.addr", "127.0.0.1:1080")
	v.SetDefault("upstream.username", "")
	v.SetDefault("upstream.password", "")

	// Create config directory if it doesn't exist
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	// If config file doesn't exist, create default
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		defaultConfig := DefaultConfig()
		if err := SaveConfig(configPath, defaultConfig); err != nil {
			return nil, fmt.Errorf("failed to create default config: %w", err)
		}
	}

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate configuration
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &config, nil
}

// SaveConfig saves configuration to file
func SaveConfig(configPath string, config *Config) error {
	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")
	v.Set("server", config.Server)
	v.Set("dns", config.DNS)
	v.Set("proxy", config.Proxy)
	v.Set("traffic", config.Traffic)
	v.Set("firewall", config.Firewall)
	v.Set("upstream", config.Upstream)

	if err := v.WriteConfig(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	return nil
}

// GenerateConfig generates a sample configuration file
func GenerateConfig(configPath string) error {
	config := DefaultConfig()
	return SaveConfig(configPath, config)
}

// GetConfigHash returns MD5 hash of config file
func GetConfigHash(configPath string) (string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}

	hash := md5.Sum(data)
	return hex.EncodeToString(hash[:]), nil
}

// validateConfig validates the configuration
func validateConfig(config *Config) error {
	if config.Server.ListenAddr == "" {
		return fmt.Errorf("server listen address cannot be empty")
	}

	if config.DNS.ListenAddr == "" {
		return fmt.Errorf("DNS listen address cannot be empty")
	}

	if len(config.DNS.DomesticDNS) == 0 {
		return fmt.Errorf("at least one domestic DNS server is required")
	}

	if len(config.DNS.ForeignDNS) == 0 {
		return fmt.Errorf("at least one foreign DNS server is required")
	}

	if config.DNS.IPDBPath == "" {
		config.DNS.IPDBPath = "data/geoip.mmdb"
	}

	if config.Traffic.DBPath == "" {
		config.Traffic.DBPath = "data/traffic.db"
	}

	// Create data directory if it doesn't exist
	dataDir := filepath.Dir(config.DNS.IPDBPath)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	return nil
}

// ConfigExists checks if configuration file exists
func ConfigExists(configPath string) (bool, error) {
	_, err := os.Stat(configPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

// EnsureDirectories ensures all required directories exist
func EnsureDirectories(config *Config) error {
	dirs := []string{
		filepath.Dir(config.DNS.IPDBPath),
		filepath.Dir(config.Traffic.DBPath),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}