package net

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
)

var (
	_ Listener      = &pipeListener{}
	_ Dialer        = &pipeListener{}
	_ ContextDialer = &pipeListener{}
)

type pipeListener struct {
	ch    chan net.Conn
	close chan struct{}
	done  uint32
	m     sync.Mutex
}

// ListenPipe creates a pipeListener that implement net.Listener and Dialer.
//
// The returned Listener and Dialer operate on the same in-memory channel.
// Calling Dial on the Dialer will return a Conn that can be used to
// communicate with a Conn accepted from the Listener.
func ListenPipe() *pipeListener {
	pl := &pipeListener{
		ch:    make(chan net.Conn),
		close: make(chan struct{}),
	}
	return pl
}

// Accept waits for and returns the next connection to the listener.
func (l *pipeListener) Accept() (c net.Conn, e error) {
	select {
	case c = <-l.ch:
	case <-l.close:
		e = net.ErrClosed
	}
	return
}

// Close closes the listener.
// Any blocked Accept operations will be unblocked and return errors.
func (l *pipeListener) Close() (e error) {
	if atomic.LoadUint32(&l.done) == 0 {
		l.m.Lock()
		defer l.m.Unlock()
		if l.done == 0 {
			defer atomic.StoreUint32(&l.done, 1)
			close(l.close)
			return
		}
	}
	e = net.ErrClosed
	return
}

// Addr returns the listener's network address.
func (l *pipeListener) Addr() net.Addr { return pipeAddr{} }
func (l *pipeListener) Dial(network, addr string) (net.Conn, error) {
	return l.DialContext(context.Background(), network, addr)
}
func (l *pipeListener) DialContext(ctx context.Context, network, addr string) (conn net.Conn, e error) {
	// check closed
	if atomic.LoadUint32(&l.done) != 0 {
		e = net.ErrClosed
		return
	}

	// pipe
	c0, c1 := net.Pipe()
	// waiting accepted or closed or done
	select {
	case <-ctx.Done():
		e = ctx.Err()
	case l.ch <- c0:
		conn = c1
	case <-l.close:
		c0.Close()
		c1.Close()
		e = net.ErrClosed
	}
	return
}

type pipeAddr struct{}

func (pipeAddr) Network() string { return `pipe` }
func (pipeAddr) String() string  { return `pipe` }
