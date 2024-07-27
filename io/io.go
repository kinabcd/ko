package io

import "io"

// Bind establishes a bidirectional data transfer between two connections.
// Two connections will be closed if anyone is closed.
func BidirectionalCopy(conn1, conn2 io.ReadWriteCloser) {
	go func() {
		io.Copy(conn1, conn2)
		conn1.Close()
		conn2.Close()
	}()
	io.Copy(conn2, conn1)
	conn2.Close()
	conn1.Close()
}
