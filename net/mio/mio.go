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

// MaxWriteSize, the data payload size, must be between 1 and 65535 bytes.
type MaxWriteSize int

// PingInterval specifies the interval at which ping messages are sent to check the connection latency.
type PingInterval time.Duration

// PingTimeout specifies the maximum amount of time to wait for a response to a ping message when checking connection latency.
type PingTimeout time.Duration

// Multi-connections in one connection. A pipeListener on net.Conn.
type Conn interface {
	koNet.ContextDialer
	koNet.Listener
	Done() <-chan struct{}
	SubConns() map[uint16]SubConn
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	Latency() time.Duration
}
type SubConn interface {
	Id() uint16
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
}
