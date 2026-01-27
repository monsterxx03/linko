package main

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "linko",
	Short: "Linko - Network proxy and traffic analysis tool",
	Long: `Linko is a high-performance network proxy server with DNS splitting,
traffic analysis, and multi-protocol support.

Features:
  - Transparent proxy with DNS splitting
  - Multi-protocol support (SOCKS5, HTTP, Shadowsocks)
  - Real-time traffic analysis
  - SNI-based host extraction`,
}

func main() {
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(mitmCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(updateCnIPCmd)
	rootCmd.AddCommand(isCnIPCmd)
	rootCmd.AddCommand(genCaCmd)

	if err := rootCmd.Execute(); err != nil {
		slog.Error("failed to execute command", "error", err)
		os.Exit(1)
	}
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
