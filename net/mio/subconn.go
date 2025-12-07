package mio

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"time"
)

var (
	_ SubConn     = &subConn{}
	_ net.Conn    = &subConn{}
	_ io.WriterTo = &subConn{}
)

type subConn struct {
	id       uint16
	idBytes  []byte
	mainConn *conn

	dialContext context.Context

	localAddr  net.Addr
	remoteAddr net.Addr

	readChan chan []byte
	readBuf  []byte

	dialing     context.Context
	dialingDone func()
	ctx         context.Context
	close       context.CancelCauseFunc

	readDeadline  time.Time
	writeDeadline time.Time

	readAckCount  uint16
	writeAckCount uint16
	writeAckChan  chan struct{}
}

func newMioSubConn(ctx context.Context, id uint16, mainConn *conn, localAddr, remoteAddr net.Addr, dialContext context.Context) *subConn {
	idBytes := binary.BigEndian.AppendUint16([]byte{}, id)
	baseContext, cancelFunc := context.WithCancelCause(ctx)
	dialing, dialingDone := context.WithCancel(baseContext)
	return &subConn{
		id:          id,
		idBytes:     idBytes,
		mainConn:    mainConn,
		dialContext: dialContext,

		localAddr:  localAddr,
		remoteAddr: remoteAddr,

		ctx:   baseContext,
		close: cancelFunc,

		readChan: make(chan []byte, 1024),

		dialing:     dialing,
		dialingDone: dialingDone,

		readAckCount:  0,
		writeAckCount: 0,
		writeAckChan:  make(chan struct{}, 8),
	}
}

func (m *subConn) Id() uint16 {
	return m.id
}

// Close implements net.Conn.
func (m *subConn) Close() (e error) {
	return m.CloseCause(nil)
}

func (m *subConn) CloseCause(cause error) (e error) {
	m.mainConn.clearSubConn(m)
	m.close(cause)
	m.mainConn.writePack(CLOSE, m.id)
	return
}

// LocalAddr implements net.Conn.
func (m *subConn) LocalAddr() net.Addr {
	if m.localAddr != nil {
		return m.localAddr
	}
	return m.mainConn.LocalAddr()
}
func (m *subConn) ackRead() {
	m.readAckCount += 1
	if m.readAckCount >= 128 {
		m.mainConn.writePack(ACK, m.id)
		m.readAckCount -= 128
	}
}

// Read implements net.Conn.
func (m *subConn) Read(b []byte) (n int, err error) {
	if m.ctx.Err() != nil {
		return 0, m.ctx.Err()
	}
	deadLine := context.Background()
	if !m.readDeadline.Equal(time.Time{}) {
		var cancelFunc func()
		deadLine, cancelFunc = context.WithDeadline(deadLine, m.readDeadline)
		defer cancelFunc()
	}
	for len(m.readBuf) == 0 {
		select {
		case m.readBuf = <-m.readChan:
			m.ackRead()
		case <-m.ctx.Done():
			return 0, m.ctx.Err()
		case <-deadLine.Done():
			return 0, os.ErrDeadlineExceeded
		}
	}

	n = copy(b, m.readBuf)
	m.readBuf = m.readBuf[n:]
	return
}

// Write implements io.WriterTo.
func (m *subConn) WriteTo(w io.Writer) (n int64, err error) {
	deadLine := context.Background()
	if !m.readDeadline.Equal(time.Time{}) {
		var cancelFunc func()
		deadLine, cancelFunc = context.WithDeadline(deadLine, m.readDeadline)
		defer cancelFunc()
	}
	var nn int
	for {
		if len(m.readBuf) == 0 {
			select {
			case m.readBuf = <-m.readChan:
				m.ackRead()
			case <-m.ctx.Done():
				return n, m.ctx.Err()
			case <-deadLine.Done():
				return 0, os.ErrDeadlineExceeded
			}
		}
		nn, err = w.Write(m.readBuf)
		n += int64(nn)
		m.readBuf = m.readBuf[nn:]
		if err != nil {
			return
		}
	}
}

// Write implements net.Conn.
func (m *subConn) Write(b []byte) (n int, err error) {
	if m.ctx.Err() != nil {
		return 0, m.ctx.Err()
	}
	n = 0
	ctx := m.ctx
	if !m.writeDeadline.Equal(time.Time{}) {
		var cancelFunc func()
		ctx, cancelFunc = context.WithDeadline(m.ctx, m.writeDeadline)
		defer cancelFunc()
	}
	maxSize := m.mainConn.maxWriteSize
	for len(b) > n {
		m.applyWriteAck()
		if m.mainConn.version >= 2 && (m.writeAckCount >= 1024) {
			select {
			case <-m.writeAckChan:
				m.writeAckCount -= 128
			case <-ctx.Done():
				return n, ctx.Err()
			}
		}
		wn := min(len(b)-n, maxSize)
		if wn, err = m.mainConn.writeData(ctx, m.id, b[n:n+wn]); err == nil {
			m.writeAckCount += 1
		}

		n += wn
		if err != nil {
			return
		}
	}
	return
}

func (m *subConn) applyWriteAck() {
	for {
		select {
		case <-m.writeAckChan:
			m.writeAckCount -= 128
		default:
			return
		}
	}
}

// RemoteAddr implements net.Conn.
func (m *subConn) RemoteAddr() net.Addr {
	return m.remoteAddr
}

// SetDeadline implements net.Conn.
func (m *subConn) SetDeadline(t time.Time) error {
	return errors.Join(m.SetReadDeadline(t), m.SetWriteDeadline(t))
}

// SetReadDeadline implements net.Conn.
func (m *subConn) SetReadDeadline(t time.Time) error {
	m.readDeadline = t
	return nil
}

// SetWriteDeadline implements net.Conn.
func (m *subConn) SetWriteDeadline(t time.Time) error {
	m.writeDeadline = t
	return nil
}

func (m *subConn) Context() context.Context {
	if m.dialContext != nil {
		return m.dialContext
	} else {
		return context.Background()
	}
}
