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
)

var (
	_ koNet.ContextDialer = &conn{}
	_ koNet.Listener      = &conn{}
	_ Conn                = &conn{}
)

type PackType byte

const (
	NOOP   PackType = 0
	DIAL   PackType = 1
	ACCEPT PackType = 2
	CLOSE  PackType = 3
	DATA   PackType = 4
	PING   PackType = 5
	ACK    PackType = 6
)

type conn struct {
	maxWriteSize int
	pingInterval time.Duration
	pingTimeout  time.Duration

	conn       net.Conn
	acceptChan chan *subConn

	isMain bool
	ctx    context.Context
	close  func()

	handshake sync.Once

	writeChan chan *writeMsg

	subConnLock sync.Mutex
	subIdNext   uint16
	subConns    map[uint16]*subConn

	pingLock sync.Mutex
	pingNext uint16
	pings    map[uint16](chan struct{})
	latency  []time.Duration

	version byte
}

func New(c net.Conn, options ...any) (m *conn) {
	baseContext, cancel := context.WithCancel(context.Background())
	m = &conn{
		conn:     c,
		subConns: make(map[uint16]*subConn),
		pings:    make(map[uint16]chan struct{}),

		acceptChan: make(chan *subConn, 256),
		writeChan:  make(chan *writeMsg, 1024),

		ctx:   baseContext,
		close: cancel,

		maxWriteSize: 65535,
		pingInterval: 15 * time.Second,
	}
	for _, o := range options {
		switch v := o.(type) {
		case MaxWriteSize:
			m.maxWriteSize = int(v)
		case PingInterval:
			m.pingInterval = time.Duration(v)
		case PingTimeout:
			m.pingTimeout = time.Duration(v)
		}
	}
	return
}
func (m *conn) Done() <-chan struct{} {
	m.makesureHandshake()
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
func (m *conn) makesureHandshake() {
	m.handshake.Do(func() {
		rawWriteChan := make(chan []byte, 1)
		done := make(chan struct{})
		go m.loopWrite(rawWriteChan)
		go m.loopRead(rawWriteChan, done)
		select {
		case <-done:
		case <-m.ctx.Done():
			return
		}

		if m.pingInterval > 0 {
			go m.testLatency()
		}
	})
}
func (m *conn) loopWrite(rawWriteChan chan []byte) {
	for b := range rawWriteChan {
		if _, err := m.conn.Write(b); err != nil {
			m.Close()
			return
		}
	}
	// handshake done
	rawWriteChan = nil

	// normal loop
	header := make([]byte, 5)
	pingTick := time.Tick(m.pingInterval)
	if pingTick == nil {
		pingTick = make(<-chan time.Time) // never trigger
	}
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-pingTick:
			go m.testLatency()
		case msg := <-m.writeChan:
			m.conn.SetWriteDeadline(time.Now().Add(60 * time.Second))
			header[0] = byte(msg.Type)
			binary.BigEndian.PutUint16(header[1:3], msg.ID)
			r := writeResult{}
			switch msg.Type {
			case DIAL:
				r.N, r.Error = m.conn.Write(append(append(header[:3], msg.Data...), 0))
			case DATA:
				binary.BigEndian.PutUint16(header[3:5], uint16(len(msg.Data)))
				_, r.Error = m.conn.Write(header[:5])
				if r.Error == nil {
					r.N, r.Error = m.conn.Write(msg.Data)
				}
			default:
				r.N, r.Error = m.conn.Write(header[:3])
			}

			if msg.ReplyTo != nil {
				msg.ReplyTo <- r
			}
			msgPool.Put(msg.Reset())
			if r.Error != nil {
				m.Close()
				return
			}
		}

	}
}

func (m *conn) loopRead(rawWriteChan chan []byte, handshakeDoneChan chan struct{}) {
	m.version = 2
	// say hello "MIO" (3), Version(1)
	rawWriteChan <- []byte{'M', 'I', 'O', m.version}
	if bs, err := koIo.ReadN(m.conn, 4); err != nil {
		m.Close()
		return
	} else {
		if m.version > bs[3] {
			m.version = bs[3]
		}
	}
	// high card. winner is main.
	for {
		myNum := rand.N(byte(0xFF))
		rawWriteChan <- []byte{myNum}
		if peerNum, err := koIo.ReadByte(m.conn); err != nil {
			m.Close()
			return
		} else if myNum != peerNum {
			m.isMain = myNum > peerNum
			break
		}
	}

	// handshake done. release temp chan
	close(handshakeDoneChan)
	close(rawWriteChan)
	handshakeDoneChan = nil
	rawWriteChan = nil

	// normal loop
	for {
		if err := m.processNextPack(); err != nil {
			m.Close()
			return
		}
	}
}
func (m *conn) processNextPack() (err error) {
	var b []byte
	if b, err = koIo.ReadN(m.conn, 3); err != nil {
		return
	}
	packType := PackType(b[0])
	id := binary.BigEndian.Uint16(b[1:3])
	switch packType {
	case NOOP:
	case DIAL:
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
			localAddr = koNet.NewStaticAddr(u.Scheme, u.Host)
		}
		c := newMioSubConn(m.ctx, id, m, localAddr, m.conn.RemoteAddr(), context.Background())
		m.subConns[c.id] = c
		select {
		case m.acceptChan <- c:
		default:
			go c.CloseCause(ErrDialingIsCanceled)
		}
	case ACCEPT:
		m.subConnLock.Lock()
		defer m.subConnLock.Unlock()
		if c, ok := m.subConns[id]; !ok {
			m.writePack(CLOSE, id)
		} else {
			c.dialingDone()
		}
	case CLOSE:
		m.subConnLock.Lock()
		defer m.subConnLock.Unlock()
		if c, ok := m.subConns[id]; ok {
			go c.CloseCause(ErrClosedByRemote)
		}
	case DATA:
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
	case PING:
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
			m.writePack(PING, id)
		}
	case ACK:
		m.subConnLock.Lock()
		defer m.subConnLock.Unlock()
		if c, ok := m.subConns[id]; ok {
			c.writeAckChan <- struct{}{}
		}
	}
	return nil
}

func (m *conn) maskId(n uint16) uint16 {
	if m.isMain {
		return n & 0x7FFF
	} else {
		return n | 0x8000
	}
}

func (m *conn) ping() (time.Duration, error) {
	var n uint16
	ch := make(chan struct{})
	m.pingLock.Lock()
	for {
		n = m.maskId(m.pingNext)
		m.pingNext += 1
		if _, ok := m.pings[n]; !ok {
			break
		}
	}
	m.pings[n] = ch
	m.pingLock.Unlock()
	ctx, cancel := context.WithTimeout(m.ctx, m.pingTimeout)
	defer cancel()
	defer func() {
		m.pingLock.Lock()
		delete(m.pings, n)
		m.pingLock.Unlock()
	}()

	startTime := time.Now()
	m.writePack(PING, n)
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
	m.makesureHandshake()

	select {
	case c := <-m.acceptChan:
		m.writePack(ACCEPT, c.id)
		return c, nil
	case <-m.ctx.Done():
		return nil, net.ErrClosed
	}
}

func (m *conn) testLatency() {
	latency, err := m.ping()
	if err != nil {
		latency = m.pingTimeout
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

func (m *conn) newSubConn(network, addr string, dialContext context.Context) *subConn {
	m.subConnLock.Lock()
	defer m.subConnLock.Unlock()
	var n uint16
	for {
		n = m.maskId(m.subIdNext)
		m.subIdNext += 1
		if _, ok := m.subConns[n]; !ok {
			break
		}
	}
	c := newMioSubConn(m.ctx, n, m, m.conn.LocalAddr(), koNet.NewStaticAddr(network, addr), dialContext)
	m.subConns[n] = c
	return c
}
func (m *conn) DialContext(ctx context.Context, network, addr string) (conn net.Conn, e error) {
	m.makesureHandshake()
	sc := m.newSubConn(network, addr, ctx)
	m.writeDial(sc.id, &url.URL{Scheme: sc.remoteAddr.Network(), Host: sc.remoteAddr.String()})

	select {
	case <-ctx.Done():
		go sc.CloseCause(ctx.Err())
		return nil, ctx.Err()
	case <-sc.dialing.Done():
		if sc.ctx.Err() != nil {
			return nil, sc.ctx.Err()
		}
		return sc, nil
	}
}

func (m *conn) clearSubConn(c *subConn) {
	m.subConnLock.Lock()
	defer m.subConnLock.Unlock()
	delete(m.subConns, c.id)
}
func (m *conn) writePack(packType PackType, id uint16) {
	m.writeChan <- msgPool.Get().(*writeMsg).Apply(packType, id, nil, nil)
}
func (m *conn) writeDial(id uint16, u *url.URL) {
	m.writeChan <- msgPool.Get().(*writeMsg).Apply(DIAL, id, []byte(u.String()), nil)
}
func (m *conn) writeData(ctx context.Context, id uint16, data []byte) (n int, err error) {
	replyTo := resultPool.Get().(chan writeResult)
	select {
	case m.writeChan <- msgPool.Get().(*writeMsg).Apply(DATA, id, data, replyTo):
		result := <-replyTo
		resultPool.Put(replyTo)
		return result.N, result.Error
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

var resultPool sync.Pool = sync.Pool{New: func() any { return make(chan writeResult) }}
var msgPool sync.Pool = sync.Pool{New: func() any { return &writeMsg{} }}

type writeResult struct {
	N     int
	Error error
}

type writeMsg struct {
	Type    PackType
	ID      uint16
	Data    []byte
	ReplyTo chan writeResult
}

func (m *writeMsg) Apply(Type PackType, ID uint16, Data []byte, ReplyTo chan writeResult) *writeMsg {
	m.Type, m.ID, m.Data, m.ReplyTo = Type, ID, Data, ReplyTo
	return m
}
func (m *writeMsg) Reset() *writeMsg {
	return m.Apply(NOOP, 0, nil, nil)
}
