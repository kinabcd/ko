package mio

import "net"

var (
	_ net.Addr = &subAddr{}
)

type subAddr struct {
	network string
	address string
}

func (a *subAddr) Network() string {
	return a.network
}

func (a *subAddr) String() string {
	return a.address
}
