package traffic

// applyFilters applies search filter and updates filteredEvents
// Must be called with eventsMu held (at least RLock)
func (m *Model) applyFilters() {
	m.eventsMu.Lock()
	defer m.eventsMu.Unlock()
	m.applyFiltersLocked()
}

// applyFiltersLocked applies filters assuming lock is already held
func (m *Model) applyFiltersLocked() {
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

// AddEvent adds or updates an event
func (m *Model) AddEvent(event TrafficEvent) {
	m.eventsMu.Lock()
	defer m.eventsMu.Unlock()

	// Truncate body if needed
	if event.Request != nil && event.Request.Body != "" {
		event.Request.Body = truncateBody(event.Request.Body)
	}
	if event.Response != nil && event.Response.Body != "" {
		event.Response.Body = truncateBody(event.Response.Body)
	}

	// Try to find existing event with same ID for merging
	if idx, ok := m.eventIndex[event.ID]; ok {
		// Merge event data
		m.events[idx] = mergeEvents(m.events[idx], event)
	} else {
		// Add as new event at the beginning
		m.events = append([]TrafficEvent{event}, m.events...)
		// Update index for all events
		m.eventIndex[event.ID] = 0
		for i := 1; i < len(m.events); i++ {
			m.eventIndex[m.events[i].ID] = i
		}

		// Limit max events
		if len(m.events) > MaxEvents {
			// Remove oldest events from index
			for i := MaxEvents; i < len(m.events); i++ {
				delete(m.eventIndex, m.events[i].ID)
			}
			m.events = m.events[:MaxEvents]
		}
	}

	m.applyFiltersLocked()

	// Adjust selection if needed
	if m.selectedIndex >= len(m.filteredEvents) && len(m.filteredEvents) > 0 {
		m.selectedIndex = len(m.filteredEvents) - 1
	}
}
