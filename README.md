 # Pool ![Tests](https://github.com/banknovo/pool/workflows/Tests/badge.svg)

**Note**: Although we use this library in our Production environment, it is still evolving and API is not yet final. There maybe breaking changes. Consider this in Beta until there is a v1.0 release.

Pool is a connection pool for net.Conn interface.

This implementation is a extracted from the db connection pool used in [Go's database/sql/sql.go](https://github.com/golang/go/blob/master/src/database/sql/sql.go) and necessary changes made to work with net.Conn connections.

The majority of open source implementations of a connection pool on GitHub are derived or forked from [fatih/pool](https://github.com/fatih/pool).
But the base library had missing features, and forking and modifying the code results in a different implementation enough not to be called a fork.

The pool returns a wrapper object `*pool.Conn` instead of `net.Conn` from the pool's `Get()` method.
The reason this was done is because we need a way for external clients to close a connection `conn.Close()` and also return it back to the pool `conn.Release()`.   

## Installation

Install the package with:

```bash
go get github.com/banknovo/pool
```

## Usage

```go
// create options
o := &Options{
    Factory:            func() (net.Conn, error) { return net.Dial("tcp", "127.0.0.1:4000") },
    MaxConnections:     30,
    MaxIdleConnections: 10,
    ConnLifeTime:       time.Minute * 5,
    GetTimeout:         time.Seconds * 5,
}

// create a new channel based pool with an maximum capacity of 30 connections in pool
// it will keep maximum of 10 idle connections in pool
p, err := pool.New(o)

// now you can get a connection from the pool, if there is no connection
// available it will create a new one via the factory function.
conn, err := p.Get()

// do something with conn and put it back to the pool by releasing the connection
// (this doesn't close the underlying connection instead it's putting it back
// to the pool).
conn.Release()

// if connection was priviledged or had an error, mark it as unusable
// Unusable connections are closed and not returned to the pool
conn.MarkUnusable()
conn.Release()

// Stats on connection pool
stats := p.Stats()

// close pool, this closes all the connections inside a pool and it cannot be used again
p.Close()
```