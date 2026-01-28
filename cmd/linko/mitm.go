package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/monsterxx03/linko/pkg/admin"
	"github.com/monsterxx03/linko/pkg/config"
	"github.com/monsterxx03/linko/pkg/mitm"
	"github.com/monsterxx03/linko/pkg/proxy"
	"github.com/spf13/cobra"
)

var (
	mitmConfigPath string
	mitmLogLevel   string
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
	cfg, err := config.LoadConfig(mitmConfigPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	if err := syscall.Setgid(cfg.MITM.GID); err != nil {
		slog.Error("failed to set gid")
		os.Exit(1)
	}

	if mitmLogLevel != "" {
		cfg.Server.LogLevel = mitmLogLevel
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel(cfg.Server.LogLevel),
	}))
	slog.SetDefault(logger)

	if err := config.EnsureDirectories(cfg); err != nil {
		slog.Error("failed to ensure directories", "error", err)
		os.Exit(1)
	}

	// Direct mode: disable upstream proxy
	cfg.Upstream.Enable = false
	upstreamClient := proxy.NewUpstreamClient(cfg.Upstream)

	// Start transparent proxy (HTTPS only)
	slog.Info("starting transparent proxy for MITM", "address", "127.0.0.1:"+cfg.ProxyPort())
	transparentProxy := proxy.NewTransparentProxy("127.0.0.1:"+cfg.ProxyPort(), upstreamClient)
	if err := transparentProxy.Start(); err != nil {
		slog.Error("failed to start transparent proxy", "error", err)
		os.Exit(1)
	}
	defer transparentProxy.Stop()

	// Initialize MITM Manager
	if !cfg.MITM.Enable {
		cfg.MITM.Enable = true
	}

	mitmManager, err := mitm.NewManager(mitm.ManagerConfig{
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
		os.Exit(1)
	}

	// Setup MITM handler with whitelist from config
	// If whitelist is empty, intercept all HTTPS traffic
	whitelist := cfg.MITM.Whitelist
	if len(whitelist) == 0 {
		whitelist = nil
	}

	mitmHandler := proxy.NewMITMHandler(transparentProxy, mitmManager, whitelist, logger)
	transparentProxy.SetMITMHandler(mitmHandler)

	if len(whitelist) > 0 {
		slog.Info("MITM targets configured", "targets", whitelist)
	} else {
		slog.Info("MITM enabled for all HTTPS traffic")
	}

	slog.Info("MITM CA certificate", "path", mitmManager.GetCACertificatePath())

	// Start admin server (optional, for traffic inspection)
	var adminServer *admin.AdminServer
	if cfg.Admin.Enable {
		adminServer = admin.NewAdminServer(cfg.Admin.ListenAddr, cfg.Admin.UIPath, cfg.Admin.UIEmbed, nil, mitmManager.GetEventBus())
		if err := adminServer.Start(); err != nil {
			slog.Error("failed to start admin server", "error", err)
			os.Exit(1)
		}
		defer adminServer.Stop()
	}

	// Setup firewall rules (HTTPS only)
	firewallManager := setupMITMFirewall(cfg)
	if firewallManager != nil {
		defer func() {
			deferFunc(firewallManager)
		}()
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	slog.Info("shutting down MITM server...")
}

func setupMITMFirewall(cfg *config.Config) *proxy.FirewallManager {
	slog.Info("setting up firewall rules for MITM (HTTPS only)")

	firewallManager := proxy.NewFirewallManager(
		cfg.ProxyPort(),
		cfg.DNSServerPort(),
		cfg.DNS.DomesticDNS,
		proxy.RedirectOption{
			RedirectDNS:   false,
			RedirectHTTP:  false,
			RedirectHTTPS: true,
			RedirectSSH:   false,
		},
		nil,
		cfg.MITM.GID,
		false, // skipCN: false for MITM mode (intercept all HTTPS traffic)
	)

	if err := firewallManager.SetupFirewallRules(); err != nil {
		slog.Warn("failed to setup firewall rules", "error", err)
		slog.Info("please ensure you have sudo privileges")
		return nil
	}
	slog.Info("firewall rules configured successfully (HTTPS only)")
	return firewallManager
}

func init() {
	mitmCmd.Flags().StringVarP(&mitmConfigPath, "config", "c", "config/linko.yaml", "Configuration file path")
	mitmCmd.Flags().StringVar(&mitmLogLevel, "log-level", "info", "Log level (debug, info, warn, error)")
}
