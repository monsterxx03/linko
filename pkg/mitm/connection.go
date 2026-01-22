package mitm

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
)

// ConnectionHandler handles MITM connections
type ConnectionHandler struct {
	siteCertManager *SiteCertManager
	logger          *slog.Logger
	upstream        UpstreamClient
	peekReader      *PeekReader // Optional pre-wrapped connection for whitelist check
	inspector       *InspectorChain
	ctx             interface{}
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
		serverConn, err = net.DialTCP("tcp", nil, &net.TCPAddr{IP: targetIP, Port: targetPort})
		if err != nil {
			return fmt.Errorf("failed to connect to target: %w", err)
		}
	}
	defer serverConn.Close()

	// Create TLS config for client side (MITM side)
	clientTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{*siteCert},
		ServerName:   hostname,
		// Accept any certificate from the server
		InsecureSkipVerify: true,
	}

	// Create TLS config for server side (connecting to actual server)
	serverTLSConfig := &tls.Config{
		ServerName: hostname,
		// Verify server certificate
		InsecureSkipVerify: false,
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
	totalLen := 5 + recordLen

	// Peek at the complete TLS record
	if totalLen > 16384 {
		totalLen = 16384 // Cap at buffer size
	}

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

// relayTraffic relays data between client and server
func (h *ConnectionHandler) relayTraffic(client, server net.Conn, hostname string) error {
	var wg sync.WaitGroup

	// Create inspectable ReadWriters if inspector is active
	var clientReader, clientWriter io.ReadWriter = client, client
	var serverReader, serverWriter io.ReadWriter = server, server

	if h.inspector.ShouldInspect(hostname) {
		clientReader = NewReadWriter(client, h.inspector, hostname, DirectionServerToClient, h.logger)
		clientWriter = NewReadWriter(client, h.inspector, hostname, DirectionClientToServer, h.logger)
		serverReader = NewReadWriter(server, h.inspector, hostname, DirectionClientToServer, h.logger)
		serverWriter = NewReadWriter(server, h.inspector, hostname, DirectionServerToClient, h.logger)
	}

	// Client -> Server
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(serverWriter, clientReader)
	}()

	// Server -> Client
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(clientWriter, serverReader)
	}()

	wg.Wait()
	return nil
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
		reader: bufio.NewReaderSize(conn, 16384),
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
