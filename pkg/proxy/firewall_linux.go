//go:build linux
// +build linux

package proxy

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

func (f *FirewallManager) SetupTransparentProxy() error {
	cmd := exec.Command("sudo", "sh", "-c", "echo 1 > /proc/sys/net/ipv4/ip_forward")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to enable IP forwarding: %w", err)
	}

	rules := []string{
		fmt.Sprintf("iptables -t nat -A OUTPUT -p tcp --dport 80 -j REDIRECT --to-port %s", f.proxyPort),
		fmt.Sprintf("iptables -t nat -A OUTPUT -p tcp --dport 443 -j REDIRECT --to-port %s", f.proxyPort),
		fmt.Sprintf("iptables -A INPUT -p tcp --dport %s -j ACCEPT", f.proxyPort),
	}

	for _, rule := range rules {
		cmd := exec.Command("sudo", "sh", "-c", rule)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to execute rule %s: %w", rule, err)
		}
	}

	return nil
}

func (f *FirewallManager) RemoveTransparentProxy() error {
	rules := []string{
		fmt.Sprintf("iptables -t nat -D OUTPUT -p tcp --dport 80 -j REDIRECT --to-port %s", f.proxyPort),
		fmt.Sprintf("iptables -t nat -D OUTPUT -p tcp --dport 443 -j REDIRECT --to-port %s", f.proxyPort),
		fmt.Sprintf("iptables -D INPUT -p tcp --dport %s -j ACCEPT", f.proxyPort),
	}

	for _, rule := range rules {
		cmd := exec.Command("sudo", "sh", "-c", rule)
		if err := cmd.Run(); err != nil {
			continue
		}
	}

	return nil
}

func (f *FirewallManager) CheckFirewallStatus() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	cmd := exec.Command("sudo", "iptables", "-t", "nat", "-L", "-n", "-v")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		stats["enabled"] = false
		stats["error"] = err.Error()
	} else {
		stats["enabled"] = true
		stats["output"] = stdout.String()
	}

	return stats, nil
}

func (f *FirewallManager) GetCurrentRules() ([]FirewallRule, error) {
	cmd := exec.Command("sudo", "iptables", "-t", "nat", "-L", "OUTPUT", "-n")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to get Linux rules: %w", err)
	}

	return f.parseLinuxRules(stdout.String()), nil
}

func (f *FirewallManager) parseLinuxRules(output string) []FirewallRule {
	var rules []FirewallRule
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		if strings.Contains(line, "REDIRECT") && strings.Contains(line, "80") {
			rules = append(rules, FirewallRule{
				Protocol: "tcp",
				DstPort:  "80",
				Target:   "REDIRECT",
			})
		}
		if strings.Contains(line, "REDIRECT") && strings.Contains(line, "443") {
			rules = append(rules, FirewallRule{
				Protocol: "tcp",
				DstPort:  "443",
				Target:   "REDIRECT",
			})
		}
	}

	return rules
}
