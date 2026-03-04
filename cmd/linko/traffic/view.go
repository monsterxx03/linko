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
	colorDarkGray = lipgloss.Color("236")
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
			Background(colorDarkGray).
			Foreground(colorWhite)

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
	expandedIdx := m.ExpandedIndex()

	// Render as a list with selection indicator
	var sb strings.Builder

	for i, event := range events {
		if i >= availableHeight {
			break
		}

		isSelected := i == selectedIdx
		isExpanded := expandedIdx != nil && *expandedIdx == i

		// Render item
		item := renderEventItem(event, m, isSelected, isExpanded)
		sb.WriteString(item)

		if i < len(events)-1 && i < availableHeight-1 {
			sb.WriteString("\n")
		}
	}

	// Add frame
	frame := lipgloss.NewStyle().
		Width(m.Width()).
		Height(availableHeight + 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorCyan).
		Foreground(colorWhite)

	return frame.Render(sb.String())
}

func renderEventItem(event TrafficEvent, m Model, isSelected, isExpanded bool) string {
	// Direction indicator
	dirIndicator := "→"
	dirSty := dirOutStyle
	if event.Direction == "server->client" {
		dirIndicator = "←"
		dirSty = dirInStyle
	}

	// Method
	method := ""
	if event.Request != nil {
		method = event.Request.Method
	}
	methodSty := getMethodStyle(method)

	// Hostname
	hostname := event.Hostname
	if len(hostname) > 25 {
		hostname = hostname[:22] + "..."
	}

	// URL path
	path := ""
	if event.Request != nil {
		url := event.Request.URL
		if idx := strings.Index(url, event.Hostname); idx >= 0 {
			path = url[idx+len(event.Hostname):]
		} else {
			path = url
		}
		// Truncate but preserve query string if possible
		if len(path) > 30 {
			// Try to keep query string
			if qIdx := strings.Index(path, "?"); qIdx > 0 && qIdx < 25 {
				// Keep path up to query and truncate query
				path = path[:25] + "..."
			} else {
				path = path[:27] + "..."
			}
		}
	}

	// Status and latency
	statusStr := ""
	latencyStr := ""
	if event.Response != nil {
		statusStr = statusCodeStyle(event.Response.StatusCode).Render(fmt.Sprintf("%d", event.Response.StatusCode))
		latencyStr = latencyStyle.Render(FormatLatency(event.Response.Latency))
	}

	// Build the line content
	var lineContent string
	if isSelected {
		// Selected row with background
		lineContent = fmt.Sprintf("│ %s %s │ %s │ %s",
			dirSty.Render(dirIndicator),
			methodSty.Render(padRight(method, 7)),
			hostnameStyle.Render(padRight(hostname, 25)),
			padRight(path, 30))
		if statusStr != "" {
			lineContent += fmt.Sprintf("│ %s %s │", statusStr, latencyStr)
		} else {
			lineContent += "│"
		}

		// Apply background to entire line
		result := selectedBgStyle.Render(lineContent)

		// Add expand indicator and details
		if isExpanded {
			expandIndicator := "▼"
			expandSty := lipgloss.NewStyle().Foreground(colorCyan).Bold(true)
			result += "\n" + expandSty.Render("  "+expandIndicator+" ")
			result += renderEventDetails(event, m, m.Width()-4)
		} else {
			expandIndicator := "▶"
			expandSty := lipgloss.NewStyle().Foreground(colorCyan).Bold(true)
			result += " " + expandSty.Render(expandIndicator)
		}

		return result
	} else {
		// Normal row
		lineContent = fmt.Sprintf("│ %s %s │ %s │ %s",
			dirSty.Render(dirIndicator),
			methodSty.Render(padRight(method, 7)),
			hostnameStyle.Render(padRight(hostname, 25)),
			padRight(path, 30))
		if statusStr != "" {
			lineContent += fmt.Sprintf(" │ %s %s │", statusStr, latencyStr)
		} else {
			lineContent += " │"
		}

		return lineContent
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
			sb.WriteString(detailHeaderStyle.Render("┌─ Request Headers ─┐"))
			sb.WriteString("\n")
			for k, v := range event.Request.Headers {
				sb.WriteString(fmt.Sprintf("│ %s: %s", headerKeyStyle.Render(k), headerValueStyle.Render(truncate(v, width-20))))
				sb.WriteString(strings.Repeat(" ", max(0, width-len(k)-len(v)-25)))
				sb.WriteString("│\n")
			}
			sb.WriteString(detailHeaderStyle.Render("└────────────────────┘"))
			sb.WriteString("\n")
		}

		// Response headers
		if event.Response != nil && len(event.Response.Headers) > 0 {
			sb.WriteString(detailHeaderStyle.Render("┌─ Response Headers ─┐"))
			sb.WriteString("\n")
			for k, v := range event.Response.Headers {
				sb.WriteString(fmt.Sprintf("│ %s: %s", headerKeyStyle.Render(k), headerValueStyle.Render(truncate(v, width-20))))
				sb.WriteString(strings.Repeat(" ", max(0, width-len(k)-len(v)-25)))
				sb.WriteString("│\n")
			}
			sb.WriteString(detailHeaderStyle.Render("└─────────────────────┘"))
			sb.WriteString("\n")
		}
	} else {
		// Request body
		if event.Request != nil && event.Request.Body != "" {
			sb.WriteString(detailHeaderStyle.Render("┌─ Request Body ─┐"))
			sb.WriteString("\n")
			body := truncateBody(event.Request.Body, 500)
			lines := strings.Split(body, "\n")
			for _, line := range lines {
				if len(line) > width-4 {
					line = line[:width-7] + "..."
				}
				sb.WriteString("│ ")
				sb.WriteString(bodyStyle.Render(line))
				sb.WriteString(strings.Repeat(" ", max(0, width-len(line)-6)))
				sb.WriteString("│\n")
			}
			sb.WriteString(detailHeaderStyle.Render("└──────────────────┘"))
			sb.WriteString("\n")
		}

		// Response body
		if event.Response != nil && event.Response.Body != "" {
			sb.WriteString(detailHeaderStyle.Render("┌─ Response Body ─┐"))
			sb.WriteString("\n")
			body := truncateBody(event.Response.Body, 1000)
			lines := strings.Split(body, "\n")
			for _, line := range lines {
				if len(line) > width-4 {
					line = line[:width-7] + "..."
				}
				sb.WriteString("│ ")
				sb.WriteString(bodyStyle.Render(line))
				sb.WriteString(strings.Repeat(" ", max(0, width-len(line)-6)))
				sb.WriteString("│\n")
			}
			sb.WriteString(detailHeaderStyle.Render("└───────────────────┘"))
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func renderHelp(m Model) string {
	filter := m.DirectionFilter()

	var filterStr string
	switch filter {
	case DirectionAll:
		filterStr = filterActiveStyle.Render("[All]") + filterInactiveStyle.Render(" Req Resp")
	case DirectionClientServer:
		filterStr = filterInactiveStyle.Render("All ") + filterActiveStyle.Render("[Req]") + filterInactiveStyle.Render(" Resp")
	case DirectionServerClient:
		filterStr = filterInactiveStyle.Render("All  Req ") + filterActiveStyle.Render("[Resp]")
	}

	helpText := fmt.Sprintf(" %s │ [↑↓] Navigate │ [Enter] Expand │ [Tab] Headers/Body │ [/] Search │ [c] Clear │ [r] Reconnect │ [q] Quit",
		filterStr)

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
