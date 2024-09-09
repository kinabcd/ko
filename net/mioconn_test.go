package net_test

import (
	"context"
	"io"
	"net"
	"testing"

	koNet "github.com/kinabcd/ko/net"
	koSync "github.com/kinabcd/ko/sync"
	koTesting "github.com/kinabcd/ko/testing"
)

func TestHandleNewConn(t *testing.T) {
	wg := &koSync.WaitGroup{}
	c1, c2 := net.Pipe()
	b11 := make([]byte, 6)
	b12 := make([]byte, 6)
	b21 := make([]byte, 6)
	b22 := make([]byte, 6)
	cc1 := koNet.NewMioConn(c1)
	cc2 := koNet.NewMioConn(c2)
	wg.Go(
		func() {
			c11, err1 := cc1.DialContext(context.Background(), "", "")
			koTesting.AssertNoError(t, err1)
			c11.Read(b11)
			c11.Write([]byte{2, 2, 2})
			c11.Write([]byte{2, 2, 2})
		},
		func() {
			c12, err2 := cc1.Accept()
			koTesting.AssertNoError(t, err2)
			c12.Read(b12)
			c12.Write([]byte{4, 4, 4, 4, 4, 4})
		},
		func() {
			c21, err1 := cc2.Accept()
			koTesting.AssertNoError(t, err1)
			c21.Write([]byte{1, 1, 1, 1, 1, 1})
			io.ReadFull(c21, b21)
		},
		func() {
			c22, err2 := cc2.DialContext(context.Background(), "", "")
			koTesting.AssertNoError(t, err2)
			c22.Write([]byte{3, 3, 3, 3, 3, 3})
			c22.Read(b22)
		},
	)
	wg.Wait()
	koTesting.AssertSliceEquals(t, []byte{1, 1, 1, 1, 1, 1}, b11)
	koTesting.AssertSliceEquals(t, []byte{2, 2, 2, 2, 2, 2}, b21)
	koTesting.AssertSliceEquals(t, []byte{3, 3, 3, 3, 3, 3}, b12)
	koTesting.AssertSliceEquals(t, []byte{4, 4, 4, 4, 4, 4}, b22)
}
