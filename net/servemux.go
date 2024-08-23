package net

import (
	"bytes"
	"context"
	"errors"
	"net"
)

var (
	_ Forwarder = &ServeMux{}
)

type muxPrefixRule struct {
	Prefix    []byte
	Forwarder Forwarder
}

// ServeMux will forward the connection (Conn) to different Forwarders based on the rules.
type ServeMux struct {
	rules []muxPrefixRule
}

func (r *ServeMux) HandlePrefix(prefix []byte, forwarder Forwarder) {
	r.rules = append(r.rules, muxPrefixRule{Prefix: prefix, Forwarder: forwarder})
}

func (r *ServeMux) Forward(ctx context.Context, c net.Conn) error {
	var buffer []byte = make([]byte, 1024)
	if n, err := c.Read(buffer[:1024]); err != nil {
		c.Close()
		return err
	} else if forwarder := r.findForwarder(buffer[:n]); forwarder == nil {
		c.Close()
		return errors.New("unknown protocol")
	} else {
		return forwarder.Forward(ctx, &PrefixConn{
			Conn:   c,
			Prefix: bytes.NewBuffer(buffer[:n]),
		})
	}
}

func (r *ServeMux) findForwarder(header []byte) Forwarder {
	for _, rule := range r.rules {
		if bytes.HasPrefix(header, rule.Prefix) {
			return rule.Forwarder
		}
	}
	return nil
}
