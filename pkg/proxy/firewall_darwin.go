//go:build darwin
// +build darwin

package proxy

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"text/template"

	"github.com/monsterxx03/linko/pkg/ipdb"
)

const pfAnchorName = "com.apple/linko"
const pfTableName = "linko_reserved"
const pfForceTableName = "linko_force"
const pfConfPath = "/etc/pf.linko.conf"

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
	cnDNS := d.fm.cnDNS
	forceProxyIPs := d.fm.forceProxyIPs

	ruleConfig, err := d.renderFirewallRules(proxyPort, dnsServerPort, cnDNS, pfTableName, pfForceTableName, allCIDRs, forceProxyIPs)
	if err != nil {
		return fmt.Errorf("failed to render firewall rules: %w", err)
	}

	if err := d.writeMacOSRules(ruleConfig); err != nil {
		return fmt.Errorf("failed to write MacOS rules: %w", err)
	}

	if err := d.ensurePfAnchorLine(); err != nil {
		return fmt.Errorf("failed to ensure pf anchor line: %w", err)
	}

	if err := d.loadMacOSAnchor(); err != nil {
		return err
	}

	return d.enablePf()
}

type firewallRuleData struct {
	ProxyPort      string
	DNSPort        string
	CNDNS          []string
	TableName      string
	ForceTableName string
	CIDRs          []string
	ForceProxyIPs  []string
	RedirectDNS    bool
	RedirectPorts  []int // 通用重定向端口列表: 80(HTTP), 443(HTTPS), 22(SSH)
}

func (d *darwinFirewallManager) renderFirewallRules(proxyPort, dnsPort string, cnDNS []string, tableName string, forceTableName string, cidrs []string, forceProxyIPs []string) (string, error) {
	// 构建重定向端口列表
	var redirectPorts []int
	if d.fm.redirectOpt.RedirectHTTP {
		redirectPorts = append(redirectPorts, 80)
	}
	if d.fm.redirectOpt.RedirectHTTPS {
		redirectPorts = append(redirectPorts, 443)
	}
	if d.fm.redirectOpt.RedirectSSH {
		redirectPorts = append(redirectPorts, 22)
	}

	const ruleTemplate = `# Linko Transparent Proxy Rules
ext_if = "en0"
lo_if = "lo0"
linko_port = "{{.ProxyPort}}"
linko_user = "nobody"
dns_port = "{{.DNSPort}}"

# Options and table definition
table <{{.TableName}}> const { {{range $i, $cidr := .CIDRs}}{{if $i}}, {{end}}{{$cidr}}{{end}} }
table <{{.ForceTableName}}> { {{range $i, $ip := .ForceProxyIPs}}{{if $i}}, {{end}}{{$ip}}{{end}} }

{{if .RedirectDNS}}
rdr pass on $lo_if inet proto udp from $ext_if to any port 53 -> 127.0.0.1 port $dns_port
{{end}}

{{range .RedirectPorts}}
rdr pass on $lo_if inet proto tcp from $ext_if to any port {{.}} -> 127.0.0.1 port $linko_port
{{end}}

pass out proto tcp from any to <{{.ForceTableName}}> tag FORCE_PROXY

# Filtering rules (must come after translation)
{{if .RedirectDNS}}
pass out on $ext_if route-to $lo_if inet proto udp from $ext_if to any port 53 user { != $linko_user }
{{end}}

{{range .RedirectPorts}}
pass out on $ext_if route-to $lo_if inet proto tcp from $ext_if to any port {{.}} user { != $linko_user }
{{end}}

pass out proto udp from any to { {{range $i, $ip := .CNDNS}}{{if $i}}, {{end}}{{$ip}}{{end}} } port 53 # skip cn dns
pass out quick proto tcp from any to <{{.TableName}}> ! tagged FORCE_PROXY
`

	data := firewallRuleData{
		ProxyPort:      proxyPort,
		DNSPort:        dnsPort,
		CNDNS:          cnDNS,
		TableName:      tableName,
		ForceTableName: forceTableName,
		CIDRs:          cidrs,
		ForceProxyIPs:  forceProxyIPs,
		RedirectDNS:    d.fm.redirectOpt.RedirectDNS,
		RedirectPorts:  redirectPorts,
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
	cmd := exec.Command("sudo", "rm", "-f", pfConfPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove config file: %w", err)
	}
	return nil
}

func (d *darwinFirewallManager) writeMacOSRules(rules string) error {
	cmd := exec.Command("sudo", "sh", "-c", fmt.Sprintf("cat > %s << 'EOF'\n%s\nEOF", pfConfPath, rules))
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

func (d *darwinFirewallManager) ensurePfAnchorLine() error {
	anchorLine := fmt.Sprintf(`load anchor "%s" from "%s"`, pfAnchorName, pfConfPath)

	data, err := os.ReadFile("/etc/pf.conf")
	if err != nil {
		return fmt.Errorf("failed to read /etc/pf.conf: %w", err)
	}

	if strings.Contains(string(data), anchorLine) {
		return nil
	}

	cmd := exec.Command("sudo", "sh", "-c", fmt.Sprintf("echo '%s' >> /etc/pf.conf", anchorLine))
	return cmd.Run()
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
