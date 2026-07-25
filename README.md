# Linko - Transparent MITM Proxy for LLM Traffic Analysis

![Demo](./screenshots/linko.gif)

[![CI](https://github.com/monsterxx03/linko/actions/workflows/ci.yml/badge.svg)](https://github.com/monsterxx03/linko/actions/workflows/ci.yml)

Linko includes a built-in MITM (Man-in-the-Middle) proxy that intercepts HTTPS traffic and decrypts it for analysis. It also supports visualizing LLM API messages.

**Platform support:** macOS (pfctl) · Linux (nftables)

## Installation

### Homebrew (macOS)

```bash
brew tap monsterxx03/tap
brew install linko
```

### Linux (Debian/Ubuntu)

For Debian-based systems, download the Linux binary from [Releases](https://github.com/monsterxx03/linko/releases) and install:

```bash
# Ensure nftables and conntrack are installed
sudo apt update && sudo apt install -y nftables conntrack

# Download and install the binary
sudo curl -L -o /usr/local/bin/linko https://github.com/monsterxx03/linko/releases/latest/download/linko-linux-amd64
sudo chmod +x /usr/local/bin/linko
```

### Manual (All Platforms)

Download the latest release from the [Releases](https://github.com/monsterxx03/linko/releases) page and install manually. Pre-built binaries are available for:

- macOS: `linko-darwin-arm64`, `linko-darwin-amd64`
- Linux: `linko-linux-arm64`, `linko-linux-amd64`

### Build from Source

```bash
git clone https://github.com/monsterxx03/linko.git
cd linko
make deps
make ui-build   # Build the admin UI (requires bun)
make build      # Build the Go binary to bin/linko
```

To build for Linux from macOS:

```bash
make build-linux
```

## MITM Proxy Working Principle

Linko's MITM proxy works as a **transparent proxy (transparent MITM)**.
Unlike traditional HTTP proxies that require applications to manually configure proxy settings (e.g., `http_proxy=127.0.0.1:8080`), Linko uses the system firewall to redirect network traffic at the kernel level.

### How It Works

1. **Traffic Redirection via Firewall Rules**: Linko configures the system firewall to redirect outgoing HTTPS traffic (port 443) to the local MITM proxy. This happens at the kernel level, so applications are unaware their traffic is being intercepted.

   - **macOS**: Uses `pfctl` to set up `rdr` (redirect) rules in a dedicated pf anchor
   - **Linux**: Uses `nftables` with `nat output` hook to DNAT traffic to the proxy

2. **Certificate Generation**: Linko generates a CA certificate that signs on-the-fly certificates for each intercepted domain, enabling decryption of HTTPS traffic.

3. **Transparent Interception**: Since the redirection happens at the network layer, no application configuration is needed. All HTTPS traffic from all applications flows through the MITM proxy automatically.

This is called "transparent" because the proxy is invisible to applications—they think they're communicating directly with the remote server.

### Platform Firewall Details

| Mechanism | macOS | Linux |
|-----------|-------|-------|
| Firewall framework | `pfctl` (Packet Filter) | `nftables` |
| Traffic redirect | `rdr` rule on `lo0` | `dnat` in `nat output` hook |
| Original dst query | `DIOCNATLOOK` ioctl | `SO_ORIGINAL_DST` getsockopt |
| Loop prevention | GID-based (`setgid`) | cgroupv2 + socket mark |
| State refresh | `pfctl -k 0.0.0.0/0` | `conntrack -D` |
| Config path | `/etc/pf.linko.conf` | `/etc/nftables/linko.conf` |

> **Linux dependency:** nftables and conntrack tools must be installed. On Ubuntu/Debian: `sudo apt install nftables conntrack`

### Step 1: Generate CA Certificate

```bash
linko gen-ca
```

This generates a CA certificate and private key in `~/.config/linko/certs/`:

- `ca.crt` - CA certificate
- `ca.key` - CA private key

### Step 2: Trust the CA Certificate

**macOS:**

```bash
# Add to system keychain (requires admin privileges)
sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain ~/.config/linko/certs/ca.crt
```

**Linux (Debian/Ubuntu):**

```bash
sudo cp ~/.config/linko/certs/ca.crt /usr/local/share/ca-certificates/linko-ca.crt
sudo update-ca-certificates
```

**Linux (RHEL/Fedora):**

```bash
sudo cp ~/.config/linko/certs/ca.crt /etc/pki/ca-trust/source/anchors/linko-ca.crt
sudo update-ca-trust
```

### Step 3: Start MITM Proxy

```bash
sudo linko mitm
```

The MITM proxy server starts on port 9890 by default. This command requires **sudo** because it sets up firewall rules to redirect HTTPS traffic (port 443) through the MITM proxy.

### Whitelist (Optional)

By default, MITM intercepts all HTTPS traffic on your system. You can use `--whitelist` to restrict interception to specific domains only:

```bash
sudo linko mitm --whitelist "api.anthropic.com,api.minimaxi.com"
```

Supported whitelist formats:

- **Exact match:** `api.anthropic.com`
- **Wildcard:** `*.anthropic.com` (matches any subdomain)

Traffic to domains not in the whitelist will pass through without interception.

### Step 4: Access Admin Interface

Open your browser and navigate to:

```
http://localhost:9810
```

Go to the **MITM Traffic** page to view intercepted HTTPS traffic in real-time.

## Testing MITM Proxy with curl

Verify that MITM is working by checking the certificate:

```bash
curl -v https://api.anthropic.com
```

In the output, you should see the certificate is issued by Linko CA:

```
SSL certificate chain:
 0. s:CN=api.anthropic.com
   i:C=US O=Linko MITM CA
```

If you see a certificate chain starting with "Linko MITM CA", the traffic is being intercepted successfully.

## Using with Claude Code

If you want to inspect Claude Code's HTTPS traffic through MITM, you need to disable TLS certificate verification due to self-signed CA:

```bash
NODE_TLS_REJECT_UNAUTHORIZED=0 claude
```

This allows Claude Code to work with the MITM proxy's self-signed certificates.

## Using with Gemini CLI

If you want to inspect Gemini CLI's HTTPS traffic through MITM, you need to disable TLS certificate verification:

```bash
NODE_TLS_REJECT_UNAUTHORIZED=0 gemini
```

## Using with OpenCLAW (macOS)

If you want to inspect OpenCLAW's HTTPS traffic through MITM, add the following to `~/Library/LaunchAgents/ai.openclaw.gateway.plist` in the `EnvironmentVariables` dict, then restart the gateway:

```xml
<key>EnvironmentVariables</key>
<dict>
    <key>NODE_TLS_REJECT_UNAUTHORIZED</key>
    <string>0</string>
</dict>
```

```bash
launchctl stop ai.openclaw.gateway && launchctl start ai.openclaw.gateway
```

## LLM Message Visualization

Linko can parse and display LLM API requests and responses.

### Custom LLM Provider Matching

By default, Linko automatically detects requests to known LLM providers. You can use `--anthropic-match` and `--openai-match` to add custom API endpoints:

```bash
sudo linko mitm --anthropic-match "api.example.com/v1/messages" --openai-match "api.myai.com/v1/chat/completions"
```

Multiple patterns can be separated by commas:

```bash
sudo linko mitm --anthropic-match "api.example.com/v1/messages,api2.example.com/v1/anthropic"
```

Pattern format: `hostname/path` - requests matching the hostname and path prefix will be parsed as the corresponding LLM API type.

When you make requests to supported LLM providers through the MITM proxy, the admin interface will display:

- Conversation ID
- Model name
- Messages (user/assistant/system)
- Tool calls
- Streaming deltas

### Supported LLM APIs

| Provider | API | Supported |
|----------|-----|-----------|
| Anthropic | Messages API | Yes |
| OpenAI | Chat Completions API | Yes |
| OpenAI | Responses API | Not yet |
| Google Gemini | Generate Content API | Yes |
| Google Gemini | Cloud Code API | Yes |

For OpenAI-compatible APIs (e.g., OpenAI, Azure OpenAI, Ollama, DeepSeek), Linko supports the `/chat/completions` endpoint.

## TUI Traffic Monitor

Linko includes a real-time terminal-based traffic monitor built with Bubble Tea. It connects to the Admin API via Server-Sent Events (SSE) and displays MITM traffic in a TUI interface.

![TUI Demo](./screenshots/linko-tui.gif)

### Start TUI

Make sure MITM proxy is running first, then launch the TUI:

```bash
linko tui
```

By default, it connects to `http://localhost:9810/api/mitm/traffic/sse`. You can specify a different server with the `-s` flag:

```bash
linko tui -s http://localhost:9810/api/mitm/traffic/sse
```

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `↑` / `↓` / `j` / `k` | Navigate through traffic list |
| `Enter` | Expand traffic details |
| `g` / `G` | Jump to top / bottom |
| `/` | Search/filter traffic |
| `Tab` | Toggle between Headers and Body view |
| `u` / `d` | Page up / down |
| `c` | Clear all traffic |
| `d` | Delete selected traffic |
| `r` | Reconnect to server |
| `q` | Quit |

### Features

- Real-time traffic streaming via SSE
- Auto-reconnect on connection loss
- View request/response headers and body
- Search and filter traffic
- Delete individual traffic entries
- Color-coded status indicators

## Command Reference

| Command                                         | Description                                                    |
| ----------------------------------------------- | -------------------------------------------------------------- |
| `linko gen-ca`                                  | Generate CA certificate for MITM                               |
| `sudo linko mitm`                               | Start MITM proxy, intercepts all HTTPS traffic (requires sudo) |
| `sudo linko mitm --whitelist "domain1,domain2"` | Start MITM proxy with whitelist (requires sudo)                |
| `linko mitm -h`                                 | Show MITM command help                                         |
| `linko tui`                                     | Start TUI traffic monitor (requires MITM running)              |
| `sudo linko cleanup`                            | Remove firewall rules and config after crash/SIGKILL           |

## Troubleshooting

### Network broken after crash / kill -9

If linko was killed unexpectedly (e.g., `kill -9`, OOM, system crash), the firewall rules may still be active, redirecting traffic to a dead proxy. Run:

```bash
sudo linko cleanup
```

Platform-specific cleanup behavior:

- **macOS:** Flushes pf anchor rules, disables pf, removes `/etc/pf.linko.conf`, and cleans the anchor line from `/etc/pf.conf`.
- **Linux:** Deletes the `ip linko` nftables table (atomic removal of all rules, sets, and chains), removes `/etc/nftables/linko.conf`, and cleans up the linko cgroup.

### Certificate not trusted

- Make sure you've added the CA certificate to your system trust store
- On Linux, verify it appears in the trusted list: `awk -v cmd='openssl x509 -noout -subject' '/BEGIN/{close(cmd)};{print | cmd}' < /etc/ssl/certs/ca-certificates.crt | grep -i linko`
- Restart your browser after trusting the certificate

### Traffic not showing

- Ensure MITM proxy is running with sudo
- Check firewall rules are properly configured:
  - **macOS:** `sudo pfctl -a com.apple/linko -s nat`
  - **Linux:** `sudo nft list table ip linko`

### Connection errors

- Some applications use certificate pinning and won't work with MITM
- You may need to disable certificate pinning for specific apps
- On Linux, verify that nftables and conntrack tools are installed: `which nft && which conntrack`
