//go:build darwin

package dns

import (
	"log/slog"
	"os/exec"
)

func clearDNSCache() {
	if err := exec.Command("killall", "-HUP", "mDNSResponder").Run(); err != nil {
		slog.Warn("Failed to clear DNS cache", "error", err)
	} else {
		slog.Info("DNS cache cleared")
	}
}
