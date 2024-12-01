package socks

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"net"
	"net/url"
	"slices"
	"strconv"

	koIo "github.com/kinabcd/ko/io"
	koNet "github.com/kinabcd/ko/net"
)

type Client struct {
	ProxyUrl *url.URL

	// Dialer specifies an optional dial function with context for
	// creating connections for requests.
	//
	// If Dialer is nil, &net.Dialer{} is used.
	Dialer koNet.ContextDialer
}

func (s *Client) DialContext(ctx context.Context, network, addr string) (conn net.Conn, err error) {
	dialer := s.Dialer
	if dialer == nil {
		dialer = &net.Dialer{}
	}
	if s.ProxyUrl == nil {
		err = errors.New("proxy url not set")
		return
	}
	if slices.Contains([]string{"socks5h", "socks5", "socks5+tls"}, s.ProxyUrl.Scheme) {
		if conn, err = dialer.DialContext(ctx, "tcp", s.ProxyUrl.Host); err != nil {
			return
		}
		if s.ProxyUrl.Scheme == "socks5+tls" {
			conn = tls.Client(conn, &tls.Config{
				ServerName:         s.ProxyUrl.Hostname(),
				InsecureSkipVerify: s.ProxyUrl.Query().Has("insecure"),
			})
		}
		return SOCKS5Client(ctx, conn, network, addr, s.ProxyUrl.User)
	}

	err = errors.New("unknown scheme " + s.ProxyUrl.Scheme)
	return
}

func SOCKS5Client(ctx context.Context, conn net.Conn, network, addr string, user *url.Userinfo) (net.Conn, error) {
	e := func(err error) (net.Conn, error) {
		conn.Close()
		return nil, err
	}
	var err error
	var host, port string
	if host, port, err = net.SplitHostPort(addr); err != nil {
		return e(err)
	}
	var portInt uint64
	if portInt, err = strconv.ParseUint(port, 10, 16); err != nil {
		return e(err)
	}
	portByte := []byte{0, 0}
	binary.BigEndian.PutUint16(portByte, uint16(portInt))
	hasAuth := user != nil
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-done:
		}
	}()
	header := []byte{Version5, 1, AuthMethodNotRequired}
	if hasAuth {
		header = []byte{Version5, 2, AuthMethodNotRequired, AuthMethodUsernamePassword}
	}
	if _, err = conn.Write(header); err != nil {
		return e(err)
	}
	var resB []byte
	if resB, err = koIo.ReadN(conn, 2); err != nil {
		return e(err)
	} else if resB[0] != Version5 {
		return e(ErrWrongProtocol)
	}
	if resB[1] != AuthMethodNotRequired {
		if resB[1] == AuthMethodUsernamePassword {
			username := user.Username()
			password, _ := user.Password()
			_, _ = conn.Write([]byte{1})
			_ = koIo.WritePascalString(conn, username)
			if err = koIo.WritePascalString(conn, password); err != nil {
				return e(err)
			}
			if resB, err = koIo.ReadN(conn, 2); err != nil {
				return e(err)
			}
			if resB[1] != StatusSucceeded {
				return e(ErrAuthFailed)
			}
		} else {
			return e(ErrAuthMethodNotSupported)
		}
	}

	conn.Write([]byte{Version5, CmdConnect, 0, AddrTypeFQDN})
	koIo.WritePascalString(conn, host)
	if _, err = conn.Write(portByte); err != nil {
		return e(err)
	}

	if resB, err = koIo.ReadN(conn, 4); err != nil {
		return e(err)
	}
	if resB[1] != StatusSucceeded {
		return e(errors.New("connect failed, status " + string(resB[1])))
	}
	if _, err = readSOCKS5Addr(conn, resB[3]); err != nil {
		return e(err)
	}

	return conn, nil
}
