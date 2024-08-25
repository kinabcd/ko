package http

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"

	koIo "github.com/kinabcd/ko/io"
	koNet "github.com/kinabcd/ko/net"
)

// Hop-by-hop headers. These are removed when sent to the backend.
// http://www.w3.org/Protocols/rfc2616/rfc2616-sec13.html
var hopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te", // canonicalized version of "TE"
	"Trailers",
	"Transfer-Encoding",
	"Upgrade",
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func delHopHeaders(header http.Header) {
	for _, h := range hopHeaders {
		header.Del(h)
	}
}

func appendHostToXForwardHeader(header http.Header, host string) {
	// If we aren't the first proxy retain prior
	// X-Forwarded-For information as a comma+space
	// separated list and fold multiple headers into one.
	if prior, ok := header["X-Forwarded-For"]; ok {
		host = strings.Join(prior, ", ") + ", " + host
	}
	header.Set("X-Forwarded-For", host)
}

// A Server defines parameters for running an HTTP PROXY server.
// The zero value for Server is a valid configuration.
type ProxyServer struct {
	// Dialer specifies an optional ContextDialer.
	// If non-nil, it will be used in http.Transport of outbound client.
	Dialer koNet.ContextDialer

	// Logger specifies an optional logger for errors.
	// If nil, logging is done via the log package's standard logger.
	Logger *log.Logger

	// Log non-error messages if Verbose is true.
	Verbose bool

	// handle authorization. AuthMethodNotRequired if nil
	AuthHandler func(username, password string) bool
}

func (p *ProxyServer) Serve(l net.Listener) error {
	s := http.Server{Handler: p}
	return s.Serve(l)
}

func (p *ProxyServer) ServeHTTP(wr http.ResponseWriter, req *http.Request) {
	if p.AuthHandler != nil {
		pa := req.Header.Get("Proxy-Authorization")
		pau, pap, ok := DecodeBasicAuth(pa)
		ok = ok && p.AuthHandler(pau, pap)
		if !ok {
			wr.Header().Add("Proxy-Authenticate", "Basic")
			if p.Verbose {
				p.getLogger().Println("HttpProxy", http.StatusText(http.StatusProxyAuthRequired), pau, pap)
			}
			http.Error(wr, http.StatusText(http.StatusProxyAuthRequired), http.StatusProxyAuthRequired)
			return
		}
	}

	if req.Method == http.MethodConnect {
		p.serveConnect(wr, req)
	} else {
		p.serveOthers(wr, req)
	}
}

func (srv *ProxyServer) getLogger() *log.Logger {
	if srv.Logger != nil {
		return srv.Logger
	} else {
		return log.Default()
	}
}

func (p *ProxyServer) serveOthers(wr http.ResponseWriter, req *http.Request) {
	if p.Verbose {
		p.getLogger().Println("HttpProxy", req.Method, req.URL)
	}

	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		msg := "unsupported protocal scheme " + req.URL.Scheme
		http.Error(wr, msg, http.StatusBadRequest)
		p.getLogger().Println("HttpProxy", msg)
		return
	}

	client := &http.Client{}
	if p.Dialer != nil {
		client.Transport = &http.Transport{
			DialContext: p.Dialer.DialContext,
		}
	}

	//http: Request.RequestURI can't be set in client requests.
	//http://golang.org/src/pkg/net/http/client.go
	req.RequestURI = ""

	delHopHeaders(req.Header)

	if clientIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		appendHostToXForwardHeader(req.Header, clientIP)
	}

	resp, err := client.Do(req)
	if err != nil {
		http.Error(wr, "Server Error", http.StatusInternalServerError)
		p.getLogger().Println("HttpProxy", err)
		return
	}
	defer resp.Body.Close()

	delHopHeaders(resp.Header)

	copyHeader(wr.Header(), resp.Header)
	wr.WriteHeader(resp.StatusCode)
	io.Copy(wr, resp.Body)
	client.CloseIdleConnections()
}

func (p *ProxyServer) serveConnect(wr http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()
	if p.Verbose {
		p.getLogger().Println("HttpProxy", req.Method, req.URL.Host)
	}
	if hostname, port, err := net.SplitHostPort(req.URL.Host); err != nil || hostname == "" {
		wr.WriteHeader(http.StatusBadRequest)
		return
	} else if portInt, err := strconv.ParseInt(port, 10, 64); err != nil || portInt > 65535 || portInt < 1 {
		wr.WriteHeader(http.StatusBadRequest)
		return
	}
	dialer := p.Dialer
	if dialer == nil {
		dialer = &net.Dialer{}
	}

	if outConn, err := dialer.DialContext(context.Background(), "tcp", req.URL.Host); err == nil {
		defer outConn.Close()
		rc := http.NewResponseController(wr)

		conn, brf, err := rc.Hijack()
		if err != nil {
			wr.WriteHeader(http.StatusInternalServerError)
			p.getLogger().Println("HttpProxy hijack failed", err)
			return
		}
		defer conn.Close()
		(&http.Response{StatusCode: 200, ProtoMajor: req.ProtoMajor, ProtoMinor: req.ProtoMinor}).Write(conn)
		koIo.BidirectionalCopy(&koNet.PrefixConn{Prefix: brf.Reader, Conn: conn}, outConn)
	} else {
		wr.WriteHeader(http.StatusNotFound)
		p.getLogger().Printf("HttpProxy dial failed %v", err)
	}
}
