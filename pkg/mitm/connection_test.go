package mitm

import (
	"bytes"
	"crypto/tls"
	"io"
	"log/slog"
	"net"
	"strings"
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

// newTestSiteCert issues a site certificate for handshake tests.
func newTestSiteCert(t *testing.T, hostname string) *tls.Certificate {
	t.Helper()
	caCert, caKey := generateTestCA(t)
	scm, err := NewSiteCertManager(caCert, caKey, t.TempDir(), time.Hour)
	if err != nil {
		t.Fatalf("failed to create site cert manager: %v", err)
	}
	cert, err := scm.GetCertificate(hostname)
	if err != nil {
		t.Fatalf("failed to get site certificate: %v", err)
	}
	return cert
}

// doHandshake runs a client/server TLS handshake over an in-memory pipe and
// returns both handshake results. The raw pipe connections are closed via
// t.Cleanup: closing a *tls.Conn over a synchronous net.Pipe would block up
// to 5s in closeNotify() waiting for the peer to read the alert.
func doHandshake(t *testing.T, serverCfg, clientCfg *tls.Config) (serverConn, clientConn *tls.Conn, serverErr, clientErr error) {
	t.Helper()
	serverRaw, clientRaw := net.Pipe()
	t.Cleanup(func() {
		serverRaw.Close()
		clientRaw.Close()
	})
	serverConn = tls.Server(serverRaw, serverCfg)
	clientConn = tls.Client(clientRaw, clientCfg)

	sErrCh := make(chan error, 1)
	cErrCh := make(chan error, 1)
	go func() { sErrCh <- serverConn.Handshake() }()
	go func() { cErrCh <- clientConn.Handshake() }()

	deadline := time.After(3 * time.Second)
	for range 2 {
		select {
		case serverErr = <-sErrCh:
		case clientErr = <-cErrCh:
		case <-deadline:
			t.Fatal("TLS handshake timed out")
		}
	}
	return serverConn, clientConn, serverErr, clientErr
}

// TestALPNForcesHTTP11 verifies that a client offering h2 is negotiated down
// to HTTP/1.1 and that a warning is logged about the forced downgrade.
func TestALPNForcesHTTP11(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))
	h := NewConnectionHandler(nil, logger, nil, NewInspectorChain(), nil)

	serverCfg := h.newClientTLSConfig(newTestSiteCert(t, "example.com"), "example.com")
	if len(serverCfg.NextProtos) != 1 || serverCfg.NextProtos[0] != alpnHTTP11 {
		t.Fatalf("server NextProtos = %v, want [%q]", serverCfg.NextProtos, alpnHTTP11)
	}

	server, client, serverErr, clientErr := doHandshake(t, serverCfg, &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         "example.com",
		NextProtos:         []string{alpnH2, alpnHTTP11},
	})

	if clientErr != nil {
		t.Fatalf("client handshake failed: %v", clientErr)
	}
	if serverErr != nil {
		t.Fatalf("server handshake failed: %v", serverErr)
	}
	if proto := client.ConnectionState().NegotiatedProtocol; proto != alpnHTTP11 {
		t.Fatalf("client negotiated protocol = %q, want %q", proto, alpnHTTP11)
	}
	if proto := server.ConnectionState().NegotiatedProtocol; proto != alpnHTTP11 {
		t.Fatalf("server negotiated protocol = %q, want %q", proto, alpnHTTP11)
	}
	if !strings.Contains(logBuf.String(), "forcing HTTP/1.1") {
		t.Fatalf("expected warning about forced HTTP/1.1, got logs: %q", logBuf.String())
	}
}

// TestALPNRejectsH2OnlyClient verifies that a client offering only h2 fails
// the handshake explicitly instead of silently bypassing inspection.
func TestALPNRejectsH2OnlyClient(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewConnectionHandler(nil, logger, nil, NewInspectorChain(), nil)
	serverCfg := h.newClientTLSConfig(newTestSiteCert(t, "example.com"), "example.com")

	_, _, serverErr, clientErr := doHandshake(t, serverCfg, &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         "example.com",
		NextProtos:         []string{alpnH2},
	})

	if clientErr == nil {
		t.Fatal("expected client handshake to fail for h2-only client")
	}
	if serverErr == nil {
		t.Fatal("expected server handshake to fail for h2-only client")
	}
}

// TestALPNAbsentClientStillWorks verifies that a client not offering ALPN at
// all handshakes fine (HTTP/1.1 by default) and triggers no h2 warning.
func TestALPNAbsentClientStillWorks(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))
	h := NewConnectionHandler(nil, logger, nil, NewInspectorChain(), nil)
	serverCfg := h.newClientTLSConfig(newTestSiteCert(t, "example.com"), "example.com")

	_, client, serverErr, clientErr := doHandshake(t, serverCfg, &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         "example.com",
	})

	if clientErr != nil {
		t.Fatalf("client handshake failed: %v", clientErr)
	}
	if serverErr != nil {
		t.Fatalf("server handshake failed: %v", serverErr)
	}
	if proto := client.ConnectionState().NegotiatedProtocol; proto != "" {
		t.Fatalf("negotiated protocol = %q, want empty (no ALPN)", proto)
	}
	if strings.Contains(logBuf.String(), "forcing HTTP/1.1") {
		t.Fatalf("unexpected h2 warning for ALPN-less client, got logs: %q", logBuf.String())
	}
}
