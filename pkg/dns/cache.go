package dns

import (
	"crypto/sha1"
	"encoding/hex"
	"sort"
	"sync"
	"time"

	"github.com/miekg/dns"
)

var typeMap = map[uint16]string{
	dns.TypeA:     "A",
	dns.TypeAAAA:  "AAAA",
	dns.TypeCNAME: "CNAME",
	dns.TypeMX:    "MX",
	dns.TypeNS:    "NS",
	dns.TypeTXT:   "TXT",
	dns.TypeSOA:   "SOA",
}

// CacheEntry represents a cached DNS response
type CacheEntry struct {
	Response  *dns.Msg
	ExpiresAt time.Time
	CreatedAt time.Time
}

// DNSCache manages DNS response caching
type DNSCache struct {
	cache   map[string]*CacheEntry
	Mutex   sync.RWMutex
	ttl     time.Duration
	maxSize int
	hits    int64
	misses  int64
}

// NewDNSCache creates a new DNS cache
func NewDNSCache(ttl time.Duration, maxSize int) *DNSCache {
	return &DNSCache{
		cache:   make(map[string]*CacheEntry),
		ttl:     ttl,
		maxSize: maxSize,
	}
}

// Get retrieves a cached DNS response
func (c *DNSCache) Get(question *dns.Msg) *dns.Msg {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	if len(question.Question) == 0 {
		return nil
	}

	key := c.generateKey(question)
	entry, exists := c.cache[key]

	if !exists {
		c.misses++
		return nil
	}

	if time.Now().After(entry.ExpiresAt) {
		delete(c.cache, key)
		c.misses++
		return nil
	}

	c.hits++
	return entry.Response.Copy()
}

// Set caches a DNS response
func (c *DNSCache) Set(question, response *dns.Msg) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	if len(question.Question) == 0 || len(response.Answer) == 0 {
		return
	}

	key := c.generateKey(question)

	// Calculate TTL from response
	ttl := c.ttl
	if response.Answer[0].Header().Ttl > 0 {
		ttl = time.Duration(response.Answer[0].Header().Ttl) * time.Second
		if ttl > c.ttl {
			ttl = c.ttl
		}
	}

	entry := &CacheEntry{
		Response:  response.Copy(),
		ExpiresAt: time.Now().Add(ttl),
		CreatedAt: time.Now(),
	}

	// Check if cache is full
	if len(c.cache) >= c.maxSize {
		c.evictOldest()
	}

	c.cache[key] = entry
}

// generateKey generates a cache key from DNS question
func (c *DNSCache) generateKey(question *dns.Msg) string {
	if len(question.Question) == 0 {
		return ""
	}

	q := question.Question[0]
	typeStr, exists := typeMap[q.Qtype]
	if !exists {
		typeStr = "UNKNOWN"
	}

	keyData := q.Name + ":" + typeStr
	hash := sha1.Sum([]byte(keyData))
	return hex.EncodeToString(hash[:])
}

// evictOldest removes the oldest entry from cache
func (c *DNSCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range c.cache {
		if oldestKey == "" || entry.CreatedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.CreatedAt
		}
	}

	if oldestKey != "" {
		delete(c.cache, oldestKey)
	}
}

// Clear removes all entries from cache
func (c *DNSCache) Clear() {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()
	c.cache = make(map[string]*CacheEntry)
}

// Remove removes a specific entry from cache
func (c *DNSCache) Remove(question *dns.Msg) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	if len(question.Question) == 0 {
		return
	}

	key := c.generateKey(question)
	delete(c.cache, key)
}

// CleanExpired removes all expired entries from cache
func (c *DNSCache) CleanExpired() {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	now := time.Now()
	keysToDelete := make([]string, 0)

	for key, entry := range c.cache {
		if now.After(entry.ExpiresAt) {
			keysToDelete = append(keysToDelete, key)
		}
	}

	for _, key := range keysToDelete {
		delete(c.cache, key)
	}
}

// GetStats returns cache statistics
func (c *DNSCache) GetStats() map[string]interface{} {
	c.Mutex.RLock()
	defer c.Mutex.RUnlock()

	stats := make(map[string]interface{})
	stats["size"] = len(c.cache)
	stats["max_size"] = c.maxSize
	stats["ttl"] = c.ttl.String()
	stats["hits"] = c.hits
	stats["misses"] = c.misses

	total := c.hits + c.misses
	if total > 0 {
		stats["hit_rate"] = float64(c.hits) / float64(total)
	} else {
		stats["hit_rate"] = 0.0
	}

	return stats
}

// PreloadCache preloads cache with common domains
func (c *DNSCache) PreloadCache(commonDomains []string) {
	// This is a placeholder for preload functionality
	// In a real implementation, you might want to query these domains
	// and cache their responses
}

// IsHealthy checks if cache is healthy
func (c *DNSCache) IsHealthy() bool {
	c.Mutex.RLock()
	defer c.Mutex.RUnlock()
	return len(c.cache) > 0 || c.hits > 0 || c.misses == 0
}

// GetAllKeys returns all cache keys
func (c *DNSCache) GetAllKeys() []string {
	c.Mutex.RLock()
	defer c.Mutex.RUnlock()

	keys := make([]string, 0, len(c.cache))
	for key := range c.cache {
		keys = append(keys, key)
	}

	sort.Strings(keys)
	return keys
}

// GetEntry returns a specific cache entry
func (c *DNSCache) GetEntry(key string) (*CacheEntry, bool) {
	c.Mutex.RLock()
	defer c.Mutex.RUnlock()

	entry, exists := c.cache[key]
	return entry, exists
}
