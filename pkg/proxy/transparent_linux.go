//go:build linux

package proxy

import (
	"fmt"
	"net"
	"syscall"
	"unsafe"
)

// SO_ORIGINAL_DST is the Linux socket option (SOL_IP, 80) that retrieves
// the original destination address after DNAT/REDIRECT. It is the Linux
// equivalent of pf's DIOCNATLOOK ioctl.
const SO_ORIGINAL_DST = 80

// getOriginalDestination retrieves the original destination address for a
// connection that was redirected to the proxy via DNAT. On Linux this is
// done via getsockopt(SO_ORIGINAL_DST).
func (p *TransparentProxy) getOriginalDestination(conn net.Conn) (OriginalDst, error) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return OriginalDst{}, fmt.Errorf("connection is not TCP")
	}

	file, err := tcpConn.File()
	if err != nil {
		return OriginalDst{}, err
	}
	defer file.Close()

	return getSOOriginalDst(int(file.Fd()))
}

// getSOOriginalDst calls getsockopt(fd, SOL_IP, SO_ORIGINAL_DST, …) and
// returns the original destination (before DNAT) as an OriginalDst value.
//
// SO_ORIGINAL_DST returns a struct sockaddr_in (16 bytes). The previous
// implementation used syscall.GetsockoptIPv6Mreq which expects a different
// struct layout (struct ipv6_mreq, 12 bytes) — that was incorrect and
// produced garbage IP/port values.
func getSOOriginalDst(fd int) (OriginalDst, error) {
	var addr syscall.RawSockaddrInet4
	addrLen := uint32(unsafe.Sizeof(addr))

	_, _, errno := syscall.Syscall6(
		syscall.SYS_GETSOCKOPT,
		uintptr(fd),
		syscall.IPPROTO_IP, // SOL_IP = 0
		uintptr(SO_ORIGINAL_DST),
		uintptr(unsafe.Pointer(&addr)),
		uintptr(unsafe.Pointer(&addrLen)),
		0,
	)
	if errno != 0 {
		return OriginalDst{},
			fmt.Errorf("getsockopt(SOL_IP, SO_ORIGINAL_DST) failed: %w", errno)
	}

	ip := net.IP(addr.Addr[:])
	port := int(ntohs(addr.Port))

	if isLocalHost(ip.String()) {
		return OriginalDst{}, fmt.Errorf("original destination is localhost")
	}

	return OriginalDst{IP: ip, Port: port}, nil
}

// ntohs converts a 16-bit value from network byte order to host byte order.
func ntohs(v uint16) uint16 {
	return (v>>8)&0xff | (v<<8)&0xff00
}
