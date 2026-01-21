package proxy

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"

	"github.com/monsterxx03/linko/pkg/mitm"
)

// MITMHandler handles MITM proxy connections
type MITMHandler struct {
	proxy     *TransparentProxy
	manager   *mitm.Manager
	logger    *slog.Logger
	whitelist map[string]bool
}

// NewMITMHandler creates a new MITM handler
func NewMITMHandler(proxy *TransparentProxy, manager *mitm.Manager, whitelist []string, logger *slog.Logger) *MITMHandler {
	// Build whitelist map for fast lookup
	whitelistMap := make(map[string]bool)
	for _, domain := range whitelist {
		whitelistMap[strings.ToLower(domain)] = true
	}

	return &MITMHandler{
		proxy:     proxy,
		manager:   manager,
		logger:    logger,
		whitelist: whitelistMap,
	}
}

// BufferedConn wraps a net.Conn and provides buffered data that was already read
type BufferedConn struct {
	net.Conn
	 buffered []byte
}

// Read returns buffered data first, then reads from underlying connection
func (b *BufferedConn) Read(p []byte) (n int, err error) {
	if len(b.buffered) > 0 {
		n = copy(p, b.buffered)
		b.buffered = b.buffered[n:]
		if len(b.buffered) == 0 {
			b.buffered = nil
		}
		return n, nil
	}
	return b.Conn.Read(p)
}

// HandleConnection handles a MITM connection for HTTPS traffic
// It checks whitelist first using PeekReader, and only proceeds with MITM if domain is allowed
func (h *MITMHandler) HandleConnection(clientConn net.Conn, originalDst OriginalDst) (net.Conn, error) {
	if !h.manager.IsEnabled() {
		return nil, fmt.Errorf("MITM is not enabled")
	}

	// Wrap connection with PeekReader for both whitelist check and MITM
	peekReader := mitm.NewPeekReader(clientConn)

	// If whitelist is not empty, check if domain is in whitelist
	if len(h.whitelist) > 0 {
		sni, err := h.extractSNI(peekReader)
		if err != nil || sni == "" {
			h.logger.Debug("Cannot extract SNI for whitelist check, skipping MITM",
				"target", originalDst, "error", err)
			// Get buffered data and wrap connection
			buffered := h.getBufferedData(peekReader)
			return &BufferedConn{Conn: clientConn, buffered: buffered}, nil
		}

		if !h.isInWhitelist(sni) {
			h.logger.Debug("Domain not in whitelist, skipping MITM",
				"sni", sni, "target", originalDst)
			// Get buffered data and wrap connection
			buffered := h.getBufferedData(peekReader)
			return &BufferedConn{Conn: clientConn, buffered: buffered}, nil
		}
	}

	// Proceed with MITM using the same PeekReader
	handler := h.manager.ConnectionHandlerWithPeekReader(h.proxy.upstream, peekReader)
	err := handler.HandleConnection(clientConn, originalDst.IP, originalDst.Port)
	if err != nil {
		return nil, err
	}
	// MITM succeeded, return nil to indicate connection is handled
	return nil, nil
}

// getBufferedData extracts the already-buffered data from PeekReader
func (h *MITMHandler) getBufferedData(reader *mitm.PeekReader) []byte {
	buffered := make([]byte, reader.Buffered())
	reader.Read(buffered)
	return buffered
}

// IsEnabled returns whether MITM handling is enabled
func (h *MITMHandler) IsEnabled() bool {
	return h.manager != nil && h.manager.IsEnabled()
}

// extractSNI peeks at the connection to extract SNI without consuming data
func (h *MITMHandler) extractSNI(reader *mitm.PeekReader) (string, error) {
	// Peek at TLS record header first
	header, err := reader.Peek(5)
	if err != nil {
		return "", fmt.Errorf("failed to peek TLS header: %w", err)
	}

	// Get the full record length
	recordLen := int(header[3])<<8 | int(header[4])
	totalLen := 5 + recordLen
	if totalLen > 16384 {
		totalLen = 16384
	}

	// Peek at complete record
	data, err := reader.Peek(totalLen)
	if err != nil && len(data) < 200 {
		return "", fmt.Errorf("failed to peek TLS record: %w", err)
	}

	// Parse SNI
	sniInfo, err := mitm.ExtractSNIFromConn(data)
	if err != nil {
		return "", fmt.Errorf("SNI parsing failed: %w", err)
	}

	if sniInfo.IsValid && sniInfo.Hostname != "" {
		return sniInfo.Hostname, nil
	}

	return "", fmt.Errorf("SNI not found")
}

// isInWhitelist checks if a domain is in the whitelist
func (h *MITMHandler) isInWhitelist(domain string) bool {
	domainLower := strings.ToLower(domain)

	// Exact match
	if h.whitelist[domainLower] {
		return true
	}

	// Wildcard match
	for pattern := range h.whitelist {
		if strings.HasPrefix(pattern, "*.") {
			base := strings.TrimPrefix(pattern, "*.")
			if strings.HasSuffix(domainLower, "."+base) {
				return true
			}
		}
	}

	return false
}

// BufferedReader is a bufio.Reader that allows getting buffered data
type BufferedReader struct {
	*bufio.Reader
}

// NewBufferedReader creates a new BufferedReader
func NewBufferedReader(r io.Reader) *BufferedReader {
	return &BufferedReader{
		Reader: bufio.NewReaderSize(r, 16384),
	}
}

// Buffered returns the number of bytes buffered
func (b *BufferedReader) Buffered() int {
	return b.Reader.Buffered()
}
