package pool

import (
	"net"
	"time"
)

type fakeConnection struct{}

func (c *fakeConnection) Read(b []byte) (n int, err error) {
	return 0, nil
}
func (c *fakeConnection) Write(b []byte) (n int, err error) {
	return 0, nil
}
func (c *fakeConnection) Close() error {
	return nil
}
func (c *fakeConnection) LocalAddr() net.Addr {
	return nil
}
func (c *fakeConnection) RemoteAddr() net.Addr {
	return nil
}
func (c *fakeConnection) SetDeadline(t time.Time) error {
	return nil
}
func (c *fakeConnection) SetReadDeadline(t time.Time) error {
	return nil
}
func (c *fakeConnection) SetWriteDeadline(t time.Time) error {
	return nil
}
