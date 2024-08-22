package socks_test

import (
	"context"
	"io"
	"net"
	"testing"

	koIo "github.com/kinabcd/ko/io"
	koNet "github.com/kinabcd/ko/net"
	"github.com/kinabcd/ko/net/socks"
	koTesting "github.com/kinabcd/ko/testing"
	"golang.org/x/net/proxy"
)

type echoDialer struct{}

func (echo *echoDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	c1, c2 := net.Pipe()
	go func() {
		content, _ := koIo.ReadCString(c2)
		c2.Write([]byte(network + ":" + address + ":" + content))
		c2.Close()
	}()
	return c1, nil
}
func TestSock5Server(t *testing.T) {
	socks5 := &socks.Server{
		Dialer: &echoDialer{},
	}
	lp := koNet.ListenPipe()
	go socks5.Serve(lp)
	dialer, err := proxy.SOCKS5("tcp", "55.55.55.6:6644", nil, lp)
	koTesting.AssertNoError(t, err)
	c, err := dialer.Dial("tcp", "example.tw:9999")
	koTesting.AssertNoError(t, err)
	outContent := []byte("YOYOYO")
	c.Write(outContent[:3])
	c.Write(outContent[3:])
	c.Write([]byte{0})
	content, _ := io.ReadAll(c)
	c.Close()
	koTesting.AssertEquals(t, "tcp:example.tw:9999:YOYOYO", string(content))
}

func TestSock5ServerPassword(t *testing.T) {
	socks5 := &socks.Server{
		Dialer: &echoDialer{},
		AuthHandler: func(username, password string) bool {
			return username == "123" && password == "456"
		},
	}
	lp := koNet.ListenPipe()
	go socks5.Serve(lp)
	dialer, err := proxy.SOCKS5("tcp", "55.55.55.6:6644", nil, lp)
	koTesting.AssertNoError(t, err)
	_, err = dialer.Dial("tcp", "example.tw:9999")
	koTesting.Assert(t, err != nil, "expect error for no auth")

	dialer, err = proxy.SOCKS5("tcp", "55.55.55.6:6644", &proxy.Auth{User: "123", Password: "789"}, lp)
	koTesting.AssertNoError(t, err)
	_, err = dialer.Dial("tcp", "example.tw:9999")
	koTesting.Assert(t, err != nil, "expect error for wrong password")

	dialer, err = proxy.SOCKS5("tcp", "55.55.55.6:6644", &proxy.Auth{User: "777", Password: "456"}, lp)
	koTesting.AssertNoError(t, err)
	_, err = dialer.Dial("tcp", "example.tw:9999")
	koTesting.Assert(t, err != nil, "expect error for wrong user")

	dialer, err = proxy.SOCKS5("tcp", "55.55.55.6:6644", &proxy.Auth{User: "123", Password: "456"}, lp)
	koTesting.AssertNoError(t, err)
	c, err := dialer.Dial("tcp", "example.tw:9999")
	koTesting.AssertNoError(t, err)
	outContent := []byte("YOYOYO")
	c.Write(outContent)
	c.Write([]byte{0})
	content, _ := io.ReadAll(c)
	c.Close()
	koTesting.AssertEquals(t, "tcp:example.tw:9999:YOYOYO", string(content))
}
