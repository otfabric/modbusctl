package modbus

import (
	"context"
	"time"
)

// sleepContext waits for d or until ctx is canceled. A non-positive d returns immediately with nil.
func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
