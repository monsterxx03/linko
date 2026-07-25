# Linux nftables 透明代理重设计

> 日期：2026-07-24
> 状态：设计稿
> 关联：macOS pfctl 实现重构 → nftables

---

## 1. 背景与动机

### 1.1 当前状态

Linko 的透明代理部分在 **macOS** 上基于 `pfctl` + `DIOCNATLOOK` ioctl，架构成熟稳定；在 **Linux** 上则基于 `iptables` + `ipset`，存在多个已知问题：

| 维度 | macOS (pfctl) | Linux (iptables/ipset，当前) |
|------|--------------|---------------------------|
| 防火墙框架 | pf（原生，稳定） | iptables + ipset（**legacy，已废弃**） |
| 原始目标获取 | `DIOCNATLOOK` ioctl（正确） | `SO_ORIGINAL_DST` + `GetsockoptIPv6Mreq`（**实现有 bug，结构体不匹配**） |
| IPv6 支持 | 完整 | **无**（ipset 仅 `family inet`） |
| 预留 IP 集合 | pf table（区间匹配） | ipset `hash:net`（区间匹配有限） |
| 规则复载/卸载 | anchor 管理（原子化） | 逐条 add/delete（非原子化） |
| 防回环 | `group != <gid>` | `SO_MARK` socket mark |
| 状态清除 | `pfctl -k 0.0.0.0/0` | **无** |

### 1.2 为什么要用 nftables

- **iptables 已进入维护模式**，`ipset` 功能已被 nftables 内置的 sets 完全覆盖
- **nftables 是一站式框架**：替代 iptables + ip6tables + ebtables + arptables + ipset
- **原子化规则替换**：`nft -f` 一次性提交/回滚整个规则集，避免逐条操作中间状态
- **原生 IPv4/IPv6 双栈支持**：`inet` 地址族同时处理 v4 和 v6
- **sets 原生支持 interval（CIDR 区间匹配）** 和 `auto-merge`
- **`meta mark` 表达式 + `SO_MARK` socket option**：精确到每个 socket 的内核级标记，比 GID 更适合 Go M:N 线程模型
- **`verdict map`**：可将条件到动作的映射声明式地写入规则集

---

## 2. macOS pfctl 实现分析（逆向参考）

### 2.1 架构概览

```
┌─────────────────────────────────────────────────────┐
│                    pf firewall                       │
│  ┌────────────────────────────────────────────────┐ │
│  │ Anchor: com.apple/linko                        │ │
│  │  /etc/pf.linko.conf                            │ │
│  │                                                │ │
│  │  table <linko_reserved>    ← 国内 IP + 保留地址  │ │
│  │  table <linko_force>       ← 强制代理 IP        │ │
│  │                                                │ │
│  │  rdr on lo0: 53 → 127.0.0.1:6363 (DNS)        │ │
│  │  rdr on lo0: {80,443} → 127.0.0.1:9890 (HTTP/S)│ │
│  │                                                │ │
│  │  route-to lo0: 国内DNS直通, 保留IP跳过代理       │ │
│  │  group != 8001: MITM GID 排除                   │ │
│  └────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────┘
```

### 2.2 核心机制

#### 2.2.1 Anchor（锚点）隔离

pf 使用 **anchor** 机制将 Linko 的规则与应用户/系统的规则隔离：

```pf
# 在 /etc/pf.conf 追加一行
load anchor "com.apple/linko" from "/etc/pf.linko.conf"
```

好处：
- Linko 的规则封装在独立 anchor 中，`pfctl -a com.apple/linko -F all` 一键清除
- 不影响系统已有的 pf 规则
- `pfctl -f /etc/pf.conf` 重新加载整个 pf 配置时会自动包含 anchor

#### 2.2.2 rdr（重定向）+ route-to（路由回环）

两个机制配合实现透明代理：

1. **`rdr`（重定向）**：在 lo0 接口上将目标端口改写为本地代理端口
   ```pf
   rdr pass on $lo_if inet proto tcp from $ext_if to any port 443 -> 127.0.0.1 port $linko_port
   ```

2. **`route-to`（路由回环）**：将原本要出外网的数据包路由回 lo0，使 rdr 规则生效
   ```pf
   pass out on $ext_if route-to $lo_if inet proto tcp from $ext_if to any port 443 group != 8001 keep state
   ```

#### 2.2.3 DIOCNATLOOK（原始目标查询）

代理收到连接后，通过 `/dev/pf` 的 `DIOCNATLOOK` ioctl 查询 pf NAT 引擎，获取原始目标地址：

```c
struct pfioc_natlook {
    struct pf_addr saddr, daddr, rsaddr, rdaddr;  // 源/目标/原始源/原始目标
    u_int16_t sxport, dxport, rsxport, rdxport;   // 端口
    u_int8_t af, proto, proto_variant, direction;  // 地址族、协议、方向
};
```

调用时填入 **当前连接** 的 `(saddr, sport, daddr, dport)`，返回 **原始目标** `(rdaddr, rdport)`。

#### 2.2.4 表（Table）与 CIDR 匹配

pf 的 `table` 支持 `const` 常量表和运行时动态表：

```pf
table <linko_reserved> const { 127.0.0.0/8, 10.0.0.0/8, 172.16.0.0/12, ... }
table <linko_force> { 8.8.8.8 }
```

- `<linko_reserved>`：包含私有地址 + （可选）中国 IP 段 + 预保留域名解析出的 IP — 这些目标**跳过代理**
- `<linko_force>`：包含强制代理的 IP — 即使命中 reserved 也**强制走代理**（优先级更高）

顺序逻辑：
```
pass out ... to <linko_force> tag FORCE_PROXY     # 先标记强制代理
pass out quick ... to <linko_reserved> ! tagged FORCE_PROXY  # 再跳过保留地址（排除已标记的）
```

#### 2.2.5 MITM GID 排除（macOS）

代理进程启动后调用 `syscall.Setgid(8001)` 切换到专用 GID。pf 规则通过 `group != 8001` 排除代理自身流量，防止回环：

```pf
pass out on $ext_if route-to $lo_if inet proto tcp from $ext_if to any port 443 group != 8001 keep state
```

> **注**：Linux 上不使用 GID 防回环，改用 `SO_MARK`（见 3.4.2）。原因是 Go 的 M:N 线程模型下 `syscall.Setgid()` 只能修改当前 OS 线程的 GID，无法保证所有 goroutine 的出站 socket 都带有正确的 GID。

### 2.3 透明代理 TCP 连接生命周期

```
Client (浏览器)                    pf                     TransparentProxy
     │                            │                           │
     │  SYN → api.anthropic.com:443                          │
     │                            │                           │
     │  ────────────►  ext_if      │                           │
     │                            │                           │
     │              ┌─ route-to $lo_if (group != 8001) ─┐    │
     │              │  rdr to 127.0.0.1:9890           │    │
     │              └───────────────────────────────────┘    │
     │                            │                           │
     │  ────────────►  lo0:9890   │                           │
     │                            │  Accept conn              │
     │                            │  ├─ local: 127.0.0.1:9890│
     │                            │  └─ remote: 127.0.0.1:xxx│
     │                            │                           │
     │                            │  DIOCNATLOOK             │
     │                            │  → api.anthropic.com:443  │
     │                            │                           │
     │                            │  ├─ MITM? ← whitelist    │
     │                            │  ├─ Yes: TLS handshake    │
     │                            │  └─ No:  TCP relay        │
     │                            │         to upstream       │
     │                            │         or direct         │
     │                            │                           │
```

---

## 3. nftables 设计方案

### 3.1 架构总览

```
┌───────────────────────────────────────────────────────────────┐
│                    nftables (inet linko)                       │
│                                                               │
│   sets:                                                       │
│     reserved_cidrs    ← 私有IP + 中国IP + 保留域名IP (IPv4)    │
│     reserved_cidrs6   ← 同上, IPv6                            │
│     force_proxy_ips   ← 强制代理 IP (IPv4)                    │
│     force_proxy_ips6  ← 强制代理 IP (IPv6)                    │
│     china_dns         ← 国内 DNS 服务器列表                    │
│                                                               │
│   chain: output_nat (type nat hook output, priority -100)     │
│     1. meta mark <LINKO_MARK> → accept (防回环)                │
│     2. force_proxy            → dnat to proxy                 │
│     3. reserved_cidrs         → accept (直通)                  │
│     4. china_dns              → accept (DNS 直通)             │
│     5. port 80/443/22         → dnat to proxy                │
│     6. port 53                → dnat to local DNS            │
└───────────────────────────────────────────────────────────────┘
```

### 3.2 文件变更计划

| 文件 | 操作 | 说明 |
|------|------|------|
| `pkg/proxy/firewall_linux.go` | **重写** | nftables 实现替换 iptables+ipset |
| `pkg/proxy/transparent_linux.go` | **重写** | 修复 `SO_ORIGINAL_DST` 获取逻辑 |
| `pkg/proxy/transparent_generic.go` | 不变 | 兜底实现 |

接口层（`firewall.go`, `transparent.go`）**无需改动**。

构建标签沿用现有约定：

```go
//go:build linux
```

### 3.3 新增文件（建议）

| 文件 | 类型 | 用途 |
|------|------|------|
| `pkg/proxy/nftables.go` | 共享工具 | nftables 命令封装层（与 `nft` CLI 交互） |

将 nftables 交互逻辑从 `firewall_linux.go` 中抽离为一个独立的共享层，方便单元测试和复用。

### 3.4 nftables 表与链设计

#### 3.4.1 完整规则集

```nftables
# 表: inet linko（同时处理 IPv4 和 IPv6）
table inet linko {

    # ─── 集合（Sets） ─────────────────────────────────

    # 保留 CIDR：这些目标跳过代理，直接连接
    set reserved_cidrs {
        type ipv4_addr
        flags interval
        auto-merge
    }

    set reserved_cidrs6 {
        type ipv6_addr
        flags interval
        auto-merge
    }

    # 强制代理 IP：这些目标始终走代理（覆盖 reserved）
    set force_proxy_ips {
        type ipv4_addr
        flags interval
    }

    set force_proxy_ips6 {
        type ipv6_addr
        flags interval
    }

    # 国内 DNS 服务器：其 DNS 查询不重定向
    set china_dns {
        type ipv4_addr
        flags constant
        elements = { 223.5.5.5, 114.114.114.114 }
    }

    # ─── NAT 输出链（核心重定向逻辑） ────────────────

    chain output_nat {
        type nat hook output priority -100; policy accept

        # 0. ⭐ 防回环：代理自身出站连接（已设置 SO_MARK）直接放行
        meta mark <LINKO_MARK> accept

        # 1. 强制代理：不管 reserved 状态，始终重定向
        ip  daddr @force_proxy_ips  tcp dport { 80, 443, 22 } dnat to 127.0.0.1:<PROXY_PORT>
        ip6 daddr @force_proxy_ips6 tcp dport { 80, 443, 22 } dnat to [::1]:<PROXY_PORT>

        # 2. 保留 CIDR：跳过重定向，直接连接
        ip  daddr @reserved_cidrs  accept
        ip6 daddr @reserved_cidrs6 accept

        # 3. 国内 DNS：不重定向
        udp dport 53 ip daddr @china_dns accept

        # 4. 默认重定向：HTTP/S/SSH → 透明代理
        tcp dport { 80, 443, 22 } dnat to 127.0.0.1:<PROXY_PORT>

        # 5. DNS 重定向 → 本地 DNS 拆分器
        udp dport 53 dnat to 127.0.0.1:<DNS_PORT>
    }
}
```

#### 3.4.2 设计要点分析

**为什么选择 `nat` type + `output` hook？**

- Linux 本地进程发出的出站流量在 `nat OUTPUT` 链中做 DNAT，与 pf 的 `rdr on lo0` 语义等价
- `output` hook 的 `priority -100`（NF_IP_PRI_NAT_DST）确保在 conntrack 之前改写目标地址
- DNAT 后内核会重新路由（reroute），将流量导向 `127.0.0.1:<PROXY_PORT>`

**为什么不需要单独的 `filter` 链？**

- `nat` 类型的链在 DNAT 同时支持 `accept`/`drop` 判决策略
- 对 `reserved_cidrs` 使用 `accept`，数据包不经过 DNAT、直接发送原目标
- 保持单链处理、规则顺序清晰

**为什么用 `meta mark` + `SO_MARK` 而不是 `meta skgid`？**

- Go 使用 M:N 线程模型（goroutine 可在不同 OS 线程间迁移），`syscall.Setgid()` 只修改**当前 OS 线程**的 GID，无法保证所有 goroutine 创建的出站 socket 都带有正确的 GID
- `SO_MARK` 是 **socket 级别的属性**，在 fd 上通过 `setsockopt()` 设置，与 goroutine/线程解耦，在 Go 中完全可靠
- 代理在每个出站 socket 创建后设置 `SO_MARK`，精确控制哪些连接跳过 DNAT，不出意外
- `CAP_NET_ADMIN` 是唯一要求——Linko 部署 nftables 规则时已有 root 权限，零额外成本

**为什么保留 `china_dns` 单独集合？**

- 中国 IP 段（APNIC）包含的是**目标服务器 IP**，而非 DNS 服务器 IP
- `223.5.5.5`（AliDNS）和 `114.114.114.114` 是公开 DNS 解析器，不属于任何中国 IP 段
- 需要单独一条规则放行对它们的 DNS 查询，避免重定向到本地 DNS 导致的递归

### 3.5 原始目标地址获取（`SO_ORIGINAL_DST`）

#### 3.5.1 问题分析

当前 `transparent_linux.go` 的实现：

```go
addr, err := syscall.GetsockoptIPv6Mreq(int(file.Fd()), syscall.IPPROTO_IP, SO_ORIGINAL_DST)
```

这是**错误的**。`SO_ORIGINAL_DST`（option 80）在 `SOL_IP` 层返回的是 `struct sockaddr_in`（16 字节），而 `GetsockoptIPv6Mreq` 期望 `struct ipv6_mreq`（12 字节）。结构体不匹配导致解析出的 IP 和端口完全错误。

#### 3.5.2 正确实现

```go
import (
    "net"
    "syscall"
    "unsafe"
)

// getSOOriginalDst retrieves the original destination address
// after DNAT/REDIRECT via getsockopt(SO_ORIGINAL_DST).
// This is the Linux equivalent of pf's DIOCNATLOOK.
func getSOOriginalDst(fd int) (OriginalDst, error) {
    var addr syscall.RawSockaddrInet4
    addrLen := uint32(unsafe.Sizeof(addr))

    _, _, errno := syscall.Syscall6(
        syscall.SYS_GETSOCKOPT,
        uintptr(fd),
        syscall.IPPROTO_IP,   // SOL_IP = 0
        uintptr(80),          // SO_ORIGINAL_DST
        uintptr(unsafe.Pointer(&addr)),
        uintptr(unsafe.Pointer(&addrLen)),
        0,
    )
    if errno != 0 {
        return OriginalDst{}, fmt.Errorf("SO_ORIGINAL_DST failed: %w", errno)
    }

    ip := net.IP(addr.Addr[:])
    port := int(ntohs(addr.Port))  // 网络序转主机序

    if isLocalHost(ip.String()) {
        return OriginalDst{}, fmt.Errorf("original destination is localhost")
    }

    return OriginalDst{IP: ip, Port: port}, nil
}

// ntohs converts a 16-bit value from network byte order to host byte order.
func ntohs(v uint16) uint16 {
    return (v >> 8) | (v << 8)
}
```

> **注意**：IPv6 场景使用 `SOL_IPV6` + `IP6T_SO_ORIGINAL_DST`（option 80），返回 `RawSockaddrInet6`。首次实现可仅支持 IPv4，IPv6 后续添加。

#### 3.5.3 与 macOS 的对比

| 维度 | macOS (pf) | Linux (nftables + SO_ORIGINAL_DST) |
|------|-----------|-----------------------------------|
| API | `DIOCNATLOOK` ioctl on `/dev/pf` | `getsockopt(fd, SOL_IP, 80, ...)` |
| 输入 | `(saddr, sport, daddr, dport)` | 无需输入（内核 conntrack 自动追踪） |
| 输出 | `(rdaddr, rdport)` | `(orig_addr, orig_port)` |
| 依赖 | pf 必须启用 | conntrack 必须启用（默认开启） |
| 开销 | 一次 ioctl 系统调用 | 一次 getsockopt 系统调用 |

### 3.6 规则管理生命周期

```
          ┌─────────────────┐
          │  SetupFirewall  │
          └────────┬────────┘
                   │
          ┌────────▼────────┐
          │  resolveReserved│  ← 解析保留域名 => IP 列表
          │  Domains()      │
          └────────┬────────┘
                   │
          ┌────────▼────────┐
          │  Create nft sets│  ← 创建 reserved_cidrs, force_proxy_ips
          │  & populate     │  ← 填充私有IP, 中国IP, 域名IP, 强制代理IP
          └────────┬────────┘
                   │
          ┌────────▼────────┐
          │  Apply rule set │  ← nft -f 一次性提交规则集
          └────────┬────────┘
                   │
          ┌────────▼────────┐
          │  Kill existing  │  ← 断开已有连接让其重连以匹配新规则
          │  states         │
          └────────┬────────┘
                   │
          ┌────────▼────────┐
          │     运行中       │
          │  (动态更新 sets) │  ← 进程运行时可增减 set 内元素
          └────────┬────────┘
                   │
          ┌────────▼────────┐
          │ CleanupFirewall │
          └────────┬────────┘
                   │
          ┌────────▼────────┐
          │  nft delete     │  ← 删除整个表（原子化清除所有规则）
          │  table inet     │
          │  linko          │
          └─────────────────┘
```

### 3.7 关键操作的 nft 命令

#### 3.7.1 初始化表与链

```bash
# 创建表（如果不存在）
nft add table inet linko

# 创建集合
nft add set inet linko reserved_cidrs  { type ipv4_addr\; flags interval\; auto-merge\; }
nft add set inet linko reserved_cidrs6 { type ipv6_addr\; flags interval\; auto-merge\; }
nft add set inet linko force_proxy_ips  { type ipv4_addr\; flags interval\; }
nft add set inet linko force_proxy_ips6 { type ipv6_addr\; flags interval\; }

# 创建 NAT 输出链
nft add chain inet linko output_nat { type nat hook output priority -100\; policy accept\; }
```

#### 3.7.2 添加元素到集合

```bash
# 批量添加私有地址
nft add element inet linko reserved_cidrs { 127.0.0.0/8, 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16 }

# 批量添加中国 IP 段
nft add element inet linko reserved_cidrs { 1.0.1.0/24, 1.0.2.0/23, ... }

# 添加域名解析出的 IP
nft add element inet linko reserved_cidrs { 1.2.3.4 }

# 添加强制代理 IP
nft add element inet linko force_proxy_ips { 8.8.8.8, 1.1.1.1 }
```

#### 3.7.3 应用规则

使用 `nft -f` **原子化提交**规则文件：

```bash
nft -f /etc/nftables/linko.conf
```

好处：全部规则写入临时文件后一次提交，语法错误时不会导致部分规则残留。

#### 3.7.4 清除

```bash
# 原子化删除整个表（所有规则和集合一次清除）
nft delete table inet linko
```

#### 3.7.5 状态刷新（kill states）

Linux 连接追踪中存储的连接在 DNAT 规则变更后仍然维持原状态，需要清除已有连接使其重走新规则：

```bash
# 清除指定端口的 conntrack 条目
conntrack -D -p tcp --dport 443
conntrack -D -p tcp --dport 80
conntrack -D -p udp --dport 53

# 或更激进地全部清除
conntrack -F
```

### 3.8 实现细节

#### 3.8.1 `firewall_linux.go` 设计

```go
//go:build linux

package proxy

const LINKO_MARK = 0x1C0 // 用于防回环的 socket mark 值

type linuxFirewallManager struct {
    fm *FirewallManager
}

func newFirewallManagerImpl(fm *FirewallManager) FirewallManagerInterface {
    return &linuxFirewallManager{fm: fm}
}

func (l *linuxFirewallManager) SetupFirewallRules() error {
    // 1. 启用 IP 转发
    //    echo 1 > /proc/sys/net/ipv4/ip_forward

    // 2. 解析保留域名
    //    l.fm.resolveReservedDomains()

    // 3. 准备集合元素
    //    - 私有地址: ipdb.GetReservedCIDRs()
    //    - 中国IP: ipdb.GetChinaCIDRs() (当 skipCN=true)
    //    - 域名解析IP: l.fm.resolvedDomainIPs
    //    - 强制代理IP: l.fm.forceProxyIPs

    // 4. 渲染 nftables 规则文件（含 meta mark <LINKO_MARK> accept）

    // 5. nft -f 提交

    // 6. 刷新 conntrack
}

func (l *linuxFirewallManager) CleanupFirewallRules() error {
    // 1. nft delete table inet linko
    // 2. conntrack -F（可选）
}

// 渲染 nftables 配置（模板方式，复用现有 template 模式）
func (l *linuxFirewallManager) renderNftablesConfig(...) (string, error) { ... }

// 辅助方法
func (l *linuxFirewallManager) ensureIPForward() error { ... }
func (l *linuxFirewallManager) flushConntrack() error { ... }
```

#### 3.8.2 `transparent_linux.go` 设计

```go
//go:build linux

package proxy

import (
    "fmt"
    "net"
    "syscall"
    "unsafe"

    "golang.org/x/sys/unix"
)

const SO_ORIGINAL_DST = 80

// setSocketMark sets SO_MARK on a TCP connection's underlying socket.
// This is used for loop prevention: the proxy marks its outbound connections
// so nftables can skip redirecting them back to itself.
//
// Must be called before the connection is used for I/O.
// Requires CAP_NET_ADMIN (which the process already has as root).
func setSocketMark(conn net.Conn, mark uint32) error {
    tcpConn, ok := conn.(*net.TCPConn)
    if !ok {
        return nil // non-TCP, skip (e.g. UDP for DNS)
    }
    raw, err := tcpConn.SyscallConn()
    if err != nil {
        return fmt.Errorf("get syscall conn: %w", err)
    }
    var setErr error
    raw.Control(func(fd uintptr) {
        setErr = unix.SetsockoptInt(
            int(fd), unix.SOL_SOCKET, unix.SO_MARK, int(mark),
        )
    })
    return setErr
}

// dialMarked creates a TCP connection with SO_MARK already set,
// combining dial + mark into one call for convenience.
func dialMarked(network, addr string, mark uint32) (net.Conn, error) {
    conn, err := net.Dial(network, addr)
    if err != nil {
        return nil, err
    }
    if err := setSocketMark(conn, mark); err != nil {
        conn.Close()
        return nil, fmt.Errorf("set SO_MARK: %w", err)
    }
    return conn, nil
}

func (p *TransparentProxy) getOriginalDestination(conn net.Conn) (OriginalDst, error) {
    tcpConn, ok := conn.(*net.TCPConn)
    if !ok {
        return OriginalDst{}, fmt.Errorf("connection is not TCP")
    }

    file, err := tcpConn.File()
    if err != nil {
        return OriginalDst{}, err
    }
    defer file.Close()

    return getSOOriginalDst(int(file.Fd()))
}

func getSOOriginalDst(fd int) (OriginalDst, error) {
    var addr syscall.RawSockaddrInet4
    addrLen := uint32(unsafe.Sizeof(addr))

    _, _, errno := syscall.Syscall6(
        syscall.SYS_GETSOCKOPT,
        uintptr(fd),
        syscall.IPPROTO_IP,
        uintptr(SO_ORIGINAL_DST),
        uintptr(unsafe.Pointer(&addr)),
        uintptr(unsafe.Pointer(&addrLen)),
        0,
    )
    if errno != 0 {
        return OriginalDst{}, fmt.Errorf("SO_ORIGINAL_DST failed: %w", errno)
    }

    ip := net.IP(addr.Addr[:])
    port := int(ntohs(addr.Port))

    if isLocalHost(ip.String()) {
        return OriginalDst{}, fmt.Errorf("original destination is localhost")
    }

    return OriginalDst{IP: ip, Port: port}, nil
}

func ntohs(v uint16) uint16 {
    return (v >> 8) | (v << 8)
}
```

### 3.9 完整的 nftables 规则模板

```nftables
#!/usr/sbin/nft -f

# Linko Transparent Proxy - nftables Ruleset
# Auto-generated by linko

flush table inet linko 2>/dev/null || true

table inet linko {

    # ── 集合 ──────────────────────────────

    set reserved_cidrs {
        type ipv4_addr
        flags interval
        auto-merge
        elements = {
            {{- range $i, $cidr := .ReservedCIDRs }}
            {{- if $i }}, {{ end }}{{ $cidr }}
            {{- end }}
        }
    }

    set reserved_cidrs6 {
        type ipv6_addr
        flags interval
        auto-merge
    }

    set force_proxy_ips {
        type ipv4_addr
        flags interval
        elements = {
            {{- range $i, $ip := .ForceProxyIPs }}
            {{- if $i }}, {{ end }}{{ $ip }}
            {{- end }}
        }
    }

    set force_proxy_ips6 {
        type ipv6_addr
        flags interval
    }

    set china_dns {
        type ipv4_addr
        flags constant
        elements = {
            {{- range $i, $dns := .CNDNS }}
            {{- if $i }}, {{ end }}{{ $dns }}
            {{- end }}
        }
    }

    # ── NAT 输出链 ────────────────────────

    chain output_nat {
        type nat hook output priority -100; policy accept

        # 防回环：已设置 SO_MARK 的出站连接（代理自身流量）跳过 DNAT
        meta mark {{ .LinkoMark }} accept

        # 强制代理
        ip  daddr @force_proxy_ips  tcp dport { {{ range $i, $port := .RedirectPorts }}{{ if $i }}, {{ end }}{{ $port }}{{ end }} } dnat to 127.0.0.1:{{ .ProxyPort }}
        ip6 daddr @force_proxy_ips6 tcp dport { {{ range $i, $port := .RedirectPorts }}{{ if $i }}, {{ end }}{{ $port }}{{ end }} } dnat to [::1]:{{ .ProxyPort }}

        # 保留 CIDR 跳过（直连）
        ip  daddr @reserved_cidrs accept
        ip6 daddr @reserved_cidrs6 accept

        # 国内 DNS 直连
        {{- if .RedirectDNS }}
        udp dport 53 ip daddr @china_dns accept

        # DNS 重定向
        udp dport 53 dnat to 127.0.0.1:{{ .DNSPort }}
        {{- end }}

        # HTTP/HTTPS/SSH 重定向到透明代理
        {{- if .RedirectPorts }}
        tcp dport { {{ range $i, $port := .RedirectPorts }}{{ if $i }}, {{ end }}{{ $port }}{{ end }} } dnat to 127.0.0.1:{{ .ProxyPort }}
        {{- end }}
    }
}
```

### 3.10 与现有代码的集成点

#### `server.go`（`RunServer`）

无需改动。`FirewallManager` 接口不变，`setupFirewall()` 自动调用新的 Linux 实现：

```go
firewallManager := proxy.NewFirewallManager(
    cfg.ProxyPort(),
    cfg.DNSServerPort(),
    cfg.DNS.DomesticDNS,
    sc.RedirectOption,
    forceProxyIPs,
    cfg.Firewall.ReservedDomains,
    cfg.MITM.GID,
    sc.SkipCN,
)
```

#### `mitm.go`（`runMITM`）

MITM 模式中 `RedirectHTTPS: true`，nftables 方案自动仅重定向 443 端口到代理。

#### `config.go`

无需改动。`FirewallConfig` 结构体中的字段含义保持不变。

### 3.11 与 macOS pf 实现的能力对照

| 功能 | macOS pf | Linux nftables（设计方案） |
|------|----------|--------------------------|
| 透明代理重定向 | `rdr` on `lo0` | `dnat` in `nat output` |
| 原始目标查询 | `DIOCNATLOOK` ioctl | `SO_ORIGINAL_DST` getsockopt |
| 保留 IP 跳过 | `table <reserved>` + `pass out quick` | `set reserved_cidrs` + `accept` + 规则顺序 |
| 强制代理覆盖 | `tag FORCE_PROXY` + `! tagged` | 规则顺序（先 `force_proxy` 后 `reserved`） |
| MITM 防回环 | `group != 8001` | `SO_MARK` socket mark（`meta mark`） |
| DNS 重定向 | `rdr port 53` | `udp dport 53 dnat` |
| 规则隔离 | `anchor` 机制 | `table inet linko` 独立命名空间（`nft delete table` 一键清除） |
| 规则提交 | `pfctl -f /etc/pf.conf` | `nft -f /etc/nftables/linko.conf`（原子化） |
| IPv6 | 部分支持 | `inet` 地址族原生双栈 |
| 状态刷新 | `pfctl -k 0.0.0.0/0` | `conntrack -D` |

### 3.12 实施建议与注意事项

#### 3.12.1 实施优先级

1. **P0** — 修复 `transparent_linux.go` 的 `SO_ORIGINAL_DST`
2. **P0** — 用 nftables 替换 iptables 实现基础的 HTTP/S 重定向
3. **P0** — 实现 `dialMarked()` + `setSocketMark()`，在 `UpstreamClient` 和直接 dial 处设置 `SO_MARK`
4. **P1** — 实现 sets（reserved_cidrs / force_proxy_ips）与元素填充
5. **P2** — conntrack 刷新
6. **P2** — IPv6 支持
7. **P3** — 改用 Go nftables 库（`github.com/google/nftables`）替代 `exec` 调用

#### 3.12.2 可选：Go nftables 库方案

如果未来希望用 Go 原生方式管理 nftables（而非 `exec` 调用 `nft` CLI），可使用 Google 的 nftables 库：

```go
import "github.com/google/nftables"

func setupNftables(conn *nftables.Conn, cfg *config) error {
    // 创建表
    t := conn.AddTable(&nftables.Table{
        Name:   "linko",
        Family: nftables.TableFamilyINet,
    })

    // 创建集合
    s := &nftables.Set{
        Table:   t,
        Name:    "reserved_cidrs",
        KeyType: nftables.TypeIPv4Addr,
        Flags:   nftables.SetFlagInterval | nftables.SetFlagAutoMerge,
    }
    conn.AddSet(s, nil)

    // 添加元素
    conn.SetAddElements(s, []nftables.SetElement{
        {Key: net.ParseIP("127.0.0.0").To4(), IntervalEnd: false},
        // ... 利用 interval 范围
    })

    // 创建链和规则...
    return conn.Flush()
}
```

优点：类型安全、不需解析 nft 命令输出、不需外部依赖（nft 工具）
缺点：增加包依赖、API 较底层、需要处理 nftables 内核协议细节

建议 **第一阶段先用 `exec("nft", ...)` 方式**（与现有 pfctl 模式一致），**后续迭代再考虑迁移到 Go 库**。

#### 3.12.3 边界情况与异常处理

| 场景 | 处理方式 |
|------|----------|
| 系统未安装 `nft` | 返回清晰错误，提示安装 `nftables` 包 |
| `nft -f` 语法错误 | 原子化提交，错误时不残留部分规则 |
| `conntrack` 工具不存在 | 降级为警告，不影响规则安装 |
| 集合元素超多（中国 IP 段约 8000+） | nftables sets 性能优秀，但需测试加载时间 |
| 与已有 iptables/nftables 规则冲突 | 使用独立表名 `linko`，不影响其他规则 |
| `CAP_NET_ADMIN` 未赋予 | `setsockopt(SO_MARK)` 返回 `EPERM`，需提示用 `setcap cap_net_admin=ep` 或提权运行 |

#### 3.12.4 与现有 iptables 实现的兼容性

- 本次重写是**完全替代**，不保留 iptables + ipset 路径
- 构建标签 `//go:build linux` 确保只在 Linux 上编译
- 不引入 `//go:build linux,!nftables` 之类的条件编译，保持简洁
- 通过 Go `exec` 调用 `nft`，Ubuntu 18.04+/Debian 10+/RHEL 8+/Arch Linux 均预装

---

## 4. 测试计划

### 4.1 单元测试

- `TestRenderNftablesConfig` — 模板渲染正确性
- `TestSOOriginalDst` — 在一个 Docker 容器中模拟 DNAT 后验证 getsockopt
- `TestNftablesSetManagement` — 集合元素的增删查

### 4.2 集成测试（需 root）

```bash
# 1. 验证规则安装
sudo go test -run TestSetupIptables ./pkg/proxy/...

# 2. 验证 curl 走代理
curl -v http://httpbin.org/get  # 应看到重定向到代理

# 3. 验证 HTTPS 重定向
curl -v https://api.anthropic.com  # 应看到代理的证书

# 4. 验证 conntrack 刷新
conntrack -L -p tcp --dport 443 | head -5

# 5. 验证清理
sudo nft list table inet linko  # 清理后应返回空
```

### 4.3 Docker 测试环境

鉴于透明代理需 root + 防火墙操作，推荐使用 Docker 容器进行测试：

```dockerfile
FROM golang:1.25
RUN apt-get update && apt-get install -y nftables conntrack curl
# 测试容器内需要 --cap-add=NET_ADMIN --privileged
```

---

## 5. 总结

本设计文档提出了基于 nftables 重构 Linko Linux 透明代理的完整方案，核心要点：

1. **架构对齐** — 保持与 macOS pf 实现**能力等价**：重定向、原始目标查询、保留/强制 IP 集、`SO_MARK` 防回环
2. **技术升级** — 从废弃的 iptables + ipset 迁移到现代化的 nftables，原生双栈支持
3. **修复现有 bug** — `transparent_linux.go` 的 `SO_ORIGINAL_DST` 实现错误
4. **Go 线程安全** — 使用 `SO_MARK` 而非 `syscall.Setgid` 防回环，避免 Go M:N 线程模型的 GID 不可靠问题
5. **接口不变** — `FirewallManager` 和 `TransparentProxy` 接口层无需改动，对调用方透明
6. **渐进实施** — 先 nft CLI + exec 模式（快速可用），再考虑 Go nftables 库（更健壮）
