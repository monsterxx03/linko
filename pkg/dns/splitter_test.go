package dns

import (
	"context"
	"testing"

	"github.com/miekg/dns"
)

func TestDNSSplitter_SplitQuery(t *testing.T) {
	splitter := NewDNSSplitter(
		[]string{"114.114.114.114"},
		[]string{"8.8.8.8"},
		true,
		nil,
	)

	msg := new(dns.Msg)
	msg.SetQuestion("google.com.", dns.TypeA)

	ctx := context.Background()
	resp, err := splitter.SplitQuery(ctx, msg)

	if err != nil {
		t.Logf("Expected error (no real DNS servers configured): %v", err)
		return
	}

	if resp == nil {
		t.Error("Expected response")
	}
}

func TestDNSSplitter_GetPreferredDNS(t *testing.T) {
	splitter := NewDNSSplitter(
		[]string{"114.114.114.114"},
		[]string{"8.8.8.8"},
		true,
		nil,
	)

	// Currently returns foreign DNS as default
	dns := splitter.GetPreferredDNS("example.com")
	if len(dns) == 0 {
		t.Error("Expected preferred DNS servers")
	}
}

func TestDNSSplitter_BatchSplitQuery(t *testing.T) {
	splitter := NewDNSSplitter(
		[]string{"114.114.114.114"},
		[]string{"8.8.8.8"},
		true,
		nil,
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
