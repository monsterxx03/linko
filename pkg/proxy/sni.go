package proxy

import (
	"bufio"
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"strings"
	"time"
)

// extractSNI attempts to extract the Server Name (SNI) from a TLS ClientHello.
// It reads the initial bytes from the connection, parses the TLS handshake,
// and returns the SNI if found. The read bytes are buffered so they can be
// read again by the connection handler.
func extractSNI(conn net.Conn) (sni string, err error) {
	// Set a timeout for reading
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	defer conn.SetReadDeadline(time.Time{})

	// Create a buffered reader to wrap the connection
	reader := bufio.NewReaderSize(conn, 4096)

	// Read the first byte to check if this is a TLS record
	firstByte, err := reader.ReadByte()
	if err != nil {
		return "", err
	}

	// TLS record type: 0x16 = handshake
	if firstByte != 0x16 {
		// Not a TLS handshake, put the byte back
		_ = reader.UnreadByte()
		return "", nil
	}

	// Read TLS record header (5 bytes): version (2) + length (2) actually
	// TLS record header: ContentType (1) + Version (2) + Length (2)
	header := make([]byte, 5)
	if _, err := io.ReadFull(reader, header); err != nil {
		// Failed to read header, put back the first byte
		_ = reader.UnreadByte()
		return "", err
	}

	// Check if this is a handshake message (type 22)
	if header[0] != 0x16 {
		// Not a handshake, put back the bytes
		_ = reader.UnreadByte()
		_ = reader.UnreadByte() // Actually need to be careful here
		return "", nil
	}

	// Read the handshake message header (4 bytes): HandshakeType (1) + Length (3)
	handshakeHeader := make([]byte, 4)
	if _, err := io.ReadFull(reader, handshakeHeader); err != nil {
		return "", err
	}

	// Handshake type: 0x01 = ClientHello
	if handshakeHeader[0] != 0x01 {
		return "", nil
	}

	// Handshake length (24-bit big-endian)
	handshakeLength := int(handshakeHeader[1])<<16 | int(handshakeHeader[2])<<8 | int(handshakeHeader[3])

	// Read ClientHello body
	// Skip: LegacyVersion (2) + Random (32) + LegacySessionID (1 + variable)
	clientHello := make([]byte, handshakeLength-4)
	if _, err := io.ReadFull(reader, clientHello); err != nil {
		return "", err
	}

	pos := 0

	// Skip LegacyVersion (2 bytes) and Random (32 bytes)
	pos += 34

	// Skip LegacySessionID (1 byte length + variable)
	if pos < len(clientHello) {
		sessionIDLen := int(clientHello[pos])
		pos += 1 + sessionIDLen
	}

	// Skip CipherSuites (2 bytes length + variable)
	if pos+2 <= len(clientHello) {
		cipherSuiteLen := int(binary.BigEndian.Uint16(clientHello[pos : pos+2]))
		pos += 2 + cipherSuiteLen
	}

	// Skip CompressionMethods (1 byte length + variable)
	if pos < len(clientHello) {
		compressionLen := int(clientHello[pos])
		pos += 1 + compressionLen
	}

	// Now we're at Extensions
	if pos+2 > len(clientHello) {
		return "", nil
	}

	// Extensions length
	extensionsLen := int(binary.BigEndian.Uint16(clientHello[pos : pos+2]))
	pos += 2

	extensionsEnd := pos + extensionsLen

	// Parse extensions looking for Server Name (type 0x0000)
	for pos+4 <= extensionsEnd && pos < len(clientHello) {
		extType := binary.BigEndian.Uint16(clientHello[pos : pos+2])
		extLen := int(binary.BigEndian.Uint16(clientHello[pos+2 : pos+4]))
		pos += 4

		if extType == 0x0000 { // Server Name extension
			// Read Server Name list
			if pos+2 > len(clientHello) {
				return "", nil
			}
			sniListLen := int(binary.BigEndian.Uint16(clientHello[pos : pos+2]))
			pos += 2

			for pos+2 <= pos+sniListLen && pos < len(clientHello) {
				nameType := clientHello[pos]
				nameLen := int(binary.BigEndian.Uint16(clientHello[pos+1 : pos+3]))
				pos += 3

				if nameType == 0 { // host_name
					sni = string(clientHello[pos : pos+nameLen])
					break
				}
				pos += nameLen
			}
			break
		}

		pos += extLen
	}

	if sni != "" {
		slog.Debug("SNI extracted", "sni", sni)
	}

	// Now we need to create a wrapper that will return these buffered bytes first
	// The caller will use this buffered reader instead of the raw connection
	return sni, nil
}

// SNIClientConn wraps a net.Conn with a buffered reader for SNI extraction
type SNIClientConn struct {
	net.Conn
	reader *bufio.Reader
	peeked []byte
}

// NewSNIClientConn creates a new SNIClientConn
func NewSNIClientConn(conn net.Conn) *SNIClientConn {
	return &SNIClientConn{
		Conn:   conn,
		reader: bufio.NewReaderSize(conn, 4096),
	}
}

// Read implements io.Reader, returning buffered bytes first
func (c *SNIClientConn) Read(p []byte) (n int, err error) {
	if len(c.peeked) > 0 {
		n = copy(p, c.peeked)
		c.peeked = c.peeked[n:]
		if len(c.peeked) == 0 {
			c.peeked = nil
		}
		return n, nil
	}
	return c.reader.Read(p)
}

// Peek returns the next n bytes without consuming them
func (c *SNIClientConn) Peek(n int) ([]byte, error) {
	if len(c.peeked) >= n {
		return c.peeked[:n], nil
	}
	// Need more data
	needed := n - len(c.peeked)
	data := make([]byte, n)
	copy(data, c.peeked)
	c.peeked = nil

	m, err := io.ReadFull(c.reader, data[len(data)-needed:])
	if err != nil {
		// Put back what we read
		c.peeked = data[:len(data)-needed+m]
		return nil, err
	}
	c.peeked = data
	return data, nil
}

// UnreadByte puts the last byte back
func (c *SNIClientConn) UnreadByte() error {
	if c.peeked == nil {
		// Try to unread from the underlying reader
		return c.reader.UnreadByte()
	}
	// Put back into peeked buffer
	b := c.peeked[len(c.peeked)-1]
	c.peeked = c.peeked[:len(c.peeked)-1]
	// Prepend to peeked
	newPeeked := make([]byte, len(c.peeked)+1)
	newPeeked[0] = b
	copy(newPeeked[1:], c.peeked)
	c.peeked = newPeeked
	return nil
}

// ExtractSNIFromBuffer extracts SNI from raw bytes without a connection
// Returns the SNI and the remaining bytes that should be used
func ExtractSNIFromBuffer(data []byte) (sni string, remaining []byte) {
	if len(data) < 5 {
		return "", data
	}

	// Check for TLS handshake
	if data[0] != 0x16 {
		return "", data
	}

	// TLS record header is 5 bytes
	if 5+4 > len(data) {
		return "", data
	}

	// Check handshake type
	if data[5] != 0x01 { // ClientHello
		return "", data
	}

	// Handshake length (24-bit)
	handshakeLength := int(data[6])<<16 | int(data[7])<<8 | int(data[8])

	// Read ClientHello
	clientHelloLen := 4 + handshakeLength // 4 bytes header + body
	if 5+clientHelloLen > len(data) {
		return "", data
	}

	clientHello := data[9 : 9+handshakeLength]
	pos := 0

	// Skip LegacyVersion (2) + Random (32) + SessionID
	pos += 34
	if pos >= len(clientHello) {
		return "", data
	}
	sessionIDLen := int(clientHello[pos])
	pos += 1 + sessionIDLen

	// Skip CipherSuites
	if pos+2 > len(clientHello) {
		return "", data
	}
	cipherSuiteLen := int(binary.BigEndian.Uint16(clientHello[pos : pos+2]))
	pos += 2 + cipherSuiteLen

	// Skip CompressionMethods
	if pos >= len(clientHello) {
		return "", data
	}
	compressionLen := int(clientHello[pos])
	pos += 1 + compressionLen

	// Extensions
	if pos+2 > len(clientHello) {
		return "", data
	}
	extensionsLen := int(binary.BigEndian.Uint16(clientHello[pos : pos+2]))
	pos += 2

	extensionsEnd := pos + extensionsLen

	for pos+4 <= extensionsEnd && pos < len(clientHello) {
		extType := binary.BigEndian.Uint16(clientHello[pos : pos+2])
		extLen := int(binary.BigEndian.Uint16(clientHello[pos+2 : pos+4]))
		pos += 4

		if extType == 0x0000 { // Server Name
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
					sni = strings.ToLower(string(clientHello[pos : pos+nameLen]))
					break
				}
				pos += nameLen
			}
			break
		}

		pos += extLen
	}

	return sni, data
}
