package traffic

import (
	"context"
	"fmt"

	"charm.land/bubbletea/v2"
)

// Run starts the Bubble Tea program with SSE connection
func Run(serverURL string) error {
	m := NewModel(serverURL)

	// Create the program with alternative screen enabled in View
	p := tea.NewProgram(m)

	// Start the program in a goroutine and handle events
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start SSE connection in background
	go func() {
		client := NewSSEClient(serverURL)

		err := client.Connect(ctx)
		if err != nil {
			p.Send(errorMsg{err})
			return
		}

		p.Send(connectionStatusMsg{StatusConnected})

		// Listen for events
		for {
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
				}
			}
		}
	}()

	// Run the program
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("failed to run program: %w", err)
	}

	return nil
}
