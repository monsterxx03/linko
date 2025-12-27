package proxy

type FirewallRule struct {
	Protocol string
	SrcIP    string
	SrcPort  string
	DstIP    string
	DstPort  string
	Target   string
}

type FirewallManager struct {
	proxyPort string
}

func NewFirewallManager(proxyPort string) *FirewallManager {
	return &FirewallManager{
		proxyPort: proxyPort,
	}
}
