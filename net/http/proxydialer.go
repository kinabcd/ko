package http

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"

	koNet "github.com/kinabcd/ko/net"
)

// ProxyDialer is a HTTP/HTTPS connect proxy.
type ProxyDialer struct {
	ProxyUrl *url.URL

	// Dialer specifies an optional dial function with context for
	// creating connections for requests.
	//
	// If Dialer is nil, &net.Dialer{} is used.
	Dialer koNet.ContextDialer
}

func (s *ProxyDialer) DialContext(ctx context.Context, network, addr string) (conn net.Conn, err error) {
	dialer := s.Dialer
	if dialer == nil {
		dialer = &net.Dialer{}
	}
	proxyURL := s.ProxyUrl
	if proxyURL == nil {
		err = errors.New("proxy url not set")
		return
	}

	// HACK. http.ReadRequest also does this.
	var reqURL *url.URL
	if reqURL, err = url.Parse("http://" + addr); err != nil {
		return
	}
	reqURL.Scheme = ""

	var req *http.Request
	if req, err = http.NewRequest("CONNECT", reqURL.String(), nil); err != nil {
		return
	}
	req.Close = false
	if proxyURL.User != nil {
		username := proxyURL.User.Username()
		password, _ := proxyURL.User.Password()
		req.Header.Add("Proxy-Authorization", EncodeBasicAuth(username, password))
	}

	// Dial and create the https client connection.
	if conn, err = dialer.DialContext(ctx, "tcp", proxyURL.Host); err != nil {
		return nil, err
	}

	if proxyURL.Scheme == "https" {
		var host string
		if host, _, err = net.SplitHostPort(proxyURL.Host); err != nil {
			return
		}
		tlsConfig := &tls.Config{}
		tlsConfig.ServerName = host
		tlsConfig.InsecureSkipVerify = proxyURL.Query().Has("insecure")
		conn = tls.Client(conn, tlsConfig)
	}

	if err = req.Write(conn); err == nil {
		var resp *http.Response
		if resp, err = http.ReadResponse(bufio.NewReader(conn), req); err == nil {
			if resp.StatusCode == 200 {
				return
			}
			err = fmt.Errorf("connect server using proxy error, statusCode %d", resp.StatusCode)
		}
	}
	conn.Close()
	return
}
