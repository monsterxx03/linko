package traffic

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// Styles defines the UI styles
var (
	// Colors - using true color codes for better compatibility
	colorGreen    = lipgloss.Color("99")
	colorCyan     = lipgloss.Color("45")
	colorBlue     = lipgloss.Color("39")
	colorYellow   = lipgloss.Color("226")
	colorRed      = lipgloss.Color("203")
	colorMagenta  = lipgloss.Color("212")
	colorGray     = lipgloss.Color("243")
	colorWhite    = lipgloss.Color("15")
	colorBlack    = lipgloss.Color("16")

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
	methodOtherStyle = lipgloss.NewStyle().Foreground(colorWhite).Bold(true)

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
)

// Render renders the view
func Render(m Model) string {
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
func renderPopup(m Model) string {
	event := m.SelectedEvent()
	if event == nil {
		return "No event selected"
	}

	width := m.Width()
	height := m.Height()

	// Build the popup content
	var sb strings.Builder

	// Header
	sb.WriteString(renderHeader(m))
	sb.WriteString("\n\n")

	// Event summary (use full width)
	sb.WriteString(renderEventSummary(event, width))
	sb.WriteString("\n\n")

	// Full details (use full width)
	sb.WriteString(renderEventDetailsFull(event, m, width, height-10))

	// Help text
	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render(" [Enter] Close │ [Tab] Toggle Headers/Body │ [q] Quit "))

	return sb.String()
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

func renderHeader(m Model) string {
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

	return headerStyle.Render(" MITM Traffic ") + statusStr
}

func renderSearchInput(m Model) string {
	searchWidth := m.Width() - 10
	if searchWidth < 20 {
		searchWidth = 20
	}
	searchInput := m.SearchInput()
	searchInput.SetWidth(searchWidth)

	// View already includes the prompt from the model
	return searchInput.View()
}

func renderEventsFrame(m Model) string {
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
		isExpanded := false // popup instead of inline expansion

		// Render item
		item := renderEventItem(event, m, isSelected, isExpanded)
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

func renderEventItem(event TrafficEvent, m Model, isSelected, isExpanded bool) string {
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
	// Format: │ <method> <hostPath> <reqID> <timestamp> <status> │
	// Borders take 2 chars, so content = width - 2
	contentWidth := width - 2

	// For different widths, calculate hostPath width
	hostPathWidth := contentWidth - 6 - len(reqID) - 8 - 4 // method + reqID + timestamp + status + spaces
	if hostPathWidth < 10 {
		hostPathWidth = 10
	}
	if len(hostPath) > hostPathWidth {
		hostPath = hostPath[:hostPathWidth-3] + "..."
	}

	// Build content with proper padding to fill terminal
	// Format: <method> <hostPath> <reqID> <timestamp> <status>
	// All columns except hostPath have fixed width, hostPath fills remaining space
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

func renderEventDetails(event TrafficEvent, m Model, width int) string {
	var sb strings.Builder

	// Show headers or body based on toggle
	if m.ShowHeaders() {
		// Request headers
		if event.Request != nil && len(event.Request.Headers) > 0 {
			sb.WriteString(detailHeaderStyle.Render("Request Headers:"))
			sb.WriteString("\n")
			for k, v := range event.Request.Headers {
				sb.WriteString(fmt.Sprintf("  %s: %s\n", headerKeyStyle.Render(k), headerValueStyle.Render(truncate(v, width-20))))
			}
			sb.WriteString("\n")
		}

		// Response headers
		if event.Response != nil && len(event.Response.Headers) > 0 {
			sb.WriteString(detailHeaderStyle.Render("Response Headers:"))
			sb.WriteString("\n")
			for k, v := range event.Response.Headers {
				sb.WriteString(fmt.Sprintf("  %s: %s\n", headerKeyStyle.Render(k), headerValueStyle.Render(truncate(v, width-20))))
			}
			sb.WriteString("\n")
		}
	} else {
		// Request body
		if event.Request != nil && event.Request.Body != "" {
			sb.WriteString(detailHeaderStyle.Render("Request Body:"))
			sb.WriteString("\n")
			body := truncateBody(event.Request.Body, 500)
			lines := strings.Split(body, "\n")
			for _, line := range lines {
				sb.WriteString(bodyStyle.Render(line))
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
		}

		// Response body
		if event.Response != nil && event.Response.Body != "" {
			sb.WriteString(detailHeaderStyle.Render("Response Body:"))
			sb.WriteString("\n")
			body := truncateBody(event.Response.Body, 1000)
			lines := strings.Split(body, "\n")
			for _, line := range lines {
				sb.WriteString(bodyStyle.Render(line))
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// renderEventDetailsFull shows full event details without truncation
func renderEventDetailsFull(event *TrafficEvent, m Model, width, maxHeight int) string {
	var sb strings.Builder

	// Show headers or body based on toggle
	if m.ShowHeaders() {
		// Request headers
		if event.Request != nil && len(event.Request.Headers) > 0 {
			sb.WriteString(detailHeaderStyle.Render("Request Headers:"))
			sb.WriteString("\n")
			for k, v := range event.Request.Headers {
				sb.WriteString(fmt.Sprintf("  %s: %s\n", headerKeyStyle.Render(k), headerValueStyle.Render(v)))
			}
			sb.WriteString("\n")
		}

		// Response headers
		if event.Response != nil && len(event.Response.Headers) > 0 {
			sb.WriteString(detailHeaderStyle.Render("Response Headers:"))
			sb.WriteString("\n")
			for k, v := range event.Response.Headers {
				sb.WriteString(fmt.Sprintf("  %s: %s\n", headerKeyStyle.Render(k), headerValueStyle.Render(v)))
			}
			sb.WriteString("\n")
		}
	} else {
		// Request body - full content with word wrap
		if event.Request != nil && event.Request.Body != "" {
			sb.WriteString(detailHeaderStyle.Render("Request Body:"))
			sb.WriteString("\n")
			wrapped := wrapText(event.Request.Body, width-4)
			for _, line := range wrapped {
				sb.WriteString(bodyStyle.Render(line))
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
		}

		// Response body - full content with word wrap
		if event.Response != nil && event.Response.Body != "" {
			sb.WriteString(detailHeaderStyle.Render("Response Body:"))
			sb.WriteString("\n")
			wrapped := wrapText(event.Response.Body, width-4)
			for _, line := range wrapped {
				sb.WriteString(bodyStyle.Render(line))
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
		}
	}

	// No outer border, just return the content
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

func renderHelp(m Model) string {
	helpText := fmt.Sprintf(" [↑↓/jk] Navigate │ [Enter] Expand │ [Tab] Headers/Body │ [/] Search │ [c] Clear │ [r] Reconnect │ [q] Quit")

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

func truncateBody(body string, maxLen int) string {
	if len(body) <= maxLen {
		return body
	}
	return body[:maxLen-3] + "..."
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
