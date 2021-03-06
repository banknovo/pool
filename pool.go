package pool

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// ErrPoolClosed is returned when trying to get connection from a closed Pool
var ErrPoolClosed = errors.New("pool is closed")

// ErrTimedOut is returned when Pool has Options.MaxConnections connections open and no connection was released in Options.GetTimeout
var ErrTimedOut = errors.New("timed out in obtaining a connection")

// Factory is a function to create new connections
type Factory func() (net.Conn, error)

// connReuseStrategy determines how Pool.conn returns database connections.
type connReuseStrategy uint8

const (
	// alwaysNewConn forces a new connection to the database.
	alwaysNewConn connReuseStrategy = iota
	// cachedOrNewConn returns a cached connection, if available, else waits
	// for one to become available (if Pool.maxOpen has been reached) or
	// creates a new database connection.
	cachedOrNewConn
)

// connRequest represents one request for a new Conn
// When there are no idle connections available, Pool.conn will create
// a new connRequest and put it on the Pool.connRequests list.
type connRequest struct {
	conn *Conn
	err  error
}

// Pool is a connection pool of underlying connections.
// It is safe to be used by multiple goroutines
type Pool struct {
	waitDuration int64 // Total time waited for new connections.

	// factory is a function to create new connections
	factory Factory

	mu           sync.Mutex
	freeConn     []*Conn
	connRequests map[uint64]chan connRequest
	nextRequest  uint64 // Next key to use in connRequests
	numOpen      int    // number of opened and pending open connections
	maxOpen      int    // max number of open connections
	maxIdle      int    // max number of idle connections

	cleanerCh chan struct{} // struct to signal cleaning of free connections

	maxLifeTime time.Duration // maximum amount of time a Conn may be reused
	maxWaitTime time.Duration // maximum amount of time to wait for a Conn before throwing error
	cleanTime   time.Duration // run the cleaner at this interval to check for expired connections
	closed      bool

	// used for stats
	waitCount         uint64 // Total number of connections waited for.
	maxIdleClosed     uint64 // Total number of connections closed due to idle.
	maxLifetimeClosed uint64 // Total number of connections closed due to max free limit.
}

// New returns a new pool which needs to be closed by calling Pool.Close.
func New(opts *Options) (*Pool, error) {

	// validate the options
	err := opts.validate()
	if err != nil {
		return nil, err
	}

	p := &Pool{
		factory:      opts.Factory,
		connRequests: make(map[uint64]chan connRequest),
		maxOpen:      opts.MaxConnections,
		maxIdle:      opts.MaxIdleConnections,
		maxLifeTime:  opts.ConnLifeTime,
		maxWaitTime:  opts.GetTimeout,
		cleanTime:    opts.ConnCleanTime,
	}

	return p, nil
}

// Get returns a single Conn by either opening a new Conn
// or returning an existing Conn from the Conn pool.
//
// Every Conn must be returned to the database pool after use by
// calling Conn.Release or Conn.Close if connection is to be closed.
func (p *Pool) Get() (*Conn, error) {
	var conn *Conn
	var err error

	// try to get a cached connection twice, if it fails, open a new connection and return
	for i := 0; i < 2; i++ {
		conn, err = p.conn(cachedOrNewConn)
		if err != errBadConn {
			break
		}
	}
	if err == errBadConn {
		conn, err = p.conn(alwaysNewConn)
	}
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// Close closes the pool and all underlying connections.
// A pool cannot be used after it is closed.
func (p *Pool) Close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	if p.cleanerCh != nil {
		close(p.cleanerCh)
	}
	for _, c := range p.freeConn {
		p.numOpen--
		c.close()
	}
	p.freeConn = nil
	p.closed = true
	for _, req := range p.connRequests {
		close(req)
	}
	p.mu.Unlock()
}

func (p *Pool) conn(strategy connReuseStrategy) (*Conn, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, ErrPoolClosed
	}
	lifetime := p.maxLifeTime

	// check if there is a free Conn
	numFree := len(p.freeConn)
	if strategy == cachedOrNewConn && numFree > 0 {
		conn := p.freeConn[0]
		copy(p.freeConn, p.freeConn[1:])
		p.freeConn = p.freeConn[:numFree-1]
		conn.inUse = true
		if conn.expired(lifetime) {
			p.maxLifetimeClosed++
			p.numOpen--
			p.mu.Unlock()
			conn.close()
			return nil, errBadConn
		}
		p.mu.Unlock()
		return conn, nil
	}

	// No free connections. Try to open a new one or wait for one to get free.
	if p.maxOpen > 0 && p.numOpen >= p.maxOpen {
		req := make(chan connRequest, 1)
		reqKey := p.nextRequestKeyLocked()
		p.connRequests[reqKey] = req
		p.waitCount++
		p.mu.Unlock()

		waitStart := time.Now()

		select {
		case ret, ok := <-req:
			atomic.AddInt64(&p.waitDuration, int64(time.Since(waitStart)))
			if !ok {
				return nil, ErrPoolClosed
			}
			if ret.err != nil {
				return nil, ret.err
			}
			if ret.conn.expired(lifetime) {
				ret.conn.close()
				return nil, errBadConn
			}
			return ret.conn, ret.err
		case <-time.After(p.maxWaitTime):
			atomic.AddInt64(&p.waitDuration, int64(time.Since(waitStart)))

			// Remove the connRequest and ensure no value has been sent
			// on it after removing.
			p.mu.Lock()
			delete(p.connRequests, reqKey)
			p.mu.Unlock()

			select {
			default:
			case ret, ok := <-req:
				if ok && ret.conn != nil {
					p.putConn(ret.conn, ret.err)
				}
			}
			return nil, ErrTimedOut
		}
	}

	// no free connections, and we are allowed to create new ones
	p.numOpen++ // optimistically
	p.mu.Unlock()
	c, err := p.factory()
	if err != nil {
		p.mu.Lock()
		p.numOpen-- // correct for earlier optimism
		p.mu.Unlock()
		return nil, err
	}
	return &Conn{p: p, createdAt: time.Now(), Conn: c, inUse: true}, nil
}

// nextRequestKeyLocked returns the next Conn request key.
// It is assumed that nextRequest will not overflow.
func (p *Pool) nextRequestKeyLocked() uint64 {
	next := p.nextRequest
	p.nextRequest++
	return next
}

// putConnLocked will satisfy a connRequest if there is one, or it will
// return the *Conn to the freeConn list if err == nil and the idle
// Conn limit will not be exceeded.
// If err != nil, the value of c is ignored.
// If err == nil, then c must not equal nil.
// If a connRequest was fulfilled or the *Conn was placed in the
// freeConn list, then true is returned, otherwise false is returned.
func (p *Pool) putConnLocked(c *Conn, err error) bool {
	if p.closed {
		return false
	}
	if p.maxOpen > 0 && p.numOpen > p.maxOpen {
		return false
	}
	if len(p.connRequests) > 0 {
		var req chan connRequest
		var reqKey uint64
		for reqKey, req = range p.connRequests {
			break
		}
		delete(p.connRequests, reqKey) // Remove from pending requests.
		if err == nil {
			c.inUse = true
		}
		req <- connRequest{conn: c, err: err}
		return true
	} else if err == nil {
		if p.maxIdle > len(p.freeConn) {
			p.freeConn = append(p.freeConn, c)
			p.startCleanerLocked()
			return true
		}
		p.maxIdleClosed++
	}
	return false
}

// putConn adds a Conn to the free pool.
// err is optionally the last error that occurred on this Conn.
func (p *Pool) putConn(conn *Conn, err error) {
	p.mu.Lock()
	conn.inUse = false

	// Don't reuse bad/unusable connections
	// Since the conn is considered bad and is being discarded, treat it
	// as closed.
	if conn.unusable || err == errBadConn {
		p.numOpen--
		p.mu.Unlock()
		conn.close()
		return
	}

	added := p.putConnLocked(conn, nil)
	if !added {
		p.numOpen--
		p.mu.Unlock()
		conn.close()
		return
	}
	p.mu.Unlock()
}

// startCleanerLocked starts connectionCleaner if needed.
func (p *Pool) startCleanerLocked() {
	if p.cleanerCh == nil && p.numOpen > 0 && p.cleanTime > 0 {
		p.cleanerCh = make(chan struct{}, 1)
		go p.connectionCleaner(p.cleanTime)
	}
}

func (p *Pool) connectionCleaner(d time.Duration) {
	t := time.NewTimer(d)

	for {
		select {
		case <-t.C:
		case <-p.cleanerCh: // pool was closed
		}

		p.mu.Lock()
		if p.closed || p.numOpen == 0 {
			p.cleanerCh = nil
			p.mu.Unlock()
			return
		}

		var closing []*Conn
		for i := 0; i < len(p.freeConn); i++ {
			conn := p.freeConn[i]
			if conn.expired(p.maxLifeTime) {
				closing = append(closing, conn)
				last := len(p.freeConn) - 1
				p.freeConn[i] = p.freeConn[last]
				p.freeConn[last] = nil
				p.freeConn = p.freeConn[:last]
				i--
			}
		}

		p.maxLifetimeClosed += uint64(len(closing))
		p.numOpen -= len(closing)
		p.mu.Unlock()

		for _, conn := range closing {
			conn.close()
		}

		t.Reset(d)
	}
}
