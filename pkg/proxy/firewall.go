package proxy

import (
	"log/slog"
)

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
	proxyPort         string
	dnsServerPort     string
	redirectOpt       RedirectOption
	cnDNS             []string
	forceProxyIPs     []string
	reservedDomains   []string
	resolvedDomainIPs []string
	mitmGID           int
	skipCN            bool // whether to skip China IP ranges in firewall rules
	impl              FirewallManagerInterface
}

func NewFirewallManager(proxyPort string, dnsServerPort string, cnDNS []string, redirectOpt RedirectOption, forceProxyIPs []string, reservedDomains []string, mitmGID int, skipCN bool) *FirewallManager {
	fm := &FirewallManager{
		proxyPort:       proxyPort,
		dnsServerPort:   dnsServerPort,
		cnDNS:           cnDNS,
		redirectOpt:     redirectOpt,
		forceProxyIPs:   forceProxyIPs,
		reservedDomains: reservedDomains,
		mitmGID:         mitmGID,
		skipCN:          skipCN,
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

// resolveReservedDomains resolves reserved domains using Chinese DNS
func (fm *FirewallManager) resolveReservedDomains() error {
	if len(fm.reservedDomains) == 0 {
		return nil
	}

	ips, err := ResolveHosts(fm.reservedDomains, fm.cnDNS)
	if err != nil {
		slog.Warn("Failed to resolve reserved domains", "error", err)
		return err
	}
	fm.resolvedDomainIPs = ips
	slog.Info("Resolved reserved domains", "domains", fm.reservedDomains, "ips", ips)
	return nil
}
