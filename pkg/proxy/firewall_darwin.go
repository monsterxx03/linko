//go:build darwin
// +build darwin

package proxy

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

const pfTableName = "linko_reserved"

func (f *FirewallManager) SetupTransparentProxy() error {
	cmd := exec.Command("sudo", "pfctl", "-s", "info")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pf is not enabled: %w", err)
	}

	if err := f.createTable(); err != nil {
		return fmt.Errorf("failed to create pf table: %w", err)
	}

	ruleConfig := fmt.Sprintf(`
# Linko Transparent Proxy Rules
ext_if = "en0"
linko_port = "%s"

rdr on $ext_if inet proto tcp from any to not <%s> port 80 -> 127.0.0.1 port $linko_port
rdr on $ext_if inet proto tcp from any to not <%s> port 443 -> 127.0.0.1 port $linko_port

pass in on $ext_if inet proto tcp from any to 127.0.0.1 port $linko_port
pass out on $ext_if inet proto tcp from 127.0.0.1 port $linko_port to any
`, f.proxyPort, pfTableName, pfTableName)

	if err := f.writeMacOSRules(ruleConfig); err != nil {
		return fmt.Errorf("failed to write MacOS rules: %w", err)
	}

	return f.loadMacOSRules()
}

func (f *FirewallManager) createTable() error {
	addresses := strings.Join(reservedCIDRs, "\n")
	cmd := exec.Command("sudo", "sh", "-c", fmt.Sprintf("echo -e '%s' | pfctl -t %s -T add -", addresses, pfTableName))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}
	return nil
}

func (f *FirewallManager) RemoveTransparentProxy() error {
	cmd := exec.Command("sudo", "pfctl", "-F", "rdr")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove rdr rules: %w", err)
	}

	cmd = exec.Command("sudo", "pfctl", "-t", pfTableName, "-T", "flush")
	cmd.Run()

	return nil
}

func (f *FirewallManager) writeMacOSRules(rules string) error {
	cmd := exec.Command("sudo", "sh", "-c", fmt.Sprintf("cat > /etc/pf.linko.conf << 'EOF'\n%s\nEOF", rules))
	return cmd.Run()
}

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

func (f *FirewallManager) CheckFirewallStatus() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

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

	cmd = exec.Command("sudo", "pfctl", "-t", pfTableName, "-T", "show")
	var tableBuf bytes.Buffer
	cmd.Stdout = &tableBuf
	cmd.Run()
	stats["table"] = tableBuf.String()

	return stats, nil
}

func (f *FirewallManager) GetCurrentRules() ([]FirewallRule, error) {
	cmd := exec.Command("sudo", "pfctl", "-s", "nat")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to get MacOS rules: %w", err)
	}

	return f.parseMacOSRules(stdout.String()), nil
}

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
