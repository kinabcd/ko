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
	// Done returns a channel that will be closed when the connection is closed.
	Done() <-chan struct{}
	// SubConns returns a map of the currently active sub-connections. The keys of the map are the sub-connection IDs.
	SubConns() map[uint16]SubConn
	// LocalAddr returns the local network address.
	LocalAddr() net.Addr
	// RemoteAddr returns the remote network address.
	RemoteAddr() net.Addr
	// Latency returns the current estimated round-trip time (RTT) or latency to the remote peer.
	Latency() time.Duration
}
type SubConn interface {
	// Id returns the unique identifier for this sub-connection.
	Id() uint16
	// LocalAddr returns the local network address for this sub-connection.
	LocalAddr() net.Addr
	// RemoteAddr returns the remote network address for this sub-connection.
	RemoteAddr() net.Addr
}
