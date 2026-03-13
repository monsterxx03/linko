# Reserved Domains 设计文档

## 概述

增加一个配置项，用于强制加白一组域名。这组域名的 IP 使用中国 DNS 解析，解析出的 IP 添加到防火墙的 `linko_reserved` 表中，使这些域名的流量直连不走代理。

## 配置变更

在 `pkg/config/config.go` 的 `FirewallConfig` 中添加：

```go
// ReservedDomains 是需要用中国 DNS 解析并加白的域名列表
// 这些域名解析出的 IP 会添加到 linko_reserved，直连不走代理
ReservedDomains []string `mapstructure:"reserved_domains" yaml:"reserved_domains"`
```

## 数据流

```
配置文件 → ReservedDomains → 启动时用 cnDNS 解析 → IP 列表 → 添加到 linko_reserved
```

## 实现细节

### 1. 域名解析

- **时机**: 启动时一次性解析
- **DNS 服务器**: 使用配置中的 `DomesticDNS` (cnDNS) 解析
- **解析方式**: 使用 `net.LookupIP` 或自定义 DNS 查询指向 cnDNS

### 2. Linux 实现 (firewall_linux.go)

在 `addReservedIPsToIPSet()` 函数中，追加解析出的域名 IP：

```go
func (l *linuxFirewallManager) addReservedIPsToIPSet() error {
    // 原有 reserved CIDRs
    addresses := strings.Join(ipdb.GetReservedCIDRs(), "\n")
    cmd := exec.Command("sudo", "sh", "-c", fmt.Sprintf("echo -e '%s' | ipset add - %s", addresses, ipsetName))
    // ...

    // 新增：添加域名解析出的 IP
    for _, ip := range l.fm.resolvedDomainIPs {
        cmd := exec.Command("sudo", "ipset", "add", ipsetName, ip)
        // ...
    }
}
```

### 3. macOS 实现 (firewall_darwin.go)

在渲染 pf 规则时，将域名 IP 追加到 `allCIDRs` 列表：

```go
// 原有
allCIDRs = append(reservedCIDRs, chinaCIDRs...)
// 新增
allCIDRs = append(allCIDRs, l.fm.resolvedDomainIPs...)
```

## 需要修改的文件

1. `pkg/config/config.go` - 添加 `ReservedDomains` 配置字段
2. `pkg/proxy/firewall.go` - 添加 `reservedDomains` 和 `resolvedDomainIPs` 字段到 `FirewallManager`
3. `pkg/proxy/firewall_linux.go` - 实现域名解析并添加到 ipset
4. `pkg/proxy/firewall_darwin.go` - 实现域名解析并添加到 pf table

## 错误处理

- 如果域名解析失败，记录警告日志并跳过该域名
- 防火墙规则设置应尽可能宽容，单个域名解析失败不影响其他域名
