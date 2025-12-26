//go:build !darwin && !linux
// +build !darwin,!linux

package proxy

import (
	"fmt"
	"net"
)

// Socket options for getting original destination
const (
	SO_ORIGINAL_DST      = 80  // Linux socket option number
	SO_RECVORIGDSTADDR   = 74  // macOS/Linux socket option number
)

// getOriginalDestination gets the original destination address for unsupported platforms
func (p *TransparentProxy) getOriginalDestination(conn net.Conn) (string, error) {
	// Fallback: try to parse from connection remote addr
	// This works when using REDIRECT target but not all cases
	addr := conn.RemoteAddr().String()
	if host, port, err := net.SplitHostPort(addr); err == nil {
		// If it's a local address, we can't determine the original destination
		if !isLocalHost(host) {
			return fmt.Sprintf("%s:%s", host, port), nil
		}
	}

	return "", fmt.Errorf("unable to determine original destination: platform not supported")
}
