package mio

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/url"
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

	writeReplyChan chan mioDataMessage

	dialing     context.Context
	dialingDone func()
	ctx         context.Context
	close       context.CancelCauseFunc

	readDeadline  time.Time
	writeDeadline time.Time
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

		readChan:       make(chan []byte, 1024),
		writeReplyChan: make(chan mioDataMessage),

		dialing:     dialing,
		dialingDone: dialingDone,
	}
}

func (m *subConn) dial(ctx context.Context) (e error) {
	if m.mainConn.version >= 1 {
		u := url.URL{Scheme: m.remoteAddr.Network(), Host: m.remoteAddr.String()}
		bs := append([]byte{1, m.idBytes[0], m.idBytes[1]}, []byte(u.String())...)
		bs = append(bs, 0)
		m.mainConn.writeAsync(bs)
	} else {
		m.mainConn.writeAsync([]byte{1, m.idBytes[0], m.idBytes[1]})
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-m.dialing.Done():
		return m.ctx.Err()
	}
}

func (m *subConn) accept() (e error) {
	m.mainConn.writeAsync([]byte{2, m.idBytes[0], m.idBytes[1]})
	return
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
	m.mainConn.writeAsync([]byte{3, m.idBytes[0], m.idBytes[1]})
	return
}

// LocalAddr implements net.Conn.
func (m *subConn) LocalAddr() net.Addr {
	if m.localAddr != nil {
		return m.localAddr
	}
	return m.mainConn.LocalAddr()
}

// Read implements net.Conn.
func (m *subConn) Read(b []byte) (n int, err error) {
	for len(m.readBuf) == 0 {
		if m.ctx.Err() != nil {
			return 0, m.ctx.Err()
		}
		deadLine := m.ctx
		if !m.readDeadline.Equal(time.Time{}) {
			timeout, cancelFunc := context.WithDeadline(m.ctx, m.readDeadline)
			deadLine = timeout
			defer cancelFunc()
		}
		select {
		case m.readBuf = <-m.readChan:
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
	var nn int
	for {
		if len(m.readBuf) == 0 {
			select {
			case m.readBuf = <-m.readChan:
			case <-m.ctx.Done():
				return n, m.ctx.Err()
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
	deadLine := m.ctx
	if !m.writeDeadline.Equal(time.Time{}) {
		timeout, cancelFunc := context.WithDeadline(m.ctx, m.writeDeadline)
		deadLine = timeout
		defer cancelFunc()
	}
	maxSize := m.mainConn.maxWriteSize
	for len(b) > n {
		wn := min(len(b)-n, maxSize)
		lenBytes := binary.BigEndian.AppendUint16([]byte{}, uint16(wn))
		select {
		case m.mainConn.writeChan <- mioDataMessage{
			[]byte{4, m.idBytes[0], m.idBytes[1], lenBytes[0], lenBytes[1]}, b[n : n+wn],
			0, nil,
			m.writeReplyChan,
		}:
			msg := <-m.writeReplyChan
			wn = msg.N
			err = msg.Err
		case <-m.ctx.Done():
			return 0, m.ctx.Err()
		case <-deadLine.Done():
			return n, os.ErrDeadlineExceeded
		}

		n += wn
		if err != nil {
			return
		}
	}
	return
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
