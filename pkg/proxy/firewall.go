package proxy

type FirewallManagerInterface interface {
	SetupFirewallRules() error
	CleanupFirewallRules() error
	GetCurrentRules() ([]FirewallRule, error)
	CheckFirewallStatus() (map[string]interface{}, error)
}

type FirewallRule struct {
	Protocol string
	SrcIP    string
	SrcPort  string
	DstIP    string
	DstPort  string
	Target   string
}

// RedirectOption contains redirect-related settings
type RedirectOption struct {
	// Enable DNS redirect (UDP 53 -> local DNS server)
	RedirectDNS bool

	// Enable HTTP redirect (TCP 80 -> proxy)
	RedirectHTTP bool

	// Enable HTTPS redirect (TCP 443 -> proxy)
	RedirectHTTPS bool

	// Enable SSH redirect (TCP 22 -> proxy)
	RedirectSSH bool
}

type FirewallManager struct {
	proxyPort     string
	dnsServerPort string
	redirectOpt   RedirectOption
	cnDNS         []string
	impl          FirewallManagerInterface
}

func NewFirewallManager(proxyPort string, dnsServerPort string, cnDNS []string, redirectOpt RedirectOption) *FirewallManager {
	fm := &FirewallManager{
		proxyPort:     proxyPort,
		dnsServerPort: dnsServerPort,
		cnDNS:         cnDNS,
		redirectOpt:   redirectOpt,
	}
	fm.impl = newFirewallManagerImpl(fm)
	return fm
}

func (fm *FirewallManager) SetupFirewallRules() error {
	return fm.impl.SetupFirewallRules()
}

func (fm *FirewallManager) CleanupFirewallRules() error {
	return fm.impl.CleanupFirewallRules()
}

func (fm *FirewallManager) GetCurrentRules() ([]FirewallRule, error) {
	return fm.impl.GetCurrentRules()
}

func (fm *FirewallManager) CheckFirewallStatus() (map[string]interface{}, error) {
	return fm.impl.CheckFirewallStatus()
}
