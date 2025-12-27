package proxy

var (
	reservedCIDRs = []string{
		"127.0.0.0/8",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16",
		"224.0.0.0/4",
		"240.0.0.0/4",
	}
)

func isReservedAddress(ip string) bool {
	for _, cidr := range reservedCIDRs {
		if ip == cidr {
			return true
		}
	}
	return false
}

type FirewallRule struct {
	Protocol string
	SrcIP    string
	SrcPort  string
	DstIP    string
	DstPort  string
	Target   string
}

// FirewallManager manages firewall rules for transparent proxy
type FirewallManager struct {
	proxyPort      string
	dnsServerPort  string
	redirectDNS    bool
	redirectHTTP   bool
	redirectHTTPS  bool
}

// NewFirewallManager creates a new firewall manager
func NewFirewallManager(proxyPort string, dnsServerPort string, redirectDNS, redirectHTTP, redirectHTTPS bool) *FirewallManager {
	return &FirewallManager{
		proxyPort:     proxyPort,
		dnsServerPort: dnsServerPort,
		redirectDNS:   redirectDNS,
		redirectHTTP:  redirectHTTP,
		redirectHTTPS: redirectHTTPS,
	}
}
