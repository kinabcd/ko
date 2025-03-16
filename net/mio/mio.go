package mio

import (
	"errors"
	"net"
	"time"

	koNet "github.com/kinabcd/ko/net"
)

var (
	ErrClosedByRemote    error = errors.Join(net.ErrClosed, errors.New("closed by remote"))
	ErrMainConnClosed    error = errors.Join(net.ErrClosed, errors.New("main conn closed"))
	ErrDialingIsCanceled error = errors.New("dial is canceled")
	ErrReadBufIsFull     error = errors.New("read buf is full")
)

// Multi-connections in one connection. A pipeListener on net.Conn.
type Conn interface {
	koNet.ContextDialer
	koNet.Listener
	Done() <-chan struct{}
	SubConns() map[uint16]SubConn
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	KeepAlive(duration time.Duration)
	Latency() time.Duration
}
type SubConn interface {
	Id() uint16
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
}
