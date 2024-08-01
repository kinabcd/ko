package sync

import "sync"

// GoWithWaitGroup runs the provided block function in a new goroutine and increments the WaitGroup counter while it's executing.
func GoWithWaitGroup(wg *sync.WaitGroup, blocks ...func()) {
	wg.Add(len(blocks))
	for _, f := range blocks {
		go func() {
			defer wg.Done()
			f()
		}()
	}
}
