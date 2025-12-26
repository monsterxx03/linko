# Linko - 网络代理和流量分析工具

Linko 是一个高性能的网络代理服务器，具有DNS分流、流量分析和多协议支持功能。

## ✨ 功能特性

- **透明代理**：支持TCP流量转发和DNS UDP 53端口
- **DNS分流**：基于IP地理位置的智能DNS解析（国内/国外分流）
- **多协议支持**：SOCKS5、HTTP隧道、Shadowsocks协议（计划中）
- **流量分析**：实时流量统计和历史数据分析
- **SNI提取**：从HTTPS握手包中提取主机信息

## 🚀 快速开始

### 系统要求

- Go 1.19+
- macOS 或 Linux
- 管理员权限（用于配置防火墙规则）

### 安装

1. 克隆项目：
```bash
git clone https://github.com/monsterxx03/linko.git
cd linko
```

2. 安装依赖：
```bash
make deps
```

3. 下载GeoIP数据库：
```bash
make download-geoip
# 或者手动下载并放置到 data/geoip.mmdb
```

4. 生成默认配置：
```bash
make config
```

5. 构建二进制文件：
```bash
make build
```

### 运行

```bash
./bin/linko -c config/linko.yaml
```

### 开发模式

```bash
make run
```

## 📋 配置说明

详细配置选项请参考 `config/linko.yaml`：

### DNS配置

- `listen_addr`: DNS服务器监听地址
- `domestic_dns`: 国内DNS服务器列表
- `foreign_dns`: 国外DNS服务器列表
- `ipdb_path`: GeoIP数据库文件路径
- `tcp_for_foreign`: 对国外DNS查询使用TCP协议

### 代理配置

- `socks5`: 启用SOCKS5代理
- `http_tunnel`: 启用HTTP CONNECT隧道
- `shadowsocks`: 启用Shadowsocks协议

### 流量统计

- `enable_realtime`: 启用实时统计
- `enable_history`: 启用历史统计
- `update_interval`: 统计更新间隔

## 🧪 测试

运行所有测试：
```bash
make test
```

运行测试并生成覆盖率报告：
```bash
make test-coverage
```

运行性能基准测试：
```bash
make bench
```

## 📊 项目架构

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   透明代理层      │    │    DNS分流层      │    │   协议适配层     │
│                 │    │                  │    │                 │
│ • TCP转发       │◄──►│ • 国内/国外分流   │◄──►│ • SOCKS5       │
│ • UDP(53端口)  │    │ • IP地理位置查询  │    │ • HTTP Tunnel   │
│ • pfctl配置     │    │ • DNS缓存        │    │ • Shadowsocks   │
└─────────────────┘    └──────────────────┘    └─────────────────┘
```

## 🗺️ 开发路线图

### Phase 1: 基础架构和DNS分流 ✅
- [x] 项目结构设计
- [x] IP地理数据库集成
- [x] DNS分流器实现
- [x] 基础配置管理
- [x] DNS服务器(UDP/TCP)
- [x] DNS缓存机制

### Phase 2: 核心代理层
- [ ] SOCKS5协议实现
- [ ] 透明代理核心逻辑
- [ ] 连接池和连接管理
- [ ] 错误处理和重连机制
- [ ] macOS/Linux防火墙配置

### Phase 3: 流量分析和统计
- [ ] SNI信息提取
- [ ] 实时流量统计
- [ ] 历史数据存储
- [ ] Web管理界面

### Phase 4: 协议扩展和优化
- [ ] HTTP CONNECT隧道
- [ ] Shadowsocks协议支持
- [ ] 性能调优
- [ ] 完整测试和文档

## 🔧 开发工具

安装开发依赖：
```bash
make dev-deps
```

格式化代码：
```bash
make fmt
```

代码检查：
```bash
make lint
```

安全检查：
```bash
make security
```

性能分析：
```bash
make profile
```

## 📦 构建

### 本地构建
```bash
make build
```

### Linux构建
```bash
make build-linux
```

### Docker构建
```bash
make docker-build
make docker-run
```

## 📝 使用说明

### 配置透明代理

#### macOS
```bash
# 使用pfctl配置规则
sudo pfctl -f pf.conf
```

#### Linux
```bash
# 使用iptables配置规则
sudo iptables -t nat -A OUTPUT -p tcp --dport 80 -j REDIRECT --to-port 7890
```

### DNS配置

将系统DNS设置为：
```
127.0.0.1:5353
```

### 客户端配置

#### SOCKS5代理
- 主机: 127.0.0.1
- 端口: 7890

#### HTTP代理
- 主机: 127.0.0.1
- 端口: 7890

## 🤝 贡献

欢迎提交Pull Request和Issue！

1. Fork项目
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 打开Pull Request

## 📄 许可证

本项目采用MIT许可证 - 查看 [LICENSE](LICENSE) 文件了解详情

## 🙏 致谢

- [miekg/dns](https://github.com/miekg/dns) - Go语言DNS库
- [geoip2-golang](https://github.com/oschwald/geoip2-golang) - GeoIP2数据库支持
- [MaxMind GeoIP2](https://dev.maxmind.com/geoip/) - IP地理位置数据库

## ⚠️ 注意事项

1. 需要管理员权限运行（用于配置防火墙规则）
2. GeoIP数据库需要定期更新
3. 在生产环境使用前请充分测试
4. 透明代理功能需要系统级配置

## 📞 支持

如有问题或建议，请提交 [Issue](https://github.com/monsterxx03/linko/issues)