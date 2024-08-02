package sync

import "sync"

type anyWaitGroup interface {
	Add(delta int)
	Done()
	Wait()
}

// GoWithWaitGroup runs the provided block function in a new goroutine and increments the WaitGroup counter while it's executing.
func GoWithWaitGroup(wg anyWaitGroup, blocks ...func()) {
	wg.Add(len(blocks))
	for _, f := range blocks {
		go func() {
			defer wg.Done()
			f()
		}()
	}
}

type WaitGroup struct {
	sync.WaitGroup
}

func (wg *WaitGroup) Go(block ...func()) {
	GoWithWaitGroup(wg, block...)
}
