//go:build darwin
// +build darwin

package proxy

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

const pfAnchorName = "com.apple/linko"
const pfTableName = "linko_reserved"

func (f *FirewallManager) SetupTransparentProxy() error {
	ruleConfig := fmt.Sprintf(`# Linko Transparent Proxy Rules
ext_if = "en0"
lo_if = "lo0"
linko_port = "%s"
dns_port = "%s"

# Options and table definition
table <%s> const { %s }

# Translation rules (rdr must come before filtering)
# rdr pass on $lo_if inet proto udp from $ext_if to any port 53 -> 127.0.0.1 port $dns_port
rdr pass on $lo_if inet proto tcp from $ext_if to any port 80 -> 127.0.0.1 port $linko_port
rdr pass on $lo_if inet proto tcp from $ext_if to any port 443 -> 127.0.0.1 port $linko_port

# Filtering rules (must come after translation)
# pass out on $ext_if route-to $lo_if inet proto udp from $ext_if to any port 53
pass out on $ext_if route-to $lo_if inet proto tcp from $ext_if to any port 80
pass out on $ext_if route-to $lo_if inet proto tcp from $ext_if to any port 443
`, f.proxyPort, f.dnsServerPort, pfTableName, strings.Join(reservedCIDRs, ", "))

	if err := f.writeMacOSRules(ruleConfig); err != nil {
		return fmt.Errorf("failed to write MacOS rules: %w", err)
	}

	return f.loadMacOSAnchor()
}

func (f *FirewallManager) RemoveTransparentProxy() error {
	cmd := exec.Command("sudo", "pfctl", "-a", pfAnchorName, "-F", "all")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to flush linko anchor: %w", err)
	}
	return nil
}

func (f *FirewallManager) disablePf() error {
	cmd := exec.Command("sudo", "pfctl", "-d")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to disable pf: %w", err)
	}
	return nil
}

func (f *FirewallManager) enablePf() error {
	cmd := exec.Command("sudo", "pfctl", "-e")
	return cmd.Run()
}

func (f *FirewallManager) removeConfigFile() error {
	cmd := exec.Command("sudo", "rm", "-f", "/etc/pf.linko.conf")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove config file: %w", err)
	}
	return nil
}

func (f *FirewallManager) writeMacOSRules(rules string) error {
	cmd := exec.Command("sudo", "sh", "-c", fmt.Sprintf("cat > /etc/pf.linko.conf << 'EOF'\n%s\nEOF", rules))
	return cmd.Run()
}

func (f *FirewallManager) loadMacOSAnchor() error {
	cmd := exec.Command("sudo", "pfctl", "-f", "/etc/pf.conf")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to load pf config: %w\nstderr: %s", err, stderr.String())
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

	cmd = exec.Command("sudo", "pfctl", "-a", pfAnchorName, "-s", "nat")
	var anchorBuf bytes.Buffer
	cmd.Stdout = &anchorBuf
	cmd.Run()
	stats["anchor_rules"] = anchorBuf.String()

	return stats, nil
}

func (f *FirewallManager) GetCurrentRules() ([]FirewallRule, error) {
	cmd := exec.Command("sudo", "pfctl", "-a", pfAnchorName, "-s", "nat")
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
		if strings.Contains(line, "rdr") && strings.Contains(line, "53") {
			rules = append(rules, FirewallRule{
				Protocol: "udp",
				DstPort:  "53",
				Target:   "REDIRECT",
			})
		}
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
