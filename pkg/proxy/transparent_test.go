package proxy

import (
	"io"
	"net"
	"testing"
	"time"
)

// TestRelayBidirectionalTerminatesWhenPeerCloses verifies that when one peer
// disconnects, relayBidirectional returns promptly instead of leaving the
// opposite copy blocked on Read forever (goroutine/fd leak regression test).
// net.Pipe connections do not implement CloseWrite, so this exercises the
// full-close fallback path of closeWrite.
func TestRelayBidirectionalTerminatesWhenPeerCloses(t *testing.T) {
	p := &TransparentProxy{}

	clientProxy, clientPeer := net.Pipe()
	targetProxy, targetPeer := net.Pipe()
	defer targetPeer.Close()

	done := make(chan struct{})
	go func() {
		_, _ = p.relayBidirectional(clientProxy, targetProxy)
		close(done)
	}()

	// Client disconnects while the target side stays open.
	_ = clientPeer.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("relayBidirectional did not return after one peer closed (goroutine leak)")
	}
}

// TestRelayBidirectionalHalfClosePreservesInFlightData verifies the TCP
// half-close path: after the client closes its write side, the target must
// see EOF (so it can finish up) while data it sends afterwards is still
// delivered back to the client without truncation.
func TestRelayBidirectionalHalfClosePreservesInFlightData(t *testing.T) {
	// Backend: drains the request until EOF, then sends a final response.
	backendLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer backendLn.Close()

	go func() {
		srv, err := backendLn.Accept()
		if err != nil {
			return
		}
		_, _ = io.Copy(io.Discard, srv) // returns when the relay half-closes the write side
		_, _ = srv.Write([]byte("final-response"))
		_ = srv.Close()
	}()

	// Client-side connection pair.
	clientLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer clientLn.Close()

	proxyClientCh := make(chan net.Conn, 1)
	go func() {
		if c, err := clientLn.Accept(); err == nil {
			proxyClientCh <- c
		}
	}()

	clientApp, err := net.Dial("tcp", clientLn.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer clientApp.Close()
	proxyClient := <-proxyClientCh

	target, err := net.Dial("tcp", backendLn.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	p := &TransparentProxy{}
	done := make(chan struct{})
	go func() {
		_, _ = p.relayBidirectional(proxyClient, target)
		close(done)
	}()

	// Client sends its request, then half-closes the write side but keeps reading.
	if _, err := clientApp.Write([]byte("request")); err != nil {
		t.Fatal(err)
	}
	if err := clientApp.(*net.TCPConn).CloseWrite(); err != nil {
		t.Fatal(err)
	}

	// The final response must still arrive intact (no truncation).
	if err := clientApp.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(clientApp)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}
	if string(got) != "final-response" {
		t.Fatalf("response truncated or corrupted: got %q, want %q", got, "final-response")
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("relayBidirectional did not terminate after both sides closed")
	}
}
