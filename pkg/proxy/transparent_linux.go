//go:build linux
// +build linux

package proxy

import (
	"fmt"
	"net"
	"syscall"
)

// Socket options for getting original destination
const (
	SO_ORIGINAL_DST      = 80  // Linux socket option number
	SO_RECVORIGDSTADDR   = 74  // macOS/Linux socket option number
)

// getOriginalDestination gets the original destination address on Linux
func (p *TransparentProxy) getOriginalDestination(conn net.Conn) (string, error) {
	// Try SO_ORIGINAL_DST socket option first
	if originalDst, err := p.getSOOriginalDst(conn); err == nil {
		return originalDst, nil
	}

	// Try SO_RECVORIGDSTADDR as fallback on Linux
	if originalDst, err := p.getSORecvOrigDstAddr(conn); err == nil {
		return originalDst, nil
	}

	return "", fmt.Errorf("unable to determine original destination on Linux")
}

// getSOOriginalDst gets the original destination using SO_ORIGINAL_DST socket option (Linux)
func (p *TransparentProxy) getSOOriginalDst(conn net.Conn) (string, error) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return "", fmt.Errorf("connection is not TCP")
	}

	// Get underlying socket file descriptor
	file, err := tcpConn.File()
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Get SO_ORIGINAL_DST address using syscall
	addr, err := syscall.GetsockoptIPv6Mreq(int(file.Fd()), syscall.IPPROTO_IP, SO_ORIGINAL_DST)
	if err != nil {
		return "", err
	}

	// Parse the address
	ip := net.IP(addr.Multiaddr[4:8]).String()
	port := int(addr.Multiaddr[2])<<8 + int(addr.Multiaddr[3])

	return fmt.Sprintf("%s:%d", ip, port), nil
}

// getSORecvOrigDstAddr gets the original destination using SO_RECVORIGDSTADDR socket option (Linux)
func (p *TransparentProxy) getSORecvOrigDstAddr(conn net.Conn) (string, error) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return "", fmt.Errorf("connection is not TCP")
	}

	// Get underlying socket file descriptor
	file, err := tcpConn.File()
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Try to get SO_RECVORIGDSTADDR using a generic approach
	// Note: This may not be available on all systems
	return "", fmt.Errorf("SO_RECVORIGDSTADDR method not implemented in syscall")
}
