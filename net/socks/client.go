package socks

import (
	"context"
	"errors"
	"io"
	"net"
	"net/url"
	"strconv"
	"time"

	koNet "github.com/kinabcd/ko/net"
)

// An Addr represents a SOCKS-specific address.
// Either Name or IP is used exclusively.
type Addr struct {
	Name string // fully-qualified domain name
	IP   net.IP
	Port int
}

func (a *Addr) Network() string { return "socks" }

func (a *Addr) String() string {
	if a == nil {
		return "<nil>"
	}
	port := strconv.Itoa(a.Port)
	if a.IP == nil {
		return net.JoinHostPort(a.Name, port)
	}
	return net.JoinHostPort(a.IP.String(), port)
}

// A Conn represents a forward proxy connection.
type Conn struct {
	net.Conn

	boundAddr net.Addr
}

// BoundAddr returns the address assigned by the proxy server for
// connecting to the command target address from the proxy server.
func (c *Conn) BoundAddr() net.Addr {
	if c == nil {
		return nil
	}
	return c.boundAddr
}

// A Client holds SOCKS-specific options.
type Client struct {
	cmd Command // either CmdConnect or cmdBind

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
	if d.cmd == 0 {
		d.cmd = CmdConnect
	}
	dialer := d.Dialer
	if dialer == nil {
		dialer = &net.Dialer{}
	}
	if err := d.validateTarget(network, address); err != nil {
		proxy, dst, _ := d.pathAddrs(address)
		return nil, &net.OpError{Op: d.cmd.String(), Net: network, Source: proxy, Addr: dst, Err: err}
	}
	if ctx == nil {
		proxy, dst, _ := d.pathAddrs(address)
		return nil, &net.OpError{Op: d.cmd.String(), Net: network, Source: proxy, Addr: dst, Err: errors.New("nil context")}
	}
	c, err := dialer.DialContext(ctx, "tcp", d.ProxyUrl.Host)
	if err != nil {
		proxy, dst, _ := d.pathAddrs(address)
		return nil, &net.OpError{Op: d.cmd.String(), Net: network, Source: proxy, Addr: dst, Err: err}
	}
	a, err := d.connect(ctx, c, address)
	if err != nil {
		c.Close()
		proxy, dst, _ := d.pathAddrs(address)
		return nil, &net.OpError{Op: d.cmd.String(), Net: network, Source: proxy, Addr: dst, Err: err}
	}
	return &Conn{Conn: c, boundAddr: a}, nil
}

// DialWithConn initiates a connection from SOCKS server to the target
// network and address using the connection c that is already
// connected to the SOCKS server.
//
// It returns the connection's local address assigned by the SOCKS
// server.
func (d *Client) DialWithConn(ctx context.Context, c net.Conn, network, address string) (net.Addr, error) {
	if d.cmd == 0 {
		d.cmd = CmdConnect
	}

	if err := d.validateTarget(network, address); err != nil {
		proxy, dst, _ := d.pathAddrs(address)
		return nil, &net.OpError{Op: d.cmd.String(), Net: network, Source: proxy, Addr: dst, Err: err}
	}
	if ctx == nil {
		proxy, dst, _ := d.pathAddrs(address)
		return nil, &net.OpError{Op: d.cmd.String(), Net: network, Source: proxy, Addr: dst, Err: errors.New("nil context")}
	}
	a, err := d.connect(ctx, c, address)
	if err != nil {
		proxy, dst, _ := d.pathAddrs(address)
		return nil, &net.OpError{Op: d.cmd.String(), Net: network, Source: proxy, Addr: dst, Err: err}
	}
	return a, nil
}

func (d *Client) validateTarget(network, address string) error {
	switch network {
	case "tcp", "tcp6", "tcp4":
	default:
		return errors.New("network not implemented")
	}
	switch d.cmd {
	case CmdConnect, cmdBind:
	default:
		return errors.New("command not implemented")
	}
	return nil
}

func (d *Client) pathAddrs(address string) (proxy, dst net.Addr, err error) {
	proxyAddress := ""
	if d.ProxyUrl != nil {
		proxyAddress = d.ProxyUrl.Host
	}
	for i, s := range []string{proxyAddress, address} {
		host, port, err := splitHostPort(s)
		if err != nil {
			return nil, nil, err
		}
		a := &Addr{Port: port}
		a.IP = net.ParseIP(host)
		if a.IP == nil {
			a.Name = host
		}
		if i == 0 {
			proxy = a
		} else {
			dst = a
		}
	}
	return
}

const (
	authUsernamePasswordVersion = 0x01
	authStatusSucceeded         = 0x00
)

// usernamePassword are the credentials for the username/password
// authentication method.
type usernamePassword struct {
	Username string
	Password string
}

// Authenticate authenticates a pair of username and password with the
// proxy server.
func (up *usernamePassword) Authenticate(ctx context.Context, rw io.ReadWriter, auth AuthMethod) error {
	switch auth {
	case AuthMethodNotRequired:
		return nil
	case AuthMethodUsernamePassword:
		if len(up.Username) == 0 || len(up.Username) > 255 || len(up.Password) > 255 {
			return errors.New("invalid username/password")
		}
		b := []byte{authUsernamePasswordVersion}
		b = append(b, byte(len(up.Username)))
		b = append(b, up.Username...)
		b = append(b, byte(len(up.Password)))
		b = append(b, up.Password...)
		// TODO(mikio): handle IO deadlines and cancellation if
		// necessary
		if _, err := rw.Write(b); err != nil {
			return err
		}
		if _, err := io.ReadFull(rw, b[:2]); err != nil {
			return err
		}
		if b[0] != authUsernamePasswordVersion {
			return errors.New("invalid username/password version")
		}
		if b[1] != authStatusSucceeded {
			return errors.New("username/password authentication failed")
		}
		return nil
	}
	return errors.New("unsupported authentication method " + strconv.Itoa(int(auth)))
}

var (
	noDeadline   = time.Time{}
	aLongTimeAgo = time.Unix(1, 0)
)

func (d *Client) connect(ctx context.Context, c net.Conn, address string) (_ net.Addr, ctxErr error) {
	var authenticate func(context.Context, io.ReadWriter, AuthMethod) error = nil
	var authMethods []AuthMethod = []AuthMethod{}
	var user *url.Userinfo = nil
	if d.ProxyUrl != nil {
		user = d.ProxyUrl.User
	}
	if user != nil {
		p, _ := user.Password()
		up := usernamePassword{
			Username: user.Username(),
			Password: p,
		}
		authMethods = []AuthMethod{
			AuthMethodNotRequired,
			AuthMethodUsernamePassword,
		}
		authenticate = up.Authenticate
	}
	host, port, err := splitHostPort(address)
	if err != nil {
		return nil, err
	}
	if deadline, ok := ctx.Deadline(); ok && !deadline.IsZero() {
		c.SetDeadline(deadline)
		defer c.SetDeadline(noDeadline)
	}
	if ctx != context.Background() {
		errCh := make(chan error, 1)
		done := make(chan struct{})
		defer func() {
			close(done)
			if ctxErr == nil {
				ctxErr = <-errCh
			}
		}()
		go func() {
			select {
			case <-ctx.Done():
				c.SetDeadline(aLongTimeAgo)
				errCh <- ctx.Err()
			case <-done:
				errCh <- nil
			}
		}()
	}

	b := make([]byte, 0, 6+len(host)) // the size here is just an estimate
	b = append(b, Version5)
	if len(authMethods) == 0 || authenticate == nil {
		b = append(b, 1, byte(AuthMethodNotRequired))
	} else {
		ams := authMethods
		if len(ams) > 255 {
			return nil, errors.New("too many authentication methods")
		}
		b = append(b, byte(len(ams)))
		for _, am := range ams {
			b = append(b, byte(am))
		}
	}
	if _, ctxErr = c.Write(b); ctxErr != nil {
		return
	}

	if _, ctxErr = io.ReadFull(c, b[:2]); ctxErr != nil {
		return
	}
	if b[0] != Version5 {
		return nil, errors.New("unexpected protocol version " + strconv.Itoa(int(b[0])))
	}
	am := AuthMethod(b[1])
	if am == AuthMethodNoAcceptableMethods {
		return nil, errors.New("no acceptable authentication methods")
	}
	if authenticate != nil {
		if ctxErr = authenticate(ctx, c, am); ctxErr != nil {
			return
		}
	}

	b = b[:0]
	b = append(b, Version5, byte(d.cmd), 0)
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			b = append(b, AddrTypeIPv4)
			b = append(b, ip4...)
		} else if ip6 := ip.To16(); ip6 != nil {
			b = append(b, AddrTypeIPv6)
			b = append(b, ip6...)
		} else {
			return nil, errors.New("unknown address type")
		}
	} else {
		if len(host) > 255 {
			return nil, errors.New("FQDN too long")
		}
		b = append(b, AddrTypeFQDN)
		b = append(b, byte(len(host)))
		b = append(b, host...)
	}
	b = append(b, byte(port>>8), byte(port))
	if _, ctxErr = c.Write(b); ctxErr != nil {
		return
	}

	if _, ctxErr = io.ReadFull(c, b[:4]); ctxErr != nil {
		return
	}
	if b[0] != Version5 {
		return nil, errors.New("unexpected protocol version " + strconv.Itoa(int(b[0])))
	}
	if cmdErr := Reply(b[1]); cmdErr != StatusSucceeded {
		return nil, errors.New("unknown error " + cmdErr.String())
	}
	if b[2] != 0 {
		return nil, errors.New("non-zero reserved field")
	}
	l := 2
	var a Addr
	switch b[3] {
	case AddrTypeIPv4:
		l += net.IPv4len
		a.IP = make(net.IP, net.IPv4len)
	case AddrTypeIPv6:
		l += net.IPv6len
		a.IP = make(net.IP, net.IPv6len)
	case AddrTypeFQDN:
		if _, err := io.ReadFull(c, b[:1]); err != nil {
			return nil, err
		}
		l += int(b[0])
	default:
		return nil, errors.New("unknown address type " + strconv.Itoa(int(b[3])))
	}
	if cap(b) < l {
		b = make([]byte, l)
	} else {
		b = b[:l]
	}
	if _, ctxErr = io.ReadFull(c, b); ctxErr != nil {
		return
	}
	if a.IP != nil {
		copy(a.IP, b)
	} else {
		a.Name = string(b[:len(b)-2])
	}
	a.Port = int(b[len(b)-2])<<8 | int(b[len(b)-1])
	return &a, nil
}

func splitHostPort(address string) (string, int, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", 0, err
	}
	portnum, err := strconv.Atoi(port)
	if err != nil {
		return "", 0, err
	}
	if 1 > portnum || portnum > 0xffff {
		return "", 0, errors.New("port number out of range " + port)
	}
	return host, portnum, nil
}
