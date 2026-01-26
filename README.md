# [WIP 请勿使用] Linko - 网络代理和流量分析工具

Linko 是一个高性能的透明代理服务器，具有 DNS 分流、流量分析和 HTTPS MITM 检查功能。

## 功能特性

- **透明代理**：支持 TCP 流量转发和 DNS UDP 53 端口透明代理
- **DNS 分流**：基于 IP 地理位置的智能 DNS 解析（国内/国外分流）
- **多协议支持**：SOCKS5、HTTP CONNECT 隧道
- **流量分析**：实时流量监控
- **HTTPS MITM**：支持 HTTPS 流量解密和实时检查
- **管理 API**：提供 HTTP API 查看流量统计和系统状态
- **Web 管理界面**：基于 React 的可视化流量监控面板

## 系统要求

- Go 1.25+
- macOS
- 管理员权限（用于配置防火墙规则）

## 安装依赖

```bash
make deps
```

## 构建

```bash
make build
```

## 启动服务

```bash
# 启动 DNS 和 SOCKS5 代理服务器
./bin/linko serve -c config/linko.yaml

# 启用自动防火墙配置 (需要 sudo 权限)
sudo ./bin/linko serve --firewall -c config/linko.yaml
```

## 开发模式

### 后端开发

编译后运行:

```bash
make build && sudo ./bin/linko serve -c config/linko.yaml
```

### 前端开发

```bash
# 安装 UI 依赖
make ui-deps

# 启动前端开发服务器 (http://localhost:5173)
make ui-dev
```

前端开发服务器支持热更新，修改代码后会自动刷新页面。

## 生产打包

### 打包 UI 并嵌入 Go 二进制

```bash
make ui
```

此命令会：

1. 构建前端 UI 到 `pkg/ui/dist`
2. 将 UI 文件内嵌到 Go 二进制
3. 生成最终的可执行文件 `bin/linko`

### 单独构建 UI

```bash
make ui-build
```

### 预览 UI 构建结果

```bash
make ui-preview
```

## 配置说明

详细配置选项请参考 `config/linko.yaml`：

```yaml
server:
  listen_addr: 127.0.0.1:9890 # 代理服务监听地址
  log_level: info # 日志级别

dns:
  listen_addr: 127.0.0.1:6363 # DNS 服务器监听地址
  domestic_dns: # 国内 DNS 服务器
    - 223.5.5.5
    - 114.114.114.114
  foreign_dns: # 国外 DNS 服务器
    - 8.8.8.8
    - 1.1.1.1
  cache_ttl: 5m # DNS 缓存 TTL
  tcp_for_foreign: true # 国外 DNS 使用 TCP

traffic:
  enable_realtime: true # 启用实时流量统计
  enable_history: true # 启用历史流量存储
  update_interval: 1s # 统计更新间隔
  db_path: data/traffic.db # 流量数据库路径

upstream:
  enable: true # 启用上游代理
  type: socks5 # 上游类型 (socks5/http)
  addr: 127.0.0.1:7891 # 上游地址

admin:
  enable: true # 启用管理服务器
  listen_addr: 0.0.0.0:9810 # 管理 API 监听地址
  ui_path: pkg/ui # UI 静态文件路径
  ui_embed: true # 内嵌 UI 到二进制文件

mitm:
  enable: false # 启用 MITM 代理
  ca_cert_path: certs/ca.crt # CA 证书路径
  ca_key_path: certs/ca.key # CA 密钥路径

firewall:
  enable_auto: true # 自动配置防火墙
  redirect_dns: true # 重定向 DNS
  redirect_http: true # 重定向 HTTP
  redirect_https: true # 重定向 HTTPS
```

## 客户端配置

**SOCKS5 代理**

- 主机: 127.0.0.1
- 端口: 9890

**HTTP 代理**

- 主机: 127.0.0.1
- 端口: 9890

**DNS 配置**

- 主 DNS: 127.0.0.1:6363

**管理界面**

- 地址: http://127.0.0.1:9810

## 透明代理配置

### 自动配置 (推荐)

```bash
sudo ./bin/linko serve --firewall -c config/linko.yaml
```

或在配置文件中设置：

```yaml
firewall:
  enable_auto: true
  redirect_dns: true
  redirect_http: true
  redirect_https: true
```

自动配置会设置以下规则：

- DNS (53) -> 6363
- HTTP (80) -> 9890
- HTTPS (443) -> 9890

## MITM 代理配置

1. 生成 CA 证书：

```bash
./bin/linko gen-ca -o certs/
```

2. 安装 CA 证书到系统信任库

3. 启用 MITM 功能：

```yaml
mitm:
  enable: true
  ca_cert_path: certs/ca.crt
  ca_key_path: certs/ca.key
```

4. 访问管理界面的 MITM Traffic 页面查看实时流量

## 管理 API

| 端点                    | 方法 | 描述               |
| ----------------------- | ---- | ------------------ |
| `/health`               | GET  | 健康检查           |
| `/stats/dns`            | GET  | DNS 查询统计       |
| `/stats/dns/clear`      | POST | 清空 DNS 统计      |
| `/cache/dns/clear`      | POST | 清除 DNS 缓存      |
| `/api/traffic`          | GET  | 流量统计           |
| `/api/traffic/sse`      | GET  | SSE 实时流量推送   |
| `/api/mitm/traffic/sse` | GET  | MITM 流量 SSE 推送 |

## 项目架构

```
linko/
├── cmd/linko/          # CLI 入口
├── pkg/
│   ├── admin/          # 管理服务器 (HTTP API)
│   ├── config/         # 配置管理
│   ├── dns/            # DNS 服务器和分流
│   ├── ipdb/           # 中国 IP 检测 (APNIC + cidranger)
│   ├── mitm/           # HTTPS MITM 代理
│   ├── proxy/          # 透明代理和防火墙配置
│   └── ui/             # 管理 UI (React + TypeScript + Vite)
├── config/             # 配置文件
├── data/               # 流量数据库
└── Makefile            # 构建脚本
```

### 核心组件

```
+-------------------------------------------------------------+
|                        Linko Server                          |
+-------------------------------------------------------------+
|  +-------------+  +-------------+  +---------------------+  |
|  |  DNS Server |  |   Proxy     |  |    Admin Server     |  |
|  |    (UDP)    |  | Transparent |  |   (HTTP API + UI)   |  |
|  +------+------+  +------+------+  +----------+----------+  |
|         |                |                     |           |
|         v                v                     v           |
|  +-------------+  +-------------+  +---------------------+  |
|  | DNS Splitter|  |  Upstream    |  |   Traffic Monitor   |  |
|  | + Cache     |  |  SOCKS5/HTTP |  |   (SQLite Storage)  |  |
|  +------+------+  +-------------+  +----------+----------+  |
|         |                                     |             |
|         v                                     v             |
|  +------------------------------------------+              |
|  |       China IP Database (APNIC + cidranger)              |
|  +--------------------------------------------------+      |
|         |                                      |           |
|         v                                      v           |
|  +-------------+                      +-----------------+  |
|  | MITM Proxy  |                      |   Admin UI      |  |
|  | (Optional)  |                      | (React + Vite)  |  |
|  +-------------+                      +-----------------+  |
+-------------------------------------------------------------+
```

## 常用命令速查

| 命令                 | 描述                     |
| -------------------- | ------------------------ |
| `make deps`          | 安装依赖                 |
| `make build`         | 构建二进制               |
| `make ui-deps`       | 安装 UI 依赖             |
| `make ui-dev`        | 启动前端开发服务器       |
| `make ui`            | 打包 UI 并构建生产二进制 |
| `make ui-build`      | 单独构建 UI              |
| `make test`          | 运行测试                 |
| `make test-coverage` | 运行测试并生成覆盖率报告 |
| `make fmt`           | 格式化代码               |
| `make lint`          | 代码检查                 |

## 注意事项

1. 需要管理员权限运行（用于配置防火墙规则）
2. MITM 功能需要安装 CA 证书到系统/浏览器信任库

## 许可证

MIT License - 查看 [LICENSE](LICENSE) 文件了解详情
