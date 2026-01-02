package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/monsterxx03/linko/pkg/admin"
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

var updateCnIPCmd = &cobra.Command{
	Use:   "update-cn-ip",
	Short: "Download China IP ranges from APNIC",
	Long:  "Fetch the latest China IP address ranges from APNIC and save to data directory",
	Run: func(cmd *cobra.Command, args []string) {
		slog.Info("fetching China IP ranges from APNIC...")
		if err := ipdb.FetchChinaIPRanges(); err != nil {
			slog.Error("failed to fetch China IP ranges", "error", err)
			os.Exit(1)
		}
		slog.Info("China IP ranges updated successfully", "output_dir", "pkg/ipdb/china_ip_ranges.go")
	},
}

var isCnIPCmd = &cobra.Command{
	Use:   "is-cn-ip <ip>",
	Short: "Check if an IP is a China IP address",
	Long:  "Check if the given IP address falls within China IP ranges",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := ipdb.LoadChinaIPRanges(); err != nil {
			slog.Error("failed to load China IP ranges", "error", err)
			os.Exit(1)
		}
		ip := args[0]
		isCN := ipdb.IsChinaIP(ip)
		if isCN {
			fmt.Printf("%s is a China IP address\n", ip)
		} else {
			fmt.Printf("%s is NOT a China IP address\n", ip)
		}
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
	rootCmd.AddCommand(updateCnIPCmd)
	rootCmd.AddCommand(isCnIPCmd)

	if err := rootCmd.Execute(); err != nil {
		slog.Error("failed to execute command", "error", err)
		os.Exit(1)
	}
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

	slog.Info("initializing DNS cache")
	dnsCache := dns.NewDNSCache(cfg.DNS.CacheTTL, 10000)

	upstreamClient := proxy.NewUpstreamClient(cfg.Upstream)

	slog.Info("initializing DNS splitter")
	dnsSplitter := dns.NewDNSSplitter(
		cfg.DNS.DomesticDNS,
		cfg.DNS.ForeignDNS,
		cfg.DNS.TCPForForeign,
		upstreamClient,
	)

	slog.Info("starting DNS server", "address", cfg.DNS.ListenAddr)
	dnsServer := dns.NewDNSServer(cfg.DNS.ListenAddr, dnsSplitter, dnsCache)
	if err := dnsServer.Start(); err != nil {
		slog.Error("failed to start DNS server", "error", err)
		os.Exit(1)
	}
	defer dnsServer.Stop()

	var adminServer *admin.AdminServer
	if cfg.Admin.Enable {
		slog.Info("starting admin server", "address", cfg.Admin.ListenAddr)
		adminServer = admin.NewAdminServer(cfg.Admin.ListenAddr, dnsServer)
		if err := adminServer.Start(); err != nil {
			slog.Error("failed to start admin server", "error", err)
			os.Exit(1)
		}
		defer adminServer.Stop()
	}

	var transparentProxy *proxy.TransparentProxy
	if cfg.Firewall.RedirectHTTP || cfg.Firewall.RedirectHTTPS {
		slog.Info("starting transparent proxy", "address", "127.0.0.1:"+cfg.ProxyPort())
		transparentProxy = proxy.NewTransparentProxy("127.0.0.1:"+cfg.ProxyPort(), upstreamClient)
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
			cfg.DNS.DomesticDNS,
			cfg.Firewall.RedirectDNS,
			cfg.Firewall.RedirectHTTP,
			cfg.Firewall.RedirectHTTPS,
		)
		if err := firewallManager.SetupFirewallRules(); err != nil {
			slog.Warn("failed to setup firewall rules", "error", err)
			slog.Info("please ensure you have sudo privileges")
		} else {
			slog.Info("firewall rules configured successfully")
		}

		defer func() {
			if r := recover(); r != nil {
				slog.Error("server panicked", "panic", r)
			}
			slog.Info("cleaning up firewall rules")
			if err := firewallManager.CleanupFirewallRules(); err != nil {
				slog.Warn("failed to remove firewall rules", "error", err)
			} else {
				slog.Info("firewall rules removed successfully")
			}
		}()
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	slog.Info("shutting down server...")
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
