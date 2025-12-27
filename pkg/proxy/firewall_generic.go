//go:build !darwin && !linux
// +build !darwin,!linux

package proxy

import (
	"fmt"
	"runtime"
)

func (f *FirewallManager) SetupTransparentProxy() error {
	return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
}

func (f *FirewallManager) RemoveTransparentProxy() error {
	return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
}

func (f *FirewallManager) CheckFirewallStatus() (map[string]interface{}, error) {
	return map[string]interface{}{
		"enabled": false,
		"error":   fmt.Sprintf("unsupported operating system: %s", runtime.GOOS),
	}, nil
}

func (f *FirewallManager) GetCurrentRules() ([]FirewallRule, error) {
	return nil, fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
}

func (f *FirewallManager) parseMacOSRules(output string) []FirewallRule {
	return nil
}

func (f *FirewallManager) parseLinuxRules(output string) []FirewallRule {
	return nil
}
