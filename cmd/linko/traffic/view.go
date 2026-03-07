package traffic

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
)

// Styles defines the UI styles
var (
	// Colors - using true color codes for better compatibility
	colorGreen   = lipgloss.Color("99")
	colorCyan    = lipgloss.Color("45")
	colorBlue    = lipgloss.Color("39")
	colorYellow  = lipgloss.Color("226")
	colorRed     = lipgloss.Color("203")
	colorMagenta = lipgloss.Color("212")
	colorGray    = lipgloss.Color("243")
	colorWhite   = lipgloss.Color("15")
	colorBlack   = lipgloss.Color("16")

	// Header styles
	headerStyle = lipgloss.NewStyle().
			Foreground(colorGreen).
			Bold(true)

	statusLiveStyle = lipgloss.NewStyle().
			Foreground(colorGreen).
			Bold(true)

	statusConnectingStyle = lipgloss.NewStyle().
				Foreground(colorYellow).
				Bold(true)

	statusErrorStyle = lipgloss.NewStyle().
				Foreground(colorRed).
				Bold(true)

	statusDisconnectedStyle = lipgloss.NewStyle().
				Foreground(colorGray)

	// Method styles
	methodGetStyle    = lipgloss.NewStyle().Foreground(colorCyan).Bold(true)
	methodPostStyle   = lipgloss.NewStyle().Foreground(colorGreen).Bold(true)
	methodPutStyle    = lipgloss.NewStyle().Foreground(colorYellow).Bold(true)
	methodDeleteStyle = lipgloss.NewStyle().Foreground(colorRed).Bold(true)
	methodPatchStyle  = lipgloss.NewStyle().Foreground(colorMagenta).Bold(true)
	methodOtherStyle  = lipgloss.NewStyle().Foreground(colorWhite).Bold(true)

	// Status code styles
	statusCodeStyle = func(code int) lipgloss.Style {
		switch {
		case code >= 200 && code < 300:
			return lipgloss.NewStyle().Foreground(colorGreen).Bold(true)
		case code >= 300 && code < 400:
			return lipgloss.NewStyle().Foreground(colorYellow).Bold(true)
		case code >= 400 && code < 500:
			return lipgloss.NewStyle().Foreground(colorRed).Bold(true)
		case code >= 500:
			return lipgloss.NewStyle().Foreground(colorMagenta).Bold(true)
		default:
			return lipgloss.NewStyle().Foreground(colorWhite)
		}
	}

	// Hostname style
	hostnameStyle = lipgloss.NewStyle().
			Foreground(colorBlue)

	// Direction indicator
	dirOutStyle = lipgloss.NewStyle().
			Foreground(colorGreen).
			Bold(true)

	dirInStyle = lipgloss.NewStyle().
			Foreground(colorCyan).
			Bold(true)

	// Latency style
	latencyStyle = lipgloss.NewStyle().
			Foreground(colorGray)

	// Selected item style - use background to highlight
	selectedBgStyle = lipgloss.NewStyle().
			Background(colorCyan).
			Foreground(colorBlack)

	// Detail styles
	detailHeaderStyle = lipgloss.NewStyle().
				Foreground(colorCyan).
				Bold(true)

	headerKeyStyle = lipgloss.NewStyle().
			Foreground(colorYellow)

	headerValueStyle = lipgloss.NewStyle().
				Foreground(colorWhite)

	bodyStyle = lipgloss.NewStyle().
			Foreground(colorWhite)

	// Help text
	helpStyle = lipgloss.NewStyle().
			Foreground(colorGray)

	filterActiveStyle = lipgloss.NewStyle().
				Foreground(colorGreen).
				Bold(true)

	filterInactiveStyle = lipgloss.NewStyle().
				Foreground(colorGray)

	// Box styles
	boxBorderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorCyan).
			Foreground(colorWhite)

	// Scroll indicator style
	scrollIndicatorStyle = lipgloss.NewStyle().
				Foreground(colorYellow).
				Bold(true)
)

// Render renders the view
func Render(m *Model) string {
	// If popup is shown, render only the popup
	if m.ShowPopup() {
		return renderPopup(m)
	}

	var sb strings.Builder

	// Header
	sb.WriteString(renderHeader(m))
	sb.WriteString("\n\n")

	// Search input
	sb.WriteString(renderSearchInput(m))
	sb.WriteString("\n\n")

	// Events list with frame
	sb.WriteString(renderEventsFrame(m))

	// Help text
	sb.WriteString("\n")
	sb.WriteString(renderHelp(m))

	return sb.String()
}

// renderPopup renders a full-screen popup with event details
func renderPopup(m *Model) string {
	event := m.SelectedEvent()
	if event == nil {
		return "No event selected"
	}

	width := m.Width()
	height := m.Height()
	scrollOffset := m.ScrollOffset()

	// Build the popup content
	var sb strings.Builder

	// Header
	sb.WriteString(renderHeader(m))
	sb.WriteString("\n\n")

	// Event summary (use full width)
	sb.WriteString(renderEventSummary(event, width))
	sb.WriteString("\n\n")

	// Full details with scroll offset
	sb.WriteString(renderEventDetailsFull(event, m, width, height-10, scrollOffset))

	// Help text
	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render(" [↑↓/j/k] Scroll │ [u/d] Page │ [g/G] Top/Bottom │ [Tab] Headers/Body │ [Enter/q] Close "))

	// Wrap in a bordered box with no inner padding
	popupStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorCyan).
		Padding(0, 1).
		Width(width)

	return popupStyle.Render(sb.String())
}

func renderEventSummary(event *TrafficEvent, width int) string {
	method := ""
	if event.Request != nil {
		method = event.Request.Method
	}
	methodSty := getMethodStyle(method)

	hostPath := event.Hostname
	if event.Request != nil {
		url := event.Request.URL
		if idx := strings.Index(url, event.Hostname); idx >= 0 {
			path := url[idx+len(event.Hostname):]
			if path != "" {
				hostPath = event.Hostname + path
			}
		} else if strings.HasPrefix(url, "/") {
			hostPath = event.Hostname + url
		} else {
			hostPath = event.Hostname + "/" + url
		}
	}

	statusStr := ""
	if event.Response != nil {
		statusStr = fmt.Sprintf("%d", event.Response.StatusCode)
	}

	// No border
	return fmt.Sprintf("%s %s %s", methodSty.Render(method), hostnameStyle.Render(hostPath), statusCodeStyle(event.Response.StatusCode).Render(statusStr))
}

func renderHeader(m *Model) string {
	var statusStr string
	switch m.Status() {
	case StatusConnecting:
		statusStr = statusConnectingStyle.Render(" ● Connecting...")
	case StatusConnected:
		statusStr = statusLiveStyle.Render(" ● Live")
	case StatusDisconnected:
		statusStr = statusDisconnectedStyle.Render(" ○ Disconnected")
	case StatusError:
		statusStr = statusErrorStyle.Render(" ✗ Error")
	}

	if m.ErrMsg() != "" {
		statusStr = statusStr + " " + statusErrorStyle.Render(m.ErrMsg())
	}

	// Add connection info
	info := fmt.Sprintf(" [%d/%d]", m.FilteredCount(), m.TotalEvents())
	if m.ReconnectCount() > 0 {
		info = fmt.Sprintf(" [%d/%d] [reconnect: %d]", m.FilteredCount(), m.TotalEvents(), m.ReconnectCount())
	}

	return headerStyle.Render(" MITM Traffic ") + statusStr + helpStyle.Render(info)
}

func renderSearchInput(m *Model) string {
	searchWidth := m.Width() - 10
	if searchWidth < 20 {
		searchWidth = 20
	}
	searchInput := m.SearchInput()
	searchInput.SetWidth(searchWidth)

	// View already includes the prompt from the model
	return searchInput.View()
}

func renderEventsFrame(m *Model) string {
	events := m.Events()
	if len(events) == 0 {
		emptyStyle := lipgloss.NewStyle().
			Foreground(colorGray).
			Width(m.Width()).
			Align(lipgloss.Center)
		return emptyStyle.Render("No traffic events yet...")
	}

	// Calculate available height
	availableHeight := m.Height() - 10
	if availableHeight < 5 {
		availableHeight = 5
	}

	selectedIdx := m.SelectedIndex()

	// Render as a list with selection indicator
	var sb strings.Builder

	for i, event := range events {
		if i >= availableHeight {
			break
		}

		isSelected := i == selectedIdx

		// Render item
		item := renderEventItem(event, m, isSelected)
		sb.WriteString(item)

		if i < len(events)-1 && i < availableHeight-1 {
			sb.WriteString("\n")
		}
	}

	// Frame without border (items already have borders)
	frame := lipgloss.NewStyle().
		Width(m.Width()).
		Height(availableHeight + 2).
		Foreground(colorWhite)

	return frame.Render(sb.String())
}

func renderEventItem(event TrafficEvent, m *Model, isSelected bool) string {
	width := m.Width()
	if width < 60 {
		width = 60
	}

	// Method
	method := ""
	if event.Request != nil {
		method = event.Request.Method
	}
	methodSty := getMethodStyle(method)

	// Hostname + path combined
	hostPath := event.Hostname
	if event.Request != nil {
		url := event.Request.URL
		if idx := strings.Index(url, event.Hostname); idx >= 0 {
			path := url[idx+len(event.Hostname):]
			if path != "" {
				hostPath = event.Hostname + path
			}
		} else if strings.HasPrefix(url, "/") {
			hostPath = event.Hostname + url
		} else {
			hostPath = event.Hostname + "/" + url
		}
	}

	// Status
	statusStr := ""
	if event.Response != nil {
		statusStr = fmt.Sprintf("%d", event.Response.StatusCode)
	}

	// Request ID (full, no truncate)
	reqID := event.ID

	// Timestamp (format: 15:04:05)
	timestamp := event.Timestamp.Format("15:04:05")

	// Calculate column widths - use lipgloss to fill width
	contentWidth := width

	// For different widths, calculate hostPath width
	hostPathWidth := contentWidth - 6 - len(reqID) - 8 - 4 // method + reqID + timestamp + status + spaces
	if hostPathWidth < 10 {
		hostPathWidth = 10
	}
	if len(hostPath) > hostPathWidth {
		hostPath = hostPath[:hostPathWidth-3] + "..."
	}

	// Build content with proper padding to fill terminal
	totalFixed := 6 + 1 + len(reqID) + 1 + 8 + 1 + len(statusStr) // including spaces
	if totalFixed > width {
		totalFixed = width
	}
	actualHostPathWidth := width - totalFixed - 1 // -1 for space after method
	if actualHostPathWidth < 5 {
		actualHostPathWidth = 5
	}
	if len(hostPath) > actualHostPathWidth {
		hostPath = hostPath[:actualHostPathWidth-3] + "..."
	}
	hostPathPadded := padRight(hostPath, actualHostPathWidth)

	// Build full line content
	if width >= 70 {
		// Full layout with status
		line := fmt.Sprintf("%s %s %s %s %s",
			padRight(method, 6),
			hostPathPadded,
			reqID,
			timestamp,
			statusStr)
		if isSelected {
			return selectedBgStyle.Render(line)
		}
		return fmt.Sprintf("%s %s %s %s %s",
			methodSty.Render(padRight(method, 6)),
			hostnameStyle.Render(hostPathPadded),
			reqID,
			timestamp,
			statusStr)
	} else if width >= 60 {
		// Without status
		line := fmt.Sprintf("%s %s %s %s",
			padRight(method, 6),
			hostPathPadded,
			reqID,
			timestamp)
		if isSelected {
			return selectedBgStyle.Render(line)
		}
		return fmt.Sprintf("%s %s %s %s",
			methodSty.Render(padRight(method, 6)),
			hostnameStyle.Render(hostPathPadded),
			reqID,
			timestamp)
	} else if width >= 50 {
		// Without timestamp and status
		line := fmt.Sprintf("%s %s %s",
			padRight(method, 6),
			hostPathPadded,
			reqID)
		if isSelected {
			return selectedBgStyle.Render(line)
		}
		return fmt.Sprintf("%s %s %s",
			methodSty.Render(padRight(method, 6)),
			hostnameStyle.Render(hostPathPadded),
			reqID)
	} else {
		// Minimal: method + hostPath
		line := fmt.Sprintf("%s %s",
			padRight(method, 6),
			hostPathPadded)
		if isSelected {
			return selectedBgStyle.Render(line)
		}
		return fmt.Sprintf("%s %s",
			methodSty.Render(padRight(method, 6)),
			hostnameStyle.Render(hostPathPadded))
	}
}

func getMethodStyle(method string) lipgloss.Style {
	switch method {
	case "GET":
		return methodGetStyle
	case "POST":
		return methodPostStyle
	case "PUT":
		return methodPutStyle
	case "DELETE":
		return methodDeleteStyle
	case "PATCH":
		return methodPatchStyle
	default:
		return methodOtherStyle
	}
}

// renderEventDetailsFull shows full event details with pagination
func renderEventDetailsFull(event *TrafficEvent, m *Model, width, maxHeight, scrollOffset int) string {
	var lines []string

	// Show headers or body based on toggle
	if m.ShowHeaders() {
		// Request headers
		if event.Request != nil && len(event.Request.Headers) > 0 {
			lines = append(lines, "Request Headers:")
			for _, k := range sortedKeys(event.Request.Headers) {
				lines = append(lines, fmt.Sprintf("  %s: %s", k, event.Request.Headers[k]))
			}
			lines = append(lines, "")
		}

		// Response headers
		if event.Response != nil && len(event.Response.Headers) > 0 {
			lines = append(lines, "Response Headers:")
			for _, k := range sortedKeys(event.Response.Headers) {
				lines = append(lines, fmt.Sprintf("  %s: %s", k, event.Response.Headers[k]))
			}
			lines = append(lines, "")
		}
	} else {
		// Request body - full content with word wrap
		if event.Request != nil && event.Request.Body != "" {
			lines = append(lines, "Request Body:")
			wrapped := wrapText(event.Request.Body, width-4)
			lines = append(lines, wrapped...)
			lines = append(lines, "")
		}

		// Response body - full content with word wrap
		if event.Response != nil && event.Response.Body != "" {
			lines = append(lines, "Response Body:")
			wrapped := wrapText(event.Response.Body, width-4)
			lines = append(lines, wrapped...)
		}
	}

	// Calculate maximum valid scroll offset based on content height and view height
	maxScrollOffset := len(lines) - maxHeight
	if maxScrollOffset < 0 {
		maxScrollOffset = 0 // Content fits in view, no scrolling needed
	}

	// Handle special marker -1 for "go to bottom"
	if scrollOffset == -1 {
		scrollOffset = maxScrollOffset
		// Force sync back to model
		m.SetScrollOffset(scrollOffset)
	}

	// Apply scroll offset clamping
	if scrollOffset > maxScrollOffset {
		scrollOffset = maxScrollOffset
	}
	if scrollOffset < 0 {
		scrollOffset = 0
	}

	// Update model's scroll offset if it was clamped
	if scrollOffset != m.ScrollOffset() {
		m.SetScrollOffset(scrollOffset)
	}

	visibleLines := maxHeight
	if visibleLines > len(lines)-scrollOffset {
		visibleLines = len(lines) - scrollOffset
	}
	if visibleLines < 0 {
		visibleLines = 0
	}

	var sb strings.Builder

	// Add scroll indicator at top if not at beginning
	if scrollOffset > 0 {
		sb.WriteString(scrollIndicatorStyle.Render("▲ " + fmt.Sprintf("%d lines above", scrollOffset)))
		sb.WriteString("\n")
	}

	// Known header lines that should be styled as headers
	headerLines := map[string]bool{
		"Request Headers:":  true,
		"Response Headers:": true,
		"Request Body:":     true,
		"Response Body:":     true,
	}

	for i := scrollOffset; i < scrollOffset+visibleLines && i < len(lines); i++ {
		style := bodyStyle
		if headerLines[lines[i]] {
			style = detailHeaderStyle
		}
		sb.WriteString(style.Render(lines[i]))
		sb.WriteString("\n")
	}

	// Add scroll indicator at bottom if not at end
	remaining := len(lines) - (scrollOffset + visibleLines)
	if remaining > 0 {
		sb.WriteString(scrollIndicatorStyle.Render("▼ " + fmt.Sprintf("%d lines below", remaining)))
	}

	return sb.String()
}

// wrapText wraps text to fit within maxWidth by breaking at word boundaries
func wrapText(text string, maxWidth int) []string {
	if maxWidth <= 0 {
		maxWidth = 80
	}

	var lines []string
	// Split by existing newlines first
	paragraphs := strings.Split(text, "\n")

	for _, para := range paragraphs {
		words := strings.Fields(para)
		if len(words) == 0 {
			continue
		}

		currentLine := ""
		for _, word := range words {
			if currentLine == "" {
				currentLine = word
			} else if len(currentLine)+1+len(word) <= maxWidth {
				currentLine += " " + word
			} else {
				lines = append(lines, currentLine)
				currentLine = word
			}
		}
		if currentLine != "" {
			lines = append(lines, currentLine)
		}
	}

	return lines
}

func renderHelp(m *Model) string {
	helpText := " [↑↓/j/k] Navigate │ [Enter] Expand │ [PgUp/PgDn] Page │ [g/G] Top/Bottom │ [/] Search │ [Tab] Headers/Body │ [c] Clear │ [d] Delete │ [r] Reconnect │ [q] Quit"

	return helpStyle.Render(helpText)
}

func padRight(s string, length int) string {
	if len(s) >= length {
		return s[:length]
	}
	return s + strings.Repeat(" ", length-len(s))
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// sortedKeys returns sorted keys from a map[string]string
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// countEventDetailLines counts the number of lines in event details (for calculating max scroll)
func countEventDetailLines(event *TrafficEvent, width int, showHeaders bool) int {
	var count int

	if showHeaders {
		// Request headers
		if event.Request != nil && len(event.Request.Headers) > 0 {
			count++ // "Request Headers:"
			count += len(event.Request.Headers)
			count++ // empty line
		}

		// Response headers
		if event.Response != nil && len(event.Response.Headers) > 0 {
			count++ // "Response Headers:"
			count += len(event.Response.Headers)
			count++ // empty line
		}
	} else {
		// Request body
		if event.Request != nil && event.Request.Body != "" {
			count++ // "Request Body:"
			wrapped := wrapText(event.Request.Body, width)
			count += len(wrapped)
			count++ // empty line
		}

		// Response body
		if event.Response != nil && event.Response.Body != "" {
			count++ // "Response Body:"
			wrapped := wrapText(event.Response.Body, width)
			count += len(wrapped)
		}
	}

	return count
}
