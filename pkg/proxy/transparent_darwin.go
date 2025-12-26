//go:build darwin
// +build darwin

package proxy

import (
	"fmt"
	"net"
	"unsafe"

	"golang.org/x/sys/unix"
)

// pfiocNatlook represents the pfioc_natlook structure for DIOCNATLOOK ioctl
type pfiocNatlook struct {
	aftype   int32      // Address family (AF_INET or AF_INET6)
	proto    int32      // Protocol (IPPROTO_TCP)
	direction int32     // NATLOOK_DIRECTION flags
	saddr    [16]byte   // Source address
	daddr    [16]byte   // Destination address (after NAT)
	sport    uint16     // Source port
	dport    uint16     // Destination port (after NAT)
	ExtPAD   [32]byte   // Extension padding for future use
}

// DIOCNATLOOK constants (from net/pf.c)
const (
	// NATLOOK directions
	NATLOOK_DIR_IN  = 0x00000001
	NATLOOK_DIR_OUT = 0x00000002

	// Address families
	AF_INET  = 2
	AF_INET6 = 28

	// Socket option for getting original destination
	SO_ORIGINAL_DST      = 80  // Linux socket option number
)

// _DIOCNATLOOK is the ioctl number for DIOCNATLOOK
// This is defined in net/pf.c on macOS. The ioctl number is computed using _IOW macro:
// #define DIOCNATLOOK _IOW('n', 1, struct pfioc_natlook)
// 'n' = 0x6e, 1 = 1, so the number is approximately 0x80006e01
// This may vary slightly depending on architecture and kernel version
const _DIOCNATLOOK = 0x40086e01

// getOriginalDestination gets the original destination address on macOS
func (p *TransparentProxy) getOriginalDestination(conn net.Conn) (string, error) {
	return p.getDIOCNATLOOK(conn)
}

// getDIOCNATLOOK uses the DIOCNATLOOK ioctl to get the original destination on macOS
func (p *TransparentProxy) getDIOCNATLOOK(conn net.Conn) (string, error) {
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

	fd := int(file.Fd())

	// Get socket address information
	sockaddr, err := unix.Getsockname(fd)
	if err != nil {
		return "", fmt.Errorf("failed to get socket name: %w", err)
	}

	var nl pfiocNatlook

	// Determine address family and fill structure
	switch addr := sockaddr.(type) {
	case *unix.SockaddrInet4:
		nl.aftype = AF_INET
		copy(nl.saddr[:], addr.Addr[:])
		nl.sport = uint16(addr.Port)
	case *unix.SockaddrInet6:
		nl.aftype = AF_INET6
		copy(nl.saddr[:], addr.Addr[:])
		nl.sport = uint16(addr.Port)
	default:
		return "", fmt.Errorf("unsupported address family: %T", addr)
	}

	nl.proto = unix.IPPROTO_TCP
	nl.direction = NATLOOK_DIR_OUT

	// Try ioctl with NATLOOK_DIR_OUT
	// Use unix.Ioctl to pass pointer to the natlook structure
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(_DIOCNATLOOK), uintptr(unsafe.Pointer(&nl))); errno != 0 {
		return "", fmt.Errorf("DIOCNATLOOK ioctl failed: %v", errno)
	}

	// Extract destination address from natlook structure
	var dstIP net.IP
	var dstPort int

	if nl.aftype == AF_INET {
		dstIP = net.IP(nl.daddr[:4])
		dstPort = int(nl.dport)
	} else if nl.aftype == AF_INET6 {
		dstIP = net.IP(nl.daddr[:16])
		dstPort = int(nl.dport)
	} else {
		return "", fmt.Errorf("invalid address family in natlook result")
	}

	return fmt.Sprintf("%s:%d", dstIP.String(), dstPort), nil
}
