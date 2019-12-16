package app

import (
	"errors"
	"net"
	"sync"
	"time"

	"github.com/SkycoinProject/skywire-mainnet/pkg/app/appnet"
)

// Conn is a connection from app client to the server.
// Implements `net.Conn`.
type Conn struct {
	id         uint16
	rpc        RPCClient
	local      appnet.Addr
	remote     appnet.Addr
	freeConn   func() bool
	freeConnMx sync.RWMutex
}

// Read reads from connection.
func (c *Conn) Read(b []byte) (int, error) {
	n, err := c.rpc.Read(c.id, b)
	if err != nil {
		return 0, err
	}

	return n, err
}

// Write writes to connection.
func (c *Conn) Write(b []byte) (int, error) {
	return c.rpc.Write(c.id, b)
}

// Close closes connection.
func (c *Conn) Close() error {
	c.freeConnMx.RLock()
	defer c.freeConnMx.RUnlock()

	if c.freeConn != nil {
		if freed := c.freeConn(); !freed {
			return errors.New("conn is already closed")
		}

		return c.rpc.CloseConn(c.id)
	}

	return nil
}

// LocalAddr returns local address of connection.
func (c *Conn) LocalAddr() net.Addr {
	return c.local
}

// RemoteAddr returns remote address of connection.
func (c *Conn) RemoteAddr() net.Addr {
	return c.remote
}

// SetDeadline sets read and write deadlines for connection.
func (c *Conn) SetDeadline(t time.Time) error {
	return c.rpc.SetDeadline(c.id, t)
}

// SetReadDeadline sets read deadline for connection.
func (c *Conn) SetReadDeadline(t time.Time) error {
	return c.rpc.SetReadDeadline(c.id, t)
}

// SetWriteDeadline sets write deadline for connection.
func (c *Conn) SetWriteDeadline(t time.Time) error {
	return c.rpc.SetWriteDeadline(c.id, t)
}
