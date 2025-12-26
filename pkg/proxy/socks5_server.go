package proxy

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"
)

// SOCKS5Server represents a SOCKS5 proxy server
type SOCKS5Server struct {
	listenAddr string
	server     net.Listener
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	stats      *ProxyStats
	handler    ConnectionHandler
}

// NewSOCKS5Server creates a new SOCKS5 server
func NewSOCKS5Server(listenAddr string, handler ConnectionHandler) *SOCKS5Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &SOCKS5Server{
		listenAddr: listenAddr,
		ctx:        ctx,
		cancel:     cancel,
		stats: &ProxyStats{
			startTime: time.Now(),
		},
		handler: handler,
	}
}

// Start starts the SOCKS5 server
func (s *SOCKS5Server) Start() error {
	listener, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.listenAddr, err)
	}

	s.server = listener

	s.wg.Add(1)
	go s.acceptLoop()

	fmt.Printf("SOCKS5 proxy listening on %s\n", s.listenAddr)
	return nil
}

// Stop stops the SOCKS5 server
func (s *SOCKS5Server) Stop() error {
	s.cancel()

	if s.server != nil {
		s.server.Close()
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.wg.Wait()
	}()

	select {
	case <-done:
		fmt.Println("SOCKS5 proxy stopped")
		return nil
	case <-time.After(10 * time.Second):
		return fmt.Errorf("timeout waiting for proxy to stop")
	}
}

// acceptLoop accepts incoming SOCKS5 connections
func (s *SOCKS5Server) acceptLoop() {
	defer s.wg.Done()

	for {
		conn, err := s.server.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return
			default:
				fmt.Printf("SOCKS5 accept error: %v\n", err)
				continue
			}
		}

		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

// handleConnection handles a single SOCKS5 connection
func (s *SOCKS5Server) handleConnection(conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close()

	// Update stats
	s.stats.mu.Lock()
	s.stats.totalConnections++
	s.stats.activeConnections++
	s.stats.mu.Unlock()

	defer func() {
		s.stats.mu.Lock()
		s.stats.activeConnections--
		s.stats.mu.Unlock()
	}()

	// Handle SOCKS5 connection
	if err := HandleSOCKS5Connection(conn, s.handler); err != nil {
		fmt.Printf("SOCKS5 handler error: %v\n", err)
	}
}

// GetStats returns server statistics
func (s *SOCKS5Server) GetStats() map[string]interface{} {
	s.stats.mu.RLock()
	defer s.stats.mu.RUnlock()

	uptime := time.Since(s.stats.startTime).Seconds()

	stats := make(map[string]interface{})
	stats["listen_addr"] = s.listenAddr
	stats["total_connections"] = s.stats.totalConnections
	stats["active_connections"] = s.stats.activeConnections
	stats["bytes_transferred"] = s.stats.bytesTransferred
	stats["bytes_transferred_mb"] = float64(s.stats.bytesTransferred) / (1024 * 1024)
	stats["uptime_seconds"] = uptime
	stats["uptime_hours"] = uptime / 3600

	return stats
}

// IsRunning checks if the server is running
func (s *SOCKS5Server) IsRunning() bool {
	return s.ctx.Err() == nil
}

// GetListenAddr returns the listen address
func (s *SOCKS5Server) GetListenAddr() string {
	return s.listenAddr
}