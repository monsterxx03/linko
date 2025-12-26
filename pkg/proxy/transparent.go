package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// TransparentProxy represents a transparent proxy
type TransparentProxy struct {
	listenAddr string
	targetAddr string
	server     net.Listener
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	stats      *ProxyStats
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
func NewTransparentProxy(listenAddr, targetAddr string) *TransparentProxy {
	ctx, cancel := context.WithCancel(context.Background())
	return &TransparentProxy{
		listenAddr: listenAddr,
		targetAddr: targetAddr,
		ctx:        ctx,
		cancel:     cancel,
		stats: &ProxyStats{
			startTime: time.Now(),
		},
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

	fmt.Printf("Transparent proxy listening on %s -> %s\n", p.listenAddr, p.targetAddr)
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

	// Connect to target
	targetConn, err := net.Dial("tcp", p.targetAddr)
	if err != nil {
		fmt.Printf("Failed to connect to target %s: %v\n", p.targetAddr, err)
		return
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

// GetStats returns proxy statistics
func (p *TransparentProxy) GetStats() map[string]interface{} {
	p.stats.mu.RLock()
	defer p.stats.mu.RUnlock()

	uptime := time.Since(p.stats.startTime).Seconds()

	stats := make(map[string]interface{})
	stats["listen_addr"] = p.listenAddr
	stats["target_addr"] = p.targetAddr
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

// GetTargetAddr returns the target address
func (p *TransparentProxy) GetTargetAddr() string {
	return p.targetAddr
}