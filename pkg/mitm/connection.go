package mitm

import (
	"bufio"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net"
	"slices"
	"sync"
	"time"
)

// bufferPool is a sync.Pool for managing buffers used in io.CopyBuffer
var bufferPool = &sync.Pool{
	New: func() any {
		return make([]byte, DefaultBufferSize)
	},
}

// ConnectionHandler handles MITM connections
type ConnectionHandler struct {
	siteCertManager *SiteCertManager
	logger          *slog.Logger
	upstream        UpstreamClient
	peekReader      *PeekReader // Optional pre-wrapped connection for whitelist check
	inspector       *InspectorChain
	ctx             any
}

// UpstreamClient interface for connecting through upstream proxy
type UpstreamClient interface {
	Connect(host string, port int) (net.Conn, error)
	IsEnabled() bool
}

// NewConnectionHandler creates a new MITM connection handler
func NewConnectionHandler(
	siteCertManager *SiteCertManager,
	logger *slog.Logger,
	upstream UpstreamClient,
	inspector *InspectorChain,
	peekReader *PeekReader,
) *ConnectionHandler {
	return &ConnectionHandler{
		siteCertManager: siteCertManager,
		logger:          logger,
		upstream:        upstream,
		inspector:       inspector,
		peekReader:      peekReader,
	}
}

// HandleConnection handles a MITM connection
func (h *ConnectionHandler) HandleConnection(clientConn net.Conn, targetIP net.IP, targetPort int) error {
	defer clientConn.Close()

	// Use provided peekReader or create a new one
	peekReader := h.peekReader
	if peekReader == nil {
		peekReader = NewPeekReader(clientConn)
	}

	// First, peek at the ClientHello to extract SNI
	hostname, err := h.peekSNI(peekReader, targetIP)
	if err != nil {
		h.logger.Debug("SNI extraction failed, using target IP", "error", err, "target_ip", targetIP.String())
		hostname = targetIP.String()
	}

	h.logger.Debug("MITM connection",
		"hostname", hostname,
		"target_ip", targetIP.String(),
		"target_port", targetPort,
	)

	// Get or generate certificate for this hostname
	siteCert, err := h.siteCertManager.GetCertificate(hostname)
	if err != nil {
		return fmt.Errorf("failed to get site certificate: %w", err)
	}

	// Connect to target server
	var serverConn net.Conn
	targetHost := targetIP.String()
	if h.upstream.IsEnabled() {
		serverConn, err = h.upstream.Connect(targetHost, targetPort)
		if err != nil {
			return fmt.Errorf("failed to connect to upstream: %w", err)
		}
	} else {
		serverConn, err = net.Dial("tcp", net.JoinHostPort(targetHost, fmt.Sprintf("%d", targetPort)))
		if err != nil {
			return fmt.Errorf("failed to connect to target: %w", err)
		}
	}
	defer serverConn.Close()

	// Create TLS config for the client-facing side (MITM server side).
	clientTLSConfig := h.newClientTLSConfig(siteCert, hostname)

	// Create TLS config for the upstream side (connecting to actual server).
	// ALPN is restricted to HTTP/1.1 so the origin cannot negotiate h2
	// either: the relay is a single opaque stream and the HTTP inspectors
	// only understand HTTP/1.x framing.
	serverTLSConfig := &tls.Config{
		ServerName: hostname,
		NextProtos: []string{alpnHTTP11},
	}

	// Upgrade connection to TLS with client using the peek reader
	clientTLS := tls.Server(peekReader, clientTLSConfig)
	if err := clientTLS.Handshake(); err != nil {
		return fmt.Errorf("client TLS handshake failed: %w", err)
	}
	defer clientTLS.Close()

	// Connect to server with TLS
	serverTLS := tls.Client(serverConn, serverTLSConfig)
	if err := serverTLS.Handshake(); err != nil {
		return fmt.Errorf("server TLS handshake failed: %w", err)
	}
	defer serverTLS.Close()

	// Handle the connection
	return h.relayTraffic(clientTLS, serverTLS, hostname)
}

// peekSNI extracts SNI from the connection using a PeekReader
func (h *ConnectionHandler) peekSNI(peekReader *PeekReader, targetIP net.IP) (string, error) {
	// First, peek at the TLS record header to get the full record length
	header, err := peekReader.Peek(5)
	if err != nil {
		return "", fmt.Errorf("failed to peek TLS header: %w", err)
	}

	// TLS record header: 1 byte type + 2 bytes version + 2 bytes length
	recordLen := int(header[3])<<8 | int(header[4])
	totalLen := min(
		// Peek at the complete TLS record
		5+recordLen,
		// Cap at buffer size
		DefaultBufferSize)

	peekData, err := peekReader.Peek(totalLen)
	if err != nil && len(peekData) < 200 {
		return "", fmt.Errorf("failed to peek TLS record: %w", err)
	}

	// Parse SNI from the peeked data
	sniInfo, err := ExtractSNIFromConn(peekData)
	if err != nil {
		return "", fmt.Errorf("SNI parsing failed: %w", err)
	}

	if sniInfo.IsValid && sniInfo.Hostname != "" {
		return sniInfo.Hostname, nil
	}

	// Fall back to target IP
	return targetIP.String(), nil
}

// ALPN protocol IDs. The HTTP inspectors only understand HTTP/1.x framing,
// so HTTP/2 must never be negotiated on either side of the MITM connection.
const (
	alpnHTTP11 = "http/1.1"
	alpnH2     = "h2"
)

// newClientTLSConfig builds the TLS config for the client-facing side of
// the MITM connection. ALPN is explicitly restricted to HTTP/1.1: clients
// offering h2 receive "http/1.1" in the ServerHello and must fall back.
// Clients offering h2 only fail the handshake with no_application_protocol
// instead of silently bypassing inspection.
func (h *ConnectionHandler) newClientTLSConfig(siteCert *tls.Certificate, hostname string) *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{*siteCert},
		NextProtos:   []string{alpnHTTP11},
		GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			if slices.Contains(hello.SupportedProtos, alpnH2) {
				h.logger.Warn("client offered HTTP/2, forcing HTTP/1.1 (HTTP/2 traffic cannot be inspected)",
					"hostname", hostname,
					"supported_protos", hello.SupportedProtos,
				)
			}
			// Return nil to fall back to the outer config (certificates + NextProtos).
			return nil, nil
		},
	}
}

// generateConnectionID generates a unique connection ID
func generateConnectionID() string {
	hash := sha256.Sum256([]byte(time.Now().Format(time.RFC3339Nano) + "-" + randomString(8)))
	return hex.EncodeToString(hash[:8])
}

// relayTraffic relays data between client and server until both directions
// terminate. Copy errors are logged at debug level but never returned:
// by the time relaying starts, the connection has already been fully taken
// over (TLS terminated), so callers cannot fall back to another handling
// path and an error return would carry no actionable meaning.
func (h *ConnectionHandler) relayTraffic(client, server net.Conn, hostname string) error {
	var wg sync.WaitGroup

	// Generate unique connection ID using UUID
	connectionID := generateConnectionID()

	// Create request ID generator for this connection
	idGenerator := NewRequestIDGenerator(connectionID)

	// Create inspectable ReadWriters if inspector is active
	var clientReader io.Reader = client
	var clientWriter io.Writer = client
	var serverReader io.Reader = server
	var serverWriter io.Writer = server

	if h.inspector.ShouldInspect(hostname) {
		// Only inspect on read operations to avoid duplicate inspection
		// Client -> Server: inspect when reading from client
		clientReader = NewInspectReader(client, h.inspector, hostname, DirectionClientToServer, h.logger, idGenerator)
		// Server -> Client: inspect when reading from server
		serverReader = NewInspectReader(server, h.inspector, hostname, DirectionServerToClient, h.logger, idGenerator)
		// Use original connections for write operations
		clientWriter = client
		serverWriter = server
	}

	// Client -> Server
	wg.Go(func() {
		// Get buffer from pool
		buffer := bufferPool.Get().([]byte)
		defer bufferPool.Put(buffer)
		_, err := io.CopyBuffer(serverWriter, clientReader, buffer)
		// Propagate EOF to the server so the opposite copy can terminate.
		// Without this, when one direction ends (e.g. client disconnects)
		// the other direction blocks on Read forever, leaking goroutines.
		closeWrite(server)
		if err != nil {
			h.logger.Debug("client->server relay ended", "hostname", hostname, "connection_id", connectionID, "error", err)
		}
	})

	// Server -> Client
	wg.Go(func() {
		// Get buffer from pool
		buffer := bufferPool.Get().([]byte)
		defer bufferPool.Put(buffer)
		_, err := io.CopyBuffer(clientWriter, serverReader, buffer)
		// Propagate EOF to the client (see comment above).
		closeWrite(client)
		if err != nil {
			h.logger.Debug("server->client relay ended", "hostname", hostname, "connection_id", connectionID, "error", err)
		}
	})

	wg.Wait()

	// Both copy goroutines have exited, so no Inspect call can be in flight:
	// notify inspectors to purge any leftover per-connection state (e.g.
	// streams aborted midway that never saw a terminating event).
	h.inspector.NotifyConnectionClosed(connectionID)
	return nil
}

// closeWrite shuts down the write side of c when the connection type
// supports half-close (e.g. *net.TCPConn, *tls.Conn), signaling EOF
// (TCP FIN or TLS close_notify) to the peer while still allowing
// in-flight data to be read back. For connection types without
// half-close support it falls back to closing the whole connection.
// Close errors are intentionally ignored: this is a best-effort
// teardown of an already-finished stream.
func closeWrite(c net.Conn) {
	if cw, ok := c.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
		return
	}
	_ = c.Close()
}

// PeekReader is a net.Conn that allows peeking at data
type PeekReader struct {
	net.Conn
	reader *bufio.Reader
}

// NewPeekReader creates a new PeekReader
func NewPeekReader(conn net.Conn) *PeekReader {
	return &PeekReader{
		Conn:   conn,
		reader: bufio.NewReaderSize(conn, DefaultBufferSize),
	}
}

// Peek returns the next n bytes without consuming them
func (p *PeekReader) Peek(n int) ([]byte, error) {
	return p.reader.Peek(n)
}

// Read consumes bytes from the buffered data
func (p *PeekReader) Read(b []byte) (n int, err error) {
	return p.reader.Read(b)
}

// Buffered returns the number of bytes currently buffered
func (p *PeekReader) Buffered() int {
	return p.reader.Buffered()
}
