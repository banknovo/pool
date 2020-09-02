package pool

import (
	"net"
	"testing"
	"time"
)

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
