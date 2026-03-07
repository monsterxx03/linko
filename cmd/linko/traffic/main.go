package traffic

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Run starts the Bubble Tea program with SSE connection
func Run(serverURL string) error {
	m := NewModel(serverURL)
	model := &m

	// Create the program with alternative screen enabled in View
	p := tea.NewProgram(model)

	// Start the program in a goroutine and handle events
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start SSE connection in background
	go runSSEClient(ctx, p, serverURL, model)

	// Run the program
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("failed to run program: %w", err)
	}

	return nil
}

// runSSEClient manages the SSE connection with auto-reconnect
func runSSEClient(ctx context.Context, p *tea.Program, serverURL string, model *Model) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		client := NewSSEClient(serverURL)

		err := client.Connect(ctx)
		if err != nil {
			p.Send(errorMsg{err})
			p.Send(connectionStatusMsg{StatusError})

			// Wait before reconnecting
			select {
			case <-ctx.Done():
				return
			case <-time.After(model.ReconnectBackoff()):
				model.IncreaseBackoff()
				continue
			}
		}

		p.Send(connectionStatusMsg{StatusConnected})

		// Listen for events
		connected := true
		for connected {
			select {
			case <-ctx.Done():
				client.Disconnect()
				return
			case event := <-client.Events():
				if event != nil {
					p.Send(trafficEventMsg{*event})
				}
			case err := <-client.Errors():
				if err != nil {
					p.Send(errorMsg{err})
					connected = false
				}
			}
		}

		client.Disconnect()
		p.Send(connectionStatusMsg{StatusDisconnected})

		// Wait before reconnecting
		select {
		case <-ctx.Done():
			return
		case <-time.After(model.ReconnectBackoff()):
			model.IncreaseBackoff()
		}
	}
}
