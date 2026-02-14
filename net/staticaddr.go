package net

import "net"

var _ net.Addr = &staticAddr{}

type staticAddr struct {
	network string
	string  string
}

func NewStaticAddr(network, addr string) net.Addr {
	return &staticAddr{
		network: network,
		string:  addr,
	}
}

func (a *staticAddr) Network() string { return a.network }
func (a *staticAddr) String() string  { return a.string }
