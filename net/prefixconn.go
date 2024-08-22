package net

import (
	"io"
	"net"
)

// PrefixConn will first read Prefix until EOF, and then read the content from Conn.
type PrefixConn struct {
	net.Conn
	Prefix io.Reader
}

func (b *PrefixConn) Read(p []byte) (n int, err error) {
	if b.Prefix != nil {
		n, err = b.Prefix.Read(p)
		if err == nil {
			return
		} else if err == io.EOF {
			b.Prefix = nil
		}
	}
	return b.Conn.Read(p)
}
