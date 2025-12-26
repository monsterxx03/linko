package proxy

import (
	"io"
	"sync"
	"time"
)

// RateLimiter implements a token bucket rate limiter
type RateLimiter struct {
	capacity    int64
	tokens      int64
	rate        int64 // tokens per second
	lastRefill  time.Time
	mu          sync.Mutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(rate int64, burst int64) *RateLimiter {
	return &RateLimiter{
		capacity:    burst,
		tokens:      burst,
		rate:        rate,
		lastRefill:  time.Now(),
	}
}

// Allow returns true if a token is available
func (r *RateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(r.lastRefill)

	// Calculate tokens to add
	tokensToAdd := int64(elapsed.Seconds()) * r.rate
	if tokensToAdd > 0 {
		r.tokens += tokensToAdd
		if r.tokens > r.capacity {
			r.tokens = r.capacity
		}
		r.lastRefill = now
	}

	// Check if token is available
	if r.tokens > 0 {
		r.tokens--
		return true
	}

	return false
}

// AllowN returns true if n tokens are available
func (r *RateLimiter) AllowN(n int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(r.lastRefill)

	// Calculate tokens to add
	tokensToAdd := int64(elapsed.Seconds()) * r.rate
	if tokensToAdd > 0 {
		r.tokens += tokensToAdd
		if r.tokens > r.capacity {
			r.tokens = r.capacity
		}
		r.lastRefill = now
	}

	// Check if n tokens are available
	if r.tokens >= n {
		r.tokens -= n
		return true
	}

	return false
}

// Reset resets the rate limiter
func (r *RateLimiter) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tokens = r.capacity
	r.lastRefill = time.Now()
}

// Capacity returns the capacity of the token bucket
func (r *RateLimiter) Capacity() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.capacity
}

// Tokens returns the current number of tokens
func (r *RateLimiter) Tokens() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tokens
}

// Rate returns the refill rate
func (r *RateLimiter) Rate() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rate
}

// ConnectionLimiter limits the number of concurrent connections
type ConnectionLimiter struct {
	maxConnections int64
	current        int64
	mu             sync.Mutex
	cond           *sync.Cond
}

// NewConnectionLimiter creates a new connection limiter
func NewConnectionLimiter(maxConnections int64) *ConnectionLimiter {
	l := &ConnectionLimiter{
		maxConnections: maxConnections,
	}
	l.cond = sync.NewCond(&l.mu)
	return l
}

// Acquire acquires a connection slot
func (l *ConnectionLimiter) Acquire() {
	l.mu.Lock()
	defer l.mu.Unlock()

	for l.current >= l.maxConnections {
		l.cond.Wait()
	}
	l.current++
}

// Release releases a connection slot
func (l *ConnectionLimiter) Release() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.current--
	if l.current < l.maxConnections {
		l.cond.Signal()
	}
}

// Current returns the current number of connections
func (l *ConnectionLimiter) Current() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.current
}

// Max returns the maximum number of connections
func (l *ConnectionLimiter) Max() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.maxConnections
}

// Available returns the number of available connection slots
func (l *ConnectionLimiter) Available() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.maxConnections - l.current
}

// BandwidthLimiter limits bandwidth usage
type BandwidthLimiter struct {
	rateLimiter *RateLimiter
	mu          sync.Mutex
}

// NewBandwidthLimiter creates a new bandwidth limiter
func NewBandwidthLimiter(bytesPerSecond int64, burstBytes int64) *BandwidthLimiter {
	return &BandwidthLimiter{
		rateLimiter: NewRateLimiter(bytesPerSecond, burstBytes),
	}
}

// AllowBytes checks if bytes can be transferred
func (b *BandwidthLimiter) AllowBytes(n int64) bool {
	return b.rateLimiter.AllowN(n)
}

// LimitReader wraps an io.Reader with bandwidth limiting
func (b *BandwidthLimiter) LimitReader(r io.Reader) *LimitedReader {
	return &LimitedReader{
		r:            r,
		bandwidthLimiter: b,
	}
}

// LimitWriter wraps an io.Writer with bandwidth limiting
func (b *BandwidthLimiter) LimitWriter(w io.Writer) *LimitedWriter {
	return &LimitedWriter{
		w:            w,
		bandwidthLimiter: b,
	}
}

// LimitedReader implements bandwidth-limited reading
type LimitedReader struct {
	r                io.Reader
	bandwidthLimiter *BandwidthLimiter
}

// Read implements io.Reader with bandwidth limiting
func (lr *LimitedReader) Read(p []byte) (n int, err error) {
	// Limit to 1KB chunks
	chunkSize := int64(len(p))
	if chunkSize > 1024 {
		chunkSize = 1024
	}

	if !lr.bandwidthLimiter.AllowBytes(chunkSize) {
		// Wait for tokens to become available
		for !lr.bandwidthLimiter.AllowBytes(chunkSize) {
			time.Sleep(10 * time.Millisecond)
		}
	}

	return lr.r.Read(p)
}

// LimitedWriter implements bandwidth-limited writing
type LimitedWriter struct {
	w                io.Writer
	bandwidthLimiter *BandwidthLimiter
}

// Write implements io.Writer with bandwidth limiting
func (lw *LimitedWriter) Write(p []byte) (n int, err error) {
	// Limit to 1KB chunks
	chunkSize := int64(len(p))
	if chunkSize > 1024 {
		chunkSize = 1024
	}

	if !lw.bandwidthLimiter.AllowBytes(chunkSize) {
		// Wait for tokens to become available
		for !lw.bandwidthLimiter.AllowBytes(chunkSize) {
			time.Sleep(10 * time.Millisecond)
		}
	}

	return lw.w.Write(p)
}