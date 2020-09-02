package pool

import (
	"errors"
	"time"
)

const (
	defaultGetTimeout    = time.Second * 10
	defaultConnLifeTime  = time.Minute * 60
	defaultConnCleanTime = time.Minute * 15
)

// Options to create a new Pool
type Options struct {
	// Factory is a function to create new connections
	Factory Factory

	// MaxConnections are the maximum number of open connections
	MaxConnections int

	// GetTimeout is the maximum time Pool will wait to obtain the connection before timing out
	GetTimeout time.Duration

	// MaxIdleConnections are the maximum number of idle connections that should remain the Pool
	MaxIdleConnections int

	// ConnLifeTime is the total amount of time a connection should be used before closing it
	ConnLifeTime time.Duration

	// ConnCleanTime is the periodic time interval after which pool checks for expired connections and closes them
	// this value should ideally be lesser than ConnLifeTime to avoid filling up the pool too much
	ConnCleanTime time.Duration
}

func (o *Options) validate() error {
	if o.Factory == nil {
		return errors.New("factory is required")
	}
	if o.MaxIdleConnections < 0 {
		return errors.New("MaxIdleConnections should be >=0")
	}
	if o.MaxConnections < o.MaxIdleConnections {
		return errors.New("MaxConnections should be >=MaxIdleConnections")
	}
	if o.GetTimeout == 0 {
		o.GetTimeout = defaultGetTimeout
	}
	if o.ConnLifeTime == 0 {
		o.ConnLifeTime = defaultConnLifeTime
	}
	if o.ConnCleanTime == 0 {
		o.ConnCleanTime = defaultConnCleanTime
	}
	return nil
}
