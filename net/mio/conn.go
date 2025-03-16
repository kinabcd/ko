package mio

import (
	"context"
	"encoding/binary"
	"math/rand/v2"
	"net"
	"net/url"
	"sync"
	"time"

	koIo "github.com/kinabcd/ko/io"
	koNet "github.com/kinabcd/ko/net"
	koTime "github.com/kinabcd/ko/time"
)

var (
	_ koNet.ContextDialer = &conn{}
	_ koNet.Listener      = &conn{}
	_ Conn                = &conn{}
)

type conn struct {
	// MaxWriteSize, the data payload size, must be between 1 and 65535 bytes.
	MaxWriteSize int

	conn       net.Conn
	acceptChan chan *subConn

	statusLock sync.Mutex
	isMain     bool
	ctx        context.Context
	close      func()

	handshakeContext context.Context
	handshakeDone    func()

	writeChan chan mioDataMessage

	subConnLock sync.Mutex
	subIdNext   uint16
	subConns    map[uint16]*subConn

	pingLock sync.Mutex
	pingNext uint16
	pings    map[uint16](chan struct{})
	latency  []time.Duration

	version byte
}

func New(c net.Conn) (m *conn) {
	baseContext, cancel := context.WithCancel(context.Background())
	handshakeContext, handshakeDone := context.WithCancel(baseContext)
	m = &conn{
		conn:     c,
		subConns: make(map[uint16]*subConn),
		pings:    make(map[uint16]chan struct{}),

		acceptChan: make(chan *subConn, 256),
		writeChan:  make(chan mioDataMessage, 1024),

		ctx:   baseContext,
		close: cancel,

		handshakeContext: handshakeContext,
		handshakeDone:    handshakeDone,

		MaxWriteSize: 65535,
	}
	return
}
func (m *conn) Done() <-chan struct{} {
	if m.handshakeContext.Err() == nil {
		go m.handshake()
		<-m.handshakeContext.Done()
	}
	return m.ctx.Done()
}
func (m *conn) SubConns() map[uint16]SubConn {
	r := make(map[uint16]SubConn)
	m.subConnLock.Lock()
	defer m.subConnLock.Unlock()
	for subId, conn := range m.subConns {
		r[subId] = conn
	}
	return r
}
func (m *conn) handshake() (err error) {
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
				m.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				_, msg.Err = m.conn.Write(msg.DataPrefix)
				if msg.Err != nil {
					msg.N = 0
					if msg.ReplyTo != nil {
						msg.ReplyTo <- msg
					}
					m.Close()
					return
				}

				m.conn.SetWriteDeadline(time.Now().Add(60 * time.Second))
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
	m.version = 1
	m.writeAsync([]byte{'M', 'I', 'O', m.version})
	if bs, err := koIo.ReadN(c, 4); err != nil {
		return err
	} else {
		if m.version > bs[3] {
			m.version = bs[3]
		}
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

		go m.testLatency()
		return
	}
}
func (m *conn) processNextPack() (err error) {
	var t byte
	if t, err = koIo.ReadByte(m.conn); err != nil {
		return
	}
	var idBytes []byte
	if idBytes, err = koIo.ReadN(m.conn, 2); err != nil {
		return
	}
	id := binary.BigEndian.Uint16(idBytes)
	if t == 0 {
	} else if t == 1 { // Dial
		m.subConnLock.Lock()
		defer m.subConnLock.Unlock()
		var localAddr net.Addr
		var str string
		var u *url.URL
		if str, err = koIo.ReadCString(m.conn); err != nil {
			return
		} else if u, err = url.Parse(str); err != nil {
			return
		} else {
			localAddr = &subAddr{network: u.Scheme, address: u.Host}
		}
		c := newMioSubConn(m.ctx, id, m, localAddr, m.conn.RemoteAddr())
		m.subConns[c.id] = c
		select {
		case m.acceptChan <- c:
		default:
			go c.CloseCause(ErrDialingIsCanceled)
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
			go c.CloseCause(ErrClosedByRemote)
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
			select {
			case c.readChan <- dataBytes:
			default:
				go c.CloseCause(ErrReadBufIsFull)
			}

		}
	} else if t == 5 { // Ping
		if (m.isMain && id < 0x8000) || (!m.isMain && id >= 0x8000) {
			// pong from peer. record the time.
			m.pingLock.Lock()
			if p, ok := m.pings[id]; ok {
				select {
				case p <- struct{}{}:
				default:
				}
			}
			m.pingLock.Unlock()
		} else {
			// ping from peer. pong it.
			m.writeAsync([]byte{5, idBytes[0], idBytes[1]})
		}
	}
	return nil
}

func (m *conn) nextIdLocked() uint16 {
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

func (m *conn) Ping() (time.Duration, error) {
	var n uint16
	ch := make(chan struct{})
	m.pingLock.Lock()
	for {
		n = m.pingNext
		m.pingNext += 1
		if m.isMain {
			n &= 0x7FFF
		} else {
			n |= 0x8000
		}
		if _, ok := m.pings[n]; !ok {
			break
		}
	}
	m.pings[n] = ch
	m.pingLock.Unlock()
	ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
	defer cancel()
	defer func() {
		m.pingLock.Lock()
		delete(m.pings, n)
		m.pingLock.Unlock()
	}()

	idBytes := binary.BigEndian.AppendUint16([]byte{}, n)
	startTime := time.Now()
	m.writeAsync([]byte{5, idBytes[0], idBytes[1]})
	select {
	case <-ch:
	case <-ctx.Done():
	}
	return time.Since(startTime), ctx.Err()
}

func (m *conn) Addr() net.Addr {
	return m.LocalAddr()
}

func (m *conn) LocalAddr() net.Addr {
	return m.conn.LocalAddr()
}

func (m *conn) RemoteAddr() net.Addr {
	return m.conn.RemoteAddr()
}

func (m *conn) Close() (err error) {
	m.subConnLock.Lock()
	defer m.subConnLock.Unlock()
	for _, c := range m.subConns {
		go c.CloseCause(ErrMainConnClosed)
	}
	m.close()
	return m.conn.Close()
}

func (m *conn) Accept() (conn net.Conn, e error) {
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
			c.CloseCause(e)
			return
		}
		return c, nil
	case <-m.ctx.Done():
		return nil, net.ErrClosed
	}
}
func (m *conn) KeepAlive(duration time.Duration) {
	if m.handshakeContext.Err() == nil {
		go m.handshake()
		<-m.handshakeContext.Done()
	}
	for koTime.SleepContext(m.ctx, duration) {
		go m.testLatency()
	}
}

func (m *conn) testLatency() {
	latency, err := m.Ping()
	if err != nil {
		return
	}
	m.pingLock.Lock()
	defer m.pingLock.Unlock()
	m.latency = append(m.latency, latency)
	if len(m.latency) > 5 {
		m.latency = m.latency[1:]
	}
}

func (m *conn) Latency() time.Duration {
	m.pingLock.Lock()
	defer m.pingLock.Unlock()
	lenLatency := len(m.latency)
	if lenLatency == 0 {
		return 0
	}
	sumLatency := time.Duration(0)
	for _, l := range m.latency {
		sumLatency += l
	}

	return time.Duration(int64(sumLatency) / int64(lenLatency))
}

func (m *conn) newSubConn(network, addr string) *subConn {
	m.subConnLock.Lock()
	defer m.subConnLock.Unlock()
	c := newMioSubConn(m.ctx, m.nextIdLocked(), m, m.conn.LocalAddr(), &subAddr{network: network, address: addr})
	m.subConns[c.id] = c
	return c
}
func (m *conn) DialContext(ctx context.Context, network, addr string) (conn net.Conn, e error) {
	if m.handshakeContext.Err() == nil {
		go m.handshake()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-m.handshakeContext.Done():
		}
	}
	sc := m.newSubConn(network, addr)
	if e = sc.dial(ctx); e != nil {
		go sc.CloseCause(e)
		return nil, e
	}
	return sc, nil
}

func (m *conn) clearSubConn(c *subConn) {
	m.subConnLock.Lock()
	defer m.subConnLock.Unlock()
	delete(m.subConns, c.id)
}
func (m *conn) writeAsync(b []byte) (n <-chan mioDataMessage) {
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
