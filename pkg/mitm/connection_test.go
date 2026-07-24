package mitm

import (
	"io"
	"log/slog"
	"net"
	"testing"
	"time"
)

// TestRelayTrafficTerminatesWhenPeerCloses verifies that when one peer
// disconnects, relayTraffic returns promptly instead of leaving the opposite
// copy blocked on Read forever (goroutine leak regression test).
func TestRelayTrafficTerminatesWhenPeerCloses(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewConnectionHandler(nil, logger, nil, NewInspectorChain(), nil)

	clientProxy, clientPeer := net.Pipe()
	serverProxy, serverPeer := net.Pipe()
	defer serverPeer.Close()

	done := make(chan struct{})
	go func() {
		_ = h.relayTraffic(clientProxy, serverProxy, "example.com")
		close(done)
	}()

	// Client disconnects while the server side stays open.
	_ = clientPeer.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("relayTraffic did not return after client closed (goroutine leak)")
	}
}
