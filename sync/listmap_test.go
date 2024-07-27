package sync_test

import (
	"sync"
	"testing"

	kosync "github.com/kinabcd/ko/sync"
)

func TestListMap(t *testing.T) {
	listmap := kosync.NewListMap[string, int]()
	wg := &sync.WaitGroup{}
	wg.Add(1000)
	for i := 0; i < 1000; i += 1 {
		go func() {
			for j := 0; j < 1000; j += 1 {
				listmap.Add("1", 123)
				listmap.Add("2", 123)
				listmap.Add("1", 456)
				listmap.Add("2", 456)
			}
			wg.Done()
		}()
	}
	wg.Wait()
	listmap.RemoveAll("1", func(item int) bool {
		return item > 200
	})
	if count := len(listmap.Get("1")); count != 1000000 {
		t.Errorf("len(1)=%d", count)
	}
	if v := listmap.Get("1"); v[0] != 123 {
		t.Errorf("v[0]=%d", v[0])
	}
	if count := len(listmap.Get("2")); count != 2000000 {
		t.Errorf("len(2)=%d", count)
	}
}
