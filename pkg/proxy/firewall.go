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

type FirewallManager struct {
	proxyPort     string
	dnsServerPort string
	redirectDNS   bool
	cnDNS         []string
	redirectHTTP  bool
	redirectHTTPS bool
	impl          FirewallManagerInterface
}

func NewFirewallManager(proxyPort string, dnsServerPort string, cnDNS []string, redirectDNS bool, redirectHTTP, redirectHTTPS bool) *FirewallManager {
	fm := &FirewallManager{
		proxyPort:     proxyPort,
		dnsServerPort: dnsServerPort,
		cnDNS:         cnDNS,
		redirectDNS:   redirectDNS,
		redirectHTTP:  redirectHTTP,
		redirectHTTPS: redirectHTTPS,
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
