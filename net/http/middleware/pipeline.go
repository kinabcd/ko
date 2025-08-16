package middleware

import "net/http"

type Pipeline []func(next http.Handler) http.Handler

func (p Pipeline) Then(h http.Handler) http.Handler {
	for i := len(p) - 1; i >= 0; i-- {
		h = p[i](h)
	}
	return h
}
