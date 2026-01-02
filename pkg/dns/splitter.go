package dns

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	"github.com/monsterxx03/linko/pkg/ipdb"
	"github.com/monsterxx03/linko/pkg/proxy"
)

// DNSSplitter handles DNS query splitting based on IP geolocation
type DNSSplitter struct {
	domestic         []string
	foreign          []string
	useTCPForForeign bool
	upstream         *proxy.UpstreamClient
	client           *dns.Client
}

// NewDNSSplitter creates a new DNS splitter
func NewDNSSplitter(domesticDNS, foreignDNS []string, useTCPForForeign bool, upstream *proxy.UpstreamClient) *DNSSplitter {
	return &DNSSplitter{
		domestic:         domesticDNS,
		foreign:          foreignDNS,
		useTCPForForeign: useTCPForForeign,
		upstream:         upstream,
		client:           &dns.Client{Timeout: 5 * time.Second},
	}
}

// SplitQuery splits a DNS query based on IP geolocation
func (s *DNSSplitter) SplitQuery(ctx context.Context, question *dns.Msg) (*dns.Msg, error) {
	if len(question.Question) == 0 {
		return nil, fmt.Errorf("empty DNS question")
	}

	qname := question.Question[0].Name

	// Query domestic DNS first
	domesticResp, domesticErr := s.queryDNS(ctx, question, s.domestic, false)
	if domesticErr == nil && domesticResp != nil {
		// Check if response IPs are domestic
		if s.areIPsDomestic(domesticResp) {
			slog.Debug("using domestic DNS result", "qname", qname, "dns", s.domestic)
			return domesticResp, nil
		}
		slog.Debug("domestic DNS returned foreign IPs, trying foreign DNS", "qname", qname)
	}

	// Query foreign DNS
	foreignResp, foreignErr := s.queryDNS(ctx, question, s.foreign, s.useTCPForForeign)
	if foreignErr != nil {
		// If foreign query failed, return domestic response if available
		if domesticResp != nil {
			slog.Warn("foreign DNS failed, using domestic DNS result", "qname", qname, "error", foreignErr)
			return domesticResp, nil
		}
		return nil, fmt.Errorf("both domestic and foreign DNS queries failed: domestic=%v, foreign=%v", domesticErr, foreignErr)
	}

	slog.Debug("using foreign DNS result", "qname", qname, "dns", s.foreign)
	return foreignResp, nil
}

// queryDNS sends a DNS query to the specified servers
func (s *DNSSplitter) queryDNS(ctx context.Context, msg *dns.Msg, servers []string, useTCP bool) (*dns.Msg, error) {
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

		if useTCP && s.upstream != nil && s.upstream.IsEnabled() {
			conn, err := s.upstream.Connect(server, 53)
			if err != nil {
				lastErr = err
				continue
			}
			client := &dns.Client{Net: "tcp"}
			resp, _, err = client.ExchangeWithConn(msg, &dns.Conn{Conn: conn})
			conn.Close()
		} else if useTCP {
			client := &dns.Client{Net: "tcp", Timeout: 5 * time.Second}
			resp, _, err = client.Exchange(msg, net.JoinHostPort(server, "53"))
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
			if ipdb.IsChinaIP(ipStr) {
				return true
			}
		}
	}

	return false
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
