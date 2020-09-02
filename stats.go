package pool

import (
	"sync/atomic"
	"time"
)

// Stats contains pool statistics
type Stats struct {
	MaxOpenConnections int // Maximum number of open connections to the database.

	// Pool Status
	OpenConnections int // The number of established connections both in use and idle.
	InUse           int // The number of connections currently in use.
	Idle            int // The number of idle connections.

	// Counters
	WaitCount         uint64        // The total number of connections waited for.
	WaitDuration      time.Duration // The total time blocked waiting for a new connection.
	MaxIdleClosed     uint64        // The total number of connections closed due to MaxIdleConnections.
	MaxLifetimeClosed uint64        // The total number of connections closed due to ConnLifeTime.
}

// GetStats returns pool statistics.
func (p *Pool) GetStats() Stats {
	wait := atomic.LoadInt64(&p.waitDuration)

	p.mu.Lock()
	defer p.mu.Unlock()

	stats := Stats{
		MaxOpenConnections: p.maxOpen,

		Idle:            len(p.freeConn),
		OpenConnections: p.numOpen,
		InUse:           p.numOpen - len(p.freeConn),

		WaitCount:         p.waitCount,
		WaitDuration:      time.Duration(wait),
		MaxIdleClosed:     p.maxIdleClosed,
		MaxLifetimeClosed: p.maxLifetimeClosed,
	}
	return stats
}
