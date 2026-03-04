package traffic

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// SSEClient handles Server-Sent Events connections
type SSEClient struct {
	url        string
	httpClient *http.Client
	events     chan *TrafficEvent
	errors     chan error
	status     ConnectionStatus
	done       chan struct{}
}

// NewSSEClient creates a new SSE client
func NewSSEClient(url string) *SSEClient {
	return &SSEClient{
		url: url,
		httpClient: &http.Client{
			Timeout: 0, // No timeout for SSE
		},
		events: make(chan *TrafficEvent, 100),
		errors: make(chan error, 10),
		done:   make(chan struct{}),
	}
}

// Events returns the events channel
func (c *SSEClient) Events() <-chan *TrafficEvent {
	return c.events
}

// Errors returns the errors channel
func (c *SSEClient) Errors() <-chan error {
	return c.errors
}

// Status returns the current connection status
func (c *SSEClient) Status() ConnectionStatus {
	return c.status
}

// Connect establishes the SSE connection
func (c *SSEClient) Connect(ctx context.Context) error {
	c.status = StatusConnecting

	req, err := http.NewRequestWithContext(ctx, "GET", c.url, nil)
	if err != nil {
		c.status = StatusError
		return err
	}

	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.status = StatusError
		return err
	}

	if resp.StatusCode != http.StatusOK {
		c.status = StatusError
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	c.status = StatusConnected

	go c.readStream(ctx, resp.Body)

	return nil
}

// readStream reads the SSE stream
func (c *SSEClient) readStream(ctx context.Context, body io.Reader) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024) // 10MB max line

	var eventType string
	var eventData string

	// Regex to parse SSE lines
	dataLineRegex := regexp.MustCompile(`^data:\s*(.*)$`)
	eventLineRegex := regexp.MustCompile(`^event:\s*(.*)$`)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			c.status = StatusDisconnected
			close(c.done)
			return
		case <-c.done:
			return
		default:
		}

		line := scanner.Text()

		// Empty line marks end of event
		if line == "" {
			if eventType == "traffic" && eventData != "" {
				event, err := ParseTrafficEvent([]byte(eventData))
				if err != nil {
					select {
					case c.errors <- fmt.Errorf("failed to parse event: %w", err):
					default:
					}
				} else {
					select {
					case c.events <- event:
					case <-c.done:
						return
					}
				}
			}
			eventType = ""
			eventData = ""
			continue
		}

		// Skip comments
		if strings.HasPrefix(line, ":") {
			continue
		}

		// Parse data line
		if matches := dataLineRegex.FindStringSubmatch(line); len(matches) == 2 {
			eventData = matches[1]
			continue
		}

		// Parse event line
		if matches := eventLineRegex.FindStringSubmatch(line); len(matches) == 2 {
			eventType = matches[1]
		}
	}

	if err := scanner.Err(); err != nil {
		c.status = StatusError
		select {
		case c.errors <- fmt.Errorf("scanner error: %w", err):
		default:
		}
	}

	c.status = StatusDisconnected
	close(c.done)
}

// Disconnect closes the SSE connection
func (c *SSEClient) Disconnect() {
	if c.done != nil {
		close(c.done)
	}
	c.status = StatusDisconnected
}

// JSONPretty prints JSON in a readable format
func JSONPretty(data string) string {
	var v interface{}
	if err := json.Unmarshal([]byte(data), &v); err != nil {
		return data
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return data
	}
	return string(b)
}

// Truncate truncates a string to max length
func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// FormatLatency formats latency in milliseconds to a human readable string
func FormatLatency(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.1fs", float64(ms)/1000)
}
