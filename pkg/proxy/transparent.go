package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"syscall"
	"sync"
	"time"
)

// TransparentProxy represents a transparent proxy
type TransparentProxy struct {
	listenAddr   string
	server       net.Listener
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	stats        *ProxyStats
	upstream     *UpstreamClient
	enableDirect bool // Enable direct connection when upstream is disabled
}

// ProxyStats tracks proxy statistics
type ProxyStats struct {
	totalConnections   uint64
	activeConnections  uint64
	bytesTransferred   uint64
	startTime          time.Time
	mu                 sync.RWMutex
}

// NewTransparentProxy creates a new transparent proxy
func NewTransparentProxy(listenAddr string, upstream *UpstreamClient) *TransparentProxy {
	ctx, cancel := context.WithCancel(context.Background())
	return &TransparentProxy{
		listenAddr:   listenAddr,
		ctx:          ctx,
		cancel:       cancel,
		stats: &ProxyStats{
			startTime: time.Now(),
		},
		upstream:     upstream,
		enableDirect: !upstream.IsEnabled(),
	}
}

// Start starts the transparent proxy
func (p *TransparentProxy) Start() error {
	listener, err := net.Listen("tcp", p.listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", p.listenAddr, err)
	}

	p.server = listener

	p.wg.Add(1)
	go p.acceptLoop()

	if p.upstream.IsEnabled() {
		fmt.Printf("Transparent proxy listening on %s (upstream: %s %s)\n", p.listenAddr, p.upstream.GetConfig().Type, p.upstream.GetConfig().Addr)
	} else {
		fmt.Printf("Transparent proxy listening on %s (direct mode)\n", p.listenAddr)
	}
	return nil
}

// Stop stops the transparent proxy
func (p *TransparentProxy) Stop() error {
	p.cancel()

	if p.server != nil {
		p.server.Close()
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		p.wg.Wait()
	}()

	select {
	case <-done:
		fmt.Println("Transparent proxy stopped")
		return nil
	case <-time.After(10 * time.Second):
		return fmt.Errorf("timeout waiting for proxy to stop")
	}
}

// acceptLoop accepts incoming connections
func (p *TransparentProxy) acceptLoop() {
	defer p.wg.Done()

	for {
		conn, err := p.server.Accept()
		if err != nil {
			select {
			case <-p.ctx.Done():
				return
			default:
				fmt.Printf("Accept error: %v\n", err)
				continue
			}
		}

		p.wg.Add(1)
		go p.handleConnection(conn)
	}
}

// handleConnection handles a single connection
func (p *TransparentProxy) handleConnection(clientConn net.Conn) {
	defer p.wg.Done()
	defer clientConn.Close()

	// Update stats
	p.stats.mu.Lock()
	p.stats.totalConnections++
	p.stats.activeConnections++
	p.stats.mu.Unlock()

	defer func() {
		p.stats.mu.Lock()
		p.stats.activeConnections--
		p.stats.mu.Unlock()
	}()

	// Get original destination from connection
	originalDst, err := p.getOriginalDestination(clientConn)
	if err != nil {
		fmt.Printf("Failed to get original destination: %v\n", err)
		return
	}

	fmt.Printf("Proxying connection from %s to %s\n", clientConn.RemoteAddr(), originalDst)

	// Parse destination address
	targetHost, targetPortStr, err := net.SplitHostPort(originalDst)
	if err != nil {
		fmt.Printf("Invalid destination address %s: %v\n", originalDst, err)
		return
	}

	targetPort, err := strconv.Atoi(targetPortStr)
	if err != nil {
		fmt.Printf("Invalid port %s: %v\n", targetPortStr, err)
		return
	}

	// Connect to target through upstream proxy or directly
	var targetConn net.Conn
	if p.upstream.IsEnabled() {
		// Connect through upstream proxy
		targetConn, err = p.upstream.Connect(targetHost, targetPort)
		if err != nil {
			fmt.Printf("Failed to connect to %s via upstream proxy: %v\n", originalDst, err)
			return
		}
	} else {
		// Connect directly
		targetConn, err = net.Dial("tcp", originalDst)
		if err != nil {
			fmt.Printf("Failed to connect to %s: %v\n", originalDst, err)
			return
		}
	}
	defer targetConn.Close()

	// Relay data
	bytes, err := p.relayBidirectional(clientConn, targetConn)

	// Update stats
	if err == nil {
		p.stats.mu.Lock()
		p.stats.bytesTransferred += uint64(bytes)
		p.stats.mu.Unlock()
	}
}

// relayBidirectional relays data between client and target
func (p *TransparentProxy) relayBidirectional(client, target net.Conn) (int64, error) {
	errChan := make(chan error, 2)
	bytesChan := make(chan int64, 2)

	go func() {
		n, err := io.Copy(target, client)
		bytesChan <- n
		errChan <- err
	}()

	go func() {
		n, err := io.Copy(client, target)
		bytesChan <- n
		errChan <- err
	}()

	var totalBytes int64
	var err error

	for i := 0; i < 2; i++ {
		select {
		case n := <-bytesChan:
			totalBytes += n
		case e := <-errChan:
			if err == nil {
				err = e
			}
		}
	}

	return totalBytes, err
}

// getOriginalDestination gets the original destination address from a redirected connection
func (p *TransparentProxy) getOriginalDestination(conn net.Conn) (string, error) {
	// Try to get SO_ORIGINAL_DST on Linux
	if originalDst, err := p.getSOOriginalDst(conn); err == nil {
		return originalDst, nil
	}

	// Fallback: try to parse from connection remote addr
	// This works when using REDIRECT target but not all cases
	addr := conn.RemoteAddr().String()
	if host, port, err := net.SplitHostPort(addr); err == nil {
		// If it's a local address, we can't determine the original destination
		if !isLocalHost(host) {
			return fmt.Sprintf("%s:%s", host, port), nil
		}
	}

	return "", fmt.Errorf("unable to determine original destination")
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

	// Get SO_ORIGINAL_DST address
	addr, err := syscall.GetsockoptIPv6Mreq(int(file.Fd()), syscall.IPPROTO_IP, SO_ORIGINAL_DST)
	if err != nil {
		return "", err
	}

	// Parse the address
	ip := net.IP(addr.Multiaddr[4:8]).String()
	port := int(addr.Multiaddr[2])<<8 + int(addr.Multiaddr[3])

	return fmt.Sprintf("%s:%d", ip, port), nil
}

// isLocalHost checks if an IP address is localhost
func isLocalHost(ip string) bool {
	return ip == "127.0.0.1" || ip == "::1" || ip == "localhost"
}

// SO_ORIGINAL_dst is the socket option to get original destination
const SO_ORIGINAL_DST = 80 // Linux socket option number

// GetStats returns proxy statistics
func (p *TransparentProxy) GetStats() map[string]interface{} {
	p.stats.mu.RLock()
	defer p.stats.mu.RUnlock()

	uptime := time.Since(p.stats.startTime).Seconds()

	stats := make(map[string]interface{})
	stats["listen_addr"] = p.listenAddr
	stats["upstream_enabled"] = p.upstream.IsEnabled()
	if p.upstream.IsEnabled() {
		stats["upstream_type"] = p.upstream.GetConfig().Type
		stats["upstream_addr"] = p.upstream.GetConfig().Addr
	}
	stats["total_connections"] = p.stats.totalConnections
	stats["active_connections"] = p.stats.activeConnections
	stats["bytes_transferred"] = p.stats.bytesTransferred
	stats["bytes_transferred_mb"] = float64(p.stats.bytesTransferred) / (1024 * 1024)
	stats["uptime_seconds"] = uptime
	stats["uptime_hours"] = uptime / 3600

	return stats
}

// IsRunning checks if the proxy is running
func (p *TransparentProxy) IsRunning() bool {
	return p.ctx.Err() == nil
}

// GetListenAddr returns the listen address
func (p *TransparentProxy) GetListenAddr() string {
	return p.listenAddr
}

// GetTargetAddr returns the listen address (no fixed target in transparent mode)
func (p *TransparentProxy) GetTargetAddr() string {
	return "dynamic (from original destination)"
}