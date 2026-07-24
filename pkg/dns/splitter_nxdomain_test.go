package dns

import (
	"context"
	"net"
	"sync/atomic"
	"testing"

	"github.com/miekg/dns"
)

// startTestDNSServer starts a UDP DNS server on 127.0.0.1 with a random port
// and returns its address in host:port form.
func startTestDNSServer(t *testing.T, handler dns.HandlerFunc) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	server := &dns.Server{PacketConn: pc, Handler: handler}
	go func() { _ = server.ActivateAndServe() }()
	t.Cleanup(func() { _ = server.Shutdown() })
	return pc.LocalAddr().String()
}

func nxdomainHandler(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetRcode(r, dns.RcodeNameError)
	_ = w.WriteMsg(m)
}

func successHandler(ip string) dns.HandlerFunc {
	return func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Answer = []dns.RR{&dns.A{
			Hdr: dns.RR_Header{Name: r.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
			A:   net.ParseIP(ip),
		}}
		_ = w.WriteMsg(m)
	}
}

// TestSplitQuery_NXDOMAINReturnedAsIs verifies that an NXDOMAIN answer is
// returned to the caller as NXDOMAIN instead of being dropped. Before the
// fix, queryDNS only accepted RcodeSuccess, so NXDOMAIN from all upstreams
// produced (nil, nil), which made the DNS handler synthesize SERVFAIL —
// breaking negative caching and misleading clients.
func TestSplitQuery_NXDOMAINReturnedAsIs(t *testing.T) {
	domestic := startTestDNSServer(t, nxdomainHandler)
	foreign := startTestDNSServer(t, nxdomainHandler)

	splitter := NewDNSSplitter([]string{domestic}, []string{foreign}, false, nil)

	msg := new(dns.Msg)
	msg.SetQuestion("does-not-exist.example.", dns.TypeA)

	resp, err := splitter.SplitQuery(context.Background(), msg)
	if err != nil {
		t.Fatalf("SplitQuery returned error for NXDOMAIN: %v", err)
	}
	if resp == nil {
		t.Fatal("SplitQuery returned nil response for NXDOMAIN (would become SERVFAIL)")
	}
	if resp.Rcode != dns.RcodeNameError {
		t.Fatalf("expected NXDOMAIN rcode, got %s", dns.RcodeToString[resp.Rcode])
	}
}

// TestSplitQuery_DomesticNXDOMAINSkipsForeign verifies that a domestic
// NXDOMAIN is treated as authoritative and foreign DNS is not queried at all.
func TestSplitQuery_DomesticNXDOMAINSkipsForeign(t *testing.T) {
	domestic := startTestDNSServer(t, nxdomainHandler)

	var foreignQueries atomic.Int32
	foreign := startTestDNSServer(t, func(w dns.ResponseWriter, r *dns.Msg) {
		foreignQueries.Add(1)
		successHandler("8.8.8.8")(w, r)
	})

	splitter := NewDNSSplitter([]string{domestic}, []string{foreign}, false, nil)

	msg := new(dns.Msg)
	msg.SetQuestion("does-not-exist.example.", dns.TypeA)

	resp, err := splitter.SplitQuery(context.Background(), msg)
	if err != nil {
		t.Fatalf("SplitQuery returned error: %v", err)
	}
	if resp == nil || resp.Rcode != dns.RcodeNameError {
		t.Fatalf("expected NXDOMAIN response, got %+v", resp)
	}
	if n := foreignQueries.Load(); n != 0 {
		t.Fatalf("foreign DNS queried %d times for NXDOMAIN, want 0", n)
	}
}

// TestSplitQuery_TransportErrorFallsBackToForeign verifies that only
// transport-level failures (not authoritative error rcodes) fall through to
// the next DNS server.
func TestSplitQuery_TransportErrorFallsBackToForeign(t *testing.T) {
	// Domestic server replies with garbage that cannot be decoded as a DNS
	// message, which makes the exchange fail at the transport level.
	domestic := startTestDNSServer(t, func(w dns.ResponseWriter, r *dns.Msg) {
		_, _ = w.Write([]byte("this is not a DNS packet"))
	})
	foreign := startTestDNSServer(t, successHandler("8.8.8.8"))

	splitter := NewDNSSplitter([]string{domestic}, []string{foreign}, false, nil)

	msg := new(dns.Msg)
	msg.SetQuestion("example.com.", dns.TypeA)

	resp, err := splitter.SplitQuery(context.Background(), msg)
	if err != nil {
		t.Fatalf("SplitQuery returned error: %v", err)
	}
	if resp == nil || resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("expected success response from foreign DNS, got %+v", resp)
	}
}
