package main

import (
	"log/slog"
	"os"
	"strings"
	"syscall"

	"github.com/monsterxx03/linko/pkg/config"
	"github.com/monsterxx03/linko/pkg/proxy"
	"github.com/spf13/cobra"
)

var (
	mitmLogLevel  string
	mitmWhitelist string
	mitmMode      string
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

	if mitmWhitelist != "" {
		cfg.MITM.Whitelist = strings.Split(mitmWhitelist, ",")
	}

	// Set inspector mode
	switch mitmMode {
	case "llm":
		cfg.MITM.EnableLLMInspector = true
		cfg.MITM.EnableSSEInspector = false
	default:
		cfg.MITM.EnableSSEInspector = true
		cfg.MITM.EnableLLMInspector = false
	}

	if err := syscall.Setgid(cfg.MITM.GID); err != nil {
		slog.Error("failed to set gid")
		os.Exit(1)
	}

	if mitmLogLevel != "" {
		cfg.Server.LogLevel = mitmLogLevel
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

func init() {
	mitmCmd.Flags().StringVar(&mitmLogLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	mitmCmd.Flags().StringVar(&mitmWhitelist, "whitelist", "", "Comma-separated list of domains to MITM (e.g., 'example.com,api.example.com')")
	mitmCmd.Flags().StringVar(&mitmMode, "mode", "http", "Inspector mode: http or llm")
}
