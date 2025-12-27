package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/monsterxx03/linko/pkg/ipdb"
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
	geoIP        *ipdb.GeoIPManager
	enableDirect bool // Enable direct connection when upstream is disabled
}

// ProxyStats tracks proxy statistics
type ProxyStats struct {
	totalConnections  uint64
	activeConnections uint64
	bytesTransferred  uint64
	startTime         time.Time
	mu                sync.RWMutex
}

// NewTransparentProxy creates a new transparent proxy
func NewTransparentProxy(listenAddr string, upstream *UpstreamClient, geoIP *ipdb.GeoIPManager) *TransparentProxy {
	ctx, cancel := context.WithCancel(context.Background())
	return &TransparentProxy{
		listenAddr: listenAddr,
		ctx:        ctx,
		cancel:     cancel,
		stats: &ProxyStats{
			startTime: time.Now(),
		},
		upstream:     upstream,
		geoIP:        geoIP,
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

	// Determine whether to connect directly or via upstream proxy based on GeoIP
	var targetConn net.Conn
	shouldUseUpstream := false

	// Check if GeoIP is available and upstream is enabled
	if p.upstream.IsEnabled() && p.geoIP != nil && p.geoIP.IsInitialized() {
		// Check if target IP is domestic
		isDomestic, err := p.geoIP.IsDomesticIP(targetHost)
		if err != nil {
			fmt.Printf("Warning: Failed to lookup GeoIP for %s: %v, falling back to upstream proxy\n", targetHost, err)
			shouldUseUpstream = true
		} else if isDomestic {
			fmt.Printf("Direct connection for domestic IP: %s\n", targetHost)
			shouldUseUpstream = false
		} else {
			fmt.Printf("Using upstream proxy for foreign IP: %s\n", targetHost)
			shouldUseUpstream = true
		}
	} else if p.upstream.IsEnabled() {
		// Upstream is enabled but GeoIP is not available, use upstream
		shouldUseUpstream = true
	}

	// Connect to target
	if shouldUseUpstream {
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

// isLocalHost checks if an IP address is localhost
func isLocalHost(ip string) bool {
	return ip == "127.0.0.1" || ip == "::1" || ip == "localhost"
}

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
