package http_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"testing"

	koNet "github.com/kinabcd/ko/net"
	koHttp "github.com/kinabcd/ko/net/http"
	koTesting "github.com/kinabcd/ko/testing"
)

var (
	tlsCert tls.Certificate
	certPem []byte
	keyPem  []byte
)

func init() {
	certPem = []byte(`-----BEGIN CERTIFICATE-----
MIIBhTCCASugAwIBAgIQIRi6zePL6mKjOipn+dNuaTAKBggqhkjOPQQDAjASMRAw
DgYDVQQKEwdBY21lIENvMB4XDTE3MTAyMDE5NDMwNloXDTE4MTAyMDE5NDMwNlow
EjEQMA4GA1UEChMHQWNtZSBDbzBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABD0d
7VNhbWvZLWPuj/RtHFjvtJBEwOkhbN/BnnE8rnZR8+sbwnc/KhCk3FhnpHZnQz7B
5aETbbIgmuvewdjvSBSjYzBhMA4GA1UdDwEB/wQEAwICpDATBgNVHSUEDDAKBggr
BgEFBQcDATAPBgNVHRMBAf8EBTADAQH/MCkGA1UdEQQiMCCCDmxvY2FsaG9zdDo1
NDUzgg4xMjcuMC4wLjE6NTQ1MzAKBggqhkjOPQQDAgNIADBFAiEA2zpJEPQyz6/l
Wf86aX6PepsntZv2GYlA5UpabfT2EZICICpJ5h/iI+i341gBmLiAFQOyTDT+/wQc
6MF9+Yw1Yy0t
-----END CERTIFICATE-----`)
	keyPem = []byte(`-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIIrYSSNQFaA2Hwf1duRSxKtLYX5CB04fSeQ6tF1aY/PuoAoGCCqGSM49
AwEHoUQDQgAEPR3tU2Fta9ktY+6P9G0cWO+0kETA6SFs38GecTyudlHz6xvCdz8q
EKTcWGekdmdDPsHloRNtsiCa697B2O9IFA==
-----END EC PRIVATE KEY-----`)
	tlsCert, _ = tls.X509KeyPair(certPem, keyPem)
}

type echoHttpDialer struct{}

func (record *echoHttpDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	c1, c2 := net.Pipe()
	if _, port, _ := net.SplitHostPort(address); port == "443" {
		c2 = tls.Server(c2, &tls.Config{Certificates: []tls.Certificate{tlsCert}})
	}
	go func() {
		r, _ := http.ReadRequest(bufio.NewReader(c2))
		(&http.Response{
			StatusCode:    200,
			ProtoMajor:    r.ProtoMajor,
			ProtoMinor:    r.ProtoMinor,
			Body:          io.NopCloser(bytes.NewBufferString(address)),
			ContentLength: int64(len(address)),
		}).Write(c2)
	}()
	return c1, nil
}
func TestProxyServer(t *testing.T) {
	server := &koHttp.ProxyServer{
		Dialer: &echoHttpDialer{},
		Logger: slog.Default(),
	}
	lp := koNet.ListenPipe()
	go server.Serve(lp)

	client := http.Client{
		Transport: &http.Transport{
			Proxy: func(r *http.Request) (*url.URL, error) {
				return url.Parse("http://127.0.0.1:1080")
			},
			DialContext:     lp.DialContext,
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	res, _ := client.Get("http://example.tw:9999/aaa/bbb?A=r#123")
	body, _ := io.ReadAll(res.Body)
	koTesting.AssertEquals(t, "example.tw:9999", string(body))
	res, _ = client.Get("https://example.tw/")
	body, _ = io.ReadAll(res.Body)
	koTesting.AssertEquals(t, "example.tw:443", string(body))
}

func TestProxyServerPassword(t *testing.T) {
	server := &koHttp.ProxyServer{
		Dialer: &echoHttpDialer{},
		AuthHandler: func(username, password string) bool {
			return username == "123" && password == "456"
		},
		Logger: slog.Default(),
	}
	lp := koNet.ListenPipe()
	go server.Serve(lp)

	transport := &http.Transport{
		Proxy: func(r *http.Request) (*url.URL, error) {
			return url.Parse("http://127.0.0.1:1080")
		},
		DialContext:     lp.DialContext,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := http.Client{
		Transport: transport,
	}
	var err error
	res, _ := client.Get("http://example.tw:9999/")
	koTesting.Assert(t, res.StatusCode == http.StatusProxyAuthRequired, "expect error for no auth")
	_, err = client.Get("https://example.tw:443/")
	koTesting.Assert(t, err != nil, "expect error for no auth")
	transport.Proxy = func(r *http.Request) (*url.URL, error) {
		return url.Parse("http://123:456@127.0.0.1:1080")
	}
	res, _ = client.Get("http://example.tw:9999/")
	body, _ := io.ReadAll(res.Body)
	koTesting.AssertEquals(t, "example.tw:9999", string(body))
	res, _ = client.Get("https://example.tw:443/")
	body, _ = io.ReadAll(res.Body)
	koTesting.AssertEquals(t, "example.tw:443", string(body))
}
