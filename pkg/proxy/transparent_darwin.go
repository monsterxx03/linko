//go:build darwin
// +build darwin

package proxy

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

var getPfDev = sync.OnceValues(func() (*os.File, error) {
	return os.OpenFile("/dev/pf", unix.O_RDWR, 0644)
})

type pfioc_natlook struct {
	saddr, daddr, rsaddr, rdaddr        [16]byte
	sxport, dxport, rsxport, rdxport    [4]byte
	af, proto, proto_variant, direction uint8
}

const (
	PF_NATLOOK_DIR_OUT = 2

	DIOCNATLOOK = 3226747927
)

func (p *TransparentProxy) getOriginalDestination(conn net.Conn) (string, error) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return "", fmt.Errorf("connection is not TCP")
	}

	localAddr, ok := tcpConn.LocalAddr().(*net.TCPAddr)
	if !ok {
		return "", fmt.Errorf("failed to get local address")
	}
	remoteAddr, ok := tcpConn.RemoteAddr().(*net.TCPAddr)
	if !ok {
		return "", fmt.Errorf("failed to get remote address")
	}

	nl := new(pfioc_natlook)
	nl.direction = PF_NATLOOK_DIR_OUT
	nl.proto = syscall.IPPROTO_TCP
	binary.BigEndian.PutUint16(nl.sxport[:2], uint16(remoteAddr.Port))
	binary.BigEndian.PutUint16(nl.dxport[:2], uint16(localAddr.Port))

	if localAddr.IP.To4() != nil {
		nl.af = syscall.AF_INET
		copy(nl.saddr[:4], remoteAddr.IP.To4())
		copy(nl.daddr[:4], localAddr.IP.To4())
	} else {
		nl.af = unix.AF_INET6
		copy(nl.saddr[:16], remoteAddr.IP.To16())
		copy(nl.daddr[:16], localAddr.IP.To16())
	}

	pfFd, err := getPfDev()
	if err != nil {
		return "", fmt.Errorf("failed to open /dev/pf: %v", err)
	}

	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		pfFd.Fd(),
		DIOCNATLOOK,
		uintptr(unsafe.Pointer(nl)),
	)
	if errno != 0 {
		return "", fmt.Errorf("DIOCNATLOOK ioctl failed: %v", errno)
	}

	var dstIP net.IP
	if nl.af == syscall.AF_INET {
		dstIP = net.IP(nl.rdaddr[:4])
	} else {
		dstIP = net.IP(nl.rdaddr[:16])
	}
	if isLocalHost(dstIP.String()) {
		return "", fmt.Errorf("original destination is localhost")
	}

	dstPort := int(binary.BigEndian.Uint16(nl.rdxport[:2]))

	return fmt.Sprintf("%s:%d", dstIP.String(), dstPort), nil
}
