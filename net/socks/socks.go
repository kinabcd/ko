package socks

import (
	"fmt"
	"strconv"
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

	CmdConnect Command = 0x01 // establishes an active-open forward proxy connection
	cmdBind    Command = 0x02 // establishes a passive-open forward proxy connection

	AuthMethodNotRequired         AuthMethod = 0x00 // no authentication required
	AuthMethodUsernamePassword    AuthMethod = 0x02 // use username/password
	AuthMethodNoAcceptableMethods AuthMethod = 0xff // no acceptable authentication methods

	StatusSucceeded          Reply = 0x00
	StatusFailed             Reply = 0x01
	StatusNetworkUnreachable Reply = 0x03
)

// A Command represents a SOCKS command.
type Command int

func (cmd Command) String() string {
	switch cmd {
	case CmdConnect:
		return "socks connect"
	case cmdBind:
		return "socks bind"
	default:
		return "socks " + strconv.Itoa(int(cmd))
	}
}

// A Reply represents a SOCKS command reply code.
type Reply byte

func (code Reply) String() string {
	switch code {
	case StatusSucceeded:
		return "succeeded"
	case 0x01:
		return "general SOCKS server failure"
	case 0x02:
		return "connection not allowed by ruleset"
	case 0x03:
		return "network unreachable"
	case 0x04:
		return "host unreachable"
	case 0x05:
		return "connection refused"
	case 0x06:
		return "TTL expired"
	case 0x07:
		return "command not supported"
	case 0x08:
		return "address type not supported"
	default:
		return "unknown code: " + strconv.Itoa(int(code))
	}
}

// An AuthMethod represents a SOCKS authentication method.
type AuthMethod byte
