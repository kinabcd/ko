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

type DialFunc func(network, address string) (net.Conn, error)

func (f DialFunc) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return f(network, address)
}
func (f DialFunc) Dial(network, address string) (net.Conn, error) {
	return f(network, address)
}

type DialContextFunc func(ctx context.Context, network, address string) (net.Conn, error)

func (f DialContextFunc) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return f(ctx, network, address)
}
func (f DialContextFunc) Dial(network, address string) (net.Conn, error) {
	return f(context.Background(), network, address)
}

type Listener net.Listener

type Forwarder interface {
	// Forward() a conn and someone may Accept() it from Listener
	Forward(ctx context.Context, c net.Conn) error
}

type ForwarderFunc func(ctx context.Context, c net.Conn) error

func (f ForwarderFunc) Forward(ctx context.Context, c net.Conn) error {
	return f(ctx, c)
}
