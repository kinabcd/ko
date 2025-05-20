package socks

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"slices"
	"time"

	koIo "github.com/kinabcd/ko/io"
	koNet "github.com/kinabcd/ko/net"
	koTime "github.com/kinabcd/ko/time"
)

type Request struct {
	Version int
	Address string
}

// A Server defines parameters for running an SOCKS4/SOCKS5 server.
// The zero value for Server is a valid configuration.
type Server struct {
	// Dialer specifies an optional ContextDialer.
	// If non-nil, it will be used on outbound.
	Dialer koNet.ContextDialer

	// Logger specifies an optional logger for errors.
	// If nil, log nothing.
	Logger *slog.Logger

	// Handle authorization. AuthMethodNotRequired if nil
	// If AuthHandler is not nil, SOCKS4(a) server will not serve.
	AuthHandler func(username, password string) bool

	// ConnContext optionally specifies a function that modifies
	// the context used for a new connection c. The provided ctx
	// is derived from the base context and has a ServerContextKey
	// value.
	ConnContext func(ctx context.Context, c net.Conn, req *Request) context.Context
}

func (srv *Server) Serve(l koNet.Listener) error {
	defer l.Close()
	baseCtx := context.Background()
	for {
		conn, err := l.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				srv.logW("Accept timeout", "err", err)
				koTime.SleepContext(baseCtx, 100*time.Millisecond)
				continue
			}
			return err
		}
		go func() {
			if srv.ServeConn(baseCtx, conn); err != nil {
				srv.logW("handle conn failed", "err", err)
			}
		}()
	}
}

func (srv *Server) getDialer() (dialer koNet.ContextDialer) {
	dialer = srv.Dialer
	if dialer == nil {
		dialer = &net.Dialer{}
	}
	return
}

func (p *Server) logD(msg string, args ...any) {
	if p.Logger != nil {
		p.Logger.Debug(msg, args...)
	}
}
func (p *Server) logW(msg string, args ...any) {
	if p.Logger != nil {
		p.Logger.Warn(msg, args...)
	}
}

func (srv *Server) ServeConn(ctx context.Context, conn net.Conn) error {
	defer conn.Close()
	if version, err := koIo.ReadByte(conn); err != nil {
		return err
	} else if version == Version4 {
		return srv.serveSOCKS4(ctx, conn)
	} else if version == Version5 {
		return srv.serveSOCKS5(ctx, conn)
	} else {
		return ErrWrongProtocol
	}
}

func (srv *Server) serveSOCKS4(ctx context.Context, conn net.Conn) (err error) {
	var address string
	var portByte [2]byte
	var ipByte [4]byte
	if address, portByte, ipByte, err = readSOCKS4Header(conn); err != nil {
		err = fmt.Errorf("wrong header: %w", err)
		return err
	}
	srv.logD("Connect", "proto", "SOCKS4", "address", address)
	if srv.AuthHandler != nil {
		return ErrAuthFailed
	}
	if srv.ConnContext != nil {
		ctx = srv.ConnContext(ctx, conn, &Request{
			Version: Version4,
			Address: address,
		})
	}
	var outConn net.Conn
	if outConn, err = srv.getDialer().DialContext(ctx, "tcp", address); err != nil {
		return fmt.Errorf("dial failed %w", err)
	}
	defer outConn.Close()
	if err = writeSOCKS4Response(conn, true, portByte, ipByte); err != nil {
		return fmt.Errorf("failed to response")
	}

	koIo.BidirectionalCopy(conn, outConn)
	return nil
}

func (srv *Server) serveSOCKS5(ctx context.Context, conn net.Conn) (err error) {
	var methods []byte
	if methods, err = readSOCKS5Header(conn); err != nil {
		err = fmt.Errorf("wrong header: %w", err)
		return
	}
	authRequired := srv.AuthHandler != nil
	if authRequired && slices.Contains(methods, AuthMethodUsernamePassword) {
		if err = writeSOCKS5AuthMethod(conn, AuthMethodUsernamePassword); err != nil {
			return
		}
		var account, password string
		if account, password, err = readSOCKS5AuthUsernamePassword(conn); err != nil {
			return
		}
		if !srv.AuthHandler(account, password) {
			err = ErrAuthFailed
			writeSOCKS5AuthResult(conn, false)
			return
		}
		if err = writeSOCKS5AuthResult(conn, true); err != nil {
			return
		}
	} else if !authRequired && slices.Contains(methods, AuthMethodNotRequired) {
		if err = writeSOCKS5AuthMethod(conn, AuthMethodNotRequired); err != nil {
			return
		}
	} else {
		writeSOCKS5AuthMethod(conn, AuthMethodNoAcceptableMethods)
		return
	}
	var address string
	if address, err = readSOCKS5Request(conn); err != nil {
		return
	}

	srv.logD("Connect", "proto", "SOCKS5", "address", address)
	if srv.ConnContext != nil {
		ctx = srv.ConnContext(ctx, conn, &Request{
			Version: Version5,
			Address: address,
		})
	}
	var outConn net.Conn
	if outConn, err = srv.getDialer().DialContext(ctx, "tcp", address); err != nil {
		writeSOCKS5Response(conn, StatusNetworkUnreachable)
		err = fmt.Errorf("dial failed: %w", err)
		return
	}
	defer outConn.Close()
	if err = writeSOCKS5Response(conn, StatusSucceeded); err != nil {
		err = fmt.Errorf("response failed: %w", err)
		return
	}

	koIo.BidirectionalCopy(conn, outConn)
	return nil
}
