package middleware_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/kinabcd/ko/net/http/middleware"
)

func TestPipeline(t *testing.T) {
	record1 := []int{}
	record2 := []int{}
	recordMiddleware := func(code int) func(next http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				record1 = append(record1, code)
				next.ServeHTTP(w, r)
				record2 = append(record2, code)
			})
		}
	}
	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		record1 = append(record1, 0)
		record2 = append(record2, 0)
	})
	pipeline := middleware.Pipeline{
		recordMiddleware(1),
		recordMiddleware(2),
		middleware.Pipeline{
			recordMiddleware(3),
			recordMiddleware(4),
		}.Then,
		recordMiddleware(5),
		recordMiddleware(6),
	}.Then(final)
	pipeline.ServeHTTP(nil, nil)

	if !slices.Equal(record1, []int{1, 2, 3, 4, 5, 6, 0}) || !slices.Equal(record2, []int{0, 6, 5, 4, 3, 2, 1}) {
		t.Errorf("Unexpected record values: %v and %v", record1, record2)
	}
}
