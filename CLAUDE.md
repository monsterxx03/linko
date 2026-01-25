# CLAUDE.md

This document provides guidance for working with the Linko codebase.

## Project Overview

Linko is a high-performance transparent proxy server with DNS proxy/shunting capabilities. Key features:
- **Transparent proxy** for TCP traffic (HTTP/HTTPS port 80/443 redirect)
- **DNS server** with China/foreign分流 (smart DNS based on IP geo-location)
- **Multi-protocol support**: SOCKS5, HTTP CONNECT tunnel
- **HTTPS MITM proxy** with real-time traffic inspection via SSE
- **Admin UI** built with React + TypeScript + Vite

## Tech Stack

| Category | Technology |
|----------|------------|
| Language | Go 1.25+ |
| DNS | `github.com/miekg/dns` |
| Config | `github.com/spf13/viper` |
| CLI | `github.com/spf13/cobra` |
| IP Matching | `github.com/yl2chen/cidranger` |
| UI | React + TypeScript + Vite |
| Config Format | YAML (gopkg.in/yaml.v3) |

## Project Structure

```
linko/
├── cmd/linko/              # CLI entry point
│   └── main.go             # serve/config/update-cn-ip/is-cn-ip commands
├── pkg/
│   ├── admin/              # Admin HTTP API server
│   ├── config/             # Configuration loading
│   ├── dns/                # DNS server, cache, splitter
│   ├── ipdb/               # China IP detection (APNIC + cidranger)
│   ├── mitm/               # HTTPS MITM proxy (cert management, traffic inspection)
│   ├── proxy/              # Transparent proxy, firewall, connection pool
│   ├── ui/                 # Admin UI (React + TypeScript + Vite)
│   └── version/            # Version info
├── admin-ui/               # Admin UI source code
├── config/linko.yaml       # Main configuration file
├── data/china_ip_ranges.json
└── Makefile
```

## Commands

```bash
# Serve (with optional firewall setup)
sudo ./bin/linko serve -c config/linko.yaml --firewall

# Generate default config
./bin/linko config -o config/linko.yaml

# Update China IP database
./bin/linko update-cn-ip

# Check if IP is in China
./bin/linko is-cn-ip <ip>
```

## Configuration

Config file: `config/linko.yaml`

Key sections:
- `server`: Listen address, log level
- `dns`: DNS server address, domestic/foreign DNS servers, cache TTL
- `firewall`: Auto-configure firewall rules
- `upstream`: Upstream SOCKS5/HTTP proxy
- `admin`: Admin API server address, UI path
- `mitm`: MITM proxy configuration (enabled, CA cert path, max body size)

## Admin API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/stats/dns` | GET | DNS query statistics |
| `/stats/dns/clear` | POST | Clear DNS query stats |
| `/cache/dns/clear` | POST | Clear DNS cache |
| `/api/mitm/traffic/sse` | GET | SSE stream for MITM traffic |

## Development

```bash
# Install dependencies
make deps

# Build
make build

# Run tests
make test

# Run with coverage
make test-coverage
```

## Key Implementation Details

### DNS分流 (DNS Shunting)
- Uses `china_ip_ranges.json` (from APNIC) to determine if IP is domestic
- Domestic queries → domestic DNS (e.g., 223.5.5.5)
- Foreign queries → foreign DNS (e.g., 8.8.8.8)
- DNS cache with configurable TTL

### Transparent Proxy
- Redirects TCP traffic (ports 80/443) to local proxy
- Extracts SNI from HTTPS handshake for filtering
- macOS uses `pf` firewall, Linux uses `iptables`/`nftables`

### MITM Proxy
- Generates dynamic site certificates using a custom CA
- Supports incremental SSE streaming for real-time traffic inspection
- Inspector chain pattern for extensible traffic analysis (LogInspector, HTTPInspector, SSEInspector)
- Event bus for broadcasting traffic events to multiple subscribers

### Connection Handling
- Connection pooling in `pkg/proxy/conn_pool.go`
- Retry mechanism for upstream connections
- Rate limiting for traffic control

## Notes

- Requires `sudo` for firewall operations and privileged ports (< 1024)
- On macOS, firewall rules managed via `pf` (`pfctl`)
- On Linux, firewall rules managed via `iptables` or `nftables`
- China IP database needs periodic updates via `update-cn-ip` command
- MITM feature requires installing the CA certificate in the system/browser for HTTPS inspection
