package proxy

import (
	"context"
	"net"
)

// ResolveHosts resolves a list of hosts (domains or IPs) to IP addresses.
// Domains are resolved using the provided DNS servers, or system DNS if none specified.
// IPs are returned directly.
func ResolveHosts(hosts []string, dnsServers []string) ([]string, error) {
	var result []string

	for _, host := range hosts {
		// Check if it's already an IP address
		if ip := net.ParseIP(host); ip != nil {
			// IPv4 only for firewall rules
			if ip.To4() != nil {
				result = append(result, host)
			}
			continue
		}

		// It's a domain, resolve it
		// If no DNS servers specified, use system DNS
		var ips []string
		var err error
		if len(dnsServers) == 0 {
			ips, err = resolveDomainWithSystemDNS(host)
		} else {
			ips, err = resolveDomain(host, dnsServers[0])
		}
		if err != nil {
			continue // Skip failed resolutions
		}

		result = append(result, ips...)
	}

	return result, nil
}

// resolveDomainWithSystemDNS resolves a domain using the system's default DNS.
func resolveDomainWithSystemDNS(domain string) ([]string, error) {
	// Use system DNS resolver
	addrs, err := net.DefaultResolver.LookupIP(context.Background(), "ip", domain)
	if err != nil {
		return nil, err
	}

	var ips []string
	for _, ip := range addrs {
		if ipv4 := ip.To4(); ipv4 != nil {
			ips = append(ips, ipv4.String())
		}
	}

	return ips, nil
}

// resolveDomain resolves a single domain to IPv4 addresses using the specified DNS server.
func resolveDomain(domain string, dnsServer string) ([]string, error) {
	// Create a custom resolver that uses the specified DNS server
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := &net.Dialer{}
			return d.DialContext(ctx, "udp", dnsServer+":53")
		},
	}

	ctx := context.Background()

	// Lookup A records (IPv4)
	addrs, err := resolver.LookupIP(ctx, "ip", domain)
	if err != nil {
		return nil, err
	}

	var ips []string
	for _, ip := range addrs {
		if ipv4 := ip.To4(); ipv4 != nil {
			ips = append(ips, ipv4.String())
		}
	}

	return ips, nil
}
