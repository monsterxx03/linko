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

	// Calculate dynamic widths based on terminal width
	// Layout: dir(2) + method(6) + hostPath + reqID(20) + timestamp(8) + status(4) + borders/spaces(~10)
	fixedWidth := 2 + 6 + 20 + 8 + 4 + 10
	maxHostPath := width - fixedWidth
	if maxHostPath < 15 {
		maxHostPath = 15
	}
	if len(hostPath) > maxHostPath {
		hostPath = hostPath[:maxHostPath-3] + "..."
	}

	// Request ID and timestamp styles
	reqIDStyle := lipgloss.NewStyle().Foreground(colorGray)
	timeStyle := lipgloss.NewStyle().Foreground(colorGray)

	// Build line content based on available width
	var lineContent string
	if width >= 70 {
		// Layout: method + hostPath + reqID + timestamp + status
		lineContent = fmt.Sprintf("│ %s %s %s %s %s │",
			methodSty.Render(padRight(method, 6)),
			hostnameStyle.Render(padRight(hostPath, maxHostPath)),
			reqIDStyle.Render(reqID),
			timeStyle.Render(timestamp),
			statusCodeStyle(event.Response.StatusCode).Render(padRight(statusStr, 4)))
	} else if width >= 60 {
		// Layout: method + hostPath + reqID + timestamp
		lineContent = fmt.Sprintf("│ %s %s %s %s │",
			methodSty.Render(padRight(method, 6)),
			hostnameStyle.Render(padRight(hostPath, maxHostPath)),
			reqIDStyle.Render(reqID),
			timeStyle.Render(timestamp))
	} else if width >= 50 {
		// Compact: method + hostPath + reqID
		lineContent = fmt.Sprintf("│ %s %s %s │",
			methodSty.Render(padRight(method, 6)),
			hostnameStyle.Render(padRight(hostPath, max(15, width-25))),
			reqIDStyle.Render(reqID))
	} else {
		// Minimal: method + hostPath
		lineContent = fmt.Sprintf("│ %s %s │",
			methodSty.Render(padRight(method, 6)),
			hostnameStyle.Render(padRight(hostPath, max(15, width-15))))
	}

	if isSelected {
		result := selectedBgStyle.Render(lineContent)
		if isExpanded {
			expandIndicator := "▼"
			expandSty := lipgloss.NewStyle().Foreground(colorCyan).Bold(true)
			result += "\n" + expandSty.Render("  "+expandIndicator+" ")
			result += renderEventDetails(event, m, width-4)
		} else {
			expandIndicator := "▶"
			expandSty := lipgloss.NewStyle().Foreground(colorCyan).Bold(true)
			result += " " + expandSty.Render(expandIndicator)
		}
		return result
	}

	return lineContent
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
