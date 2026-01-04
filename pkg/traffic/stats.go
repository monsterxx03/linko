package traffic

import (
	"sync"
	"sync/atomic"
	"time"
)

// TrafficStats tracks traffic for a single identifier (SNI or IP)
type TrafficStats struct {
	Identifier      string    // SNI domain or target IP
	IsDomain        bool      // true if identifier is a domain (SNI/Host), false if IP
	TotalBytes      uint64    // Total bytes transferred
	UploadBytes     uint64    // Bytes uploaded (client -> target)
	DownloadBytes   uint64    // Bytes downloaded (target -> client)
	ConnectionCount uint64    // Number of connections
	FirstAccess     time.Time // First access time
	LastAccess      time.Time // Last access time
}

// TrafficRecord represents a single traffic record to be recorded
type TrafficRecord struct {
	Identifier string    // SNI domain, Host header, or target IP
	IsDomain   bool      // true if identifier is a domain
	Upload     int64     // Upload bytes in this connection
	Download   int64     // Download bytes in this connection
	Timestamp  time.Time // Connection timestamp
}

// TrafficStatsCollector collects traffic statistics asynchronously
type TrafficStatsCollector struct {
	stats      map[string]*TrafficStats
	statsMu    sync.RWMutex
	recordChan chan *TrafficRecord
	done       chan struct{}
	wg         sync.WaitGroup
}

// NewTrafficStatsCollector creates a new traffic stats collector
func NewTrafficStatsCollector() *TrafficStatsCollector {
	c := &TrafficStatsCollector{
		stats:      make(map[string]*TrafficStats),
		recordChan: make(chan *TrafficRecord, 10000),
		done:       make(chan struct{}),
	}
	c.wg.Add(1)
	go c.processLoop()
	return c
}

// Record sends a traffic record to be processed
func (c *TrafficStatsCollector) Record(record *TrafficRecord) {
	select {
	case c.recordChan <- record:
	default:
		// Channel full, drop the record to avoid blocking
	}
}

// processLoop processes traffic records in the background
func (c *TrafficStatsCollector) processLoop() {
	defer c.wg.Done()

	for {
		select {
		case record := <-c.recordChan:
			c.updateStats(record)
		case <-c.done:
			// Process remaining records
			for len(c.recordChan) > 0 {
				record := <-c.recordChan
				c.updateStats(record)
			}
			return
		}
	}
}

// updateStats updates statistics for a single record
func (c *TrafficStatsCollector) updateStats(record *TrafficRecord) {
	c.statsMu.Lock()
	stats, exists := c.stats[record.Identifier]
	if !exists {
		stats = &TrafficStats{
			Identifier:  record.Identifier,
			IsDomain:    record.IsDomain,
			FirstAccess: record.Timestamp,
		}
		c.stats[record.Identifier] = stats
	}
	c.statsMu.Unlock()

	// Update atomic counters
	atomic.AddUint64(&stats.ConnectionCount, 1)
	atomic.AddUint64(&stats.TotalBytes, uint64(record.Upload+record.Download))
	atomic.AddUint64(&stats.UploadBytes, uint64(record.Upload))
	atomic.AddUint64(&stats.DownloadBytes, uint64(record.Download))

	// Update last access time (non-atomic, occasional race is acceptable)
	stats.LastAccess = record.Timestamp
}

// GetStats returns a copy of all traffic statistics
func (c *TrafficStatsCollector) GetStats() map[string]*TrafficStats {
	c.statsMu.RLock()
	defer c.statsMu.RUnlock()

	result := make(map[string]*TrafficStats)
	for k, v := range c.stats {
		result[k] = v.copy()
	}
	return result
}

// GetTopTraffic returns traffic statistics sorted by total bytes
func (c *TrafficStatsCollector) GetTopTraffic(limit int) []*TrafficStats {
	c.statsMu.RLock()
	statsList := make([]*TrafficStats, 0, len(c.stats))
	for _, stats := range c.stats {
		statsList = append(statsList, stats.copy())
	}
	c.statsMu.RUnlock()

	// Sort by total bytes descending
	for i := 0; i < len(statsList)-1; i++ {
		for j := i + 1; j < len(statsList); j++ {
			if atomic.LoadUint64(&statsList[j].TotalBytes) > atomic.LoadUint64(&statsList[i].TotalBytes) {
				statsList[i], statsList[j] = statsList[j], statsList[i]
			}
		}
	}

	if limit > 0 && limit < len(statsList) {
		return statsList[:limit]
	}
	return statsList
}

// ClearStats clears all traffic statistics
func (c *TrafficStatsCollector) ClearStats() {
	c.statsMu.Lock()
	defer c.statsMu.Unlock()
	c.stats = make(map[string]*TrafficStats)
}

// Shutdown stops the collector and waits for it to finish
func (c *TrafficStatsCollector) Shutdown() {
	close(c.done)
	c.wg.Wait()
}

// GetSummary returns a summary of all traffic statistics
func (c *TrafficStatsCollector) GetSummary() TrafficSummary {
	c.statsMu.RLock()
	defer c.statsMu.RUnlock()

	var totalBytes, uploadBytes, downloadBytes uint64
	var connectionCount uint64
	domainCount := 0
	ipCount := 0

	for _, stats := range c.stats {
		totalBytes += atomic.LoadUint64(&stats.TotalBytes)
		uploadBytes += atomic.LoadUint64(&stats.UploadBytes)
		downloadBytes += atomic.LoadUint64(&stats.DownloadBytes)
		connectionCount += atomic.LoadUint64(&stats.ConnectionCount)
		if stats.IsDomain {
			domainCount++
		} else {
			ipCount++
		}
	}

	return TrafficSummary{
		TotalIdentifiers: len(c.stats),
		DomainCount:      domainCount,
		IPCount:          ipCount,
		TotalBytes:       totalBytes,
		UploadBytes:      uploadBytes,
		DownloadBytes:    downloadBytes,
		ConnectionCount:  connectionCount,
	}
}

// TrafficSummary represents a summary of all traffic
type TrafficSummary struct {
	TotalIdentifiers int
	DomainCount      int
	IPCount          int
	TotalBytes       uint64
	UploadBytes      uint64
	DownloadBytes    uint64
	ConnectionCount  uint64
}

// copy creates a copy of TrafficStats with atomic values read
func (s *TrafficStats) copy() *TrafficStats {
	return &TrafficStats{
		Identifier:      s.Identifier,
		IsDomain:        s.IsDomain,
		TotalBytes:      atomic.LoadUint64(&s.TotalBytes),
		UploadBytes:     atomic.LoadUint64(&s.UploadBytes),
		DownloadBytes:   atomic.LoadUint64(&s.DownloadBytes),
		ConnectionCount: atomic.LoadUint64(&s.ConnectionCount),
		FirstAccess:     s.FirstAccess,
		LastAccess:      s.LastAccess,
	}
}

// FormatTrafficStats formats traffic stats for API response
func FormatTrafficStats(s *TrafficStats) map[string]interface{} {
	totalBytes := atomic.LoadUint64(&s.TotalBytes)
	uploadBytes := atomic.LoadUint64(&s.UploadBytes)
	downloadBytes := atomic.LoadUint64(&s.DownloadBytes)
	connectionCount := atomic.LoadUint64(&s.ConnectionCount)

	return map[string]interface{}{
		"identifier":       s.Identifier,
		"is_domain":        s.IsDomain,
		"total_bytes":      totalBytes,
		"total_bytes_mb":   float64(totalBytes) / (1024 * 1024),
		"upload_bytes":     uploadBytes,
		"upload_bytes_mb":  float64(uploadBytes) / (1024 * 1024),
		"download_bytes":   downloadBytes,
		"download_bytes_mb": float64(downloadBytes) / (1024 * 1024),
		"connection_count": connectionCount,
		"first_access":     s.FirstAccess,
		"last_access":      s.LastAccess,
	}
}

// FormatSummary formats traffic summary for API response
func FormatSummary(summary TrafficSummary) map[string]interface{} {
	return map[string]interface{}{
		"total_identifiers": summary.TotalIdentifiers,
		"domain_count":      summary.DomainCount,
		"ip_count":          summary.IPCount,
		"total_bytes":       summary.TotalBytes,
		"total_bytes_mb":    float64(summary.TotalBytes) / (1024 * 1024),
		"upload_bytes":      summary.UploadBytes,
		"upload_bytes_mb":   float64(summary.UploadBytes) / (1024 * 1024),
		"download_bytes":    summary.DownloadBytes,
		"download_bytes_mb": float64(summary.DownloadBytes) / (1024 * 1024),
		"connection_count":  summary.ConnectionCount,
	}
}
