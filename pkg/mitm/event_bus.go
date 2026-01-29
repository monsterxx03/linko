package mitm

import (
	"log/slog"
	"sync"
	"time"
)

// TrafficEvent represents a single MITM traffic event
type TrafficEvent struct {
	ID           string        `json:"id"`                 // Unique event ID
	Hostname     string        `json:"hostname"`           // Target hostname
	Timestamp    time.Time     `json:"timestamp"`          // Event timestamp
	Direction    string        `json:"direction"`          // Traffic direction
	ConnectionID string        `json:"connection_id"`      // Unique connection ID
	RequestID    string        `json:"request_id"`         // Unique request ID (per connection)
	Request      *HTTPRequest  `json:"request,omitempty"`  // Request details if available
	Response     *HTTPResponse `json:"response,omitempty"` // Response details if available
}

// HTTPRequest represents an HTTP request
type HTTPRequest struct {
	Method        string            `json:"method"`         // HTTP method
	URL           string            `json:"url"`            // Request URL
	Host          string            `json:"host"`           // Request host
	Headers       map[string]string `json:"headers"`        // Request headers
	Body          string            `json:"body"`           // Request body (truncated)
	ContentType   string            `json:"content_type"`   // Content-Type header
	ContentLength int64             `json:"content_length"` // Content-Length header
}

// HTTPResponse represents an HTTP response
type HTTPResponse struct {
	Status        string            `json:"status"`         // Status line
	StatusCode    int               `json:"status_code"`    // Status code
	Headers       map[string]string `json:"headers"`        // Response headers
	Body          string            `json:"body"`           // Response body (truncated)
	ContentType   string            `json:"content_type"`   // Content-Type header
	ContentLength int64             `json:"content_length"` // Content-Length header
	Latency       int64             `json:"latency"`        // Response latency in milliseconds
}

// Subscriber represents an event subscriber
type Subscriber struct {
	ID      string             // Unique subscriber ID
	Channel chan *TrafficEvent // Channel for receiving events
}

// EventBus manages the publishing and subscribing of traffic events
type EventBus struct {
	subscribers map[*Subscriber]bool // Active subscribers
	mu          sync.RWMutex         // Mutex for thread safety
	logger      *slog.Logger         // Logger for error and warning messages
	history     []*TrafficEvent      // Historical events for replay
	historySize int                  // Maximum number of historical events to keep
}

// NewEventBus creates a new EventBus with the specified history size
func NewEventBus(logger *slog.Logger, historySize int) *EventBus {
	if historySize <= 0 {
		historySize = 10 // Default to 10 events
	}
	return &EventBus{
		subscribers: make(map[*Subscriber]bool),
		logger:      logger,
		history:     make([]*TrafficEvent, 0, historySize),
		historySize: historySize,
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

	// Add to history (keep only the latest N events)
	eb.history = append(eb.history, event)
	if len(eb.history) > eb.historySize {
		eb.history = eb.history[len(eb.history)-eb.historySize:]
	}

	// Publish to all subscribers
	for subscriber := range eb.subscribers {
		select {
		case subscriber.Channel <- event:
			// Event sent successfully
		default:
			eb.logger.Warn("Subscriber channel is full, skipping event",
				"subscriber_id", subscriber.ID,
				"event_id", event.ID,
				"hostname", event.Hostname)
		}
	}

	eb.mu.Unlock()
}

// Subscribe creates a new subscriber and returns it
func (eb *EventBus) Subscribe() *Subscriber {
	subscriber := &Subscriber{
		ID:      time.Now().Format("20060102150405.000000") + "-sub",
		Channel: make(chan *TrafficEvent, 100), // Buffered channel to prevent blocking
	}

	eb.mu.Lock()
	eb.subscribers[subscriber] = true

	// Copy historical events for replay
	historicalEvents := make([]*TrafficEvent, len(eb.history))
	copy(historicalEvents, eb.history)
	eb.mu.Unlock()

	// Send historical events in a new goroutine (oldest to newest)
	go func() {
		for _, ev := range historicalEvents {
			select {
			case subscriber.Channel <- ev:
			default:
				// Channel full, skip this historical event
			}
		}
	}()

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

// GetSubscriberCount returns the number of active subscribers
func (eb *EventBus) GetSubscriberCount() int {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	return len(eb.subscribers)
}
