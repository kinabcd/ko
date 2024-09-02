package net

import (
	"io"
	"net"
)

var (
	_ io.ReadWriteCloser = &PrefixConn{}
	_ io.WriterTo        = &PrefixConn{}
	_ io.ReaderFrom      = &PrefixConn{}
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

func (b *PrefixConn) WriteTo(w io.Writer) (n int64, err error) {
	var nn int64
	if b.Prefix != nil {
		nn, err = io.Copy(w, b.Prefix)
		n += nn
		if err != nil && err != io.EOF {
			return
		}
	}
	nn, err = io.Copy(w, b.Conn)
	n += nn
	return
}

func (b *PrefixConn) ReadFrom(r io.Reader) (n int64, err error) {
	return io.Copy(b.Conn, r)
}
