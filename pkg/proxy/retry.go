package proxy

import (
	"fmt"
	"math"
	"math/rand"
	"net"
	"time"
)

// RetryConfig holds retry configuration
type RetryConfig struct {
	MaxAttempts    int
	InitialDelay   time.Duration
	MaxDelay       time.Duration
	BackoffFactor  float64
	Jitter         bool
	RetryableFuncs []func(error) bool
}

// NewRetryConfig creates a default retry configuration
func NewRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts:   3,
		InitialDelay:  1 * time.Second,
		MaxDelay:      30 * time.Second,
		BackoffFactor: 2.0,
		Jitter:        true,
	}
}

// AddRetryableFunc adds a function that checks if an error is retryable
func (c *RetryConfig) AddRetryableFunc(fn func(error) bool) *RetryConfig {
	c.RetryableFuncs = append(c.RetryableFuncs, fn)
	return c
}

// IsRetryable checks if an error is retryable
func (c *RetryConfig) IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Check if error matches any retryable functions
	for _, fn := range c.RetryableFuncs {
		if fn(err) {
			return true
		}
	}

	// Default retryable errors
	errStr := err.Error()
	retryablePatterns := []string{
		"connection refused",
		"connection reset",
		"temporary failure",
		"timeout",
		"network unreachable",
		"no route to host",
	}

	for _, pattern := range retryablePatterns {
		if containsString(errStr, pattern) {
			return true
		}
	}

	return false
}

// Retry executes a function with retry logic
func (c *RetryConfig) Retry(fn func() error) (err error) {
	var attempt int
	var delay time.Duration

	for attempt = 0; attempt < c.MaxAttempts; attempt++ {
		err = fn()

		if err == nil {
			return nil
		}

		if !c.IsRetryable(err) {
			return err
		}

		if attempt == c.MaxAttempts-1 {
			return fmt.Errorf("max attempts reached: %w", err)
		}

		// Calculate delay with exponential backoff
		delay = time.Duration(float64(c.InitialDelay) * math.Pow(c.BackoffFactor, float64(attempt)))

		// Cap delay at MaxDelay
		if delay > c.MaxDelay {
			delay = c.MaxDelay
		}

		// Apply jitter if enabled
		if c.Jitter {
			jitter := time.Duration(rand.Int63n(int64(delay)))
			delay = delay - time.Duration(jitter/2)
		}

		time.Sleep(delay)
	}

	return fmt.Errorf("max attempts reached: %w", err)
}

// WithRetry wraps a function with retry logic
func WithRetry(fn func() error, config *RetryConfig) error {
	if config == nil {
		config = NewRetryConfig()
	}
	return config.Retry(fn)
}

// RetryableConnection wraps a net.Conn with retry logic
type RetryableConnection struct {
	conn      net.Conn
	retryCfg  *RetryConfig
	addr      string
	network   string
}

// NewRetryableConnection creates a new retryable connection
func NewRetryableConnection(network, addr string, retryCfg *RetryConfig) *RetryableConnection {
	return &RetryableConnection{
		network:  network,
		addr:     addr,
		retryCfg: retryCfg,
	}
}

// Connect establishes a connection with retry logic
func (rc *RetryableConnection) Connect() error {
	return rc.retryCfg.Retry(func() error {
		conn, err := net.Dial(rc.network, rc.addr)
		if err != nil {
			return err
		}
		rc.conn = conn
		return nil
	})
}

// Read reads data with retry logic
func (rc *RetryableConnection) Read(p []byte) (n int, err error) {
	if rc.conn == nil {
		return 0, fmt.Errorf("connection not established")
	}

	result, err := rc.retryCfg.RetryWithValue(func() (interface{}, error) {
		n, err := rc.conn.Read(p)
		return n, err
	})
	if err != nil {
		return 0, err
	}
	return result.(int), nil
}

// Write writes data with retry logic
func (rc *RetryableConnection) Write(p []byte) (n int, err error) {
	if rc.conn == nil {
		return 0, fmt.Errorf("connection not established")
	}

	result, err := rc.retryCfg.RetryWithValue(func() (interface{}, error) {
		n, err := rc.conn.Write(p)
		return n, err
	})
	if err != nil {
		return 0, err
	}
	return result.(int), nil
}

// Close closes the connection
func (rc *RetryableConnection) Close() error {
	if rc.conn != nil {
		return rc.conn.Close()
	}
	return nil
}

// LocalAddr returns the local address
func (rc *RetryableConnection) LocalAddr() net.Addr {
	if rc.conn != nil {
		return rc.conn.LocalAddr()
	}
	return nil
}

// RemoteAddr returns the remote address
func (rc *RetryableConnection) RemoteAddr() net.Addr {
	if rc.conn != nil {
		return rc.conn.RemoteAddr()
	}
	return nil
}

// SetDeadline sets read and write deadlines
func (rc *RetryableConnection) SetDeadline(t time.Time) error {
	if rc.conn != nil {
		return rc.conn.SetDeadline(t)
	}
	return nil
}

// SetReadDeadline sets read deadline
func (rc *RetryableConnection) SetReadDeadline(t time.Time) error {
	if rc.conn != nil {
		return rc.conn.SetReadDeadline(t)
	}
	return nil
}

// SetWriteDeadline sets write deadline
func (rc *RetryableConnection) SetWriteDeadline(t time.Time) error {
	if rc.conn != nil {
		return rc.conn.SetWriteDeadline(t)
	}
	return nil
}

// RetryWithValue executes a function with retry and returns the value
func (c *RetryConfig) RetryWithValue(fn func() (interface{}, error)) (interface{}, error) {
	var attempt int
	var delay time.Duration

	for attempt = 0; attempt < c.MaxAttempts; attempt++ {
		result, err := fn()

		if err == nil {
			return result, nil
		}

		if !c.IsRetryable(err) {
			return nil, err
		}

		if attempt == c.MaxAttempts-1 {
			return nil, fmt.Errorf("max attempts reached: %w", err)
		}

		// Calculate delay with exponential backoff
		delay = time.Duration(float64(c.InitialDelay) * math.Pow(c.BackoffFactor, float64(attempt)))

		// Cap delay at MaxDelay
		if delay > c.MaxDelay {
			delay = c.MaxDelay
		}

		// Apply jitter if enabled
		if c.Jitter {
			jitter := time.Duration(rand.Int63n(int64(delay)))
			delay = delay - time.Duration(jitter/2)
		}

		time.Sleep(delay)
	}

	return nil, fmt.Errorf("max attempts reached")
}

// containsString checks if a string contains a substring (case-insensitive)
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && containsHelper(s, substr)
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if equalIgnoreCase(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

func equalIgnoreCase(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if toLower(a[i]) != toLower(b[i]) {
			return false
		}
	}
	return true
}

func toLower(b byte) byte {
	if 'A' <= b && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}