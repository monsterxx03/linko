package proxy

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
)

// SOCKS5 command types
const (
	CmdConnect      = 0x01
	CmdBind         = 0x02
	CmdUDPAssociate = 0x03
)

// SOCKS5 address types
const (
	AddrTypeIPv4   = 0x01
	AddrTypeDomain = 0x03
	AddrTypeIPv6   = 0x04
)

// SOCKS5 response codes
const (
	RepSucceeded           = 0x00
	RepGeneralFailure      = 0x01
	RepConnectionNotAllowed = 0x02
	RepNetworkUnreachable  = 0x03
	RepHostUnreachable     = 0x04
	RepConnectionRefused   = 0x05
	RepTTLExpired          = 0x06
	RepCommandNotSupported = 0x07
	RepAddrTypeNotSupported = 0x08
)

// SOCKS5Auth represents SOCKS5 authentication
type SOCKS5Auth struct {
	Methods []uint8
}

// SOCKS5Request represents a SOCKS5 request
type SOCKS5Request struct {
	Version   uint8
	Command   uint8
	Reserved  uint8
	AddressType uint8
	Address  []byte
	Port     uint16
}

// SOCKS5Response represents a SOCKS5 response
type SOCKS5Response struct {
	Version   uint8
	Reply     uint8
	Reserved  uint8
	AddressType uint8
	Address  []byte
	Port     uint16
}

// NewSOCKS5Auth creates a new SOCKS5 authentication
func NewSOCKS5Auth() *SOCKS5Auth {
	return &SOCKS5Auth{
		Methods: []uint8{0x00}, // No authentication
	}
}

// ReadAuthRequest reads the authentication request from client
func (a *SOCKS5Auth) ReadAuthRequest(r io.Reader) error {
	// Read version and number of methods
	var version, nmethods uint8
	if err := binary.Read(r, binary.BigEndian, &version); err != nil {
		return fmt.Errorf("failed to read version: %w", err)
	}
	if version != 0x05 {
		return errors.New("unsupported SOCKS version")
	}

	if err := binary.Read(r, binary.BigEndian, &nmethods); err != nil {
		return fmt.Errorf("failed to read methods count: %w", err)
	}

	// Read methods
	a.Methods = make([]uint8, nmethods)
	if _, err := io.ReadFull(r, a.Methods); err != nil {
		return fmt.Errorf("failed to read methods: %w", err)
	}

	return nil
}

// WriteAuthResponse writes the authentication response to client
func (a *SOCKS5Auth) WriteAuthResponse(w io.Writer, method uint8) error {
	response := struct {
		Version uint8
		Method  uint8
	}{
		Version: 0x05,
		Method:  method,
	}

	return binary.Write(w, binary.BigEndian, response)
}

// ReadRequest reads a SOCKS5 request from client
func ReadSOCKS5Request(r io.Reader) (*SOCKS5Request, error) {
	req := &SOCKS5Request{}

	// Read version, command, and reserved
	if err := binary.Read(r, binary.BigEndian, &req.Version); err != nil {
		return nil, fmt.Errorf("failed to read version: %w", err)
	}
	if req.Version != 0x05 {
		return nil, errors.New("unsupported SOCKS version")
	}

	if err := binary.Read(r, binary.BigEndian, &req.Command); err != nil {
		return nil, fmt.Errorf("failed to read command: %w", err)
	}

	if err := binary.Read(r, binary.BigEndian, &req.Reserved); err != nil {
		return nil, fmt.Errorf("failed to read reserved: %w", err)
	}
	if req.Reserved != 0x00 {
		return nil, errors.New("invalid reserved field")
	}

	if err := binary.Read(r, binary.BigEndian, &req.AddressType); err != nil {
		return nil, fmt.Errorf("failed to read address type: %w", err)
	}

	// Read address based on type
	switch req.AddressType {
	case AddrTypeIPv4:
		req.Address = make([]byte, 4)
		if _, err := io.ReadFull(r, req.Address); err != nil {
			return nil, fmt.Errorf("failed to read IPv4 address: %w", err)
		}
	case AddrTypeDomain:
		var addrLen uint8
		if err := binary.Read(r, binary.BigEndian, &addrLen); err != nil {
			return nil, fmt.Errorf("failed to read domain length: %w", err)
		}
		req.Address = make([]byte, addrLen)
		if _, err := io.ReadFull(r, req.Address); err != nil {
			return nil, fmt.Errorf("failed to read domain: %w", err)
		}
	case AddrTypeIPv6:
		req.Address = make([]byte, 16)
		if _, err := io.ReadFull(r, req.Address); err != nil {
			return nil, fmt.Errorf("failed to read IPv6 address: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported address type: %d", req.AddressType)
	}

	// Read port
	if err := binary.Read(r, binary.BigEndian, &req.Port); err != nil {
		return nil, fmt.Errorf("failed to read port: %w", err)
	}

	return req, nil
}

// WriteResponse writes a SOCKS5 response to client
func WriteSOCKS5Response(w io.Writer, reply uint8, bindAddr net.Addr) error {
	resp := &SOCKS5Response{
		Version:  0x05,
		Reply:    reply,
		Reserved: 0x00,
	}

	// Set bind address
	if bindAddr != nil {
		host, portStr, _ := net.SplitHostPort(bindAddr.String())
		port, _ := strconv.Atoi(portStr)
		resp.Port = uint16(port)

		// Try to parse as IP
		if ip := net.ParseIP(host); ip != nil {
			if ip.To4() != nil {
				resp.AddressType = AddrTypeIPv4
				resp.Address = ip.To4()
			} else {
				resp.AddressType = AddrTypeIPv6
				resp.Address = ip.To16()
			}
		} else {
			// Domain name
			resp.AddressType = AddrTypeDomain
			resp.Address = []byte(host)
		}
	}

	// Write response manually (binary.Write doesn't work with variable-length fields)
	if err := binary.Write(w, binary.BigEndian, resp.Version); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, resp.Reply); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, resp.Reserved); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, resp.AddressType); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, resp.Address); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, resp.Port); err != nil {
		return err
	}
	return nil
}

// GetTargetAddress returns the target address from the request
func (r *SOCKS5Request) GetTargetAddress() (string, error) {
	var host string

	switch r.AddressType {
	case AddrTypeIPv4:
		host = net.IP(r.Address).String()
	case AddrTypeDomain:
		host = string(r.Address)
	case AddrTypeIPv6:
		host = net.IP(r.Address).String()
	default:
		return "", fmt.Errorf("unsupported address type: %d", r.AddressType)
	}

	return fmt.Sprintf("%s:%d", host, r.Port), nil
}

// HandleSOCKS5Connection handles a SOCKS5 connection
func HandleSOCKS5Connection(clientConn net.Conn, handler ConnectionHandler) error {
	defer clientConn.Close()

	// Step 1: Authentication
	auth := NewSOCKS5Auth()
	if err := auth.ReadAuthRequest(clientConn); err != nil {
		return fmt.Errorf("auth request failed: %w", err)
	}

	// Select authentication method (no authentication for now)
	authMethod := uint8(0x00) // No authentication
	if err := auth.WriteAuthResponse(clientConn, authMethod); err != nil {
		return fmt.Errorf("auth response failed: %w", err)
	}

	// Step 2: Request
	req, err := ReadSOCKS5Request(clientConn)
	if err != nil {
		return fmt.Errorf("request read failed: %w", err)
	}

	// Step 3: Handle the request
	targetAddr, err := req.GetTargetAddress()
	if err != nil {
		WriteSOCKS5Response(clientConn, RepAddrTypeNotSupported, nil)
		return fmt.Errorf("get target address failed: %w", err)
	}

	// Only support CONNECT command for now
	if req.Command != CmdConnect {
		WriteSOCKS5Response(clientConn, RepCommandNotSupported, nil)
		return fmt.Errorf("unsupported command: %d", req.Command)
	}

	// Connect to target
	targetConn, err := net.Dial("tcp", targetAddr)
	if err != nil {
		// Determine error code
		var reply uint8 = RepGeneralFailure
		if isConnectionRefused(err) {
			reply = RepConnectionRefused
		} else if isNetworkUnreachable(err) {
			reply = RepNetworkUnreachable
		} else if isHostUnreachable(err) {
			reply = RepHostUnreachable
		}
		WriteSOCKS5Response(clientConn, reply, nil)
		return fmt.Errorf("dial failed: %w", err)
	}
	defer targetConn.Close()

	// Send success response
	if err := WriteSOCKS5Response(clientConn, RepSucceeded, targetConn.LocalAddr()); err != nil {
		return fmt.Errorf("response write failed: %w", err)
	}

	// Step 4: Relay data
	if handler != nil {
		return handler.Handle(clientConn, targetConn)
	}

	// Default: simple bidirectional relay
	return RelayConnections(clientConn, targetConn)
}

// isConnectionRefused checks if error is connection refused
func isConnectionRefused(err error) bool {
	return err != nil && (
		bytes.Contains([]byte(err.Error()), []byte("connection refused")) ||
		bytes.Contains([]byte(err.Error()), []byte("refused")))
}

// isNetworkUnreachable checks if error is network unreachable
func isNetworkUnreachable(err error) bool {
	return err != nil && (
		bytes.Contains([]byte(err.Error()), []byte("network unreachable")) ||
		bytes.Contains([]byte(err.Error()), []byte("no route to host")))
}

// isHostUnreachable checks if error is host unreachable
func isHostUnreachable(err error) bool {
	return err != nil && (
		bytes.Contains([]byte(err.Error()), []byte("host unreachable")) ||
		bytes.Contains([]byte(err.Error()), []byte("timeout")))
}

// ConnectionHandler handles the connection after SOCKS5 handshake
type ConnectionHandler interface {
	Handle(client, target net.Conn) error
}