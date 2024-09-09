package net

import (
	"context"
	"encoding/binary"
	"io"
	"math/rand/v2"
	"net"
	"os"
	"sync"
	"time"

	koIo "github.com/kinabcd/ko/io"
)

var (
	_ net.Conn      = &mioSubConn{}
	_ io.WriterTo   = &mioSubConn{}
	_ ContextDialer = &mioConn{}
	_ Listener      = &mioConn{}
)

// Multi-connections in one connection. A pipeListener on net.Conn.
type MioConn interface {
	ContextDialer
	Listener
	Done() <-chan struct{}
}
type mioConn struct {
	// MaxWriteSize, the data payload size, must be between 1 and 65535 bytes.
	MaxWriteSize int

	conn       net.Conn
	acceptChan chan *mioSubConn

	statusLock sync.Mutex
	isMain     bool
	ctx        context.Context
	close      func()

	handshakeContext context.Context
	handshakeDone    func()

	writeChan chan mioDataMessage

	subConnLock sync.Mutex
	subIdNext   uint16
	subConns    map[uint16]*mioSubConn
}

func NewMioConn(c net.Conn) (m *mioConn) {
	baseContext, cancel := context.WithCancel(context.Background())
	handshakeContext, handshakeDone := context.WithCancel(baseContext)
	m = &mioConn{
		conn:     c,
		subConns: make(map[uint16]*mioSubConn),

		acceptChan: make(chan *mioSubConn, 256),
		writeChan:  make(chan mioDataMessage, 16),

		ctx:   baseContext,
		close: cancel,

		handshakeContext: handshakeContext,
		handshakeDone:    handshakeDone,

		MaxWriteSize: 65535,
	}
	return
}
func (m *mioConn) Done() <-chan struct{} {
	if m.handshakeContext.Err() == nil {
		go m.handshake()
		<-m.handshakeContext.Done()
	}
	return m.ctx.Done()
}
func (m *mioConn) handshake() (err error) {
	m.statusLock.Lock()
	defer m.statusLock.Unlock()
	if m.ctx.Err() != nil {
		return net.ErrClosed
	}
	if m.handshakeContext.Err() != nil {
		return nil
	}
	defer m.handshakeDone()
	c := m.conn
	go func() {
		for {
			select {
			case msg := <-m.writeChan:
				_, msg.Err = m.conn.Write(msg.DataPrefix)
				if msg.Err != nil {
					msg.N = 0
					if msg.ReplyTo != nil {
						msg.ReplyTo <- msg
					}
					m.Close()
					return
				}

				msg.N, msg.Err = m.conn.Write(msg.Data)
				if msg.ReplyTo != nil {
					msg.ReplyTo <- msg
				}
				if msg.Err != nil {
					m.Close()
					return
				}
			case <-m.ctx.Done():
				return
			}

		}
	}()
	// say hello "MIO" (3), Version(1)
	m.writeAsync([]byte{'M', 'I', 'O', 0})
	if _, err = koIo.ReadN(c, 4); err != nil {
		return
	}
	for {
		// high card. winner is main.
		myNum := rand.N(byte(0xFF))
		peerNum := byte(0)

		m.writeAsync([]byte{myNum})
		if peerNum, err = koIo.ReadByte(c); err != nil {
			return
		}
		if myNum == peerNum {
			// Same. Try again.
			continue
		}
		m.isMain = myNum > peerNum
		go func() {
			for {
				if err := m.processNextPack(); err != nil {
					m.Close()
					return
				}
			}
		}()
		return
	}
}
func (m *mioConn) processNextPack() (err error) {
	var t byte
	if t, err = koIo.ReadByte(m.conn); err != nil {
		return
	}
	var idBytes []byte
	if idBytes, err = koIo.ReadN(m.conn, 2); err != nil {
		return
	}
	id := binary.BigEndian.Uint16(idBytes)
	if t == 1 { // Dial
		m.subConnLock.Lock()
		defer m.subConnLock.Unlock()
		c := newMioSubConn(m.ctx, id, m)
		m.subConns[c.id] = c
		select {
		case m.acceptChan <- c:
		default:
			go c.Close()
		}
	} else if t == 2 { // Accept
		m.subConnLock.Lock()
		defer m.subConnLock.Unlock()
		if c, ok := m.subConns[id]; !ok {
			m.writeAsync([]byte{3, idBytes[0], idBytes[1]})
		} else {
			c.dialingDone()
		}
	} else if t == 3 { // Close
		m.subConnLock.Lock()
		defer m.subConnLock.Unlock()
		if c, ok := m.subConns[id]; ok {
			go c.Close()
		}
	} else if t == 4 { // Data
		var lenBytes []byte
		if lenBytes, err = koIo.ReadN(m.conn, 2); err != nil {
			return
		}
		l := int(binary.BigEndian.Uint16(lenBytes))
		if l == 0 {
			return nil
		}
		var dataBytes []byte
		if dataBytes, err = koIo.ReadN(m.conn, l); err != nil {
			return
		}
		m.subConnLock.Lock()
		defer m.subConnLock.Unlock()
		if c, ok := m.subConns[id]; ok {
			c.readChan <- dataBytes
		}
	}
	return nil
}

func (m *mioConn) nextIdLocked() uint16 {
	for {
		n := m.subIdNext
		m.subIdNext += 1
		if m.isMain {
			n &= 0x7FFF
		} else {
			n |= 0x8000
		}
		if _, ok := m.subConns[n]; !ok {
			return n
		}
	}
}

func (m *mioConn) Addr() net.Addr {
	return m.conn.LocalAddr()
}

func (m *mioConn) Close() (err error) {
	m.subConnLock.Lock()
	defer m.subConnLock.Unlock()
	for _, c := range m.subConns {
		go c.Close()
	}
	m.close()
	return m.conn.Close()
}

func (m *mioConn) Accept() (conn net.Conn, e error) {
	if m.ctx.Err() != nil {
		return nil, net.ErrClosed
	}
	if m.handshakeContext.Err() == nil {
		go m.handshake()
		<-m.handshakeContext.Done()
	}

	select {
	case c := <-m.acceptChan:
		if e = c.accept(); e != nil {
			c.Close()
			return
		}
		return c, nil
	case <-m.ctx.Done():
		return nil, net.ErrClosed
	}
}

func (m *mioConn) newSubConn() *mioSubConn {
	m.subConnLock.Lock()
	defer m.subConnLock.Unlock()
	c := newMioSubConn(m.ctx, m.nextIdLocked(), m)
	m.subConns[c.id] = c
	return c
}
func (m *mioConn) DialContext(ctx context.Context, network, addr string) (conn net.Conn, e error) {
	if m.handshakeContext.Err() == nil {
		go m.handshake()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-m.handshakeContext.Done():
		}
	}
	sc := m.newSubConn()
	if e = sc.dial(ctx); e != nil {
		go sc.Close()
		return nil, e
	}
	return sc, nil
}

func (m *mioConn) clearSubConn(c *mioSubConn) {
	m.subConnLock.Lock()
	defer m.subConnLock.Unlock()
	delete(m.subConns, c.id)
}
func (m *mioConn) writeAsync(b []byte) (n <-chan mioDataMessage) {
	ch := make(chan mioDataMessage, 1)
	m.writeChan <- mioDataMessage{[]byte{}, b, 0, nil, ch}
	return ch
}

type mioDataMessage struct {
	DataPrefix []byte

	Data []byte

	N   int
	Err error

	ReplyTo chan mioDataMessage
}

type mioSubConn struct {
	id       uint16
	idBytes  []byte
	mainConn *mioConn

	readChan chan []byte
	readBuf  []byte

	writeReplyChan chan mioDataMessage

	dialing     context.Context
	dialingDone func()
	ctx         context.Context
	close       func()

	readDeadline  time.Time
	writeDeadline time.Time
}

func newMioSubConn(ctx context.Context, id uint16, mainConn *mioConn) *mioSubConn {
	idBytes := binary.BigEndian.AppendUint16([]byte{}, id)
	baseContext, cancelFunc := context.WithCancel(ctx)
	dialing, dialingDone := context.WithCancel(baseContext)
	return &mioSubConn{
		id:       id,
		idBytes:  idBytes,
		mainConn: mainConn,

		ctx:   baseContext,
		close: cancelFunc,

		readChan:       make(chan []byte, 256),
		writeReplyChan: make(chan mioDataMessage),

		dialing:     dialing,
		dialingDone: dialingDone,
	}
}

func (m *mioSubConn) dial(ctx context.Context) (e error) {
	m.mainConn.writeAsync([]byte{1, m.idBytes[0], m.idBytes[1]})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-m.dialing.Done():
		return m.ctx.Err()
	}
}

func (m *mioSubConn) accept() (e error) {
	m.mainConn.writeAsync([]byte{2, m.idBytes[0], m.idBytes[1]})
	return
}

// Close implements net.Conn.
func (m *mioSubConn) Close() (e error) {
	m.mainConn.clearSubConn(m)
	m.close()
	m.mainConn.writeAsync([]byte{3, m.idBytes[0], m.idBytes[1]})
	return
}

// LocalAddr implements net.Conn.
func (m *mioSubConn) LocalAddr() net.Addr {
	return m.mainConn.Addr()
}

// Read implements net.Conn.
func (m *mioSubConn) Read(b []byte) (n int, err error) {
	for len(m.readBuf) == 0 {
		if m.ctx.Err() != nil {
			return 0, net.ErrClosed
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
			return 0, net.ErrClosed
		case <-deadLine.Done():
			return 0, os.ErrDeadlineExceeded
		}
	}

	n = copy(b, m.readBuf)
	m.readBuf = m.readBuf[n:]
	return
}

// Write implements io.WriterTo.
func (m *mioSubConn) WriteTo(w io.Writer) (n int64, err error) {
	var nn int
	for {
		if len(m.readBuf) == 0 {
			select {
			case m.readBuf = <-m.readChan:
			case <-m.ctx.Done():
				return n, net.ErrClosed
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
func (m *mioSubConn) Write(b []byte) (n int, err error) {
	if m.ctx.Err() != nil {
		return 0, net.ErrClosed
	}
	n = 0
	deadLine := m.ctx
	if !m.writeDeadline.Equal(time.Time{}) {
		timeout, cancelFunc := context.WithDeadline(m.ctx, m.writeDeadline)
		deadLine = timeout
		defer cancelFunc()
	}
	maxSize := m.mainConn.MaxWriteSize
	for len(b) > n {
		wn := len(b) - n
		if wn > maxSize {
			wn = maxSize
		}
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
			return 0, net.ErrClosed
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
func (m *mioSubConn) RemoteAddr() net.Addr {
	return m.mainConn.conn.RemoteAddr()
}

// SetDeadline implements net.Conn.
func (m *mioSubConn) SetDeadline(t time.Time) (err error) {
	if err = m.SetReadDeadline(t); err != nil {
		return
	}
	if err = m.SetWriteDeadline(t); err != nil {
		return
	}
	return nil
}

// SetReadDeadline implements net.Conn.
func (m *mioSubConn) SetReadDeadline(t time.Time) error {
	m.readDeadline = t
	return nil
}

// SetWriteDeadline implements net.Conn.
func (m *mioSubConn) SetWriteDeadline(t time.Time) error {
	m.writeDeadline = t
	return nil
}
