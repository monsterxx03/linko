# Reserved Domains Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 增加配置项 `reserved_domains`，启动时用中国 DNS 解析域名 IP 并添加到防火墙 `linko_reserved` 表

**Architecture:** 在 FirewallConfig 添加域名列表，FirewallManager 添加解析后的 IP 列表，firewall_linux.go 和 firewall_darwin.go 在设置防火墙规则时添加这些 IP

**Tech Stack:** Go, iptables (Linux), pf (macOS)

---

### Task 1: 添加配置字段

**Files:**
- Modify: `pkg/config/config.go:67-87`

**Step 1: 添加 ReservedDomains 字段**

在 `FirewallConfig` 结构体中添加 `ReservedDomains` 字段：

```go
// FirewallConfig contains firewall-related settings
type FirewallConfig struct {
	// Enable automatic firewall rule management
	EnableAuto bool `mapstructure:"enable_auto" yaml:"enable_auto"`

	// Enable DNS redirect (UDP 53 -> local DNS server)
	RedirectDNS bool `mapstructure:"redirect_dns" yaml:"redirect_dns"`

	// Enable HTTP redirect
	RedirectHTTP bool `mapstructure:"redirect_http" yaml:"redirect_http"`

	// Enable HTTPS redirect
	RedirectHTTPS bool `mapstructure:"redirect_https" yaml:"redirect_https"`

	// Enable SSH redirect (TCP 22 -> proxy)
	RedirectSSH bool `mapstructure:"redirect_ssh" yaml:"redirect_ssh"`

	// ForceProxyHosts is a list of domains or IPs that should always be proxied
	// These hosts will not be added to the reserved list and will always be redirected
	ForceProxyHosts []string `mapstructure:"force_proxy_hosts" yaml:"force_proxy_hosts"`

	// ReservedDomains is a list of domains that should be resolved using Chinese DNS
	// and added to the firewall reserved list (bypass proxy, direct connection)
	ReservedDomains []string `mapstructure:"reserved_domains" yaml:"reserved_domains"`
}
```

**Step 2: Commit**

```bash
git add pkg/config/config.go
git commit -m "feat: add reserved_domains config field"
```

---

### Task 2: 添加 FirewallManager 字段

**Files:**
- Modify: `pkg/proxy/firewall.go:34-43`
- Modify: `pkg/proxy/firewall.go:45-57`

**Step 1: 添加字段到 FirewallManager 结构体**

在 `FirewallManager` 结构体中添加 `reservedDomains` 和 `resolvedDomainIPs` 字段：

```go
type FirewallManager struct {
	proxyPort         string
	dnsServerPort     string
	redirectOpt       RedirectOption
	cnDNS             []string
	forceProxyIPs     []string
	reservedDomains   []string        // 新增：配置的域名列表
	resolvedDomainIPs []string       // 新增：解析后的 IP 列表
	mitmGID           int
	skipCN            bool
	impl              FirewallManagerInterface
}
```

**Step 2: 修改 NewFirewallManager 函数签名**

更新 `NewFirewallManager` 函数以接受 `reservedDomains` 参数：

```go
func NewFirewallManager(proxyPort string, dnsServerPort string, cnDNS []string, redirectOpt RedirectOption, forceProxyIPs []string, reservedDomains []string, mitmGID int, skipCN bool) *FirewallManager {
	fm := &FirewallManager{
		proxyPort:       proxyPort,
		dnsServerPort:   dnsServerPort,
		cnDNS:           cnDNS,
		redirectOpt:     redirectOpt,
		forceProxyIPs:   forceProxyIPs,
		reservedDomains: reservedDomains,
		mitmGID:         mitmGID,
		skipCN:          skipCN,
	}
	fm.impl = newFirewallManagerImpl(fm)
	return fm
}
```

**Step 3: 添加域名解析方法**

在 `firewall.go` 文件末尾添加域名解析方法：

```go
// resolveReservedDomains resolves reserved domains using Chinese DNS
func (fm *FirewallManager) resolveReservedDomains() error {
	if len(fm.reservedDomains) == 0 {
		return nil
	}

	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{}
			return d.DialContext(ctx, "udp", fm.cnDNS[0]+":53")
		},
	}

	for _, domain := range fm.reservedDomains {
		ips, err := resolver.LookupIP(context.Background(), "ip", domain)
		if err != nil {
			slog.Warn("Failed to resolve domain", "domain", domain, "error", err)
			continue
		}
		for _, ip := range ips {
			fm.resolvedDomainIPs = append(fm.resolvedDomainIPs, ip.String())
		}
		slog.Info("Resolved domain", "domain", domain, "ips", ips)
	}

	return nil
}
```

需要添加 import：
```go
import (
	"context"
	"net"
	"os/exec"
	"strings"

	"github.com/monsterxx03/linko/pkg/ipdb"
)
```

**Step 4: Commit**

```bash
git add pkg/proxy/firewall.go
git commit -m "feat: add reservedDomains field and resolution method"
```

---

### Task 3: 修改 Linux 防火墙实现

**Files:**
- Modify: `pkg/proxy/firewall_linux.go:117-124`

**Step 1: 修改 addReservedIPsToIPSet 函数**

在 `addReservedIPsToIPSet` 函数中添加域名解析和 IP 添加逻辑：

```go
func (l *linuxFirewallManager) addReservedIPsToIPSet() error {
	// 添加原有 reserved CIDRs
	addresses := strings.Join(ipdb.GetReservedCIDRs(), "\n")
	cmd := exec.Command("sudo", "sh", "-c", fmt.Sprintf("echo -e '%s' | ipset add - %s", addresses, ipsetName))
	if err := cmd.Run(); err != nil {
		return err
	}

	// 新增：添加域名解析出的 IP
	for _, ip := range l.fm.resolvedDomainIPs {
		cmd := exec.Command("sudo", "ipset", "add", ipsetName, ip)
		if err := cmd.Run(); err != nil {
			slog.Warn("Failed to add domain IP to ipset", "ip", ip, "error", err)
			continue
		}
	}

	return nil
}
```

**Step 2: 在 SetupFirewallRules 中调用域名解析**

在 `SetupFirewallRules` 函数开始处添加域名解析调用：

```go
func (l *linuxFirewallManager) SetupFirewallRules() error {
	// 解析 reserved domains
	if err := l.fm.resolveReservedDomains(); err != nil {
		slog.Warn("Failed to resolve reserved domains", "error", err)
	}

	cmd := exec.Command("sudo", "sh", "-c", "echo 1 > /proc/sys/net/ipv4/ip_forward")
	// ...
}
```

**Step 3: Commit**

```bash
git add pkg/proxy/firewall_linux.go
git commit -m "feat: add domain resolution in Linux firewall"
```

---

### Task 4: 修改 macOS 防火墙实现

**Files:**
- Modify: `pkg/proxy/firewall_darwin.go:31-70`

**Step 1: 修改 SetupFirewallRules 函数**

在 `SetupFirewallRules` 函数中添加域名解析和 IP 追加到 allCIDRs：

```go
func (d *darwinFirewallManager) SetupFirewallRules() error {
	// 解析 reserved domains
	if err := d.fm.resolveReservedDomains(); err != nil {
		slog.Warn("Failed to resolve reserved domains", "error", err)
	}

	if d.fm.skipCN {
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

	// 新增：追加域名解析出的 IP
	allCIDRs = append(allCIDRs, d.fm.resolvedDomainIPs...)

	proxyPort := d.fm.proxyPort
	dnsServerPort := d.fm.dnsServerPort
	cnDNS := d.fm.cnDNS
	forceProxyIPs := d.fm.forceProxyIPs

	ruleConfig, err := d.renderFirewallRules(proxyPort, dnsServerPort, cnDNS, pfTableName, pfForceTableName, allCIDRs, forceProxyIPs)
	// ...
}
```

**Step 2: Commit**

```bash
git add pkg/proxy/firewall_darwin.go
git commit -m "feat: add domain resolution in macOS firewall"
```

---

### Task 5: 更新调用方

**Files:**
- Find and modify: 调用 NewFirewallManager 的位置

**Step 1: 查找调用位置**

```bash
grep -r "NewFirewallManager" --include="*.go"
```

**Step 2: 更新调用**

在调用 `NewFirewallManager` 的位置添加 `reservedDomains` 参数（从配置中获取）：

```go
NewFirewallManager(
	proxyPort,
	dnsServerPort,
	cfg.DNS.DomesticDNS,
	redirectOpt,
	cfg.Firewall.ForceProxyIPs,
	cfg.Firewall.ReservedDomains,  // 新增
	mitmGID,
	skipCN,
)
```

**Step 3: Commit**

```bash
git add <modified files>
git commit -m "feat: pass reservedDomains to FirewallManager"
```

---

### Task 6: 测试验证

**Step 1: 添加配置测试**

创建测试配置验证功能：

```yaml
# config/test-reserved-domains.yaml
firewall:
  enable_auto: true
  redirect_dns: true
  redirect_http: true
  redirect_https: true
  reserved_domains:
    - "example.com"
    - "cdn.example.com"
```

**Step 2: 验证构建**

```bash
make build
```

**Step 3: Commit**

```bash
git add config/test-reserved-domains.yaml
git commit -m "test: add reserved domains test config"
```

---

**Plan complete and saved to `docs/plans/2026-03-13-reserved-domains-design.md`. Two execution options:**

1. **Subagent-Driven (this session)** - I dispatch fresh subagent per task, review between tasks, fast iteration

2. **Parallel Session (separate)** - Open new session with executing-plans, batch execution with checkpoints

**Which approach?**
