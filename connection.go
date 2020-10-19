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
	unusable bool

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
	c.inUse = false
	c.closed = true
	_ = c.Conn.Close()
	return
}

// MarkUnusable marks the connection not usable any more, to let the pool close it instead of returning it to pool.
func (c *Conn) MarkUnusable() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.unusable = true
}

// Release releases the connection and puts it back to the connection pool
func (c *Conn) Release() {
	c.p.putConn(c, nil)
}
