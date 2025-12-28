//go:build !darwin && !linux
// +build !darwin,!linux

package proxy

import (
	"fmt"
	"net"
	"strconv"
)

// Socket options for getting original destination
const (
	SO_ORIGINAL_DST    = 80 // Linux socket option number
	SO_RECVORIGDSTADDR = 74 // macOS/Linux socket option number
)

// getOriginalDestination gets the original destination address for unsupported platforms
func (p *TransparentProxy) getOriginalDestination(conn net.Conn) (OriginalDst, error) {
	addr := conn.RemoteAddr().String()
	if host, portStr, err := net.SplitHostPort(addr); err == nil {
		if !isLocalHost(host) {
			port, _ := strconv.Atoi(portStr)
			return OriginalDst{IP: net.ParseIP(host), Port: port}, nil
		}
	}

	return OriginalDst{}, fmt.Errorf("unable to determine original destination: platform not supported")
}
