package httpproxy

import (
	"compress/gzip"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	koIo "github.com/kinabcd/ko/io"
	koNet "github.com/kinabcd/ko/net"
	koHttp "github.com/kinabcd/ko/net/http"
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

func getXForwardedFor(header http.Header) []string {
	if prior, ok := header["X-Forwarded-For"]; ok {
		hostsStr := strings.Join(prior, ",")
		hosts := strings.Split(hostsStr, ",")
		for i, host := range hosts {
			hosts[i] = strings.Trim(host, " ")
		}
		return hosts
	}
	return []string{}
}

func appendHostToXForwardHeader(header http.Header, host string) {
	// If we aren't the first proxy retain prior
	// X-Forwarded-For information as a comma+space
	// separated list and fold multiple headers into one.
	hosts := getXForwardedFor(header)
	hosts = append(hosts, host)
	header.Set("X-Forwarded-For", strings.Join(hosts, ", "))
}

// A Server defines parameters for running an HTTP PROXY server.
// The zero value for Server is a valid configuration.
type Server struct {
	// Dialer specifies an optional ContextDialer.
	// If non-nil, it will be used in http.Transport of outbound client.
	Dialer koNet.ContextDialer

	// Logger specifies an optional logger for errors.
	// If nil, log nothing
	Logger *slog.Logger

	// AuthContext handles user authentication for a proxy.
	//
	// It returns a boolean indicating whether the provided username and password are
	// valid. If true, a non-nil Context must also be returned, which will be used
	// for dialing. The original context is passed in via the 'ctx' parameter.
	//
	// Authentication is optional. If AuthContext is set, it takes precedence over AuthHandler.
	AuthContext func(ctx context.Context, username, password string) (bool, context.Context)

	// AuthHandler authenticates a user based on the provided username and password,
	// returning true on success. This handler is only used if AuthContext is nil.
	AuthHandler func(username, password string) bool

	// Handle proxy request.
	// Return true if proxy request is allowed, or false for forwarding to Fallback
	// If RequestHandler is nil, all requests are allowed
	RequestHandler func(host string) bool

	// Call fallback if the request is not proxy request
	Fallback http.Handler

	client *http.Client

	// ProxyCompression enables Gzip or Brotli compression on proxied responses.
	// Set to true to automatically compress responses from the backend server
	// (e.g., HTML, CSS, JSON) if the client supports it.
	ProxyCompression bool
}

func (p *Server) Serve(l net.Listener) error {
	s := http.Server{Handler: p}
	return s.Serve(l)
}

func (p *Server) ServeHTTP(wr http.ResponseWriter, req *http.Request) {
	if !p.isAllowedProxyRequest(req) {
		if p.Fallback != nil {
			p.Fallback.ServeHTTP(wr, req)
		} else {
			wr.WriteHeader(http.StatusForbidden)
		}
		return
	}
	authContext := p.AuthContext
	if authContext == nil && p.AuthHandler != nil {
		authContext = func(ctx context.Context, username, password string) (bool, context.Context) {
			return p.AuthHandler(username, password), ctx
		}
	}
	if authContext != nil {
		pa := req.Header.Get("Proxy-Authorization")
		pau, pap, ok := koHttp.DecodeBasicAuth(pa)
		if ok {
			var ctx context.Context
			ok, ctx = authContext(req.Context(), pau, pap)
			req = req.WithContext(ctx)
		}
		if !ok {
			wr.Header().Add("Proxy-Authenticate", "Basic")
			p.logD("auth failed", slog.String("user", pau), slog.String("pass", pap))
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

func (p *Server) isAllowedProxyRequest(req *http.Request) bool {
	var hostport string
	if req.Method == http.MethodConnect {
		hostport = req.RequestURI
	} else {
		if req.ProtoMajor == 1 && req.URL.Host == "" {
			// For HTTP/1.*, req.URL.Host must be target host if it is proxy request.
			return false
		} else if req.ProtoMajor == 2 {
			// For HTTP/2, req.URL.Host is always empty. https://github.com/golang/go/issues/68365
			// We can't tell whether it is a Proxy request just from http.Request
		}
		var isHttps bool
		if req.ProtoMajor == 1 {
			isHttps = req.URL.Scheme == "https"
		} else if req.ProtoMajor == 2 {
			isHttps = req.TLS != nil // nil if not scheme https. https://cs.opensource.google/go/x/net/+/refs/tags/v0.28.0:http2/server.go;l=2250
		}

		// Host must have a port if it is not http on 80 or https on 443.
		if _, _, err := net.SplitHostPort(req.Host); err == nil {
			hostport = req.Host
		} else if isHttps {
			hostport = req.Host + ":443"
		} else {
			hostport = req.Host + ":80"
		}
	}

	if p.RequestHandler != nil && !p.RequestHandler(hostport) {
		return false
	}

	if clientIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		// Request loop.
		if hosts := getXForwardedFor(req.Header); len(hosts) > 0 && hosts[len(hosts)-1] == clientIP {
			return false
		}
	}
	return true
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

func (p *Server) serveOthers(wr http.ResponseWriter, req *http.Request) {
	if req.URL.Scheme == "" {
		req.URL.Scheme = "http"
	}
	if req.URL.Host == "" {
		req.URL.Host = req.Host
	}
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		msg := "unsupported protocal scheme " + req.URL.Scheme
		http.Error(wr, msg, http.StatusBadRequest)
		p.logW(msg)
		return
	}
	p.logD(req.Method, slog.Any("url", req.URL), slog.String("proto", req.Proto))

	dialContext := koNet.DialContextFunc(nil)
	if p.Dialer != nil {
		dialContext = p.Dialer.DialContext
	}
	if p.client == nil {
		p.client = &http.Client{
			Transport: &http.Transport{
				DialContext:     dialContext,
				IdleConnTimeout: 5 * time.Minute,
			},
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Jar: nil,
		}
	}

	//http: Request.RequestURI can't be set in client requests.
	//http://golang.org/src/pkg/net/http/client.go
	req.RequestURI = ""

	delHopHeaders(req.Header)

	if clientIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		appendHostToXForwardHeader(req.Header, clientIP)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		http.Error(wr, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		p.logW(err.Error())
		return
	}
	defer resp.Body.Close()

	delHopHeaders(resp.Header)

	var resBody io.ReadCloser = resp.Body
	var bodyWriter io.Writer = wr

	isEventStream := resp.Header.Get("Content-Type") == "text/event-stream"
	isClientTransferChunkAllowed := !(req.ProtoMajor == 1 && req.ProtoMinor == 0) && !isEventStream
	acceptEncoding := strings.Split(strings.Join(req.Header.Values("Accept-Encoding"), ","), ",")
	for i := range acceptEncoding {
		acceptEncoding[i] = strings.TrimSpace(acceptEncoding[i])
	}
	if p.ProxyCompression && isClientTransferChunkAllowed &&
		resp.Header.Get("Content-Encoding") == "" && slices.Contains(acceptEncoding, "gzip") {
		// Client accepts gzip, but server does not send gzip. compress it.
		resp.Header.Set("Content-Encoding", "gzip")
		resp.Header.Del("Content-Length")
		gzipWriter := gzip.NewWriter(wr)
		defer gzipWriter.Close()
		bodyWriter = gzipWriter
	}

	copyHeader(wr.Header(), resp.Header)
	wr.WriteHeader(resp.StatusCode)
	if isEventStream {
		if f, ok := wr.(http.Flusher); ok {
			f.Flush()
			bodyWriter = &flushWriter{Writer: bodyWriter, Flusher: f}
		}
	}

	io.Copy(bodyWriter, resBody)
}

func (p *Server) serveConnect(wr http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()
	p.logD(req.Method, slog.Any("url", req.RequestURI), slog.String("proto", req.Proto))
	if hostname, port, err := net.SplitHostPort(req.RequestURI); err != nil || hostname == "" {
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

	if outConn, err := dialer.DialContext(req.Context(), "tcp", req.RequestURI); err == nil {
		defer outConn.Close()
		rc := http.NewResponseController(wr)
		if req.ProtoMajor >= 2 {
			wr.WriteHeader(200)
			rc.Flush()
			go func() {
				io.Copy(outConn, req.Body)
				outConn.Close()
				req.Body.Close()
			}()
			io.Copy(&flushWriter{Writer: wr, ResponseController: rc}, outConn)
			return
		}
		rc.EnableFullDuplex()
		conn, brf, err := rc.Hijack()
		if err != nil {
			wr.WriteHeader(http.StatusInternalServerError)
			p.logW("hijack failed", slog.Any("err", err))
			return
		}
		defer conn.Close()
		err = (&http.Response{StatusCode: 200, ProtoMajor: req.ProtoMajor, ProtoMinor: req.ProtoMinor}).Write(conn)
		if err != nil {
			p.logW("response failed", slog.Any("err", err))
		}
		koIo.BidirectionalCopy(&koNet.PrefixConn{Prefix: brf.Reader, Conn: conn}, outConn)
	} else {
		wr.WriteHeader(http.StatusNotFound)
		p.logW("dial failed", slog.Any("err", err))
	}
}

type flushWriter struct {
	io.Writer
	*http.ResponseController
	http.Flusher
}

func (f *flushWriter) Write(p []byte) (n int, err error) {
	n, err = f.Writer.Write(p)
	if err != nil {
		return
	}
	if f.ResponseController != nil {
		err = f.ResponseController.Flush()
	} else if f.Flusher != nil {
		f.Flusher.Flush()
	}
	return
}
