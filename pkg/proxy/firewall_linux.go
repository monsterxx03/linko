//go:build linux
// +build linux

package proxy

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/monsterxx03/linko/pkg/ipdb"
)

const ipsetName = "linko_reserved"

type linuxFirewallManager struct {
	fm *FirewallManager
}

func newFirewallManagerImpl(fm *FirewallManager) FirewallManagerInterface {
	return &linuxFirewallManager{fm: fm}
}

func (l *linuxFirewallManager) SetupFirewallRules() error {
	cmd := exec.Command("sudo", "sh", "-c", "echo 1 > /proc/sys/net/ipv4/ip_forward")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to enable IP forwarding: %w", err)
	}

	if err := l.createIPSet(); err != nil {
		return fmt.Errorf("failed to create ipset: %w", err)
	}

	proxyPort := l.fm.proxyPort
	dnsServerPort := l.fm.dnsServerPort

	var rules []string

	// Always add input rules
	rules = append(rules,
		fmt.Sprintf("iptables -A INPUT -p udp --dport %s -j ACCEPT", dnsServerPort),
		fmt.Sprintf("iptables -A INPUT -p tcp --dport %s -j ACCEPT", proxyPort),
	)

	// Conditionally add redirect rules based on configuration
	if l.fm.redirectOpt.RedirectDNS {
		rules = append(rules, fmt.Sprintf("iptables -t nat -A OUTPUT -p udp --dport 53 -j REDIRECT --to-port %s", dnsServerPort))
	}

	if l.fm.redirectOpt.RedirectHTTP {
		rules = append(rules,
			fmt.Sprintf("iptables -t nat -A OUTPUT -p tcp --dport 80 -m set --match-set %s dst -j ACCEPT", ipsetName),
			fmt.Sprintf("iptables -t nat -A OUTPUT -p tcp --dport 80 -j REDIRECT --to-port %s", proxyPort),
		)
	}

	if l.fm.redirectOpt.RedirectHTTPS {
		rules = append(rules,
			fmt.Sprintf("iptables -t nat -A OUTPUT -p tcp --dport 443 -m set --match-set %s dst -j ACCEPT", ipsetName),
			fmt.Sprintf("iptables -t nat -A OUTPUT -p tcp --dport 443 -j REDIRECT --to-port %s", proxyPort),
		)
	}

	if l.fm.redirectOpt.RedirectSSH {
		rules = append(rules,
			fmt.Sprintf("iptables -t nat -A OUTPUT -p tcp --dport 22 -m set --match-set %s dst -j ACCEPT", ipsetName),
			fmt.Sprintf("iptables -t nat -A OUTPUT -p tcp --dport 22 -j REDIRECT --to-port %s", proxyPort),
		)
	}

	for _, rule := range rules {
		cmd := exec.Command("sudo", "sh", "-c", rule)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to execute rule %s: %w", rule, err)
		}
	}

	return nil
}

func (l *linuxFirewallManager) createIPSet() error {
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

	if err := l.addReservedIPsToIPSet(); err != nil {
		return fmt.Errorf("failed to add reserved IPs: %w", err)
	}

	if err := l.addChinaIPsToIPSet(); err != nil {
		return fmt.Errorf("failed to add China IPs: %w", err)
	}

	return nil
}

func (l *linuxFirewallManager) addReservedIPsToIPSet() error {
	addresses := strings.Join(ipdb.GetReservedCIDRs(), "\n")
	cmd := exec.Command("sudo", "sh", "-c", fmt.Sprintf("echo -e '%s' | ipset add - %s", addresses, ipsetName))
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func (l *linuxFirewallManager) addChinaIPsToIPSet() error {
	if err := ipdb.LoadChinaIPRanges(); err != nil {
		return fmt.Errorf("failed to load China IP ranges: %w", err)
	}

	chinaIPs, _ := ipdb.GetChinaCIDRs()
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

func (l *linuxFirewallManager) destroyIPSet() {
	exec.Command("sudo", "ipset", "destroy", ipsetName).Run()
}

func (l *linuxFirewallManager) CleanupFirewallRules() error {
	proxyPort := l.fm.proxyPort
	dnsServerPort := l.fm.dnsServerPort

	var rules []string

	// Always clean up input rules
	rules = append(rules,
		fmt.Sprintf("iptables -D INPUT -p tcp --dport %s -j ACCEPT", proxyPort),
		fmt.Sprintf("iptables -D INPUT -p udp --dport %s -j ACCEPT", dnsServerPort),
	)

	// Conditionally clean up redirect rules based on configuration
	if l.fm.redirectOpt.RedirectHTTPS {
		rules = append(rules,
			fmt.Sprintf("iptables -t nat -D OUTPUT -p tcp --dport 443 -j REDIRECT --to-port %s", proxyPort),
			fmt.Sprintf("iptables -t nat -D OUTPUT -p tcp --dport 443 -m set --match-set %s dst -j ACCEPT", ipsetName),
		)
	}

	if l.fm.redirectOpt.RedirectSSH {
		rules = append(rules,
			fmt.Sprintf("iptables -t nat -D OUTPUT -p tcp --dport 22 -j REDIRECT --to-port %s", proxyPort),
			fmt.Sprintf("iptables -t nat -D OUTPUT -p tcp --dport 22 -m set --match-set %s dst -j ACCEPT", ipsetName),
		)
	}

	if l.fm.redirectOpt.RedirectHTTP {
		rules = append(rules,
			fmt.Sprintf("iptables -t nat -D OUTPUT -p tcp --dport 80 -j REDIRECT --to-port %s", proxyPort),
			fmt.Sprintf("iptables -t nat -D OUTPUT -p tcp --dport 80 -m set --match-set %s dst -j ACCEPT", ipsetName),
		)
	}

	if l.fm.redirectOpt.RedirectDNS {
		rules = append(rules, fmt.Sprintf("iptables -t nat -D OUTPUT -p udp --dport 53 -j REDIRECT --to-port %s", dnsServerPort))
	}

	for _, rule := range rules {
		cmd := exec.Command("sudo", "sh", "-c", rule)
		cmd.Run()
	}

	l.destroyIPSet()

	return nil
}

func (l *linuxFirewallManager) CheckFirewallStatus() (map[string]interface{}, error) {
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

func (l *linuxFirewallManager) GetCurrentRules() ([]FirewallRule, error) {
	cmd := exec.Command("sudo", "iptables", "-t", "nat", "-L", "OUTPUT", "-n")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to get Linux rules: %w", err)
	}

	return l.parseLinuxRules(stdout.String()), nil
}

func (l *linuxFirewallManager) parseLinuxRules(output string) []FirewallRule {
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
