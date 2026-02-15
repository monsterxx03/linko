# Linko - Transparent MITM Proxy for HTTPS Traffic Analysis

![Demo](./screenshots/linko.gif)

[![CI](https://github.com/monsterxx03/linko/actions/workflows/ci.yml/badge.svg)](https://github.com/monsterxx03/linko/actions/workflows/ci.yml)

Linko includes a built-in MITM (Man-in-the-Middle) proxy that intercepts HTTPS traffic and decrypts it for analysis. It also supports visualizing LLM API messages (currently only Anthropic format).

**Note:** Linko currently only supports macOS.

## Installation

### Homebrew (Recommended)

```bash
brew tap monsterxx03/tap
brew install linko
```

### Manual

Download the latest release from the [Releases](https://github.com/monsterxx03/linko/releases) page and install manually.

## MITM Proxy Working Principle

Linko's MITM proxy works as a **transparent proxy (transparent MITM)**.
Unlike traditional HTTP proxies that require applications to manually configure proxy settings (e.g., `http_proxy=127.0.0.1:8080`), Linko uses macOS's firewall rules (`pfctl`) to redirect network traffic at the system level.

### How It Works

1. **Traffic Redirection via pfctl**: Linko configures macOS's `pf` firewall to redirect outgoing HTTPS traffic (port 443) to the local MITM proxy (port 9890). This happens at the kernel level, so applications are unaware their traffic is being intercepted.

2. **Certificate Generation**: Linko generates a CA certificate that signs on-the-fly certificates for each intercepted domain, enabling decryption of HTTPS traffic.

3. **Transparent Interception**: Since the redirection happens at the network layer, no application configuration is needed. All HTTPS traffic from all applications flows through the MITM proxy automatically.

This is called "transparent" because the proxy is invisible to applications—they think they're communicating directly with the remote server.

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

### Step 3: Start MITM Proxy

```bash
sudo linko mitm
```

The MITM proxy server starts on port 9890 by default. This command requires **sudo** because it sets up firewall rules to redirect HTTPS traffic (port 443) through the MITM proxy.

**Why sudo is required:** The proxy uses transparent interception via macOS firewall (pf) rules to capture HTTPS traffic from all applications without requiring individual proxy configuration. Setting these firewall rules requires administrator privileges.

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

## LLM Message Visualization

Linko can parse and display LLM API requests and responses. Currently supported:

- **Anthropic API** (or any anthropic compatible api, e.g.: minimax, deepseek)

When you make requests to supported LLM providers through the MITM proxy, the admin interface will display:

- Conversation ID
- Model name
- Messages (user/assistant/system)
- Tool calls
- Streaming deltas

## Command Reference

| Command                                         | Description                                                    |
| ----------------------------------------------- | -------------------------------------------------------------- |
| `linko gen-ca`                                  | Generate CA certificate for MITM                               |
| `sudo linko mitm`                               | Start MITM proxy, intercepts all HTTPS traffic (requires sudo) |
| `sudo linko mitm --whitelist "domain1,domain2"` | Start MITM proxy with whitelist (requires sudo)                |
| `linko mitm -h`                                 | Show MITM command help                                         |

## Troubleshooting

**Certificate not trusted:**

- Make sure you've added the CA certificate to your system trust store
- Restart your browser after trusting the certificate

**Traffic not showing:**

- Ensure MITM proxy is running with sudo
- Check firewall rules are properly configured

**Connection errors:**

- Some applications use certificate pinning and won't work with MITM
- You may need to disable certificate pinning for specific apps
