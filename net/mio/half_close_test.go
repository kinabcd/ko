package mio_test

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/kinabcd/ko/net/mio"
)

type ClosableWriter interface {
	CloseWrite() error
}

type ClosableReader interface {
	CloseRead() error
}

func TestHalfClose(t *testing.T) {
	t.Run("CloseWrite", func(t *testing.T) {
		wg := &sync.WaitGroup{}
		c1, c2 := net.Pipe()
		cc1 := mio.New(c1)
		cc2 := mio.New(c2)

		var c11, c21 net.Conn
		var err1, err2 error

		wg.Go(func() {
			c11, err1 = cc1.DialContext(context.Background(), "", "")
			if err1 != nil {
				t.Errorf("DialContext failed: %v", err1)
			}
		})
		wg.Go(func() {
			c21, err2 = cc2.Accept()
			if err2 != nil {
				t.Errorf("Accept failed: %v", err2)
			}
		})
		wg.Wait()

		wg.Go(func() {
			cw := c11.(ClosableWriter)
			err := cw.CloseWrite()
			if err != nil {
				t.Errorf("CloseWrite failed: %v", err)
			}

			_, err = c11.Write([]byte("test"))
			if err != net.ErrClosed {
				t.Errorf("Expected net.ErrClosed, got %v", err)
			}
		})
		wg.Go(func() {
			buf := make([]byte, 10)
			_, err := c21.Read(buf)
			if err != io.EOF {
				t.Errorf("Expected io.EOF, got %v", err)
			}
		})
		wg.Wait()
		c11.Close()
		c21.Close()
	})

	t.Run("CloseRead", func(t *testing.T) {
		wg := &sync.WaitGroup{}
		c1, c2 := net.Pipe()
		cc1 := mio.New(c1)
		cc2 := mio.New(c2)

		var c11, c21 net.Conn
		var err1, err2 error

		wg.Go(func() {
			c11, err1 = cc1.DialContext(context.Background(), "", "")
			if err1 != nil {
				t.Errorf("DialContext failed: %v", err1)
			}
		})
		wg.Go(func() {
			c21, err2 = cc2.Accept()
			if err2 != nil {
				t.Errorf("Accept failed: %v", err2)
			}
		})
		wg.Wait()

		wg.Go(func() {
			cr := c11.(ClosableReader)
			err := cr.CloseRead()
			if err != nil {
				t.Errorf("CloseRead failed: %v", err)
			}

			buf := make([]byte, 10)
			_, err = c11.Read(buf)
			if err != io.EOF {
				t.Errorf("Expected io.EOF, got %v", err)
			}
		})
		wg.Go(func() {
			// Give some time for the CloseRead packet to be processed
			time.Sleep(50 * time.Millisecond)
			_, err := c21.Write([]byte("test"))
			if err != net.ErrClosed {
				t.Errorf("Expected net.ErrClosed, got %v", err)
			}
		})
		wg.Wait()
		c11.Close()
		c21.Close()
	})
}
