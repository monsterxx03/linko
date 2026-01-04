package proxy

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/monsterxx03/linko/pkg/traffic"
)

type OriginalDst struct {
	IP   net.IP
	Port int
}

// TransparentProxy represents a transparent proxy
type TransparentProxy struct {
	listenAddr   string
	server       net.Listener
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	stats        *ProxyStats
	upstream     *UpstreamClient
	enableDirect bool          // Enable direct connection when upstream is disabled
	trafficStats *traffic.TrafficStatsCollector
}

// ProxyStats tracks proxy statistics
type ProxyStats struct {
	totalConnections  uint64
	activeConnections uint64
	bytesTransferred  uint64
	startTime         time.Time
	mu                sync.RWMutex
}

// NewTransparentProxy creates a new transparent proxy
func NewTransparentProxy(listenAddr string, upstream *UpstreamClient, trafficStats *traffic.TrafficStatsCollector) *TransparentProxy {
	ctx, cancel := context.WithCancel(context.Background())
	return &TransparentProxy{
		listenAddr:   listenAddr,
		ctx:          ctx,
		cancel:       cancel,
		stats: &ProxyStats{
			startTime: time.Now(),
		},
		upstream:     upstream,
		enableDirect: !upstream.IsEnabled(),
		trafficStats: trafficStats,
	}
}

// Start starts the transparent proxy
func (p *TransparentProxy) Start() error {
	listener, err := net.Listen("tcp", p.listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", p.listenAddr, err)
	}

	p.server = listener

	p.wg.Add(1)
	go p.acceptLoop()

	if p.upstream.IsEnabled() {
		slog.Info("Transparent proxy listening", "address", p.listenAddr, "upstream_type", p.upstream.GetConfig().Type, "upstream_addr", p.upstream.GetConfig().Addr, "mode", "proxy")
	} else {
		slog.Info("Transparent proxy listening", "address", p.listenAddr, "mode", "direct")
	}
	return nil
}

// Stop stops the transparent proxy
func (p *TransparentProxy) Stop() {
	p.cancel()

	if p.server != nil {
		p.server.Close()
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		p.wg.Wait()
	}()

	select {
	case <-done:
		slog.Info("Transparent proxy stopped")
	case <-time.After(10 * time.Second):
		slog.Warn("Transparent proxy stop timeout")
	}
}

// acceptLoop accepts incoming connections
func (p *TransparentProxy) acceptLoop() {
	defer p.wg.Done()

	for {
		conn, err := p.server.Accept()
		if err != nil {
			select {
			case <-p.ctx.Done():
				return
			default:
				slog.Error("Accept error", "error", err)
				continue
			}
		}

		p.wg.Add(1)
		go p.handleConnection(conn)
	}
}

// handleConnection handles a single connection
func (p *TransparentProxy) handleConnection(clientConn net.Conn) {
	defer p.wg.Done()
	defer clientConn.Close()

	// Update stats
	p.stats.mu.Lock()
	p.stats.totalConnections++
	p.stats.activeConnections++
	p.stats.mu.Unlock()

	defer func() {
		p.stats.mu.Lock()
		p.stats.activeConnections--
		p.stats.mu.Unlock()
	}()

	// Get original destination from connection
	originalDst, err := p.getOriginalDestination(clientConn)
	if err != nil {
		slog.Error("Failed to get original destination", "error", err)
		return
	}

	// Extract identifier (SNI or Host) based on port
	identifier := originalDst.IP.String()
	isDomain := false

	// Create a buffered reader for identifier extraction
	br := bufio.NewReaderSize(clientConn, 4096)

	if originalDst.Port == 443 {
		// HTTPS: Try to extract SNI from TLS ClientHello
		sni, err := extractSNIFromReader(br)
		if err == nil && sni != "" {
			identifier = sni
			isDomain = true
		}
	} else if originalDst.Port == 80 {
		// HTTP: Try to extract Host header
		host, err := extractHostFromReader(br)
		if err == nil && host != "" {
			identifier = host
			isDomain = true
		}
	}

	slog.Debug("Proxying connection", "from", clientConn.RemoteAddr(), "to", "dst", "ip", originalDst.IP, "port", originalDst.Port, "identifier", identifier, "is_domain", isDomain)

	// Connect to target
	var targetConn net.Conn
	targetHost := originalDst.IP.String()
	targetPort := originalDst.Port
	if p.upstream.IsEnabled() {
		targetConn, err = p.upstream.Connect(targetHost, targetPort)
		if err != nil {
			slog.Error("Failed to connect via upstream proxy", "target", originalDst, "error", err)
			return
		}
	} else {
		targetConn, err = net.DialTCP("tcp", nil, &net.TCPAddr{IP: originalDst.IP, Port: originalDst.Port})
		if err != nil {
			slog.Error("Failed to connect to target", "target", originalDst, "error", err)
			return
		}
	}
	defer targetConn.Close()

	// Create a wrapped connection that includes our buffered reader
	wrappedClient := &bufferedConn{
		Conn:   clientConn,
		reader: br,
	}

	// Relay data
	upload, download, err := p.relayBidirectional(wrappedClient, targetConn)

	// Update stats
	if err == nil {
		p.stats.mu.Lock()
		p.stats.bytesTransferred += uint64(upload + download)
		p.stats.mu.Unlock()
	}

	// Record traffic stats
	if p.trafficStats != nil {
		p.trafficStats.Record(&traffic.TrafficRecord{
			Identifier: identifier,
			IsDomain:   isDomain,
			Upload:     upload,
			Download:   download,
			Timestamp:  time.Now(),
		})
	}
}

// relayBidirectional relays data between client and target
func (p *TransparentProxy) relayBidirectional(client, target net.Conn) (int64, int64, error) {
	errChan := make(chan error, 2)
	uploadChan := make(chan int64, 1)
	downloadChan := make(chan int64, 1)

	go func() {
		n, err := io.Copy(target, client)
		uploadChan <- n
		errChan <- err
	}()

	go func() {
		n, err := io.Copy(client, target)
		downloadChan <- n
		errChan <- err
	}()

	var upload, download int64
	var err error

	for i := 0; i < 2; i++ {
		select {
		case n := <-uploadChan:
			upload = n
		case n := <-downloadChan:
			download = n
		case e := <-errChan:
			if err == nil {
				err = e
			}
		case <-p.ctx.Done():
			return upload, download, p.ctx.Err()
		}
	}

	return upload, download, err
}

// isLocalHost checks if an IP address is localhost
func isLocalHost(ip string) bool {
	return ip == "127.0.0.1" || ip == "::1" || ip == "localhost"
}

// GetStats returns proxy statistics
func (p *TransparentProxy) GetStats() map[string]interface{} {
	p.stats.mu.RLock()
	defer p.stats.mu.RUnlock()

	uptime := time.Since(p.stats.startTime).Seconds()

	stats := make(map[string]interface{})
	stats["listen_addr"] = p.listenAddr
	stats["upstream_enabled"] = p.upstream.IsEnabled()
	if p.upstream.IsEnabled() {
		stats["upstream_type"] = p.upstream.GetConfig().Type
		stats["upstream_addr"] = p.upstream.GetConfig().Addr
	}
	stats["total_connections"] = p.stats.totalConnections
	stats["active_connections"] = p.stats.activeConnections
	stats["bytes_transferred"] = p.stats.bytesTransferred
	stats["bytes_transferred_mb"] = float64(p.stats.bytesTransferred) / (1024 * 1024)
	stats["uptime_seconds"] = uptime
	stats["uptime_hours"] = uptime / 3600

	return stats
}

// IsRunning checks if the proxy is running
func (p *TransparentProxy) IsRunning() bool {
	return p.ctx.Err() == nil
}

// GetListenAddr returns the listen address
func (p *TransparentProxy) GetListenAddr() string {
	return p.listenAddr
}

// GetTargetAddr returns the listen address (no fixed target in transparent mode)
func (p *TransparentProxy) GetTargetAddr() string {
	return "dynamic (from original destination)"
}

// bufferedConn wraps a net.Conn with a bufio.Reader for SNI/Host extraction
type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

// Read reads from the underlying connection, using buffered data first
func (c *bufferedConn) Read(p []byte) (n int, err error) {
	return c.reader.Read(p)
}

// extractSNIFromReader extracts SNI from a buffered reader
func extractSNIFromReader(reader *bufio.Reader) (string, error) {
	// Read the first byte to check if this is a TLS record
	firstByte, err := reader.ReadByte()
	if err != nil {
		return "", err
	}

	// TLS record type: 0x16 = handshake
	if firstByte != 0x16 {
		// Not a TLS handshake, put the byte back
		return "", reader.UnreadByte()
	}

	// Read TLS record header (5 bytes)
	header := make([]byte, 5)
	if _, err := io.ReadFull(reader, header); err != nil {
		return "", reader.UnreadByte()
	}

	// Check if this is a handshake message (type 22)
	if header[0] != 0x16 {
		return "", nil
	}

	// Handshake type: 0x01 = ClientHello
	handshakeType, err := reader.ReadByte()
	if err != nil {
		return "", nil
	}
	if handshakeType != 0x01 {
		return "", nil
	}

	// Read handshake length (3 bytes)
	handshakeLenBytes := make([]byte, 3)
	if _, err := io.ReadFull(reader, handshakeLenBytes); err != nil {
		return "", nil
	}
	handshakeLength := int(handshakeLenBytes[0])<<16 | int(handshakeLenBytes[1])<<8 | int(handshakeLenBytes[2])

	// Read ClientHello body
	clientHello := make([]byte, handshakeLength)
	if _, err := io.ReadFull(reader, clientHello); err != nil {
		return "", nil
	}

	pos := 0

	// Skip LegacyVersion (2 bytes) and Random (32 bytes)
	pos += 34

	// Skip LegacySessionID
	if pos < len(clientHello) {
		sessionIDLen := int(clientHello[pos])
		pos += 1 + sessionIDLen
	}

	// Skip CipherSuites
	if pos+2 <= len(clientHello) {
		cipherSuiteLen := int(binary.BigEndian.Uint16(clientHello[pos : pos+2]))
		pos += 2 + cipherSuiteLen
	}

	// Skip CompressionMethods
	if pos < len(clientHello) {
		compressionLen := int(clientHello[pos])
		pos += 1 + compressionLen
	}

	// Extensions
	if pos+2 > len(clientHello) {
		return "", nil
	}
	extensionsLen := int(binary.BigEndian.Uint16(clientHello[pos : pos+2]))
	pos += 2

	extensionsEnd := pos + extensionsLen

	for pos+4 <= extensionsEnd && pos < len(clientHello) {
		extType := binary.BigEndian.Uint16(clientHello[pos : pos+2])
		extLen := int(binary.BigEndian.Uint16(clientHello[pos+2 : pos+4]))
		pos += 4

		if extType == 0x0000 { // Server Name extension
			if pos+2 > len(clientHello) {
				break
			}
			sniListLen := int(binary.BigEndian.Uint16(clientHello[pos : pos+2]))
			pos += 2

			for pos+2 <= pos+sniListLen && pos < len(clientHello) {
				nameType := clientHello[pos]
				nameLen := int(binary.BigEndian.Uint16(clientHello[pos+1 : pos+3]))
				pos += 3

				if nameType == 0 && pos+nameLen <= len(clientHello) {
					return strings.ToLower(string(clientHello[pos : pos+nameLen])), nil
				}
				pos += nameLen
			}
			break
		}

		pos += extLen
	}

	return "", nil
}

// extractHostFromReader extracts Host header from a buffered reader
func extractHostFromReader(reader *bufio.Reader) (string, error) {
	// Read the request line
	_, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	// Read headers until we find Host or reach the end
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", nil
		}

		line = strings.TrimSpace(line)

		// End of headers
		if line == "" {
			break
		}

		// Check for Host header
		if strings.HasPrefix(strings.ToLower(line), "host:") {
			host := strings.TrimSpace(line[5:])
			// Remove port if present
			if idx := strings.LastIndex(host, ":"); idx != -1 {
				portPart := host[idx+1:]
				if len(portPart) > 0 && len(portPart) < 6 {
					allDigits := true
					for _, c := range portPart {
						if c < '0' || c > '9' {
							allDigits = false
							break
						}
					}
					if allDigits {
						host = host[:idx]
					}
				}
			}
			return host, nil
		}
	}

	return "", nil
}
