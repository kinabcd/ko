package net

import (
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"
)

type factory func(u *url.URL, dialer ContextDialer) (ContextDialer, error)

var dialerFactory map[string]factory = map[string]factory{}

func RegisterDialerFactory(scheme []string, factory func(u *url.URL, dialer ContextDialer) (ContextDialer, error)) error {
	for ck, _ := range dialerFactory {
		if slices.Contains(scheme, ck) {
			return fmt.Errorf("%s is registed by others", ck)
		}
	}
	for _, nk := range scheme {
		dialerFactory[nk] = factory
	}
	return nil
}
func CreateDialer(u *url.URL, dialer ContextDialer) (ContextDialer, error) {
	var err error
	ns := strings.Split(u.Scheme, "+")
	slices.Reverse(ns)
	for _, scheme := range ns {
		tUrl := *u
		tUrl.Scheme = scheme
		if factory, ok := dialerFactory[scheme]; ok {
			dialer, err = factory(&tUrl, dialer)
			if err != nil {
				return nil, err
			}
		} else if slices.Contains([]string{"", "tcp", "udp", "tcp4", "udp4", "tcp6", "udp6"}, scheme) {
			if dialer == nil {
				dialer = &net.Dialer{}
			}
		} else {
			return nil, fmt.Errorf("unknown scheme %s", u.Scheme)

		}
	}
	return dialer, nil
}
