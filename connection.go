package pool

import (
	"errors"
	"net"
	"sync"
	"time"
)

var errBadConn = errors.New("bad Conn")

// Conn wraps a net.Conn
type Conn struct {
	p         *Pool
	createdAt time.Time

	mu sync.Mutex // guards following
	net.Conn
	closed bool

	// guarded by pool's mutex
	inUse bool
}

func (c *Conn) expired(timeout time.Duration) bool {
	if timeout <= 0 {
		return false
	}
	return c.createdAt.Add(timeout).Before(time.Now())
}

func (c *Conn) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	_ = c.Conn.Close()
	return
}

// Close closes the underlying connection.
// This connection is not put back into the connection pool.
func (c *Conn) Close() {
	c.close()
	c.p.putConn(c, nil)
}

// Release releases the connection and puts it back to the connection pool
func (c *Conn) Release() {
	c.p.putConn(c, nil)
}
