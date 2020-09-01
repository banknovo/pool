package pool

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPool(t *testing.T) {
	o := &Options{
		Factory: func() (net.Conn, error) {
			return &fakeConnection{}, nil
		},
		MaxConnections:     5,
		MaxIdleConnections: 3,
		GetTimeout:         time.Second,
	}
	p, err := New(o)
	require.NoError(t, err)

	// create a connections upto max
	conns := make([]*Conn, p.maxOpen)
	for i := 0; i < p.maxOpen; i++ {
		conn, err := p.Get()
		require.NoError(t, err)
		require.NotNil(t, conn)

		require.Equal(t, i+1, p.NumOpenConns())

		conns[i] = conn
	}

	// try to get a connection now, it should fail
	_, err = p.Get()
	require.Error(t, err)
	require.Equal(t, ErrTimedOut, err)

	// try to get a connection in a goroutine, you should get it since code below releases it
	block := make(chan struct{}, 1)
	go func() {
		defer func() { block <- struct{}{} }()
		conn, err := p.Get()
		require.NoError(t, err)
		require.NotNil(t, conn)
		require.Equal(t, p.maxOpen, p.NumOpenConns())
		conn.Release()
	}()

	// release a connection
	<-time.After(time.Millisecond * 100)
	conns[0].Release()

	// block until above goroutine either passes or fails
	<-block

	copy(conns, conns[1:])
	conns = conns[:p.maxOpen-1]

	// Release all previous connections
	for i := 0; i < p.maxOpen-1; i++ {
		conns[i].Release()
	}
	//
	// make sure we have free connections matching open connections and are less than idle connections
	require.Equal(t, len(p.freeConn), p.NumOpenConns())
	require.LessOrEqual(t, p.NumOpenConns(), p.maxIdle)

	// close and make sure we cannot get a connection
	p.Close()
	_, err = p.Get()
	require.Error(t, err)
	require.Equal(t, ErrPoolClosed, err)
}

func TestPool_ConnectionsExpire(t *testing.T) {
	o := &Options{
		Factory: func() (net.Conn, error) {
			return &fakeConnection{}, nil
		},
		MaxConnections:     5,
		MaxIdleConnections: 3,
		GetTimeout:         time.Second,
		ConnLifeTime:       time.Millisecond * 500,
	}
	p, err := New(o)
	require.NoError(t, err)

	// create a connections less than max
	conns := make([]*Conn, p.maxOpen)
	for i := 0; i < p.maxOpen; i++ {
		conn, err := p.Get()
		require.NoError(t, err)
		require.NotNil(t, conn)

		require.Equal(t, i+1, p.NumOpenConns())

		conns[i] = conn
	}

	// Release all of them
	for i := 0; i < p.maxOpen; i++ {
		conns[i].Release()
	}

	// make sure we have free connections matching open connections and are less than idle connections
	require.Equal(t, len(p.freeConn), p.NumOpenConns())
	require.LessOrEqual(t, p.NumOpenConns(), p.maxIdle)

	conn, err := p.Get()
	require.NoError(t, err)
	require.NotNil(t, conn)

	// wait for 2 seconds for connections to expire
	<-time.After(time.Second * 2)

	// make sure all connections except the one that is open have been cleaned up
	require.Equal(t, 1, p.NumOpenConns())

	// close the conn
	conn.Close()

	// make sure returning expired connections do not add them to the pool
	require.Equal(t, 0, p.NumOpenConns())
}

func BenchmarkPool_GetAndReleaseInSequence(b *testing.B) {
	o := &Options{
		Factory: func() (net.Conn, error) {
			return &fakeConnection{}, nil
		},
		MaxConnections: b.N,
		ConnLifeTime:   time.Minute,
		GetTimeout:     time.Microsecond,
	}
	p, _ := New(o)
	conns := make([]*Conn, b.N)
	b.ReportAllocs()
	b.StartTimer()

	for i := 0; i < b.N; i++ {
		conns[i], _ = p.Get()
	}
	for i := 0; i < b.N; i++ {
		conns[i].Release()
	}
}
