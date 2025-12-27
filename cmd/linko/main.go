package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/monsterxx03/linko/pkg/config"
	"github.com/monsterxx03/linko/pkg/dns"
	"github.com/monsterxx03/linko/pkg/ipdb"
	"github.com/monsterxx03/linko/pkg/proxy"
	"github.com/spf13/cobra"
)

var (
	configPath     string
	daemon         bool
	logLevel       string
	enableFirewall bool
)

var rootCmd = &cobra.Command{
	Use:   "linko",
	Short: "Linko - Network proxy and traffic analysis tool",
	Long: `Linko is a high-performance network proxy server with DNS splitting,
traffic analysis, and multi-protocol support.

Features:
  • Transparent proxy with DNS splitting
  • Multi-protocol support (SOCKS5, HTTP, Shadowsocks)
  • Real-time traffic analysis
  • SNI-based host extraction`,
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the proxy server",
	Long:  "Start the transparent proxy server with DNS splitting and firewall rules",
	Run:   runServer,
}

var configPathFlag string

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Generate default configuration file",
	Long:  "Generate a default configuration file at the specified path",
	Run: func(cmd *cobra.Command, args []string) {
		if err := config.GenerateConfig(configPathFlag); err != nil {
			slog.Error("failed to generate config", "error", err)
			os.Exit(1)
		}
		slog.Info("default config generated", "path", configPathFlag)
	},
}

func main() {
	serveCmd.Flags().StringVarP(&configPath, "config", "c", "config/linko.yaml", "Configuration file path")
	serveCmd.Flags().BoolVarP(&daemon, "daemon", "d", false, "Run as daemon")
	serveCmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	serveCmd.Flags().BoolVar(&enableFirewall, "firewall", false, "Enable automatic firewall rule setup (requires sudo)")

	configCmd.Flags().StringVarP(&configPathFlag, "output", "o", "config/linko.yaml", "Output configuration file path")

	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(configCmd)

	if err := rootCmd.Execute(); err != nil {
		slog.Error("failed to execute command", "error", err)
		os.Exit(1)
	}
}

func runServer(cmd *cobra.Command, args []string) {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	if logLevel != "" {
		cfg.Server.LogLevel = logLevel
	}

	if enableFirewall {
		cfg.Firewall.EnableAuto = true
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel(cfg.Server.LogLevel),
	}))
	slog.SetDefault(logger)

	if err := config.EnsureDirectories(cfg); err != nil {
		slog.Error("failed to ensure directories", "error", err)
		os.Exit(1)
	}

	slog.Info("initializing GeoIP database")
	geoIP, err := ipdb.NewGeoIPManager(cfg.DNS.IPDBPath)
	if err != nil {
		slog.Warn("failed to initialize GeoIP database", "error", err)
		slog.Info("please download GeoIP database to enable IP geolocation features")
	}

	slog.Info("initializing DNS cache")
	dnsCache := dns.NewDNSCache(cfg.DNS.CacheTTL, 10000)

	slog.Info("initializing DNS splitter")
	dnsSplitter := dns.NewDNSSplitter(
		geoIP,
		cfg.DNS.DomesticDNS,
		cfg.DNS.ForeignDNS,
		cfg.DNS.TCPForForeign,
	)

	slog.Info("starting DNS server", "address", cfg.DNS.ListenAddr)
	dnsServer := dns.NewDNSServer(cfg.DNS.ListenAddr, dnsSplitter, dnsCache)
	if err := dnsServer.Start(); err != nil {
		slog.Error("failed to start DNS server", "error", err)
		os.Exit(1)
	}
	defer dnsServer.Stop()

	var transparentProxy *proxy.TransparentProxy
	if cfg.Firewall.RedirectHTTP || cfg.Firewall.RedirectHTTPS {
		slog.Info("starting transparent proxy", "address", "127.0.0.1:"+cfg.ProxyPort())
		upstreamClient := proxy.NewUpstreamClient(cfg.Upstream)
		transparentProxy = proxy.NewTransparentProxy("127.0.0.1:"+cfg.ProxyPort(), upstreamClient, geoIP)
		if err := transparentProxy.Start(); err != nil {
			slog.Error("failed to start transparent proxy", "error", err)
		}
		defer transparentProxy.Stop()
	}

	var firewallManager *proxy.FirewallManager
	if cfg.Firewall.EnableAuto {
		slog.Info("setting up firewall rules")
		firewallManager = proxy.NewFirewallManager(
			cfg.ProxyPort(),
			cfg.DNSServerPort(),
			cfg.Firewall.RedirectDNS,
			cfg.Firewall.RedirectHTTP,
			cfg.Firewall.RedirectHTTPS,
		)
		if err := firewallManager.SetupTransparentProxy(); err != nil {
			slog.Warn("failed to setup firewall rules", "error", err)
			slog.Info("please ensure you have sudo privileges")
		} else {
			slog.Info("firewall rules configured successfully")
		}
	}

	slog.Info("server started successfully. press Ctrl+C to stop")
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server")

	if firewallManager != nil && cfg.Firewall.EnableAuto {
		slog.Info("removing firewall rules")
		if err := firewallManager.RemoveTransparentProxy(); err != nil {
			slog.Warn("failed to remove firewall rules", "error", err)
		} else {
			slog.Info("firewall rules removed successfully")
		}
	}

	if geoIP != nil {
		geoIP.Close()
	}

	slog.Info("server stopped")
}

func parseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
