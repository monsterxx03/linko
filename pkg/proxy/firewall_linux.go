//go:build linux
// +build linux

package proxy

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

const ipsetName = "linko_reserved"

func (f *FirewallManager) SetupTransparentProxy() error {
	cmd := exec.Command("sudo", "sh", "-c", "echo 1 > /proc/sys/net/ipv4/ip_forward")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to enable IP forwarding: %w", err)
	}

	if err := f.createIPSet(); err != nil {
		return fmt.Errorf("failed to create ipset: %w", err)
	}

	rules := []string{
		fmt.Sprintf("iptables -t nat -A OUTPUT -p udp --dport 53 -j REDIRECT --to-port %s", f.dnsServerPort),
		fmt.Sprintf("iptables -t nat -A OUTPUT -p tcp --dport 80 -m set --match-set %s dst -j ACCEPT", ipsetName),
		fmt.Sprintf("iptables -t nat -A OUTPUT -p tcp --dport 80 -j REDIRECT --to-port %s", f.proxyPort),
		fmt.Sprintf("iptables -t nat -A OUTPUT -p tcp --dport 443 -m set --match-set %s dst -j ACCEPT", ipsetName),
		fmt.Sprintf("iptables -t nat -A OUTPUT -p tcp --dport 443 -j REDIRECT --to-port %s", f.proxyPort),
		fmt.Sprintf("iptables -A INPUT -p udp --dport %s -j ACCEPT", f.dnsServerPort),
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

func (f *FirewallManager) createIPSet() error {
	cmd := exec.Command("sudo", "ipset", "list", "-n")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ipset not available: %w", err)
	}

	cmd = exec.Command("sudo", "ipset", "destroy", ipsetName)
	cmd.Run()

	cmd = exec.Command("sudo", "ipset", "create", ipsetName, "hash:net", "family", "inet")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create ipset: %w", err)
	}

	if err := f.addReservedIPsToIPSet(); err != nil {
		return fmt.Errorf("failed to add reserved IPs: %w", err)
	}

	if err := f.addChinaIPsToIPSet(); err != nil {
		return fmt.Errorf("failed to add China IPs: %w", err)
	}

	return nil
}

func (f *FirewallManager) addReservedIPsToIPSet() error {
	addresses := strings.Join(reservedCIDRs, "\n")
	cmd := exec.Command("sudo", "sh", "-c", fmt.Sprintf("echo -e '%s' | ipset add - %s", addresses, ipsetName))
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func (f *FirewallManager) addChinaIPsToIPSet() error {
	if err := LoadChinaIPRanges(); err != nil {
		return fmt.Errorf("failed to load China IP ranges: %w", err)
	}

	chinaIPs := GetChinaCIDRs()
	if len(chinaIPs) == 0 {
		return nil
	}

	addresses := strings.Join(chinaIPs, "\n")
	cmd := exec.Command("sudo", "sh", "-c", fmt.Sprintf("echo -e '%s' | ipset add - %s", addresses, ipsetName))
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func (f *FirewallManager) destroyIPSet() {
	exec.Command("sudo", "ipset", "destroy", ipsetName).Run()
}

func (f *FirewallManager) RemoveTransparentProxy() error {
	rules := []string{
		fmt.Sprintf("iptables -D INPUT -p tcp --dport %s -j ACCEPT", f.proxyPort),
		fmt.Sprintf("iptables -D INPUT -p udp --dport %s -j ACCEPT", f.dnsServerPort),
		fmt.Sprintf("iptables -t nat -D OUTPUT -p tcp --dport 443 -j REDIRECT --to-port %s", f.proxyPort),
		fmt.Sprintf("iptables -t nat -D OUTPUT -p tcp --dport 443 -m set --match-set %s dst -j ACCEPT", ipsetName),
		fmt.Sprintf("iptables -t nat -D OUTPUT -p tcp --dport 80 -j REDIRECT --to-port %s", f.proxyPort),
		fmt.Sprintf("iptables -t nat -D OUTPUT -p tcp --dport 80 -m set --match-set %s dst -j ACCEPT", ipsetName),
		fmt.Sprintf("iptables -t nat -D OUTPUT -p udp --dport 53 -j REDIRECT --to-port %s", f.dnsServerPort),
	}

	for _, rule := range rules {
		cmd := exec.Command("sudo", "sh", "-c", rule)
		cmd.Run()
	}

	f.destroyIPSet()

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

	cmd = exec.Command("sudo", "ipset", "list", ipsetName)
	var ipsetBuf bytes.Buffer
	cmd.Stdout = &ipsetBuf
	cmd.Run()
	stats["ipset"] = ipsetBuf.String()

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
		if strings.Contains(line, "REDIRECT") && strings.Contains(line, "53") {
			rules = append(rules, FirewallRule{
				Protocol: "udp",
				DstPort:  "53",
				Target:   "REDIRECT",
			})
		}
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
