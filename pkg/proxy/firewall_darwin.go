//go:build darwin
// +build darwin

package proxy

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"

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
	slog.Info("setting up macOS firewall rules", "proxyPort", d.fm.proxyPort, "dnsPort", d.fm.dnsServerPort)

	// 解析 reserved domains
	slog.Info("resolving reserved domains...")
	if err := d.fm.resolveReservedDomains(); err != nil {
		slog.Warn("Failed to resolve reserved domains", "error", err)
	}

	if d.fm.skipCN {
		slog.Info("loading China IP ranges...")
		if err := ipdb.LoadChinaIPRanges(); err != nil {
			slog.Warn("Failed to load cached China IP ranges", "error", err)
			slog.Info("Run 'linko update-cn-ip' to download China IP data")
		}
	}

	chinaCIDRs, _ := ipdb.GetChinaCIDRs()
	reservedCIDRs := ipdb.GetReservedCIDRs()
	var allCIDRs []string
	if d.fm.skipCN {
		allCIDRs = append(reservedCIDRs, chinaCIDRs...)
	} else {
		allCIDRs = reservedCIDRs
	}
	// 追加域名解析出的 IP
	allCIDRs = append(allCIDRs, d.fm.resolvedDomainIPs...)
	proxyPort := d.fm.proxyPort
	dnsServerPort := d.fm.dnsServerPort
	cnDNS := d.fm.cnDNS
	forceProxyIPs := d.fm.forceProxyIPs

	slog.Info("CIDR summary", "reservedCIDRs", len(reservedCIDRs), "chinaCIDRs", len(chinaCIDRs),
		"resolvedDomainIPs", len(d.fm.resolvedDomainIPs), "totalCIDRs", len(allCIDRs),
		"forceProxyIPs", len(forceProxyIPs))

	slog.Info("rendering firewall rules...")
	ruleConfig, err := d.renderFirewallRules(proxyPort, dnsServerPort, cnDNS, pfTableName, pfForceTableName, allCIDRs, forceProxyIPs)
	if err != nil {
		return fmt.Errorf("failed to render firewall rules: %w", err)
	}

	slog.Info("writing pf config to", "path", pfConfPath)
	if err := d.writeMacOSRules(ruleConfig); err != nil {
		return fmt.Errorf("failed to write MacOS rules: %w", err)
	}

	slog.Info("ensuring pf anchor line in /etc/pf.conf...")
	if err := d.ensurePfAnchorLine(); err != nil {
		return fmt.Errorf("failed to ensure pf anchor line: %w", err)
	}

	slog.Info("loading pf anchor...")
	if err := d.loadMacOSAnchor(); err != nil {
		return fmt.Errorf("failed to load pf anchor: %w", err)
	}

	slog.Info("enabling pf...")
	if err := d.enablePf(); err != nil {
		return fmt.Errorf("failed to enable pf: %w", err)
	}

	slog.Info("firewall rules setup complete")
	return nil
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
	MITMGID        int
	ExtIf          string // 动态检测的默认网络接口
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
ext_if = "{{.ExtIf}}"
lo_if = "lo0"
linko_port = "{{.ProxyPort}}"
dns_port = "{{.DNSPort}}"

# Options and table definition
table <{{.TableName}}> const { {{range $i, $cidr := .CIDRs}}{{if $i}}, {{end}}{{$cidr}}{{end}} }
table <{{.ForceTableName}}> { {{range $i, $ip := .ForceProxyIPs}}{{if $i}}, {{end}}{{$ip}}{{end}} }

{{if .RedirectDNS}}
rdr pass on $lo_if inet proto udp from $ext_if to any port 53 -> 127.0.0.1 port $dns_port
{{end}}

{{if .RedirectPorts}}
rdr pass on $lo_if inet proto tcp from $ext_if to any port { {{range $i, $port := .RedirectPorts}}{{if $i}}, {{end}}{{$port}}{{end}} } -> 127.0.0.1 port $linko_port
{{end}}

pass out proto tcp from any to <{{.ForceTableName}}> tag FORCE_PROXY

# Filtering rules (must come after translation)
{{if .RedirectDNS}}
pass out on $ext_if route-to $lo_if inet proto udp from $ext_if to any port 53 group != {{.MITMGID}}
{{end}}

{{if .RedirectPorts}}
pass out on $ext_if route-to $lo_if inet proto tcp from $ext_if to any port { {{range $i, $port := .RedirectPorts}}{{if $i}}, {{end}}{{$port}}{{end}} } group != {{.MITMGID}} keep state
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
		MITMGID:        d.fm.mitmGID,
		ExtIf:          getDefaultInterface(),
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

// getDefaultInterface returns the network interface used for the default route.
// This handles cases where a Mac has both Ethernet and WiFi connected —
// "en0" is not always the active interface.
func getDefaultInterface() string {
	cmd := exec.Command("route", "-n", "get", "default")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		slog.Warn("failed to detect default interface, falling back to en0", "error", err)
		return "en0"
	}

	for _, line := range strings.Split(stdout.String(), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "interface:") {
			iface := strings.TrimSpace(strings.TrimPrefix(line, "interface:"))
			if iface != "" {
				slog.Info("detected default network interface", "interface", iface)
				return iface
			}
		}
	}

	slog.Warn("could not parse default interface, falling back to en0")
	return "en0"
}

func (d *darwinFirewallManager) CleanupFirewallRules() error {
	const cleanupTimeout = 10 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sudo", "pfctl", "-a", pfAnchorName, "-F", "all")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to flush linko anchor: %w", err)
	}

	if err := d.disablePf(ctx); err != nil {
		slog.Warn("failed to disable pf", "error", err)
	}

	if err := d.removeConfigFile(); err != nil {
		slog.Warn("failed to remove config file", "error", err)
	}

	if err := d.removePfAnchorLine(); err != nil {
		slog.Warn("failed to remove anchor line from /etc/pf.conf", "error", err)
	}

	return nil
}

func (d *darwinFirewallManager) disablePf(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "sudo", "pfctl", "-d")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pfctl -d failed: %w\nstderr: %s", err, stderr.String())
	}
	return nil
}

func (d *darwinFirewallManager) enablePf() error {
	cmd := exec.Command("sudo", "pfctl", "-e")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pfctl -e failed: %w\nstderr: %s", err, stderr.String())
	}
	return nil
}

func (d *darwinFirewallManager) removeConfigFile() error {
	cmd := exec.Command("sudo", "rm", "-f", pfConfPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove config file: %w", err)
	}
	return nil
}

func (d *darwinFirewallManager) removePfAnchorLine() error {
	anchorLine := fmt.Sprintf(`load anchor "%s" from "%s"`, pfAnchorName, pfConfPath)

	data, err := os.ReadFile("/etc/pf.conf")
	if err != nil {
		return fmt.Errorf("failed to read /etc/pf.conf: %w", err)
	}

	if !strings.Contains(string(data), anchorLine) {
		return nil // nothing to remove
	}

	// Filter out the anchor line
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) != anchorLine {
			lines = append(lines, line)
		}
	}
	newContent := strings.Join(lines, "\n")

	cmd := exec.Command("sudo", "sh", "-c", fmt.Sprintf("cat > /etc/pf.conf << 'EOF'\n%s\nEOF", newContent))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to write /etc/pf.conf: %w\nstderr: %s", err, stderr.String())
	}
	return nil
}

func (d *darwinFirewallManager) writeMacOSRules(rules string) error {
	cmd := exec.Command("sudo", "sh", "-c", fmt.Sprintf("cat > %s << 'EOF'\n%s\nEOF", pfConfPath, rules))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to write pf config: %w\nstderr: %s", err, stderr.String())
	}
	return nil
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
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to append anchor line to /etc/pf.conf: %w\nstderr: %s", err, stderr.String())
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
