package main

import (
	"log/slog"
	"net"
	"os"
	"strings"
	"syscall"

	"github.com/monsterxx03/linko/pkg/config"
	"github.com/monsterxx03/linko/pkg/proxy"
	"github.com/spf13/cobra"
)

var (
	mitmLogLevel            string
	mitmWhitelist           string
	mitmListenAddr          string
	mitmAnthropicMatch      string
	mitmOpenAIMatch         string
)

var mitmCmd = &cobra.Command{
	Use:   "mitm",
	Short: "Start MITM proxy for HTTPS traffic",
	Long: `Start a MITM proxy that only intercepts HTTPS traffic on port 443.
Does not perform DNS hijacking or DNS-based traffic splitting.
Traffic is forwarded directly to target servers (no upstream proxy).

This command automatically sets up firewall rules to redirect HTTPS traffic.`,
	Run: runMITM,
}

func runMITM(cmd *cobra.Command, args []string) {
	cfg := config.DefaultConfig()

	// 使用系统默认 DNS
	if systemDNS := getSystemDNS(); len(systemDNS) > 0 {
		cfg.DNS.DomesticDNS = systemDNS
		slog.Info("using system default DNS", "dns", systemDNS)
	}
	cfg.Admin.UIEmbed = true

	if mitmWhitelist != "" {
		cfg.MITM.Whitelist = strings.Split(mitmWhitelist, ",")
		cfg.Firewall.ForceProxyHosts = strings.Split(mitmWhitelist, ",")
	}

	if err := syscall.Setgid(cfg.MITM.GID); err != nil {
		slog.Error("failed to set gid")
		os.Exit(1)
	}

	if mitmLogLevel != "" {
		cfg.Server.LogLevel = mitmLogLevel
	}

	if mitmListenAddr != "" {
		cfg.Server.ListenAddr = mitmListenAddr
	}

	if mitmAnthropicMatch != "" {
		cfg.MITM.CustomAnthropicMatches = strings.Split(mitmAnthropicMatch, ",")
	}

	if mitmOpenAIMatch != "" {
		cfg.MITM.CustomOpenAIMatches = strings.Split(mitmOpenAIMatch, ",")
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel(mitmLogLevel),
	}))
	slog.SetDefault(logger)

	// 直连模式：禁用上游代理
	cfg.Upstream.Enable = false

	// MITM 模式配置
	sc := &ServerConfig{
		DNSSplitter: nil,
		DNSCache:    nil,
		SkipCN:      false,
		ForceMITM:   true,
		EnableDNS:   false, // MITM 模式不启动 DNS 服务器
		RedirectOption: proxy.RedirectOption{
			RedirectDNS:   false,
			RedirectHTTP:  false,
			RedirectHTTPS: true,
			RedirectSSH:   false,
		},
	}

	if err := RunServer(cfg, sc, logger); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

// getSystemDNS reads the system default DNS servers from /etc/resolv.conf
func getSystemDNS() []string {
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		slog.Warn("failed to read /etc/resolv.conf, using default DNS", "error", err)
		return nil
	}

	var nameservers []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "nameserver") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				ip := parts[1]
				// Validate IP address
				if net.ParseIP(ip) != nil {
					nameservers = append(nameservers, ip)
				}
			}
		}
	}
	return nameservers
}

func init() {
	mitmCmd.Flags().StringVar(&mitmLogLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	mitmCmd.Flags().StringVar(&mitmWhitelist, "whitelist", "", "Comma-separated list of domains to MITM (e.g., 'example.com,api.example.com')")
	mitmCmd.Flags().StringVar(&mitmListenAddr, "listen", "", "Proxy listen address (default: 127.0.0.1:9810)")
	mitmCmd.Flags().StringVar(&mitmAnthropicMatch, "anthropic-match", "", "Comma-separated list of custom Anthropic API hostname/path patterns (e.g., 'api.example.com/v1/messages')")
	mitmCmd.Flags().StringVar(&mitmOpenAIMatch, "openai-match", "", "Comma-separated list of custom OpenAI API hostname/path patterns (e.g., 'api.myai.com/v1/chat/completions')")
}
