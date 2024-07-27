package sync

import "sync"

// GoWithWaitGroup runs the provided block function in a new goroutine and increments the WaitGroup counter while it's executing.
func GoWithWaitGroup(wg *sync.WaitGroup, block func()) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		block()
	}()
}
