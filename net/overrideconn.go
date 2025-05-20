package net

import "net"

type OverrideListener struct {
	net.Listener
	OverrideLocalAddr  bool
	OverrideRemoteAddr bool
}

func (l *OverrideListener) Accept() (c net.Conn, e error) {
	c, e = l.Listener.Accept()
	if e != nil {
		return nil, e
	}
	oc := &OverrideConn{Conn: c}
	if l.OverrideLocalAddr {
		oc.OverrideLocalAddr = &OverrideAddr{OverrideNetwork: c.LocalAddr().Network(), OverrideAddress: c.LocalAddr().String()}
	}
	if l.OverrideRemoteAddr {
		oc.OverrideRemoteAddr = &OverrideAddr{OverrideNetwork: c.RemoteAddr().Network(), OverrideAddress: c.RemoteAddr().String()}
	}
	return oc, nil
}

type OverrideConn struct {
	net.Conn
	OverrideLocalAddr  net.Addr
	OverrideRemoteAddr net.Addr
}

func (c *OverrideConn) LocalAddr() net.Addr {
	if c.OverrideLocalAddr == nil {
		return c.Conn.LocalAddr()
	} else {
		return c.OverrideLocalAddr
	}
}
func (c *OverrideConn) RemoteAddr() net.Addr {
	if c.OverrideRemoteAddr == nil {
		return c.Conn.RemoteAddr()
	} else {
		return c.OverrideRemoteAddr
	}
}

type OverrideAddr struct {
	OverrideNetwork string
	OverrideAddress string
}

func (a *OverrideAddr) Network() string { return a.OverrideNetwork }
func (a *OverrideAddr) String() string  { return a.OverrideAddress }
