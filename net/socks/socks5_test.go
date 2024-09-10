package socks_test

import (
	"context"
	"io"
	"net/url"
	"testing"

	koNet "github.com/kinabcd/ko/net"
	"github.com/kinabcd/ko/net/socks"
	koTesting "github.com/kinabcd/ko/testing"
)

func TestSock5NoAuth(t *testing.T) {
	s := &socks.Server{
		Dialer: &echoDialer{},
	}
	lp := koNet.ListenPipe()
	go s.Serve(lp)
	c, _ := lp.Dial("", "")
	c, err := socks.SOCKS5Client(context.Background(), c, "tcp", "example.tw:9999", nil)
	koTesting.AssertNoError(t, err)
	outContent := []byte("YOYOYO")
	c.Write(outContent[:3])
	c.Write(outContent[3:])
	c.Write([]byte{0})
	content, _ := io.ReadAll(c)
	c.Close()
	koTesting.AssertEquals(t, "tcp:example.tw:9999:YOYOYO", string(content))
}

func TestSock5Password(t *testing.T) {
	s := &socks.Server{
		Dialer: &echoDialer{},
		AuthHandler: func(username, password string) bool {
			return username == "123" && password == "456"
		},
	}
	lp := koNet.ListenPipe()
	go s.Serve(lp)
	client := &socks.Client{
		Dialer: lp,
	}
	client.ProxyUrl, _ = url.Parse("socks5://127.0.0.1:8080")
	_, err := client.DialContext(context.Background(), "tcp", "example.tw:9999")
	koTesting.Assert(t, err != nil, "expect error for no auth")

	client.ProxyUrl, _ = url.Parse("socks5://123:789@127.0.0.1:8080")
	_, err = client.DialContext(context.Background(), "tcp", "example.tw:9999")
	koTesting.Assert(t, err != nil, "expect error for wrong password")

	client.ProxyUrl, _ = url.Parse("socks5://777:456@127.0.0.1:8080")
	_, err = client.DialContext(context.Background(), "tcp", "example.tw:9999")
	koTesting.Assert(t, err != nil, "expect error for wrong user")

	client.ProxyUrl, _ = url.Parse("socks5://123:456@127.0.0.1:8080")
	c, err := client.DialContext(context.Background(), "tcp", "example.tw:9999")
	koTesting.AssertNoError(t, err)
	outContent := []byte("YOYOYO")
	c.Write(outContent)
	c.Write([]byte{0})
	content, _ := io.ReadAll(c)
	c.Close()
	koTesting.AssertEquals(t, "tcp:example.tw:9999:YOYOYO", string(content))
}
