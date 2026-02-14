package net

import (
	"bytes"
	"context"
	"crypto/tls"
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
	tlsProtoRules map[string]Forwarder

	rules []muxPrefixRule
}

func (r *ServeMux) HandleTLSProto(proto string, forwarder Forwarder) {
	if r.tlsProtoRules == nil {
		r.tlsProtoRules = map[string]Forwarder{}
	}
	if f, ok := r.tlsProtoRules[proto]; ok && f != forwarder {
		panic("HandleTLSProto: can not handle " + proto + " twice")
	}
	r.tlsProtoRules[proto] = forwarder
}

func (r *ServeMux) HandlePrefix(prefix []byte, forwarder Forwarder) {
	r.rules = append(r.rules, muxPrefixRule{Prefix: prefix, Forwarder: forwarder})
}

func (r *ServeMux) Forward(ctx context.Context, c net.Conn) error {
	tlsConn, ok := c.(*tls.Conn)
	if ok && len(r.tlsProtoRules) > 0 {
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			c.Close()
			return err
		}

		protoNext := tlsConn.ConnectionState().NegotiatedProtocol
		if protoNext != "" {
			if forwarder := r.tlsProtoRules[protoNext]; forwarder != nil {
				return forwarder.Forward(ctx, tlsConn)
			}
		}
	}

	var buffer []byte = make([]byte, 1024)
	if n, err := c.Read(buffer[:1024]); err != nil {
		c.Close()
		return err
	} else if forwarder := r.findForwarder(buffer[:n]); forwarder == nil {
		c.Close()
		return errors.New("unknown protocol")
	} else {
		return forwarder.Forward(ctx, DecorateConn(c).WithPrefixBytes(buffer[:n]))
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
