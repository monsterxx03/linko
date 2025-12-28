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
	SO_ORIGINAL_DST    = 80 // Linux socket option number
	SO_RECVORIGDSTADDR = 74 // macOS/Linux socket option number
)

func (p *TransparentProxy) getOriginalDestination(conn net.Conn) (OriginalDst, error) {
	if originalDst, err := p.getSOOriginalDst(conn); err == nil {
		return originalDst, nil
	}

	return OriginalDst{}, fmt.Errorf("unable to determine original destination on Linux")
}

func (p *TransparentProxy) getSOOriginalDst(conn net.Conn) (OriginalDst, error) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return OriginalDst{}, fmt.Errorf("connection is not TCP")
	}

	file, err := tcpConn.File()
	if err != nil {
		return OriginalDst{}, err
	}
	defer file.Close()

	addr, err := syscall.GetsockoptIPv6Mreq(int(file.Fd()), syscall.IPPROTO_IP, SO_ORIGINAL_DST)
	if err != nil {
		return OriginalDst{}, err
	}

	ip := net.IP(addr.Multiaddr[4:8])
	port := int(addr.Multiaddr[2])<<8 + int(addr.Multiaddr[3])

	return OriginalDst{IP: ip, Port: port}, nil
}
