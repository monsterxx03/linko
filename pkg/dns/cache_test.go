package dns

import (
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestDNSCache_Get(t *testing.T) {
	cache := NewDNSCache(5*time.Minute, 100)

	// Create a test DNS message
	msg := new(dns.Msg)
	msg.SetQuestion("example.com.", dns.TypeA)

	// Test get on empty cache
	if cache.Get(msg) != nil {
		t.Error("Expected nil for empty cache")
	}

	// Create response
	resp := new(dns.Msg)
	rr, _ := dns.NewRR("example.com. 300 IN A 1.2.3.4")
	resp.Answer = append(resp.Answer, rr)

	// Set in cache
	cache.Set(msg, resp)

	// Get from cache
	cached := cache.Get(msg)
	if cached == nil {
		t.Error("Expected cached response")
	}

	if len(cached.Answer) == 0 {
		t.Error("Expected answer in cached response")
	}

	// Test cache hit/miss statistics
	stats := cache.GetStats()
	if stats["hits"].(int64) != 1 {
		t.Errorf("Expected 1 hit, got %d", stats["hits"])
	}
}

func TestDNSCache_Set(t *testing.T) {
	cache := NewDNSCache(5*time.Minute, 100)

	// Create test message and response
	msg := new(dns.Msg)
	msg.SetQuestion("test.com.", dns.TypeA)

	resp := new(dns.Msg)
	rr, _ := dns.NewRR("test.com. 300 IN A 5.6.7.8")
	resp.Answer = append(resp.Answer, rr)

	// Set in cache
	cache.Set(msg, resp)

	// Verify it was cached
	cached := cache.Get(msg)
	if cached == nil {
		t.Error("Expected response to be cached")
	}
}

func TestDNSCache_Expiry(t *testing.T) {
	cache := NewDNSCache(100*time.Millisecond, 100)

	msg := new(dns.Msg)
	msg.SetQuestion("expire.com.", dns.TypeA)

	resp := new(dns.Msg)
	rr, _ := dns.NewRR("expire.com. 1 IN A 9.9.9.9")
	resp.Answer = append(resp.Answer, rr)

	cache.Set(msg, resp)

	// Should be in cache
	if cache.Get(msg) == nil {
		t.Error("Expected response before expiry")
	}

	// Wait for expiry
	time.Sleep(200 * time.Millisecond)

	// Should be expired
	if cache.Get(msg) != nil {
		t.Error("Expected nil after expiry")
	}
}

func TestDNSCache_Clear(t *testing.T) {
	cache := NewDNSCache(5*time.Minute, 100)

	msg := new(dns.Msg)
	msg.SetQuestion("clear.com.", dns.TypeA)

	resp := new(dns.Msg)
	rr, _ := dns.NewRR("clear.com. 300 IN A 10.10.10.10")
	resp.Answer = append(resp.Answer, rr)

	cache.Set(msg, resp)
	cache.Clear()

	if cache.Get(msg) != nil {
		t.Error("Expected nil after clear")
	}
}

func TestDNSCache_Remove(t *testing.T) {
	cache := NewDNSCache(5*time.Minute, 100)

	msg := new(dns.Msg)
	msg.SetQuestion("remove.com.", dns.TypeA)

	resp := new(dns.Msg)
	rr, _ := dns.NewRR("remove.com. 300 IN A 11.11.11.11")
	resp.Answer = append(resp.Answer, rr)

	cache.Set(msg, resp)
	cache.Remove(msg)

	if cache.Get(msg) != nil {
		t.Error("Expected nil after remove")
	}
}

func TestDNSCache_GetStats(t *testing.T) {
	cache := NewDNSCache(5*time.Minute, 100)

	msg := new(dns.Msg)
	msg.SetQuestion("stats.com.", dns.TypeA)

	resp := new(dns.Msg)
	rr, _ := dns.NewRR("stats.com. 300 IN A 12.12.12.12")
	resp.Answer = append(resp.Answer, rr)

	// Test initial stats
	stats := cache.GetStats()
	if stats["size"].(int) != 0 {
		t.Error("Expected size 0 for empty cache")
	}

	// Add and retrieve
	cache.Set(msg, resp)
	cache.Get(msg)

	stats = cache.GetStats()
	if stats["size"].(int) != 1 {
		t.Errorf("Expected size 1, got %d", stats["size"])
	}
	if stats["hits"].(int64) != 1 {
		t.Errorf("Expected 1 hit, got %d", stats["hits"])
	}
}

func TestDNSCache_Eviction(t *testing.T) {
	cache := NewDNSCache(5*time.Minute, 2)

	// Add two entries
	msg1 := new(dns.Msg)
	msg1.SetQuestion("one.com.", dns.TypeA)
	resp1 := new(dns.Msg)
	rr1, _ := dns.NewRR("one.com. 300 IN A 1.1.1.1")
	resp1.Answer = append(resp1.Answer, rr1)
	cache.Set(msg1, resp1)

	msg2 := new(dns.Msg)
	msg2.SetQuestion("two.com.", dns.TypeA)
	resp2 := new(dns.Msg)
	rr2, _ := dns.NewRR("two.com. 300 IN A 2.2.2.2")
	resp2.Answer = append(resp2.Answer, rr2)
	cache.Set(msg2, resp2)

	// Add third entry to trigger eviction
	msg3 := new(dns.Msg)
	msg3.SetQuestion("three.com.", dns.TypeA)
	resp3 := new(dns.Msg)
	rr3, _ := dns.NewRR("three.com. 300 IN A 3.3.3.3")
	resp3.Answer = append(resp3.Answer, rr3)
	cache.Set(msg3, resp3)

	stats := cache.GetStats()
	if stats["size"].(int) > 2 {
		t.Errorf("Expected cache size to be limited to 2, got %d", stats["size"])
	}
}
