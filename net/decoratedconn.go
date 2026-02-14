package net

import (
	"bytes"
	"io"
	"net"
)

var (
	_ io.Reader     = (*decoratedConn)(nil)
	_ io.Writer     = (*decoratedConn)(nil)
	_ io.ReaderFrom = (*decoratedConn)(nil)
	_ io.WriterTo   = (*decoratedConn)(nil)
)

type decoratedConn struct {
	net.Conn
	localAddr  func() net.Addr
	remoteAddr func() net.Addr
	reader     io.Reader
	writer     io.Writer
}

func (c *decoratedConn) WithLocalAddr(addr net.Addr) *decoratedConn {
	newOc := *c
	newOc.localAddr = func() net.Addr { return addr }
	return &newOc
}

func (c *decoratedConn) WithRemoteAddr(addr net.Addr) *decoratedConn {
	newOc := *c
	newOc.remoteAddr = func() net.Addr { return addr }
	return &newOc
}

func (c *decoratedConn) WithPrefixBytes(buffer []byte) *decoratedConn {
	return c.WithPrefixReader(bytes.NewReader(buffer))
}

func (c *decoratedConn) WithPrefixReader(reader io.Reader) *decoratedConn {
	newOc := *c
	newOc.reader = io.MultiReader(reader, newOc.reader)
	return &newOc
}

func (c *decoratedConn) LocalAddr() net.Addr {
	return c.localAddr()
}

func (c *decoratedConn) RemoteAddr() net.Addr {
	return c.remoteAddr()
}

func (c *decoratedConn) Read(b []byte) (int, error) {
	return c.reader.Read(b)
}

func (c *decoratedConn) WriteTo(w io.Writer) (int64, error) {
	return io.Copy(w, c.reader)
}

func (c *decoratedConn) Write(b []byte) (int, error) {
	return c.writer.Write(b)
}

func (c *decoratedConn) ReadFrom(r io.Reader) (int64, error) {
	return io.Copy(c.writer, r)
}

func DecorateConn(c net.Conn) *decoratedConn {
	if oc, ok := c.(*decoratedConn); ok {
		newOc := *oc
		return &newOc
	}
	return &decoratedConn{
		Conn: c,

		localAddr:  c.LocalAddr,
		remoteAddr: c.RemoteAddr,

		reader: c,
		writer: c,
	}
}
