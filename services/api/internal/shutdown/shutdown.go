package shutdown

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

var (
	cancels sync.Map
	wg      sync.WaitGroup
	closed  atomic.Bool
)

func Go(id string, fn func(ctx context.Context)) {
	ctx, cancel := context.WithCancel(context.Background())
	if closed.Load() {
		cancel()
	}
	cancels.Store(id, cancel)
	wg.Go(func() {
		defer cancels.Delete(id)
		defer cancel()
		fn(ctx)
	})
}

func Cancel(id string) {
	if val, ok := cancels.Load(id); ok {
		val.(context.CancelFunc)()
	}
}

func Shutdown(timeout time.Duration) bool {
	closed.Store(true)
	cancels.Range(func(_, value any) bool {
		value.(context.CancelFunc)()
		return true
	})
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		slog.Info("shutdown: all background jobs finished")
		return true
	case <-time.After(timeout):
		slog.Warn("shutdown: background jobs did not finish in time", "timeout", timeout)
		return false
	}
}

func Reset() {
	cancels.Range(func(key, value any) bool {
		value.(context.CancelFunc)()
		cancels.Delete(key)
		return true
	})
	wg.Wait()
	closed.Store(false)
}
