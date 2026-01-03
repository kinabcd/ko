package middleware

import (
	"compress/gzip"
	"net/http"
	"strings"
)

func Gzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !IsGzipAllowed(w, r) {
			next.ServeHTTP(w, r) // Client doesn't accept gzip, serve normally
			return
		}

		w.Header().Set("Content-Encoding", "gzip")
		gzWriter := gzip.NewWriter(w)
		defer gzWriter.Close() // Ensure the gzip writer is closed to flush data

		// Create a custom ResponseWriter that writes to the gzip writer
		gzw := &gzipResponseWriter{ResponseWriter: w, GzipWriter: gzWriter}

		next.ServeHTTP(gzw, r) // Serve the request with the gzipped writer
	})
}
func IsGzipAllowed(w http.ResponseWriter, r *http.Request) bool {
	acceptGzip := func(r *http.Request) bool {
		for _, aes := range r.Header.Values("Accept-Encoding") {
			for sps := range strings.SplitSeq(aes, ",") {
				if strings.TrimSpace(sps) == "gzip" {
					return true
				}
			}
		}
		return false
	}
	isWebsocket := func(r *http.Request) bool {
		return r.Header.Get("Connection") == "Upgrade" && r.Header.Get("Upgrade") == "websocket"
	}
	return !isWebsocket(r) && w.Header().Get("Content-Encoding") == "" && acceptGzip(r)
}

// gzipResponseWriter wraps http.ResponseWriter to enable gzip compression.
type gzipResponseWriter struct {
	http.ResponseWriter
	GzipWriter *gzip.Writer
}

func (w *gzipResponseWriter) Write(data []byte) (int, error) {
	return w.GzipWriter.Write(data) // Write to the gzip writer
}

func (w *gzipResponseWriter) Flush() {
	if w.GzipWriter != nil {
		w.GzipWriter.Flush()
	}
	if fw, ok := w.ResponseWriter.(http.Flusher); ok {
		fw.Flush()
	}
}
