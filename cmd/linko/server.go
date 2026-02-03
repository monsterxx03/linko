package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/monsterxx03/linko/pkg/admin"
	"github.com/monsterxx03/linko/pkg/config"
	"github.com/monsterxx03/linko/pkg/dns"
	"github.com/monsterxx03/linko/pkg/mitm"
	"github.com/monsterxx03/linko/pkg/proxy"
)

// ServerConfig 通用服务器配置
type ServerConfig struct {
	DNSSplitter    *dns.DNSSplitter
	DNSCache       *dns.DNSCache
	RedirectOption proxy.RedirectOption
	SkipCN         bool // 是否跳过中国IP分流
	ForceMITM      bool // 强制启用 MITM（用于 mitm 命令）
}

// RunServer 通用服务器启动函数
func RunServer(cfg *config.Config, sc *ServerConfig, logger *slog.Logger) error {
	if err := config.EnsureDirectories(cfg); err != nil {
		return err
	}

	var transparentProxy *proxy.TransparentProxy
	var dnsServer *dns.DNSServer
	var mitmManager *mitm.Manager
	var adminServer *admin.AdminServer
	var firewallManager *proxy.FirewallManager

	// 创建 upstream client
	upstreamClient := proxy.NewUpstreamClient(cfg.Upstream)

	// 启动透明代理
	slog.Info("starting transparent proxy", "address", "127.0.0.1:"+cfg.ProxyPort())
	transparentProxy = proxy.NewTransparentProxy("127.0.0.1:"+cfg.ProxyPort(), upstreamClient)
	if err := transparentProxy.Start(); err != nil {
		return err
	}
	defer transparentProxy.Stop()

	// 启动 DNS 服务器
	slog.Info("starting DNS server", "address", cfg.DNS.ListenAddr)
	dnsServer = dns.NewDNSServer(cfg.DNS.ListenAddr, sc.DNSSplitter, sc.DNSCache)
	if err := dnsServer.Start(); err != nil {
		return err
	}
	defer dnsServer.Stop()

	// 初始化 MITM Manager
	if cfg.MITM.Enable || sc.ForceMITM {
		slog.Info("initializing MITM manager",
			"ca_cert", cfg.MITM.CACertPath,
			"cert_cache_dir", cfg.MITM.CertCacheDir,
		)

		var err error
		mitmManager, err = mitm.NewManager(mitm.ManagerConfig{
			CACertPath:       cfg.MITM.CACertPath,
			CAKeyPath:        cfg.MITM.CAKeyPath,
			CertCacheDir:     cfg.MITM.CertCacheDir,
			SiteCertValidity: cfg.MITM.SiteCertValidity,
			CACertValidity:   cfg.MITM.CACertValidity,
			Enabled:          true,
			MaxBodySize:      cfg.MITM.MaxBodySize,
			EventHistorySize: cfg.MITM.EventHistorySize,
			EnableSSEInspector:  cfg.MITM.EnableSSEInspector,
			EnableLLMInspector:  cfg.MITM.EnableLLMInspector,
		}, logger)
		if err != nil {
			slog.Error("failed to initialize MITM manager", "error", err)
		} else {
			slog.Info("MITM enabled", "ca_certificate", mitmManager.GetCACertificatePath())

			mitmHandler := proxy.NewMITMHandler(transparentProxy, mitmManager, cfg.MITM.Whitelist, logger)
			transparentProxy.SetMITMHandler(mitmHandler)
			if len(cfg.MITM.Whitelist) > 0 {
				slog.Info("MITM whitelist configured", "domains", cfg.MITM.Whitelist)
			}
		}
	}

	// 启动 Admin 服务器
	if cfg.Admin.Enable {
		slog.Info("starting admin server", "address", cfg.Admin.ListenAddr)
		var eventBus *mitm.EventBus
		if mitmManager != nil {
			eventBus = mitmManager.GetEventBus()
		}
		adminServer = admin.NewAdminServer(cfg.Admin.ListenAddr, cfg.Admin.UIPath, cfg.Admin.UIEmbed, dnsServer, eventBus)
		if err := adminServer.Start(); err != nil {
			return err
		}
		defer adminServer.Stop()
	}

	// 设置防火墙规则
	if cfg.Firewall.EnableAuto {
		firewallManager = setupFirewall(cfg, sc)
		if firewallManager != nil {
			defer func() {
				deferFunc(firewallManager)
			}()
		}
	}

	// 等待退出信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	slog.Info("shutting down server...")
	return nil
}

func setupFirewall(cfg *config.Config, sc *ServerConfig) *proxy.FirewallManager {
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
		sc.RedirectOption,
		forceProxyIPs,
		cfg.MITM.GID,
		sc.SkipCN,
	)

	if err := firewallManager.SetupFirewallRules(); err != nil {
		slog.Warn("failed to setup firewall rules", "error", err)
		slog.Info("please ensure you have sudo privileges")
		return nil
	}
	slog.Info("firewall rules configured successfully")
	return firewallManager
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
