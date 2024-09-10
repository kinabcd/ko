package socks

import (
	"fmt"
)

var ErrWrongProtocol error = fmt.Errorf("wrong protocol")
var ErrAuthFailed error = fmt.Errorf("auth failed")
var ErrAuthMethodNotSupported error = fmt.Errorf("auth method not supported")
var ErrCmdNotSupported error = fmt.Errorf("cmd not supported")
var ErrWrongFormat error = fmt.Errorf("wrong format")
var ErrBadRequest error = fmt.Errorf("bad request")

const (
	Version4 = 0x04
	Version5 = 0x05

	AddrTypeIPv4 = 0x01
	AddrTypeFQDN = 0x03
	AddrTypeIPv6 = 0x04

	CmdConnect byte = 0x01 // establishes an active-open forward proxy connection

	AuthMethodNotRequired         byte = 0x00 // no authentication required
	AuthMethodUsernamePassword    byte = 0x02 // use username/password
	AuthMethodNoAcceptableMethods byte = 0xff // no acceptable authentication methods

	StatusSucceeded          byte = 0x00
	StatusFailed             byte = 0x01
	StatusNetworkUnreachable byte = 0x03
)
