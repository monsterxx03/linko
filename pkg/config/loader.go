package config

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

func LoadConfig(configPath string) (*Config, error) {
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	config := DefaultConfig()

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := SaveConfig(configPath, config); err != nil {
			return nil, fmt.Errorf("failed to create default config: %w", err)
		}
		return config, nil
	}

	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	if err := v.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
}

func SaveConfig(configPath string, config *Config) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

func GenerateConfig(configPath string) error {
	config := DefaultConfig()
	return SaveConfig(configPath, config)
}

func GetConfigHash(configPath string) (string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}

	hash := md5.Sum(data)
	return hex.EncodeToString(hash[:]), nil
}

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

	dataDir := filepath.Dir(config.DNS.IPDBPath)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	return nil
}

func ConfigExists(configPath string) (bool, error) {
	_, err := os.Stat(configPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

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
