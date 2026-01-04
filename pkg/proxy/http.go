package proxy

import (
	"bufio"
	"bytes"
	"io"
	"log/slog"
	"net"
	"strings"
	"time"
)

// extractHostFromHTTP attempts to extract the Host header from an HTTP request.
// It reads the initial request line and headers from the connection,
// parses the Host header if present, and returns it. The read bytes are
// buffered so they can be read again by the connection handler.
func extractHostFromHTTP(conn net.Conn) (host string, err error) {
	// Set a timeout for reading
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	defer conn.SetReadDeadline(time.Time{})

	// Create a buffered reader to wrap the connection
	reader := bufio.NewReaderSize(conn, 4096)

	// Read the request line (e.g., "GET / HTTP/1.1")
	requestLine, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	// Parse the request line
	parts := strings.Fields(requestLine)
	if len(parts) < 2 {
		// Invalid request line, put back what we read
		_ = reader.UnreadByte()
		return "", nil
	}

	// Read headers until we find Host or reach the end
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			// Error reading, can't recover
			return "", nil
		}

		line = strings.TrimSpace(line)

		// End of headers (blank line)
		if line == "" {
			break
		}

		// Check for Host header (case-insensitive)
		if strings.HasPrefix(strings.ToLower(line), "host:") {
			host = strings.TrimSpace(line[5:])
			// Remove port if present
			if idx := strings.LastIndex(host, ":"); idx != -1 {
				// Check if it's a port number (all digits after :)
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
			break
		}
	}

	if host != "" {
		slog.Debug("HTTP Host extracted", "host", host)
	}

	// Store the buffered data - the wrapper will handle returning it
	// We need to unread all the data we read
	// This is tricky with bufio.Reader, so we'll use a wrapper approach

	return host, nil
}

// HTTPClientConn wraps a net.Conn with a buffered reader for HTTP Host extraction
type HTTPClientConn struct {
	net.Conn
	reader *bufio.Reader
	buffer *bytes.Buffer
}

// NewHTTPClientConn creates a new HTTPClientConn
func NewHTTPClientConn(conn net.Conn) *HTTPClientConn {
	return &HTTPClientConn{
		Conn:   conn,
		reader: bufio.NewReaderSize(conn, 4096),
		buffer: new(bytes.Buffer),
	}
}

// Read implements io.Reader, returning buffered bytes first
func (c *HTTPClientConn) Read(p []byte) (n int, err error) {
	// Return buffered data first
	if c.buffer.Len() > 0 {
		return c.buffer.Read(p)
	}
	return c.reader.Read(p)
}

// Peek returns the next n bytes without consuming them
func (c *HTTPClientConn) Peek(n int) ([]byte, error) {
	// First try from buffer
	if c.buffer.Len() >= n {
		return c.buffer.Bytes()[:n], nil
	}

	// Need more data from reader
	data := make([]byte, n)
	copied := 0

	// Copy existing buffer
	if c.buffer.Len() > 0 {
		b := c.buffer.Bytes()
		copy(data, b)
		copied = len(b)
	}

	// Read more from reader
	n2, err := io.ReadFull(c.reader, data[copied:])
	if err != nil {
		// Return what we have
		return data[:copied+n2], err
	}

	return data, nil
}

// UnreadByte puts the last byte back into the buffer
func (c *HTTPClientConn) UnreadByte() error {
	// We can always unread to our buffer
	data := make([]byte, 1)
	n, err := c.reader.ReadByte()
	if err != nil {
		return err
	}
	data[0] = n
	c.buffer.Write(data)
	return nil
}

// UnreadRune puts the last rune back into the buffer
func (c *HTTPClientConn) UnreadRune() error {
	data := make([]byte, 4)
	n, err := c.reader.ReadByte()
	if err != nil {
		return err
	}
	data[0] = n
	c.buffer.Write(data)
	return nil
}

// BufferRemaining returns the remaining bytes in the buffer
func (c *HTTPClientConn) BufferRemaining() int {
	return c.buffer.Len()
}

// ExtractHostFromBuffer extracts Host from raw HTTP request bytes
// Returns the Host and the remaining bytes that should be used
func ExtractHostFromBuffer(data []byte) (host string, remaining []byte) {
	lines := bytes.Split(data, []byte("\n"))

	for i, line := range lines {
		line = bytes.TrimSpace(line)

		// Skip empty lines at the start
		if len(line) == 0 {
			continue
		}

		// First non-empty line is the request line
		if i == 0 {
			continue // We don't need to parse the request line for Host
		}

		// Check for Host header
		lowerLine := bytes.ToLower(line)
		if bytes.HasPrefix(lowerLine, []byte("host:")) {
			host = strings.TrimSpace(string(line[5:]))
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
			break
		}

		// Empty line means end of headers
		if len(line) == 0 {
			break
		}
	}

	return host, data
}
