package sync

import (
	"sync"
	"testing"
)

func TestList(t *testing.T) {
	myList := NewArrayList[int]()
	wg := &sync.WaitGroup{}
	wg.Add(1000)
	for i := 0; i < 1000; i += 1 {
		go func() {
			for j := 0; j < 1000; j += 1 {
				myList.Add(123)
				myList.Add(456)
			}
			wg.Done()
		}()
	}
	wg.Wait()
	myList.RemoveAll(func(item int) bool {
		return item > 200
	})
	if count := len(myList.List()); count != 1000000 {
		t.Fatalf("len=%d", count)
	}
	for i, v := range myList.List() {
		if v != 123 {
			t.Fatalf("v[%d]=%d", i, v)
		}
	}
	wg.Add(1000)
	for i := 0; i < 1000; i += 1 {
		go func() {
			for j := 0; j < 1000; j += 1 {
				myList.Add(99)
			}
			wg.Done()
		}()
	}
	go func() {
		myList.RemoveAll(func(item int) bool {
			return item > 100
		})
	}()
	wg.Wait()
	if count := len(myList.List()); count != 1000000 {
		t.Fatalf("len=%d", count)
	}
	for i, v := range myList.List() {
		if v != 99 {
			t.Fatalf("v[%d]=%d", i, v)
		}
	}
}
