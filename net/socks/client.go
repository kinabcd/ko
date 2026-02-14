package socks

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	koNet "github.com/kinabcd/ko/net"
)

// A Client holds SOCKS-specific options.
type Client struct {
	Cmd Command // either CmdConnect or cmdBind

	ProxyUrl *url.URL

	// Dialer specifies an optional dial function with context for
	// creating connections for requests.
	//
	// If Dialer is nil, &net.Dialer{} is used.
	Dialer koNet.ContextDialer
}

// DialContext connects to the provided address on the provided
// network.
//
// The returned error value may be a net.OpError. When the Op field of
// net.OpError contains "socks", the Source field contains a proxy
// server address and the Addr field contains a command target
// address.
//
// See func Dial of the net package of standard library for a
// description of the network and address parameters.
func (d *Client) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	d.fillDefault()
	if err := d.validateParams(ctx, network, address); err != nil {
		return nil, err
	}
	if d.ProxyUrl.Host == "" {
		return nil, errors.New("invalid ProxyUrl " + d.ProxyUrl.String())
	}
	c, err := d.Dialer.DialContext(ctx, "tcp", d.ProxyUrl.Host)
	if err != nil {
		return nil, fmt.Errorf("failed to connect proxy server %s: %w"+d.ProxyUrl.String(), err)
	}
	a, err := d.connect(ctx, c, address)
	if err != nil {
		return nil, d.newOpError(network, address, err)
	}
	return koNet.DecorateConn(c).WithLocalAddr(a), nil
}

// DialWithConn initiates a connection from SOCKS server to the target
// network and address using the connection c that is already
// connected to the SOCKS server.
//
// It returns the connection's local address assigned by the SOCKS
// server.
func (d *Client) DialWithConn(ctx context.Context, c net.Conn, network, address string) (net.Addr, error) {
	d.fillDefault()
	if err := d.validateParams(ctx, network, address); err != nil {
		return nil, err
	}
	a, err := d.connect(ctx, c, address)
	if err != nil {
		return nil, d.newOpError(network, address, err)
	}
	return a, err
}

func (d *Client) newOpError(network, address string, err error) error {
	proxy := koNet.NewStaticAddr(d.ProxyUrl.Scheme, d.ProxyUrl.Host)
	dst := koNet.NewStaticAddr(network, address)
	return &net.OpError{Op: d.Cmd.String(), Net: network, Source: proxy, Addr: dst, Err: err}
}
func (d *Client) fillDefault() {
	if d.Cmd == CmdUnset {
		d.Cmd = CmdConnect
	}
	if d.Dialer == nil {
		d.Dialer = &net.Dialer{}
	}
	if d.ProxyUrl == nil {
		d.ProxyUrl = &url.URL{}
	}
}
func (d *Client) validateParams(ctx context.Context, network, address string) error {
	if ctx == nil {
		return errors.New("nil context")
	}
	switch network {
	case "tcp", "tcp6", "tcp4":
	default:
		return errors.New("network not implemented")
	}
	switch d.Cmd {
	case CmdConnect, cmdBind:
	default:
		return errors.New("command not implemented")
	}
	return nil
}

func (d *Client) connect(ctx context.Context, c net.Conn, address string) (net.Addr, error) {
	e := func(err error) (net.Addr, error) {
		c.Close()
		return nil, err
	}
	var user *url.Userinfo = d.ProxyUrl.User
	if deadline, ok := ctx.Deadline(); ok && !deadline.IsZero() {
		c.SetDeadline(deadline)
		defer c.SetDeadline(time.Time{})
	}
	if ctx != context.Background() {
		done := make(chan struct{})
		defer close(done)
		go func() {
			select {
			case <-ctx.Done():
				c.Close()
			case <-done:
			}
		}()
	}

	ams := []AuthMethod{AuthMethodNotRequired}
	if user != nil {
		ams = append(ams, AuthMethodUsernamePassword)
	}
	if err := writeSOCKS5Header(c, ams); err != nil {
		return e(err)
	}

	if am, err := readSOCKS5AuthMethod(c); err != nil {
		return e(err)
	} else if am == AuthMethodNoAcceptableMethods {
		return e(errors.New("no acceptable authentication methods"))
	} else if am == AuthMethodUsernamePassword {
		if user == nil {
			return e(errors.New("unexpected authentication username/password"))
		}

		username := user.Username()
		password, _ := user.Password()
		if err := writeSOCKS5AuthUsernamePassword(c, username, password); err != nil {
			return e(err)
		}
		if err := readSOCKS5AuthResult(c); err != nil {
			return e(err)
		}
	} else if am != AuthMethodNotRequired {
		return e(errors.New("unsupported authentication method " + strconv.Itoa(int(am))))
	}

	if err := writeSOCKS5Request(c, d.Cmd, address); err != nil {
		return e(err)
	}

	resAddress, err := readSOCKS5Response(c)
	if err != nil {
		return e(err)
	}
	return koNet.NewStaticAddr("socks", resAddress), nil
}
