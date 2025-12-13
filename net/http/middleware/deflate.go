package middleware

import (
	"compress/flate"
	"net/http"
	"strings"
)

func Deflate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !IsDeflateAllowed(w, r) {
			next.ServeHTTP(w, r) // Client doesn't accept deflate, serve normally
			return
		}

		w.Header().Set("Content-Encoding", "deflate")
		writer, _ := flate.NewWriter(w, flate.DefaultCompression)
		defer writer.Close()
		newW := &deflateResponseWriter{ResponseWriter: w, writer: writer}

		next.ServeHTTP(newW, r)
	})
}
func IsDeflateAllowed(w http.ResponseWriter, r *http.Request) bool {
	isAccepted := func(r *http.Request) bool {
		for _, aes := range r.Header.Values("Accept-Encoding") {
			for sps := range strings.SplitSeq(aes, ",") {
				if strings.TrimSpace(sps) == "deflate" {
					return true
				}
			}
		}
		return false
	}
	return w.Header().Get("Content-Encoding") == "" && isAccepted(r)
}

// deflateResponseWriter wraps http.ResponseWriter to enable deflate compression.
type deflateResponseWriter struct {
	http.ResponseWriter
	writer *flate.Writer
}

func (w *deflateResponseWriter) Write(data []byte) (int, error) {
	return w.writer.Write(data)
}

func (w *deflateResponseWriter) Flush() {
	if w.writer != nil {
		w.writer.Flush()
	}
	if fw, ok := w.ResponseWriter.(http.Flusher); ok {
		fw.Flush()
	}
}
