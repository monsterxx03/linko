package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/monsterxx03/linko/pkg/config"
	"github.com/monsterxx03/linko/pkg/dns"
	"github.com/monsterxx03/linko/pkg/proxy"
	"github.com/spf13/cobra"
)

var (
	configPath string
	logLevel   string
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the proxy server",
	Long:  "Start the transparent proxy server with DNS splitting and firewall rules",
	Run:   runServer,
}

func runServer(cmd *cobra.Command, args []string) {
	if os.Geteuid() != 0 {
		fmt.Println("Error: This command requires root privileges for firewall operations.")
		fmt.Println("Please run with: sudo linko serve")
		os.Exit(1)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	if logLevel != "" {
		cfg.Server.LogLevel = logLevel
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel(cfg.Server.LogLevel),
	}))
	slog.SetDefault(logger)

	// 创建 DNS 组件
	dnsCache := dns.NewDNSCache(cfg.DNS.CacheTTL, 10000)
	upstreamClient := proxy.NewUpstreamClient(cfg.Upstream)
	dnsSplitter := dns.NewDNSSplitter(
		cfg.DNS.DomesticDNS,
		cfg.DNS.ForeignDNS,
		cfg.DNS.TCPForForeign,
		upstreamClient,
	)

	sc := &ServerConfig{
		DNSSplitter: dnsSplitter,
		DNSCache:    dnsCache,
		SkipCN:      true,
		EnableDNS:   true,
		RedirectOption: proxy.RedirectOption{
			RedirectDNS:   cfg.Firewall.RedirectDNS,
			RedirectHTTP:  cfg.Firewall.RedirectHTTP,
			RedirectHTTPS: cfg.Firewall.RedirectHTTPS,
			RedirectSSH:   cfg.Firewall.RedirectSSH,
		},
	}

	if err := RunServer(cfg, sc, logger); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

func init() {
	defaultConfigPath := filepath.Join(config.GetConfigDir(), "linko.yaml")
	serveCmd.Flags().StringVarP(&configPath, "config", "c", defaultConfigPath, "Configuration file path")
	serveCmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
}
