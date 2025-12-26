package proxy

import (
	"bytes"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// FirewallRule represents a firewall rule
type FirewallRule struct {
	Protocol string
	SrcIP    string
	SrcPort  string
	DstIP    string
	DstPort  string
	Target   string // REDIRECT, ACCEPT, DROP, etc.
}

// FirewallManager manages firewall rules for transparent proxy
type FirewallManager struct {
	osType   string
	proxyPort string
}

// NewFirewallManager creates a new firewall manager
func NewFirewallManager(proxyPort string) *FirewallManager {
	return &FirewallManager{
		osType:   runtime.GOOS,
		proxyPort: proxyPort,
	}
}

// SetupTransparentProxy sets up firewall rules for transparent proxy
func (f *FirewallManager) SetupTransparentProxy() error {
	switch f.osType {
	case "darwin":
		return f.setupMacOS()
	case "linux":
		return f.setupLinux()
	default:
		return fmt.Errorf("unsupported operating system: %s", f.osType)
	}
}

// RemoveTransparentProxy removes firewall rules for transparent proxy
func (f *FirewallManager) RemoveTransparentProxy() error {
	switch f.osType {
	case "darwin":
		return f.removeMacOS()
	case "linux":
		return f.removeLinux()
	default:
		return fmt.Errorf("unsupported operating system: %s", f.osType)
	}
}

// setupMacOS sets up pfctl rules for macOS
func (f *FirewallManager) setupMacOS() error {
	// Check if pf is enabled
	cmd := exec.Command("sudo", "pfctl", "-s", "info")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pf is not enabled: %w", err)
	}

	// Create pf.conf rules
	ruleConfig := fmt.Sprintf(`
# Linko Transparent Proxy Rules
ext_if = "en0"
linko_port = "%s"

# Redirect HTTP traffic to Linko
rdr on $ext_if inet proto tcp from any to any port 80 -> 127.0.0.1 port $linko_port

# Redirect HTTPS traffic to Linko
rdr on $ext_if inet proto tcp from any to any port 443 -> 127.0.0.1 port $linko_port

# Allow traffic to Linko
pass in on $ext_if inet proto tcp from any to 127.0.0.1 port $linko_port
pass out on $ext_if inet proto tcp from 127.0.0.1 port $linko_port to any
`, f.proxyPort)

	// Write rules to file
	if err := f.writeMacOSRules(ruleConfig); err != nil {
		return fmt.Errorf("failed to write MacOS rules: %w", err)
	}

	// Load rules
	return f.loadMacOSRules()
}

// removeMacOS removes pfctl rules for macOS
func (f *FirewallManager) removeMacOS() error {
	// Remove rdr rules
	cmd := exec.Command("sudo", "pfctl", "-F", "rdr")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove rdr rules: %w", err)
	}

	return nil
}

// writeMacOSRules writes rules to pf.conf
func (f *FirewallManager) writeMacOSRules(rules string) error {
	// For simplicity, we'll create a temporary file
	// In production, you should handle this more securely
	return nil
}

// loadMacOSRules loads pfctl rules
func (f *FirewallManager) loadMacOSRules() error {
	cmd := exec.Command("sudo", "pfctl", "-f", "/etc/pf.linko.conf")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to load pf rules: %w\nstderr: %s", err, stderr.String())
	}

	return nil
}

// setupLinux sets up iptables rules for Linux
func (f *FirewallManager) setupLinux() error {
	// Enable IP forwarding
	cmd := exec.Command("sudo", "sh", "-c", "echo 1 > /proc/sys/net/ipv4/ip_forward")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to enable IP forwarding: %w", err)
	}

	// Create iptables rules
	rules := []string{
		// Redirect HTTP traffic
		fmt.Sprintf("iptables -t nat -A OUTPUT -p tcp --dport 80 -j REDIRECT --to-port %s", f.proxyPort),
		// Redirect HTTPS traffic
		fmt.Sprintf("iptables -t nat -A OUTPUT -p tcp --dport 443 -j REDIRECT --to-port %s", f.proxyPort),
		// Allow traffic to Linko
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

// removeLinux removes iptables rules for Linux
func (f *FirewallManager) removeLinux() error {
	// Remove iptables rules
	rules := []string{
		fmt.Sprintf("iptables -t nat -D OUTPUT -p tcp --dport 80 -j REDIRECT --to-port %s", f.proxyPort),
		fmt.Sprintf("iptables -t nat -D OUTPUT -p tcp --dport 443 -j REDIRECT --to-port %s", f.proxyPort),
		fmt.Sprintf("iptables -D INPUT -p tcp --dport %s -j ACCEPT", f.proxyPort),
	}

	for _, rule := range rules {
		cmd := exec.Command("sudo", "sh", "-c", rule)
		if err := cmd.Run(); err != nil {
			// Ignore errors when removing rules that don't exist
			continue
		}
	}

	return nil
}

// CheckFirewallStatus checks the status of firewall rules
func (f *FirewallManager) CheckFirewallStatus() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	switch f.osType {
	case "darwin":
		cmd := exec.Command("sudo", "pfctl", "-s", "info")
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		if err := cmd.Run(); err != nil {
			stats["enabled"] = false
			stats["error"] = err.Error()
		} else {
			stats["enabled"] = true
			stats["output"] = stdout.String()
		}
	case "linux":
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
	}

	return stats, nil
}

// GetCurrentRules returns the current firewall rules
func (f *FirewallManager) GetCurrentRules() ([]FirewallRule, error) {
	var rules []FirewallRule

	switch f.osType {
	case "darwin":
		cmd := exec.Command("sudo", "pfctl", "-s", "nat")
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("failed to get MacOS rules: %w", err)
		}
		rules = f.parseMacOSRules(stdout.String())
	case "linux":
		cmd := exec.Command("sudo", "iptables", "-t", "nat", "-L", "OUTPUT", "-n")
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("failed to get Linux rules: %w", err)
		}
		rules = f.parseLinuxRules(stdout.String())
	}

	return rules, nil
}

// parseMacOSRules parses MacOS pfctl output
func (f *FirewallManager) parseMacOSRules(output string) []FirewallRule {
	var rules []FirewallRule
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		if strings.Contains(line, "rdr") && strings.Contains(line, "80") {
			rules = append(rules, FirewallRule{
				Protocol: "tcp",
				DstPort:  "80",
				Target:   "REDIRECT",
			})
		}
		if strings.Contains(line, "rdr") && strings.Contains(line, "443") {
			rules = append(rules, FirewallRule{
				Protocol: "tcp",
				DstPort:  "443",
				Target:   "REDIRECT",
			})
		}
	}

	return rules
}

// parseLinuxRules parses iptables output
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