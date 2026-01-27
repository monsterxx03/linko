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
	"github.com/monsterxx03/linko/pkg/mitm"
	"github.com/monsterxx03/linko/pkg/proxy"
	"github.com/spf13/cobra"
)

var (
	configPath     string
	daemon         bool
	logLevel       string
	enableFirewall bool
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

	var transparentProxy *proxy.TransparentProxy
	if cfg.Firewall.RedirectHTTP || cfg.Firewall.RedirectHTTPS || cfg.Firewall.RedirectSSH {
		slog.Info("starting transparent proxy", "address", "127.0.0.1:"+cfg.ProxyPort())
		transparentProxy = proxy.NewTransparentProxy("127.0.0.1:"+cfg.ProxyPort(), upstreamClient)
		if err := transparentProxy.Start(); err != nil {
			slog.Error("failed to start transparent proxy", "error", err)
		}
		defer transparentProxy.Stop()
	}

	var mitmManager *mitm.Manager
	if cfg.MITM.Enable {
		slog.Info("initializing MITM manager",
			"ca_cert", cfg.MITM.CACertPath,
			"cert_cache_dir", cfg.MITM.CertCacheDir,
		)

		mitmManager, err = mitm.NewManager(mitm.ManagerConfig{
			CACertPath:       cfg.MITM.CACertPath,
			CAKeyPath:        cfg.MITM.CAKeyPath,
			CertCacheDir:     cfg.MITM.CertCacheDir,
			SiteCertValidity: cfg.MITM.SiteCertValidity,
			CACertValidity:   cfg.MITM.CACertValidity,
			Enabled:          true,
			MaxBodySize:      cfg.MITM.MaxBodySize,
		}, logger)
		if err != nil {
			slog.Error("failed to initialize MITM manager", "error", err)
		} else {
			slog.Info("MITM enabled", "ca_certificate", mitmManager.GetCACertificatePath())

			if transparentProxy != nil {
				mitmHandler := proxy.NewMITMHandler(transparentProxy, mitmManager, cfg.MITM.Whitelist, logger)
				transparentProxy.SetMITMHandler(mitmHandler)
				if len(cfg.MITM.Whitelist) > 0 {
					slog.Info("MITM whitelist configured", "domains", cfg.MITM.Whitelist)
				}
			}
		}
	}

	var adminServer *admin.AdminServer
	if cfg.Admin.Enable {
		slog.Info("starting admin server", "address", cfg.Admin.ListenAddr)
		eventBus := func() *mitm.EventBus {
			if mitmManager != nil {
				return mitmManager.GetEventBus()
			}
			return nil
		}()
		adminServer = admin.NewAdminServer(cfg.Admin.ListenAddr, cfg.Admin.UIPath, cfg.Admin.UIEmbed, dnsServer, eventBus)
		if err := adminServer.Start(); err != nil {
			slog.Error("failed to start admin server", "error", err)
			os.Exit(1)
		}
		defer adminServer.Stop()
	}

	var firewallManager *proxy.FirewallManager
	if cfg.Firewall.EnableAuto {
		firewallManager = setupFirewall(cfg)
		if firewallManager != nil {
			defer func() {
				deferFunc(firewallManager)
			}()
		}
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	slog.Info("shutting down server...")
}

func deferFunc(firewallManager *proxy.FirewallManager) {
	if r := recover(); r != nil {
		slog.Error("server panicked", "panic", r)
	}
	slog.Info("cleaning up firewall rules")
	if err := firewallManager.CleanupFirewallRules(); err != nil {
		slog.Warn("failed to remove firewall rules", "error", err)
	} else {
		slog.Info("firewall rules removed successfully")
	}
}

func setupFirewall(cfg *config.Config) *proxy.FirewallManager {
	slog.Info("setting up firewall rules")

	forceProxyIPs, err := proxy.ResolveHosts(cfg.Firewall.ForceProxyHosts, cfg.DNS.DomesticDNS)
	if err != nil {
		slog.Warn("failed to resolve force proxy hosts", "error", err)
	} else if len(forceProxyIPs) > 0 {
		slog.Info("resolved force proxy hosts to IPs", "ips", forceProxyIPs)
	}

	firewallManager := proxy.NewFirewallManager(
		cfg.ProxyPort(),
		cfg.DNSServerPort(),
		cfg.DNS.DomesticDNS,
		proxy.RedirectOption{
			RedirectDNS:   cfg.Firewall.RedirectDNS,
			RedirectHTTP:  cfg.Firewall.RedirectHTTP,
			RedirectHTTPS: cfg.Firewall.RedirectHTTPS,
			RedirectSSH:   cfg.Firewall.RedirectSSH,
		},
		forceProxyIPs,
		cfg.MITM.GID,
	)

	if err := firewallManager.SetupFirewallRules(); err != nil {
		slog.Warn("failed to setup firewall rules", "error", err)
		slog.Info("please ensure you have sudo privileges")
		return nil
	}
	slog.Info("firewall rules configured successfully")
	return firewallManager
}

func init() {
	serveCmd.Flags().StringVarP(&configPath, "config", "c", "config/linko.yaml", "Configuration file path")
	serveCmd.Flags().BoolVarP(&daemon, "daemon", "d", false, "Run as daemon")
	serveCmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	serveCmd.Flags().BoolVar(&enableFirewall, "firewall", false, "Enable automatic firewall rule setup (requires sudo)")
}
