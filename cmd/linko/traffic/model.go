package traffic

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// Model represents the Bubble Tea model for MITM traffic TUI
type Model struct {
	// Connection
	serverURL string

	// Traffic events - protected by eventsMu
	eventsMu       sync.RWMutex
	events         []TrafficEvent
	eventIndex     map[string]int // ID -> events index for O(1) lookup
	filteredEvents []TrafficEvent

	// UI state
	selectedIndex int
	showPopup     bool // popup/dialog for details
	scrollOffset  int  // scroll offset for popup content
	scrollToBottom bool // flag to indicate scroll to bottom is requested

	// Search
	searchInput textinput.Model

	// Filters
	showHeaders bool // true = show headers, false = show body

	// Status
	status           ConnectionStatus
	errMsg           string
	lastConnectedAt  time.Time
	reconnectCount   int
	reconnectBackoff time.Duration

	// Window size
	width  int
	height int

	// Reconnect flag
	reconnect bool
}

// NewModel creates a new model
func NewModel(serverURL string) Model {
	ti := textinput.New()
	ti.Placeholder = "Search hostname, method, URL..."
	ti.Prompt = "🔍 "

	return Model{
		serverURL:        serverURL,
		events:           make([]TrafficEvent, 0, MaxEvents),
		eventIndex:       make(map[string]int, MaxEvents),
		filteredEvents:   make([]TrafficEvent, 0, MaxEvents),
		searchInput:      ti,
		status:           StatusConnecting,
		reconnectBackoff: ReconnectDelay,
		width:            80,
		height:           24,
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return nil
}

// mergeEvents merges new event data into existing event
func mergeEvents(existing, new TrafficEvent) TrafficEvent {
	existing.Timestamp = new.Timestamp
	if new.Direction != "" {
		existing.Direction = new.Direction
	}
	if new.Request != nil {
		existing.Request = new.Request
	}
	if new.Response != nil {
		existing.Response = new.Response
	}
	if new.Hostname != "" {
		existing.Hostname = new.Hostname
	}
	return existing
}

// truncateBody limits body size to prevent memory bloat
func truncateBody(body string) string {
	if len(body) > MaxBodySize {
		return body[:MaxBodySize] + "\n... (truncated, " + formatBytes(len(body)-MaxBodySize) + " more)"
	}
	return body
}

// formatBytes formats byte count to human readable string
func formatBytes(n int) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := n, 0
	for div >= unit && exp < 3 {
		div /= unit
		exp++
	}
	units := []string{"KB", "MB", "GB"}
	return fmt.Sprintf("%d %s", div, units[exp-1])
}

// containsCI checks if s contains substr (case insensitive)
func containsCI(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// Getters for thread-safe access

// Events returns the filtered events (copy)
func (m *Model) Events() []TrafficEvent {
	m.eventsMu.RLock()
	defer m.eventsMu.RUnlock()
	result := make([]TrafficEvent, len(m.filteredEvents))
	copy(result, m.filteredEvents)
	return result
}

// AllEvents returns all events (copy)
func (m *Model) AllEvents() []TrafficEvent {
	m.eventsMu.RLock()
	defer m.eventsMu.RUnlock()
	result := make([]TrafficEvent, len(m.events))
	copy(result, m.events)
	return result
}

// SelectedIndex returns the selected index
func (m Model) SelectedIndex() int {
	return m.selectedIndex
}

// SetSelectedIndex sets the selected index
func (m *Model) SetSelectedIndex(idx int) {
	m.selectedIndex = idx
}

// ShowPopup returns whether to show popup
func (m Model) ShowPopup() bool {
	return m.showPopup
}

// TogglePopup toggles popup visibility
func (m *Model) TogglePopup() {
	m.showPopup = !m.showPopup
	if !m.showPopup {
		m.scrollOffset = 0
	}
}

// ClosePopup closes the popup
func (m *Model) ClosePopup() {
	m.showPopup = false
	m.scrollOffset = 0
}

// ScrollOffset returns the scroll offset for popup
func (m Model) ScrollOffset() int {
	return m.scrollOffset
}

// SetScrollOffset sets the scroll offset
func (m *Model) SetScrollOffset(offset int) {
	m.scrollOffset = offset
}

// ScrollPage scrolls by a page amount
func (m *Model) ScrollPage(delta int) {
	// If at bottom marker (-1), convert to 0 first before scrolling
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}

	// Get max scroll offset for current content
	event := m.SelectedEvent()
	if event != nil {
		lines := countEventDetailLines(event, m.Width()-4, m.ShowHeaders())
		maxScroll := lines - (m.Height() - 10)
		if maxScroll < 0 {
			maxScroll = 0
		}

		// Don't scroll past bottom
		if m.scrollOffset >= maxScroll && delta > 0 {
			m.scrollOffset = maxScroll
			return
		}
		// Don't scroll past top
		if m.scrollOffset <= 0 && delta < 0 {
			m.scrollOffset = 0
			return
		}
	}

	m.scrollOffset += delta
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

// SelectedEvent returns the selected event
func (m *Model) SelectedEvent() *TrafficEvent {
	m.eventsMu.RLock()
	defer m.eventsMu.RUnlock()
	if m.selectedIndex >= 0 && m.selectedIndex < len(m.filteredEvents) {
		return &m.filteredEvents[m.selectedIndex]
	}
	return nil
}

// SearchInput returns the search input
func (m Model) SearchInput() textinput.Model {
	return m.searchInput
}

// SearchQuery returns the current search query
func (m Model) SearchQuery() string {
	return m.searchInput.Value()
}

// Status returns the connection status
func (m Model) Status() ConnectionStatus {
	return m.status
}

// SetStatus sets the connection status
func (m *Model) SetStatus(status ConnectionStatus) {
	m.status = status
	if status == StatusConnected {
		m.lastConnectedAt = time.Now()
		m.reconnectCount = 0
		m.reconnectBackoff = ReconnectDelay
	}
}

// ErrMsg returns the error message
func (m Model) ErrMsg() string {
	return m.errMsg
}

// SetErrMsg sets the error message
func (m *Model) SetErrMsg(msg string) {
	m.errMsg = msg
}

// Width returns the width
func (m Model) Width() int {
	return m.width
}

// Height returns the height
func (m Model) Height() int {
	return m.height
}

// ShowHeaders returns whether to show headers
func (m Model) ShowHeaders() bool {
	return m.showHeaders
}

// ToggleHeaders toggles between headers and body view
func (m *Model) ToggleHeaders() {
	m.showHeaders = !m.showHeaders
}

// NeedsReconnect returns whether we need to reconnect
func (m Model) NeedsReconnect() bool {
	return m.reconnect
}

// ResetReconnect resets the reconnect flag
func (m *Model) ResetReconnect() {
	m.reconnect = false
}

// TriggerReconnect triggers a reconnect
func (m *Model) TriggerReconnect() {
	m.status = StatusConnecting
	m.errMsg = ""
	m.reconnect = true
	m.reconnectCount++
}

// ClearEvents clears all events
func (m *Model) ClearEvents() {
	m.eventsMu.Lock()
	defer m.eventsMu.Unlock()
	m.events = make([]TrafficEvent, 0, MaxEvents)
	m.eventIndex = make(map[string]int, MaxEvents)
	m.filteredEvents = make([]TrafficEvent, 0, MaxEvents)
	m.selectedIndex = 0
	m.showPopup = false
}

// LastConnectedAt returns the last connected time
func (m Model) LastConnectedAt() time.Time {
	return m.lastConnectedAt
}

// ReconnectCount returns the number of reconnect attempts
func (m Model) ReconnectCount() int {
	return m.reconnectCount
}

// ReconnectBackoff returns the current reconnect backoff
func (m Model) ReconnectBackoff() time.Duration {
	return m.reconnectBackoff
}

// IncreaseBackoff increases the reconnect backoff (exponential)
func (m *Model) IncreaseBackoff() {
	m.reconnectBackoff *= 2
	if m.reconnectBackoff > MaxReconnectDelay {
		m.reconnectBackoff = MaxReconnectDelay
	}
}

// SetWindowSize sets the window size
func (m *Model) SetWindowSize(width, height int) {
	m.width = width
	m.height = height
}

// TotalEvents returns the total number of events
func (m *Model) TotalEvents() int {
	m.eventsMu.RLock()
	defer m.eventsMu.RUnlock()
	return len(m.events)
}

// FilteredCount returns the number of filtered events
func (m *Model) FilteredCount() int {
	m.eventsMu.RLock()
	defer m.eventsMu.RUnlock()
	return len(m.filteredEvents)
}

// GoToTop moves selection to top
func (m *Model) GoToTop() {
	m.selectedIndex = 0
	m.scrollOffset = 0
}

// GoToBottom moves selection to bottom
func (m *Model) GoToBottom() {
	m.eventsMu.RLock()
	defer m.eventsMu.RUnlock()
	if len(m.filteredEvents) > 0 {
		m.selectedIndex = len(m.filteredEvents) - 1
	}
}

// DeleteSelected removes the selected event
func (m *Model) DeleteSelected() bool {
	m.eventsMu.Lock()
	defer m.eventsMu.Unlock()

	if m.selectedIndex < 0 || m.selectedIndex >= len(m.filteredEvents) {
		return false
	}

	// Find and remove from events
	selectedID := m.filteredEvents[m.selectedIndex].ID
	if idx, ok := m.eventIndex[selectedID]; ok {
		m.events = append(m.events[:idx], m.events[idx+1:]...)
		delete(m.eventIndex, selectedID)
		// Rebuild index
		for i := idx; i < len(m.events); i++ {
			m.eventIndex[m.events[i].ID] = i
		}
	}

	// Rebuild filtered events
	m.applyFiltersLocked()

	// Adjust selection
	if m.selectedIndex >= len(m.filteredEvents) && len(m.filteredEvents) > 0 {
		m.selectedIndex = len(m.filteredEvents) - 1
	}
	return true
}

// View returns the rendered view
func (m Model) View() tea.View {
	v := tea.NewView(Render(&m))
	v.AltScreen = true
	return v
}
