package main

import (
	"fmt"
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
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Default config generated at %s\n", configPathFlag)
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
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runServer(cmd *cobra.Command, args []string) {
	// Load configuration
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Override log level if specified
	if logLevel != "" {
		cfg.Server.LogLevel = logLevel
	}

	// Override firewall setting if specified via flag
	if enableFirewall {
		cfg.Firewall.EnableAuto = true
	}

	// Ensure directories exist
	if err := config.EnsureDirectories(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to ensure directories: %v\n", err)
		os.Exit(1)
	}

	// Initialize GeoIP database
	fmt.Println("Initializing GeoIP database...")
	geoIP, err := ipdb.NewGeoIPManager(cfg.DNS.IPDBPath)
	if err != nil {
		fmt.Printf("Warning: Failed to initialize GeoIP database: %v\n", err)
		fmt.Println("Please download GeoIP database to enable IP geolocation features")
		// Continue without GeoIP for now
	}

	// Initialize DNS cache
	fmt.Println("Initializing DNS cache...")
	dnsCache := dns.NewDNSCache(cfg.DNS.CacheTTL, 10000)

	// Initialize DNS splitter
	fmt.Println("Initializing DNS splitter...")
	dnsSplitter := dns.NewDNSSplitter(
		geoIP,
		cfg.DNS.DomesticDNS,
		cfg.DNS.ForeignDNS,
		cfg.DNS.TCPForForeign,
	)

	// Start DNS server
	fmt.Println("Starting DNS server...")
	dnsServer := dns.NewDNSServer(cfg.DNS.ListenAddr, dnsSplitter, dnsCache)
	if err := dnsServer.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start DNS server: %v\n", err)
		os.Exit(1)
	}
	defer dnsServer.Stop()

	// Start transparent proxy (listens on firewall redirect port)
	var transparentProxy *proxy.TransparentProxy
	if cfg.Firewall.RedirectHTTP || cfg.Firewall.RedirectHTTPS {
		fmt.Println("Starting transparent proxy...")
		upstreamClient := proxy.NewUpstreamClient(cfg.Upstream)
		transparentProxy = proxy.NewTransparentProxy("127.0.0.1:"+cfg.ProxyPort(), upstreamClient, geoIP)
		if err := transparentProxy.Start(); err != nil {
			fmt.Printf("Failed to start transparent proxy: %v\n", err)
			// Continue without failing
		}
		defer transparentProxy.Stop()
	}

	// Setup firewall rules if enabled
	var firewallManager *proxy.FirewallManager
	if cfg.Firewall.EnableAuto {
		fmt.Println("Setting up firewall rules...")
		firewallManager = proxy.NewFirewallManager(
			cfg.ProxyPort(),
			cfg.DNSServerPort(),
			cfg.Firewall.RedirectDNS,
			cfg.Firewall.RedirectHTTP,
			cfg.Firewall.RedirectHTTPS,
		)
		if err := firewallManager.SetupTransparentProxy(); err != nil {
			fmt.Printf("Warning: Failed to setup firewall rules: %v\n", err)
			fmt.Println("Please ensure you have sudo privileges and try again")
			// Continue without firewall setup
		} else {
			fmt.Println("Firewall rules configured successfully")
		}
	}

	// Wait for interrupt signal
	fmt.Println("Server started successfully. Press Ctrl+C to stop.")
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("\nShutting down server...")

	// Cleanup firewall rules if they were set
	if firewallManager != nil && cfg.Firewall.EnableAuto {
		fmt.Println("Removing firewall rules...")
		if err := firewallManager.RemoveTransparentProxy(); err != nil {
			fmt.Printf("Warning: Failed to remove firewall rules: %v\n", err)
		} else {
			fmt.Println("Firewall rules removed successfully")
		}
	}

	// Cleanup
	if geoIP != nil {
		geoIP.Close()
	}

	fmt.Println("Server stopped")
}
