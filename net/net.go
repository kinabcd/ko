package net

import (
	"context"
	"net"
)

// A ContextDialer dials using a context.
type ContextDialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// A Dialer is a means to establish a connection.
// Custom dialers should also implement ContextDialer.
type Dialer interface {
	// Dial connects to the given address
	Dial(network, addr string) (c net.Conn, err error)
}
type Listener net.Listener

type Forwarder interface {
	// Forward() a conn and someone may Accept() it from Listener
	Forward(ctx context.Context, c net.Conn) error
}
