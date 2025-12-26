package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/monsterxx03/linko/pkg/config"
)

// UpstreamClient represents an upstream proxy client
type UpstreamClient struct {
	config config.UpstreamConfig
	client net.Conn
	ctx    context.Context
}

// NewUpstreamClient creates a new upstream client
func NewUpstreamClient(config config.UpstreamConfig) *UpstreamClient {
	return &UpstreamClient{
		config: config,
		ctx:    context.Background(),
	}
}

// Connect establishes a connection to target through upstream proxy
func (u *UpstreamClient) Connect(targetHost string, targetPort int) (net.Conn, error) {
	if !u.config.Enable {
		// Direct connection if upstream is disabled
		return net.Dial("tcp", fmt.Sprintf("%s:%d", targetHost, targetPort))
	}

	switch u.config.Type {
	case "socks5":
		return u.connectSOCKS5(targetHost, targetPort)
	case "http":
		return u.connectHTTP(targetHost, targetPort)
	default:
		return nil, fmt.Errorf("unsupported upstream proxy type: %s", u.config.Type)
	}
}

// connectSOCKS5 connects through SOCKS5 upstream proxy
func (u *UpstreamClient) connectSOCKS5(targetHost string, targetPort int) (net.Conn, error) {
	// Connect to SOCKS5 proxy
	conn, err := net.Dial("tcp", u.config.Addr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to SOCKS5 proxy: %w", err)
	}

	// SOCKS5 handshake
	if err := u.socks5Handshake(conn, targetHost, targetPort); err != nil {
		conn.Close()
		return nil, fmt.Errorf("SOCKS5 handshake failed: %w", err)
	}

	return conn, nil
}

// socks5Handshake performs SOCKS5 authentication and connection
func (u *UpstreamClient) socks5Handshake(conn net.Conn, targetHost string, targetPort int) error {
	// Send authentication request (no authentication)
	authReq := []byte{0x05, 0x01, 0x00}
	if _, err := conn.Write(authReq); err != nil {
		return err
	}

	// Read authentication response
	authResp := make([]byte, 2)
	if _, err := io.ReadFull(conn, authResp); err != nil {
		return err
	}
	if authResp[0] != 0x05 || authResp[1] != 0x00 {
		return fmt.Errorf("SOCKS5 authentication failed")
	}

	// Build CONNECT request
	var addr []byte
	if ip := net.ParseIP(targetHost); ip != nil {
		if ip.To4() != nil {
			// IPv4
			addr = append(addr, 0x01)
			addr = append(addr, ip.To4()...)
		} else {
			// IPv6
			addr = append(addr, 0x04)
			addr = append(addr, ip.To16()...)
		}
	} else {
		// Domain name
		addr = append(addr, 0x03)
		addr = append(addr, byte(len(targetHost)))
		addr = append(addr, []byte(targetHost)...)
	}

	// Add port
	portBytes := []byte{byte(targetPort >> 8), byte(targetPort & 0xFF)}
	addr = append(addr, portBytes...)

	// Send CONNECT request
	connectReq := append([]byte{0x05, 0x01, 0x00}, addr...)
	if _, err := conn.Write(connectReq); err != nil {
		return err
	}

	// Read CONNECT response
	connectResp := make([]byte, 10)
	if _, err := io.ReadFull(conn, connectResp); err != nil {
		return err
	}
	if connectResp[0] != 0x05 || connectResp[1] != 0x00 {
		return fmt.Errorf("SOCKS5 CONNECT failed: %d", connectResp[1])
	}

	return nil
}

// connectHTTP connects through HTTP upstream proxy
func (u *UpstreamClient) connectHTTP(targetHost string, targetPort int) (net.Conn, error) {
	// Connect to HTTP proxy
	conn, err := net.Dial("tcp", u.config.Addr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to HTTP proxy: %w", err)
	}

	// Send CONNECT request
	connectReq := fmt.Sprintf("CONNECT %s:%d HTTP/1.1\r\nHost: %s:%d\r\n\r\n", targetHost, targetPort, targetHost, targetPort)
	if _, err := conn.Write([]byte(connectReq)); err != nil {
		conn.Close()
		return nil, err
	}

	// Read CONNECT response
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		conn.Close()
		return nil, err
	}
	if resp.StatusCode != 200 {
		conn.Close()
		return nil, fmt.Errorf("HTTP CONNECT failed: %d", resp.StatusCode)
	}

	return conn, nil
}

// Close closes the upstream client
func (u *UpstreamClient) Close() error {
	if u.client != nil {
		return u.client.Close()
	}
	return nil
}

// GetConfig returns the upstream configuration
func (u *UpstreamClient) GetConfig() config.UpstreamConfig {
	return u.config
}

// IsEnabled returns whether upstream proxy is enabled
func (u *UpstreamClient) IsEnabled() bool {
	return u.config.Enable
}