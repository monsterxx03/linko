//go:build darwin
// +build darwin

package proxy

import (
	"bytes"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"text/template"

	"github.com/monsterxx03/linko/pkg/ipdb"
)

const pfAnchorName = "com.apple/linko"
const pfTableName = "linko_reserved"

type darwinFirewallManager struct {
	fm *FirewallManager
}

func newFirewallManagerImpl(fm *FirewallManager) FirewallManagerInterface {
	return &darwinFirewallManager{fm: fm}
}

func (d *darwinFirewallManager) SetupFirewallRules() error {
	if err := ipdb.LoadChinaIPRanges(); err != nil {
		slog.Warn("Failed to load cached China IP ranges", "error", err)
		slog.Info("Run 'linko update-cn-ip' to download China IP data")
	}

	chinaCIDRs, _ := ipdb.GetChinaCIDRs()
	reservedCIDRs := ipdb.GetReservedCIDRs()
	allCIDRs := append(reservedCIDRs, chinaCIDRs...)
	proxyPort := d.fm.proxyPort
	dnsServerPort := d.fm.dnsServerPort

	ruleConfig, err := d.renderFirewallRules(proxyPort, dnsServerPort, pfTableName, allCIDRs)
	if err != nil {
		return fmt.Errorf("failed to render firewall rules: %w", err)
	}

	if err := d.writeMacOSRules(ruleConfig); err != nil {
		return fmt.Errorf("failed to write MacOS rules: %w", err)
	}

	if err := d.loadMacOSAnchor(); err != nil {
		return err
	}

	return d.enablePf()
}

type firewallRuleData struct {
	ProxyPort string
	DNSPort   string
	TableName string
	CIDRs     []string
}

func (d *darwinFirewallManager) renderFirewallRules(proxyPort, dnsPort, tableName string, cidrs []string) (string, error) {
	const ruleTemplate = `# Linko Transparent Proxy Rules
ext_if = "en0"
lo_if = "lo0"
linko_port = "{{.ProxyPort}}"
dns_port = "{{.DNSPort}}"

# Options and table definition
table <{{.TableName}}> const { {{range $i, $cidr := .CIDRs}}{{if $i}}, {{end}}{{$cidr}}{{end}} }

# rdr pass on $lo_if inet proto udp from $ext_if to any port 53 -> 127.0.0.1 port $dns_port
rdr pass on $lo_if inet proto tcp from $ext_if to any port 80 -> 127.0.0.1 port $linko_port
rdr pass on $lo_if inet proto tcp from $ext_if to any port 443 -> 127.0.0.1 port $linko_port

# Filtering rules (must come after translation)
# pass out on $ext_if route-to $lo_if inet proto udp from $ext_if to any port 53
pass out on $ext_if route-to $lo_if inet proto tcp from $ext_if to any port 80
pass out on $ext_if route-to $lo_if inet proto tcp from $ext_if to any port 443
pass out proto { tcp, udp } from any to <{{.TableName}}>
`

	data := firewallRuleData{
		ProxyPort: proxyPort,
		DNSPort:   dnsPort,
		TableName: tableName,
		CIDRs:     cidrs,
	}

	var buf bytes.Buffer
	tmpl, err := template.New("pfRules").Parse(ruleTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

func (d *darwinFirewallManager) CleanupFirewallRules() error {
	cmd := exec.Command("sudo", "pfctl", "-a", pfAnchorName, "-F", "all")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to flush linko anchor: %w", err)
	}

	if err := d.disablePf(); err != nil {
		slog.Warn("failed to disable pf", "error", err)
	}

	return nil
}

func (d *darwinFirewallManager) disablePf() error {
	cmd := exec.Command("sudo", "pfctl", "-d")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to disable pf: %w", err)
	}
	return nil
}

func (d *darwinFirewallManager) enablePf() error {
	cmd := exec.Command("sudo", "pfctl", "-e")
	return cmd.Run()
}

func (d *darwinFirewallManager) removeConfigFile() error {
	cmd := exec.Command("sudo", "rm", "-f", "/etc/pf.linko.conf")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove config file: %w", err)
	}
	return nil
}

func (d *darwinFirewallManager) writeMacOSRules(rules string) error {
	cmd := exec.Command("sudo", "sh", "-c", fmt.Sprintf("cat > /etc/pf.linko.conf << 'EOF'\n%s\nEOF", rules))
	return cmd.Run()
}

func (d *darwinFirewallManager) loadMacOSAnchor() error {
	cmd := exec.Command("sudo", "pfctl", "-f", "/etc/pf.conf")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to load pf config: %w\nstderr: %s", err, stderr.String())
	}

	return nil
}

func (d *darwinFirewallManager) CheckFirewallStatus() (map[string]interface{}, error) {
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

func (d *darwinFirewallManager) GetCurrentRules() ([]FirewallRule, error) {
	cmd := exec.Command("sudo", "pfctl", "-a", pfAnchorName, "-s", "nat")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to get MacOS rules: %w", err)
	}

	return d.parseMacOSRules(stdout.String()), nil
}

func (d *darwinFirewallManager) parseMacOSRules(output string) []FirewallRule {
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
