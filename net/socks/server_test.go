package socks_test

import (
	"context"
	"net"
	"testing"
	"time"

	koNet "github.com/kinabcd/ko/net"
	"github.com/kinabcd/ko/net/socks"
	koTesting "github.com/kinabcd/ko/testing"
	"golang.org/x/net/proxy"
)

type recordDialer struct {
	Network string
	Address string
	Content []byte
}

func (record *recordDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	record.Network = network
	record.Address = address
	c1, c2 := net.Pipe()
	go func() {
		buffer := make([]byte, 1024)
		if n, err := c2.Read(buffer); err != nil {
			return
		} else {
			record.Content = append(record.Content, buffer[:n]...)
		}
	}()
	return c1, nil
}
func TestSock5Server(t *testing.T) {
	record := &recordDialer{}
	socks5 := &socks.Server{
		Dialer: record,
	}
	lp := koNet.ListenPipe()
	go socks5.Serve(lp)
	dialer, err := proxy.SOCKS5("tcp", "55.55.55.6:6644", nil, lp)
	koTesting.AssertNoError(t, err)
	c, err := dialer.Dial("tcp", "example.tw:9999")
	koTesting.AssertNoError(t, err)
	outContent := []byte("YOYOYO")
	c.Write(outContent)
	c.Close()
	time.Sleep(1 * time.Second)
	koTesting.AssertEquals(t, "example.tw:9999", record.Address)
	koTesting.AssertSliceEquals(t, outContent, record.Content)
}

func TestSock5ServerPassword(t *testing.T) {
	record := &recordDialer{}
	socks5 := &socks.Server{
		Dialer: record,
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
	c.Close()
	time.Sleep(1 * time.Second)
	koTesting.AssertEquals(t, "example.tw:9999", record.Address)
	koTesting.AssertSliceEquals(t, outContent, record.Content)
}
