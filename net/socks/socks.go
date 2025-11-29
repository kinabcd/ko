package socks

import (
	"errors"
	"strconv"
)

var (
	ErrWrongProtocol          error = errors.New("wrong protocol")
	ErrAuthFailed             error = errors.New("auth failed")
	ErrAuthMethodNotSupported error = errors.New("auth method not supported")
	ErrCmdNotSupported        error = errors.New("cmd not supported")
	ErrWrongFormat            error = errors.New("wrong format")
	ErrBadRequest             error = errors.New("bad request")
	ErrUnknownAddressType     error = errors.New("unknown address type")
	ErrFQDNTooLong            error = errors.New("FQDN too long")
)

const (
	Version4 = 0x04
	Version5 = 0x05

	AddrTypeIPv4 = 0x01
	AddrTypeFQDN = 0x03
	AddrTypeIPv6 = 0x04

	CmdUnset   Command = 0x00
	CmdConnect Command = 0x01 // establishes an active-open forward proxy connection
	cmdBind    Command = 0x02 // establishes a passive-open forward proxy connection

	AuthMethodNotRequired         AuthMethod = 0x00 // no authentication required
	AuthMethodUsernamePassword    AuthMethod = 0x02 // use username/password
	AuthMethodNoAcceptableMethods AuthMethod = 0xff // no acceptable authentication methods

	StatusSucceeded          Reply = 0x00
	StatusFailed             Reply = 0x01
	StatusNetworkUnreachable Reply = 0x03

	AuthUsernamePasswordVersion = 0x01
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
