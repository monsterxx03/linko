package traffic

import (
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbletea/v2"
)

// Model represents the Bubble Tea model for MITM traffic TUI
type Model struct {
	// Connection
	sseClient    *SSEClient
	serverURL    string

	// Traffic events
	events       []TrafficEvent
	filteredEvents []TrafficEvent

	// UI state
	selectedIndex int
	showPopup     bool // popup/dialog for details

	// Search
	searchInput textinput.Model

	// Filters
	showHeaders bool // true = show headers, false = show body

	// Status
	status      ConnectionStatus
	errMsg      string

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
		serverURL:      serverURL,
		sseClient:      NewSSEClient(serverURL),
		events:         make([]TrafficEvent, 0, 100),
		filteredEvents: make([]TrafficEvent, 0, 100),
		searchInput:    ti,
		status:         StatusConnecting,
		width:          80,
		height:         24,
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return nil
}

// Update updates the model based on the message
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// Let textinput handle the key first
		m.searchInput, cmd = m.searchInput.Update(msg)

		// Then handle navigation keys
		key := msg.Key()
		switch key.Code {
		case tea.KeyUp:
			if m.selectedIndex > 0 {
				m.selectedIndex--
			}
		case tea.KeyDown:
			if m.selectedIndex < len(m.filteredEvents)-1 {
				m.selectedIndex++
			}
		case tea.KeyEnter:
			if m.selectedIndex >= 0 && m.selectedIndex < len(m.filteredEvents) {
				m.showPopup = !m.showPopup
			}
		case tea.KeyTab:
			m.showHeaders = !m.showHeaders
		case tea.KeyEscape:
			return m, tea.Quit
		default:
			// Handle Ctrl+C and q using string comparison
			if key.String() == "ctrl+c" || key.String() == "q" {
				return m, tea.Quit
			}
			// Handle character keys
			switch key.Text {
			case "r":
				m.status = StatusConnecting
				m.errMsg = ""
				m.reconnect = true
			case "c":
				m.events = make([]TrafficEvent, 0, 100)
				m.filteredEvents = make([]TrafficEvent, 0, 100)
				m.selectedIndex = 0
				m.showPopup = false
			case "/":
				focusCmd := m.searchInput.Focus()
				cmd = focusCmd
			case "j":
				if m.selectedIndex < len(m.filteredEvents)-1 {
					m.selectedIndex++
				}
			case "k":
				if m.selectedIndex > 0 {
					m.selectedIndex--
				}
			}
		}

		// Apply filters after any input changes
		m.applyFilters()
		return m, cmd

	case trafficEventMsg:
		// Try to find existing event with same ID (requestID) for merging
		eventID := msg.event.ID
		merged := false
		for i, e := range m.events {
			if e.ID == eventID {
				// Merge event data: update fields that are present in new event
				m.events[i].Timestamp = msg.event.Timestamp
				if msg.event.Direction != "" {
					m.events[i].Direction = msg.event.Direction
				}
				if msg.event.Request != nil {
					m.events[i].Request = msg.event.Request
				}
				if msg.event.Response != nil {
					m.events[i].Response = msg.event.Response
				}
				if msg.event.Hostname != "" {
					m.events[i].Hostname = msg.event.Hostname
				}
				merged = true
				break
			}
		}
		// If not merged, add as new event at the beginning
		if !merged {
			m.events = append([]TrafficEvent{msg.event}, m.events...)
			if len(m.events) > 100 {
				m.events = m.events[:100]
			}
		}
		m.applyFilters()
		if m.selectedIndex >= len(m.filteredEvents) && len(m.filteredEvents) > 0 {
			m.selectedIndex = len(m.filteredEvents) - 1
		}

	case connectionStatusMsg:
		m.status = msg.status

	case errorMsg:
		m.errMsg = msg.error.Error()

	default:
		m.searchInput, cmd = m.searchInput.Update(msg)
		m.applyFilters()
	}

	return m, cmd
}

// applyFilters applies search and direction filters
func (m *Model) applyFilters() {
	query := m.searchInput.Value()
	m.filteredEvents = make([]TrafficEvent, 0, len(m.events))

	for _, e := range m.events {
		// Search filter
		if query != "" {
			match := false
			// Search in hostname
			if containsCI(e.Hostname, query) {
				match = true
			}
			// Search in request
			if e.Request != nil {
				if containsCI(e.Request.Method, query) ||
					containsCI(e.Request.URL, query) ||
					containsCI(e.Request.Host, query) {
					match = true
				}
			}
			// Search in response
			if e.Response != nil {
				if containsCI(e.Response.Status, query) {
					match = true
				}
			}
			if !match {
				continue
			}
		}

		m.filteredEvents = append(m.filteredEvents, e)
	}
}

// containsCI checks if s contains substr (case insensitive)
func containsCI(s, substr string) bool {
	return len(s) >= len(substr) && (len(s) == 0 ||
		findCI(s, substr))
}

func findCI(s, substr string) bool {
	sLower := toLower(s)
	substrLower := toLower(substr)
	for i := 0; i <= len(s)-len(substr); i++ {
		if sLower[i:i+len(substr)] == substrLower {
			return true
		}
	}
	return false
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		result[i] = c
	}
	return string(result)
}

// Events returns the filtered events
func (m Model) Events() []TrafficEvent {
	return m.filteredEvents
}

// SelectedIndex returns the selected index
func (m Model) SelectedIndex() int {
	return m.selectedIndex
}

// ShowPopup returns whether to show popup
func (m Model) ShowPopup() bool {
	return m.showPopup
}

// SelectedEvent returns the selected event
func (m Model) SelectedEvent() *TrafficEvent {
	if m.selectedIndex >= 0 && m.selectedIndex < len(m.filteredEvents) {
		return &m.filteredEvents[m.selectedIndex]
	}
	return nil
}

// SearchInput returns the search input
func (m Model) SearchInput() textinput.Model {
	return m.searchInput
}

// Status returns the connection status
func (m Model) Status() ConnectionStatus {
	return m.status
}

// ErrMsg returns the error message
func (m Model) ErrMsg() string {
	return m.errMsg
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

// NeedsReconnect returns whether we need to reconnect
func (m Model) NeedsReconnect() bool {
	return m.reconnect
}

// ResetReconnect resets the reconnect flag
func (m *Model) ResetReconnect() {
	m.reconnect = false
}

// View returns the rendered view
func (m Model) View() tea.View {
	v := tea.NewView(Render(m))
	v.AltScreen = true
	return v
}

// Message types for Bubble Tea

type trafficEventMsg struct {
	event TrafficEvent
}

type connectionStatusMsg struct {
	status ConnectionStatus
}

type errorMsg struct {
	error error
}
