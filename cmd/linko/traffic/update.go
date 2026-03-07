package traffic

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// Update updates the model based on the message
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetWindowSize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case scrollToBottomMsg:
		// Calculate max scroll offset based on current content
		// This is called after view has rendered and updated content
		event := m.SelectedEvent()
		if event != nil {
			// Calculate lines count similar to renderEventDetailsFull
			lines := countEventDetailLines(event, m.Width()-4, m.ShowHeaders())
			maxScroll := lines - (m.Height() - 10)
			if maxScroll < 0 {
				maxScroll = 0
			}
			m.scrollOffset = maxScroll
		}

	case trafficEventMsg:
		m.AddEvent(msg.event)

	case connectionStatusMsg:
		m.SetStatus(msg.status)

	case errorMsg:
		m.SetErrMsg(msg.err.Error())

	case reconnectMsg:
		// Trigger reconnect
		return m, m.tryReconnect()

	case reconnectTickMsg:
		// Reconnect timer ticked
		m.TriggerReconnect()
		return m, nil

	default:
		// Let search input handle other messages
		m.searchInput, cmd = m.searchInput.Update(msg)
		m.applyFilters()
	}

	return m, cmd
}

// handleKeyMsg handles keyboard input
func (m *Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	// Let textinput handle the key first
	m.searchInput, cmd = m.searchInput.Update(msg)

	// Then handle navigation keys
	key := msg.Key()

	// Handle special keys first
	switch key.Code {
	case tea.KeyUp:
		if m.ShowPopup() {
			m.ScrollPage(-1)
		} else if m.selectedIndex > 0 {
			m.selectedIndex--
		}
		return m, cmd
	case tea.KeyDown:
		if m.ShowPopup() {
			m.ScrollPage(1)
		} else if m.selectedIndex < m.FilteredCount()-1 {
			m.selectedIndex++
		}
		return m, cmd
	case tea.KeyEnter:
		if m.selectedIndex >= 0 && m.selectedIndex < m.FilteredCount() {
			if m.ShowPopup() {
				m.ClosePopup()
			} else {
				m.TogglePopup()
			}
		}
		return m, cmd
	case tea.KeyTab:
		m.ToggleHeaders()
		return m, cmd
	case tea.KeyEscape:
		if m.ShowPopup() {
			m.ClosePopup()
		} else {
			return m, tea.Quit
		}
		return m, cmd
	case tea.KeyPgUp:
		if m.ShowPopup() {
			m.ScrollPage(-10)
		} else {
			m.selectedIndex -= 10
			if m.selectedIndex < 0 {
				m.selectedIndex = 0
			}
		}
		return m, cmd
	case tea.KeyPgDown:
		if m.ShowPopup() {
			m.ScrollPage(10)
		} else {
			m.selectedIndex += 10
			if m.selectedIndex >= m.FilteredCount() {
				m.selectedIndex = m.FilteredCount() - 1
				if m.selectedIndex < 0 {
					m.selectedIndex = 0
				}
			}
		}
		return m, cmd
	case tea.KeyHome:
		if m.ShowPopup() {
			m.SetScrollOffset(0)
		} else {
			m.GoToTop()
		}
		return m, cmd
	case tea.KeyEnd:
		if m.ShowPopup() {
			// Set to a large number, will be clamped in view
			m.SetScrollOffset(999999)
		} else {
			m.GoToBottom()
		}
		return m, cmd
	}

	// Handle character keys
	keyStr := key.String()
	switch keyStr {
	case "ctrl+c":
		return m, tea.Quit
	}

	// Handle text input based on mode
	if m.ShowPopup() {
		// Popup mode: scroll with j/k/u/d/g/G
		switch key.Text {
		case "j":
			m.ScrollPage(1)
		case " ":
			m.ScrollPage(10) // page down
		case "k":
			m.ScrollPage(-1)
		case "d":
			m.ScrollPage(10) // half page down
		case "u":
			m.ScrollPage(-10) // half page up
		case "g":
			m.SetScrollOffset(0) // go to top
		case "G":
			// Request scroll to bottom - will be processed after view renders
			return m, func() tea.Msg { return scrollToBottomMsg{} }
		case "q", "Escape":
			m.ClosePopup()
		}
	} else {
		// List mode
		switch key.Text {
		case "q":
			return m, tea.Quit
		case "r":
			m.TriggerReconnect()
		case "c":
			m.ClearEvents()
		case "/":
			cmd = m.searchInput.Focus()
		case "j":
			if m.selectedIndex < m.FilteredCount()-1 {
				m.selectedIndex++
			}
		case "k":
			if m.selectedIndex > 0 {
				m.selectedIndex--
			}
		case "g":
			m.GoToTop()
		case "G":
			m.GoToBottom()
		case "d":
			m.DeleteSelected()
		case "y":
			// 'yy' to copy - would need clipboard support
		}
	}

	// Apply filters after any input changes
	m.applyFilters()
	return m, cmd
}

// tryReconnect schedules a reconnect attempt
func (m *Model) tryReconnect() tea.Cmd {
	return tea.Tick(m.ReconnectBackoff(), func(t time.Time) tea.Msg {
		return reconnectTickMsg{}
	})
}
