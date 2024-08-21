package socks

import (
	"context"
	"fmt"
	"log"
	"net"
	"slices"
	"time"

	koIo "github.com/kinabcd/ko/io"
	koNet "github.com/kinabcd/ko/net"
	koTime "github.com/kinabcd/ko/time"
)

type Server struct {
	Logger  *log.Logger
	Dialer  koNet.ContextDialer
	Verbose bool

	// handle authorization. AuthMethodNotRequired if nil
	AuthHandler func(username, password string) bool
}

func (srv *Server) Serve(l koNet.Listener) error {
	if srv.Logger == nil {
		srv.Logger = log.Default()
	}
	defer l.Close()
	baseCtx := context.Background()
	for {
		conn, err := l.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				srv.getLogger().Printf("SOCKS: Accept error: %v; retrying in 100ms", err)
				koTime.SleepContext(baseCtx, 100*time.Millisecond)
				continue
			}
			return err
		}
		go func() {
			if srv.ServeSOCKS(conn); err != nil {
				srv.getLogger().Printf("SOCKS: %v\n", err)
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

func (srv *Server) getLogger() *log.Logger {
	if srv.Logger != nil {
		return srv.Logger
	} else {
		return log.Default()
	}
}

func (srv *Server) ServeSOCKS(conn net.Conn) error {
	defer conn.Close()
	if version, err := koIo.ReadByte(conn); err != nil {
		return err
	} else if version == Version4 {
		return srv.serveSOCKS4(conn)
	} else if version == Version5 {
		return srv.serveSOCKS5(conn)
	} else {
		return ErrWrongProtocol
	}
}

func (srv *Server) serveSOCKS4(conn net.Conn) (err error) {
	var address string
	var portByte [2]byte
	var ipByte [4]byte
	if address, portByte, ipByte, err = readSOCKS4Header(conn); err != nil {
		err = fmt.Errorf("wrong header: %w", err)
		return err
	}
	if srv.Verbose {
		srv.getLogger().Println("SOCKS4", address)
	}
	if srv.AuthHandler != nil {
		return ErrAuthFailed
	}
	var outConn net.Conn
	if outConn, err = srv.getDialer().DialContext(context.Background(), "tcp", address); err != nil {
		return fmt.Errorf("dial failed %w", err)
	}
	defer outConn.Close()
	if err = writeSOCKS4Response(conn, true, portByte, ipByte); err != nil {
		return fmt.Errorf("failed to response")
	}

	koIo.BidirectionalCopy(conn, outConn)
	return nil
}

func (srv *Server) serveSOCKS5(conn net.Conn) (err error) {
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

	if srv.Verbose {
		srv.getLogger().Println("SOCKS5", address)
	}
	var outConn net.Conn
	if outConn, err = srv.getDialer().DialContext(context.Background(), "tcp", address); err != nil {
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
