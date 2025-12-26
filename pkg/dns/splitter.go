package dns

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// GeoIPLookup interface for IP geolocation
type GeoIPLookup interface {
	IsDomesticIP(ipStr string) (bool, error)
}

// DNSSplitter handles DNS query splitting based on IP geolocation
type DNSSplitter struct {
	geoIP      GeoIPLookup
	domestic   []string
	foreign    []string
	useTCPForForeign bool
	client     *dns.Client
	clientTCP  *dns.Client
}

// NewDNSSplitter creates a new DNS splitter
func NewDNSSplitter(geoIP GeoIPLookup, domesticDNS, foreignDNS []string, useTCPForForeign bool) *DNSSplitter {
	return &DNSSplitter{
		geoIP:           geoIP,
		domestic:        domesticDNS,
		foreign:         foreignDNS,
		useTCPForForeign: useTCPForForeign,
		client:          &dns.Client{Timeout: 5 * time.Second},
		clientTCP:       &dns.Client{Timeout: 5 * time.Second, Net: "tcp"},
	}
}

// SplitQuery splits a DNS query based on IP geolocation
func (s *DNSSplitter) SplitQuery(ctx context.Context, question *dns.Msg) (*dns.Msg, error) {
	if len(question.Question) == 0 {
		return nil, fmt.Errorf("empty DNS question")
	}

	q := question.Question[0]
	qname := q.Name
	qtype := q.Qtype

	// Query domestic DNS first
	domesticResp, domesticErr := s.queryDNS(ctx, qname, qtype, s.domestic, false)
	if domesticErr == nil && domesticResp != nil {
		// Check if response IPs are domestic
		if s.areIPsDomestic(domesticResp) {
			return domesticResp, nil
		}
	}

	// Query foreign DNS
	foreignResp, foreignErr := s.queryDNS(ctx, qname, qtype, s.foreign, s.useTCPForForeign)
	if foreignErr != nil {
		// If foreign query failed, return domestic response if available
		if domesticResp != nil {
			return domesticResp, nil
		}
		return nil, fmt.Errorf("both domestic and foreign DNS queries failed: domestic=%v, foreign=%v", domesticErr, foreignErr)
	}

	return foreignResp, nil
}

// queryDNS sends a DNS query to the specified servers
func (s *DNSSplitter) queryDNS(ctx context.Context, qname string, qtype uint16, servers []string, useTCP bool) (*dns.Msg, error) {
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(qname), qtype)
	msg.RecursionDesired = true

	var lastErr error
	var response *dns.Msg

	for _, server := range servers {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		var resp *dns.Msg
		var err error

		if useTCP {
			resp, _, err = s.clientTCP.Exchange(msg, net.JoinHostPort(server, "53"))
		} else {
			resp, _, err = s.client.Exchange(msg, net.JoinHostPort(server, "53"))
		}

		if err != nil {
			lastErr = err
			continue
		}

		if resp != nil && resp.Rcode == dns.RcodeSuccess {
			response = resp
			break
		}
	}

	if response == nil {
		return nil, lastErr
	}

	return response, nil
}

// areIPsDomestic checks if all IPs in the response are domestic
func (s *DNSSplitter) areIPsDomestic(resp *dns.Msg) bool {
	for _, answer := range resp.Answer {
		switch a := answer.(type) {
		case *dns.A:
			ipStr := a.A.String()
			isDomestic, err := s.geoIP.IsDomesticIP(ipStr)
			if err != nil {
				return false
			}
			if !isDomestic {
				return false
			}
		case *dns.AAAA:
			ipStr := a.AAAA.String()
			isDomestic, err := s.geoIP.IsDomesticIP(ipStr)
			if err != nil {
				return false
			}
			if !isDomestic {
				return false
			}
		}
	}

	return true
}

// SplitAndMerge splits queries and merges responses for multiple domains
func (s *DNSSplitter) SplitAndMerge(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	resp, err := s.SplitQuery(ctx, msg)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// BatchSplitQuery handles multiple DNS queries in batch
func (s *DNSSplitter) BatchSplitQuery(ctx context.Context, questions []*dns.Msg) ([]*dns.Msg, error) {
	var responses []*dns.Msg
	var errors []error

	sem := make(chan struct{}, 10) // Limit concurrent queries
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, msg := range questions {
		wg.Add(1)
		go func(m *dns.Msg) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			resp, err := s.SplitQuery(ctx, m)
			mu.Lock()
			if err != nil {
				errors = append(errors, err)
			} else {
				responses = append(responses, resp)
			}
			mu.Unlock()
		}(msg)
	}

	wg.Wait()

	if len(errors) == len(questions) {
		return nil, fmt.Errorf("all queries failed: %v", errors)
	}

	return responses, nil
}

// GetPreferredDNS returns the preferred DNS servers based on the domain
func (s *DNSSplitter) GetPreferredDNS(domain string) []string {
	// This can be extended to check if a domain is in a whitelist/blacklist
	// For now, return foreign servers as default
	return s.foreign
}

// IsDomainForeign checks if a domain typically resolves to foreign IPs
func (s *DNSSplitter) IsDomainForeign(domain string) bool {
	domain = strings.ToLower(domain)

	// Simple heuristic: if domain contains certain TLDs or patterns, consider foreign
	foreignTLDs := []string{".com", ".org", ".net", ".edu", ".gov", ".mil"}
	for _, tld := range foreignTLDs {
		if strings.HasSuffix(domain, tld) {
			return true
		}
	}

	// Add more heuristics as needed
	return false
}

// Close closes the DNS splitter
func (s *DNSSplitter) Close() error {
	// No resources to close currently
	return nil
}