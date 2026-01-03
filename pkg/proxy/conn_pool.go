package proxy

import (
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Connection represents a managed connection
type Connection struct {
	net.Conn
	createdAt  time.Time
	lastActive time.Time
	id         uint64
}

// ConnectionPool manages a pool of connections
type ConnectionPool struct {
	connections map[uint64]*Connection
	maxSize     int
	maxIdle     time.Duration
	count       atomic.Uint64
	mu          sync.RWMutex
	cond        *sync.Cond
}

// NewConnectionPool creates a new connection pool
func NewConnectionPool(maxSize int, maxIdle time.Duration) *ConnectionPool {
	pool := &ConnectionPool{
		connections: make(map[uint64]*Connection),
		maxSize:     maxSize,
		maxIdle:     maxIdle,
	}
	pool.cond = sync.NewCond(&pool.mu)
	return pool
}

// Get retrieves a connection from the pool
func (p *ConnectionPool) Get(conn net.Conn) *Connection {
	p.mu.Lock()
	defer p.mu.Unlock()

	id := p.count.Add(1)
	c := &Connection{
		Conn:       conn,
		createdAt:  time.Now(),
		lastActive: time.Now(),
		id:         id,
	}

	p.connections[id] = c
	return c
}

// Put returns a connection to the pool
func (p *ConnectionPool) Put(conn *Connection) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.connections, conn.id)
}

// Remove removes a connection from the pool
func (p *ConnectionPool) Remove(conn *Connection) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.connections, conn.id)
}

// Close closes all connections in the pool
func (p *ConnectionPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, conn := range p.connections {
		conn.Conn.Close()
	}
	p.connections = make(map[uint64]*Connection)
}

// Len returns the number of connections in the pool
func (p *ConnectionPool) Len() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.connections)
}

// Cleanup removes idle connections
func (p *ConnectionPool) Cleanup() {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	idsToRemove := make([]uint64, 0)

	for id, conn := range p.connections {
		if now.Sub(conn.lastActive) > p.maxIdle {
			idsToRemove = append(idsToRemove, id)
			conn.Conn.Close()
		}
	}

	for _, id := range idsToRemove {
		delete(p.connections, id)
	}
}

// ConnectionStats returns connection statistics
func (p *ConnectionPool) ConnectionStats() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["active_connections"] = len(p.connections)
	stats["max_size"] = p.maxSize
	stats["max_idle"] = p.maxIdle.String()
	stats["total_created"] = p.count.Load()

	return stats
}

// RelayConnections relays data between two connections
func RelayConnections(client, target net.Conn) error {
	errChan := make(chan error, 2)

	go func() {
		_, err := io.Copy(target, client)
		errChan <- err
	}()

	go func() {
		_, err := io.Copy(client, target)
		errChan <- err
	}()

	// Wait for first error or successful transfer
	var err error
	for i := 0; i < 2; i++ {
		if e := <-errChan; e != nil && err == nil {
			err = e
		}
	}

	return err
}
