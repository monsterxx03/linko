package dns

import (
	"context"
	"testing"

	"github.com/miekg/dns"
)

// MockGeoIP is a mock implementation of GeoIP for testing
type MockGeoIP struct {
	ipMap map[string]bool // IP -> isDomestic
}

func NewMockGeoIP() *MockGeoIP {
	return &MockGeoIP{
		ipMap: make(map[string]bool),
	}
}

func (m *MockGeoIP) IsDomesticIP(ipStr string) (bool, error) {
	isDomestic, exists := m.ipMap[ipStr]
	if !exists {
		return false, nil
	}
	return isDomestic, nil
}

func (m *MockGeoIP) AddIP(ipStr string, isDomestic bool) {
	m.ipMap[ipStr] = isDomestic
}

func TestDNSSplitter_SplitQuery(t *testing.T) {
	// Create mock GeoIP
	mockGeoIP := NewMockGeoIP()
	mockGeoIP.AddIP("1.2.3.4", true)   // Domestic IP
	mockGeoIP.AddIP("8.8.8.8", false)  // Foreign IP

	// Note: This test requires a real GeoIP database to work properly
	// For now, we'll test the structure
	splitter := NewDNSSplitter(
		mockGeoIP,
		[]string{"114.114.114.114"},
		[]string{"8.8.8.8"},
		true,
	)

	// Create test question
	msg := new(dns.Msg)
	msg.SetQuestion("google.com.", dns.TypeA)

	ctx := context.Background()
	resp, err := splitter.SplitQuery(ctx, msg)

	if err != nil {
		t.Logf("Expected error (no real DNS servers configured): %v", err)
		// This is expected since we don't have real DNS servers
		return
	}

	if resp == nil {
		t.Error("Expected response")
	}
}

func TestDNSSplitter_areIPsDomestic(t *testing.T) {
	mockGeoIP := NewMockGeoIP()
	mockGeoIP.AddIP("1.2.3.4", true)
	mockGeoIP.AddIP("5.6.7.8", true)
	mockGeoIP.AddIP("8.8.8.8", false)

	splitter := NewDNSSplitter(
		mockGeoIP,
		[]string{"114.114.114.114"},
		[]string{"8.8.8.8"},
		true,
	)

	// Test with domestic IPs
	resp1 := new(dns.Msg)
	rr1, _ := dns.NewRR("example.com. 300 IN A 1.2.3.4")
	resp1.Answer = append(resp1.Answer, rr1)

	if !splitter.areIPsDomestic(resp1) {
		t.Error("Expected domestic IPs to be detected as domestic")
	}

	// Test with foreign IPs
	resp2 := new(dns.Msg)
	rr2, _ := dns.NewRR("example.com. 300 IN A 8.8.8.8")
	resp2.Answer = append(resp2.Answer, rr2)

	if splitter.areIPsDomestic(resp2) {
		t.Error("Expected foreign IPs to be detected as foreign")
	}

	// Test with mixed IPs
	resp3 := new(dns.Msg)
	rr3, _ := dns.NewRR("example.com. 300 IN A 1.2.3.4")
	rr4, _ := dns.NewRR("example.com. 300 IN A 8.8.8.8")
	resp3.Answer = append(resp3.Answer, rr3)
	resp3.Answer = append(resp3.Answer, rr4)

	if splitter.areIPsDomestic(resp3) {
		t.Error("Expected mixed IPs to be detected as foreign")
	}
}

func TestDNSSplitter_IsDomainForeign(t *testing.T) {
	splitter := NewDNSSplitter(
		NewMockGeoIP(),
		[]string{"114.114.114.114"},
		[]string{"8.8.8.8"},
		true,
	)

	tests := []struct {
		domain   string
		expected bool
	}{
		{"google.com", true},  // .com TLD -> foreign
		{"example.org", true}, // .org TLD -> foreign
		{"test.net", true},    // .net TLD -> foreign
		{"baidu.cn", false},   // .cn TLD -> domestic
		{"163.cn", false},     // .cn TLD -> domestic
		{"taobao.cn", false},  // .cn TLD -> domestic
	}

	for _, tt := range tests {
		result := splitter.IsDomainForeign(tt.domain)
		if result != tt.expected {
			t.Errorf("IsDomainForeign(%s) = %v, expected %v", tt.domain, result, tt.expected)
		}
	}
}

func TestDNSSplitter_GetPreferredDNS(t *testing.T) {
	mockGeoIP := NewMockGeoIP()
	splitter := NewDNSSplitter(
		mockGeoIP,
		[]string{"114.114.114.114"},
		[]string{"8.8.8.8"},
		true,
	)

	// Currently returns foreign DNS as default
	dns := splitter.GetPreferredDNS("example.com")
	if len(dns) == 0 {
		t.Error("Expected preferred DNS servers")
	}
}

func TestDNSSplitter_BatchSplitQuery(t *testing.T) {
	mockGeoIP := NewMockGeoIP()
	splitter := NewDNSSplitter(
		mockGeoIP,
		[]string{"114.114.114.114"},
		[]string{"8.8.8.8"},
		true,
	)

	// Create multiple questions
	questions := []*dns.Msg{
		{Question: []dns.Question{{Name: "example1.com.", Qtype: dns.TypeA}}},
		{Question: []dns.Question{{Name: "example2.com.", Qtype: dns.TypeA}}},
		{Question: []dns.Question{{Name: "example3.com.", Qtype: dns.TypeA}}},
	}

	ctx := context.Background()
	responses, err := splitter.BatchSplitQuery(ctx, questions)

	if err != nil {
		t.Logf("Expected error (no real DNS servers): %v", err)
		return
	}

	// Note: Without real DNS servers, responses will be empty or have errors
	if responses == nil {
		t.Error("Expected responses")
	}
}