package mitm

import (
	"sync"
	"time"
)

// TrafficEvent represents a single MITM traffic event
type TrafficEvent struct {
	ID           string       `json:"id"`            // Unique event ID
	Hostname     string       `json:"hostname"`      // Target hostname
	Timestamp    time.Time    `json:"timestamp"`     // Event timestamp
	Direction    string       `json:"direction"`     // Traffic direction
	ConnectionID string       `json:"connection_id"` // Unique connection ID
	Request      *HTTPRequest `json:"request,omitempty"`  // Request details if available
	Response     *HTTPResponse `json:"response,omitempty"` // Response details if available
}

// HTTPRequest represents an HTTP request
type HTTPRequest struct {
	Method      string            `json:"method"`      // HTTP method
	URL         string            `json:"url"`         // Request URL
	Host        string            `json:"host"`        // Request host
	Headers     map[string]string `json:"headers"`     // Request headers
	Body        string            `json:"body"`        // Request body (truncated)
	ContentType string            `json:"content_type"` // Content-Type header
	ContentLength int64          `json:"content_length"` // Content-Length header
}

// HTTPResponse represents an HTTP response
type HTTPResponse struct {
	Status        string            `json:"status"`        // Status line
	StatusCode    int               `json:"status_code"`   // Status code
	Headers       map[string]string `json:"headers"`       // Response headers
	Body          string            `json:"body"`          // Response body (truncated)
	ContentType   string            `json:"content_type"`  // Content-Type header
	ContentLength int64             `json:"content_length"` // Content-Length header
	Latency       int64             `json:"latency"`       // Response latency in milliseconds
}

// Subscriber represents an event subscriber
type Subscriber struct {
	ID      string                // Unique subscriber ID
	Channel chan *TrafficEvent    // Channel for receiving events
}

// EventBus manages the publishing and subscribing of traffic events
type EventBus struct {
	subscribers map[*Subscriber]bool // Active subscribers
	mu          sync.RWMutex         // Mutex for thread safety
	buffer      []*TrafficEvent      // Historical events buffer
	bufferSize  int                  // Maximum buffer size
}

// NewEventBus creates a new EventBus with the given buffer size
func NewEventBus(bufferSize int) *EventBus {
	if bufferSize <= 0 {
		bufferSize = 100 // Default buffer size
	}

	return &EventBus{
		subscribers: make(map[*Subscriber]bool),
		buffer:      make([]*TrafficEvent, 0, bufferSize),
		bufferSize:  bufferSize,
	}
}

// Publish publishes a traffic event to all subscribers
func (eb *EventBus) Publish(event *TrafficEvent) {
	// Generate a unique ID if not provided
	if event.ID == "" {
		event.ID = time.Now().Format("20060102150405.000000") + "-" + event.Hostname
	}

	// Set timestamp if not provided
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Lock for writing
	eb.mu.Lock()
	defer eb.mu.Unlock()

	// Add to buffer
	eb.buffer = append(eb.buffer, event)
	// Trim buffer if it exceeds size
	if len(eb.buffer) > eb.bufferSize {
		eb.buffer = eb.buffer[len(eb.buffer)-eb.bufferSize:]
	}

	// Publish to all subscribers
	for subscriber := range eb.subscribers {
		select {
		case subscriber.Channel <- event:
			// Event sent successfully
		default:
			// Subscriber channel is full, skip
		}
	}
}

// Subscribe creates a new subscriber and returns it
func (eb *EventBus) Subscribe() *Subscriber {
	subscriber := &Subscriber{
		ID:      time.Now().Format("20060102150405.000000") + "-sub",
		Channel: make(chan *TrafficEvent, 100), // Buffered channel to prevent blocking
	}

	eb.mu.Lock()
	eb.subscribers[subscriber] = true
	eb.mu.Unlock()

	return subscriber
}

// Unsubscribe removes a subscriber from the event bus
func (eb *EventBus) Unsubscribe(subscriber *Subscriber) {
	eb.mu.Lock()
	if _, ok := eb.subscribers[subscriber]; ok {
		delete(eb.subscribers, subscriber)
		close(subscriber.Channel)
	}
	eb.mu.Unlock()
}

// GetHistory returns the recent traffic events
func (eb *EventBus) GetHistory() []*TrafficEvent {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	// Return a copy of the buffer
	history := make([]*TrafficEvent, len(eb.buffer))
	copy(history, eb.buffer)
	return history
}

// GetSubscriberCount returns the number of active subscribers
func (eb *EventBus) GetSubscriberCount() int {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	return len(eb.subscribers)
}
