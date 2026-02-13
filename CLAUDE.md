# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Linko is a high-performance transparent proxy server with DNS splitting, traffic analysis, and HTTPS MITM inspection capabilities. It consists of a Go backend and a React/TypeScript frontend.

## Common Commands

```bash
# Install dependencies
make deps              # Go dependencies
make ui-deps          # UI dependencies (uses bun)

# Build
make build            # Build Go binary to bin/linko
make ui-build         # Build React UI to pkg/ui/dist
make all              # Build UI + Go binary

# Development
make ui-dev           # Start Vite dev server (http://localhost:5173)

# Code quality
make fmt              # Format code (goimports)
make lint             # Lint with golangci-lint

# Testing
make test             # Run all tests
go test -v ./pkg/dns/...    # Run tests for specific package
```

## Architecture

```
linko/
├── cmd/linko/           # CLI entry point (main.go, serve.go, config.go)
├── pkg/
│   ├── admin/           # HTTP admin API server
│   ├── config/          # Configuration management
│   ├── dns/             # DNS server with geo-based splitting
│   │   ├── splitter.go  # Domestic/foreign DNS routing
│   │   ├── cache.go     # DNS caching
│   │   └── dns.go       # DNS server implementation
│   ├── ipdb/            # China IP detection using cidranger
│   ├── mitm/            # HTTPS MITM proxy
│   ├── proxy/           # Transparent proxy & firewall config
│   │   ├── transparent_darwin.go  # macOS firewall (pf)
│   │   └── transparent_linux.go    # Linux firewall (iptables)
│   └── ui/              # React admin UI (Vite + TypeScript + Tailwind)
├── config/              # Sample YAML configs
└── data/                # SQLite traffic database
```

## Key Implementation Details

- **DNS Splitting**: Uses `pkg/ipdb/china_ip.go` with cidranger to determine if an IP is domestic or foreign, routing DNS queries to appropriate upstream servers
- **Transparent Proxy**: Implemented separately for macOS (pf) and Linux (iptables), redirects port 53 (DNS), 80 (HTTP), 443 (HTTPS)
- **MITM**: HTTP/HTTPS traffic interception
- **Admin API**: Runs on port 9810, serves both API endpoints and embedded UI

## Running Tests

Single test:

```bash
go test -v ./pkg/dns/... -run TestCache
```

With coverage:

```bash
make test-coverage
```

## UI Stack

- React 18 + TypeScript
- Vite 5 for bundling
- Tailwind CSS for styling
- Bun as package manager (set via `BUN` env var in Makefile)
