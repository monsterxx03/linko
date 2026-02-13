package dns

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

type QueryType string

type DomainStats struct {
	Domain          string
	TotalQueries    uint64
	SuccessQueries  uint64
	FailedQueries   uint64
	TotalResponseNs uint64
	QueryTypes      map[QueryType]*QueryTypeStats
	FirstQueryTime  time.Time
	LastQueryTime   time.Time
}

type QueryTypeStats struct {
	Count        uint64
	SuccessCount uint64
	FailedCount  uint64
	TotalNs      uint64
}

type DNSStatsCollector struct {
	domains           map[string]*DomainStats
	domainsMu         sync.RWMutex
	queryChan         chan *QueryRecord
	done              chan struct{}
	wg                sync.WaitGroup
	aggregationTicker *time.Ticker
}

type QueryRecord struct {
	Domain       string
	QueryType    QueryType
	ResponseTime time.Duration
	Success      bool
	Timestamp    time.Time
}

func NewDNSStatsCollector() *DNSStatsCollector {
	c := &DNSStatsCollector{
		domains:           make(map[string]*DomainStats),
		queryChan:         make(chan *QueryRecord, 10000),
		done:              make(chan struct{}),
		aggregationTicker: time.NewTicker(5 * time.Minute),
	}
	c.wg.Go(c.processLoop)
	return c
}

func (c *DNSStatsCollector) RecordQuery(record *QueryRecord) {
	select {
	case c.queryChan <- record:
	default:
	}
}

func (c *DNSStatsCollector) processLoop() {
	for {
		select {
		case record := <-c.queryChan:
			c.updateStats(record)
		case <-c.aggregationTicker.C:
			c.aggregateStats()
		case <-c.done:
			for len(c.queryChan) > 0 {
				record := <-c.queryChan
				c.updateStats(record)
			}
			return
		}
	}
}

func (c *DNSStatsCollector) updateStats(record *QueryRecord) {
	c.domainsMu.Lock()
	stats, exists := c.domains[record.Domain]
	if !exists {
		stats = &DomainStats{
			Domain:         record.Domain,
			QueryTypes:     make(map[QueryType]*QueryTypeStats),
			FirstQueryTime: record.Timestamp,
		}
		c.domains[record.Domain] = stats
	}
	stats.TotalQueries++
	stats.LastQueryTime = record.Timestamp
	atomic.AddUint64(&stats.TotalResponseNs, uint64(record.ResponseTime))

	if record.Success {
		atomic.AddUint64(&stats.SuccessQueries, 1)
	} else {
		atomic.AddUint64(&stats.FailedQueries, 1)
	}

	if _, exists := stats.QueryTypes[record.QueryType]; !exists {
		stats.QueryTypes[record.QueryType] = &QueryTypeStats{}
	}
	typeStats := stats.QueryTypes[record.QueryType]
	atomic.AddUint64(&typeStats.Count, 1)
	atomic.AddUint64(&typeStats.TotalNs, uint64(record.ResponseTime))
	if record.Success {
		atomic.AddUint64(&typeStats.SuccessCount, 1)
	} else {
		atomic.AddUint64(&typeStats.FailedCount, 1)
	}

	c.domainsMu.Unlock()
}

func (c *DNSStatsCollector) aggregateStats() {
	c.domainsMu.Lock()
	defer c.domainsMu.Unlock()

	cutoff := time.Now().AddDate(0, 0, -7)
	cleanedCount := 0

	for domain, stats := range c.domains {
		if stats.LastQueryTime.Before(cutoff) {
			delete(c.domains, domain)
			cleanedCount++
		}
	}

	if cleanedCount > 0 {
		slog.Info("Cleaned up inactive domain stats", "count", cleanedCount, "remaining", len(c.domains))
	}
}

func (c *DNSStatsCollector) GetDomainStats(domain string) (*DomainStats, bool) {
	c.domainsMu.RLock()
	defer c.domainsMu.RUnlock()
	stats, exists := c.domains[domain]
	return stats, exists
}

func (c *DNSStatsCollector) GetAllStats() map[string]*DomainStats {
	c.domainsMu.RLock()
	defer c.domainsMu.RUnlock()
	result := make(map[string]*DomainStats)
	for k, v := range c.domains {
		result[k] = v.copy()
	}
	return result
}

func (c *DNSStatsCollector) ClearStats() {
	c.domainsMu.Lock()
	defer c.domainsMu.Unlock()
	c.domains = make(map[string]*DomainStats)
}

func (c *DNSStatsCollector) GetTopDomains(limit int, sortBy string) []*DomainStats {
	c.domainsMu.RLock()
	domains := make([]*DomainStats, 0, len(c.domains))
	for _, stats := range c.domains {
		domains = append(domains, stats.copy())
	}
	c.domainsMu.RUnlock()

	switch sortBy {
	case "queries":
		for i := 0; i < len(domains)-1; i++ {
			for j := i + 1; j < len(domains); j++ {
				if atomic.LoadUint64(&domains[j].TotalQueries) > atomic.LoadUint64(&domains[i].TotalQueries) {
					domains[i], domains[j] = domains[j], domains[i]
				}
			}
		}
	case "response_time":
		for i := 0; i < len(domains)-1; i++ {
			for j := i + 1; j < len(domains); j++ {
				avgI := float64(atomic.LoadUint64(&domains[i].TotalResponseNs)) / float64(atomic.LoadUint64(&domains[i].TotalQueries))
				avgJ := float64(atomic.LoadUint64(&domains[j].TotalResponseNs)) / float64(atomic.LoadUint64(&domains[j].TotalQueries))
				if avgJ > avgI {
					domains[i], domains[j] = domains[j], domains[i]
				}
			}
		}
	}

	if limit > 0 && limit < len(domains) {
		return domains[:limit]
	}
	return domains
}

func (c *DNSStatsCollector) GetStatsSummary() StatsSummary {
	c.domainsMu.RLock()
	defer c.domainsMu.RUnlock()

	var totalQueries, totalSuccess, totalFailed uint64
	var totalResponseNs uint64
	domainCount := len(c.domains)

	for _, stats := range c.domains {
		totalQueries += atomic.LoadUint64(&stats.TotalQueries)
		totalSuccess += atomic.LoadUint64(&stats.SuccessQueries)
		totalFailed += atomic.LoadUint64(&stats.FailedQueries)
		totalResponseNs += atomic.LoadUint64(&stats.TotalResponseNs)
	}

	avgResponseTime := time.Duration(0)
	if totalQueries > 0 {
		avgResponseTime = time.Duration(totalResponseNs / totalQueries)
	}

	successRate := 0.0
	if totalQueries > 0 {
		successRate = float64(totalSuccess) / float64(totalQueries) * 100
	}

	return StatsSummary{
		TotalDomains:    domainCount,
		TotalQueries:    totalQueries,
		TotalSuccess:    totalSuccess,
		TotalFailed:     totalFailed,
		SuccessRate:     successRate,
		AvgResponseTime: avgResponseTime,
	}
}

func (c *DNSStatsCollector) Shutdown() {
	close(c.done)
	c.wg.Wait()
}

type StatsSummary struct {
	TotalDomains    int
	TotalQueries    uint64
	TotalSuccess    uint64
	TotalFailed     uint64
	SuccessRate     float64
	AvgResponseTime time.Duration
}

func (s *DomainStats) copy() *DomainStats {
	stats := &DomainStats{
		Domain:          s.Domain,
		TotalQueries:    atomic.LoadUint64(&s.TotalQueries),
		SuccessQueries:  atomic.LoadUint64(&s.SuccessQueries),
		FailedQueries:   atomic.LoadUint64(&s.FailedQueries),
		TotalResponseNs: atomic.LoadUint64(&s.TotalResponseNs),
		QueryTypes:      make(map[QueryType]*QueryTypeStats),
		FirstQueryTime:  s.FirstQueryTime,
		LastQueryTime:   s.LastQueryTime,
	}
	for qt, ts := range s.QueryTypes {
		stats.QueryTypes[qt] = &QueryTypeStats{
			Count:        atomic.LoadUint64(&ts.Count),
			SuccessCount: atomic.LoadUint64(&ts.SuccessCount),
			FailedCount:  atomic.LoadUint64(&ts.FailedCount),
			TotalNs:      atomic.LoadUint64(&ts.TotalNs),
		}
	}
	return stats
}

func FormatDomainStats(d *DomainStats) map[string]interface{} {
	avgResponseTime := time.Duration(0)
	if d.TotalQueries > 0 {
		avgResponseTime = time.Duration(d.TotalResponseNs / d.TotalQueries)
	}

	queryTypes := make(map[string]interface{})
	for qt, ts := range d.QueryTypes {
		avgNs := time.Duration(0)
		if ts.Count > 0 {
			avgNs = time.Duration(ts.TotalNs / ts.Count)
		}
		queryTypes[string(qt)] = map[string]interface{}{
			"count":         ts.Count,
			"success_count": ts.SuccessCount,
			"failed_count":  ts.FailedCount,
			"avg_ns":        avgNs.String(),
		}
	}

	return map[string]interface{}{
		"domain":            d.Domain,
		"total_queries":     d.TotalQueries,
		"success_queries":   d.SuccessQueries,
		"failed_queries":    d.FailedQueries,
		"avg_response_time": avgResponseTime.String(),
		"first_query":       d.FirstQueryTime,
		"last_query":        d.LastQueryTime,
		"query_types":       queryTypes,
	}
}
