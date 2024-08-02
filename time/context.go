package time

import (
	"context"
	"time"
)

// Sleep until ctx done or timeout
// return false if ctx is Done
func SleepContext(ctx context.Context, duration time.Duration) bool {
	timeout, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	<-timeout.Done()
	return ctx.Err() == nil
}
