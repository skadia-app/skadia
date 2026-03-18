// Tests are sequential — shared package-level state. Do NOT add t.Parallel().
package shutdown

import (
	"context"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestGo_LaunchesGoroutineWithCancellableContext(t *testing.T) {
	Reset()
	var ran atomic.Bool
	Go("test-job", func(ctx context.Context) {
		ran.Store(true)
		<-ctx.Done()
	})
	time.Sleep(50 * time.Millisecond)
	if !ran.Load() {
		t.Fatal("goroutine did not run")
	}
	Shutdown(2 * time.Second)
}

func TestShutdown_CancelsAllJobs(t *testing.T) {
	Reset()
	var cancelled atomic.Int32
	for i := range 3 {
		id := "job-" + strconv.Itoa(i)
		Go(id, func(ctx context.Context) {
			<-ctx.Done()
			cancelled.Add(1)
		})
	}
	time.Sleep(50 * time.Millisecond)
	ok := Shutdown(2 * time.Second)
	if !ok {
		t.Fatal("shutdown timed out")
	}
	if cancelled.Load() != 3 {
		t.Fatalf("expected 3 cancelled, got %d", cancelled.Load())
	}
}

func TestCancel_SingleJob(t *testing.T) {
	Reset()
	var done atomic.Bool
	Go("cancel-me", func(ctx context.Context) {
		<-ctx.Done()
		done.Store(true)
	})
	time.Sleep(50 * time.Millisecond)
	Cancel("cancel-me")
	time.Sleep(50 * time.Millisecond)
	if !done.Load() {
		t.Fatal("job was not cancelled")
	}
	Shutdown(1 * time.Second)
}

func TestGo_AfterShutdown_ReceivesCancelledContext(t *testing.T) {
	Reset()
	Shutdown(100 * time.Millisecond)
	var ctxDone atomic.Bool
	Go("post-shutdown", func(ctx context.Context) {
		if ctx.Err() != nil {
			ctxDone.Store(true)
		}
	})
	time.Sleep(50 * time.Millisecond)
	if !ctxDone.Load() {
		t.Fatal("goroutine after shutdown should receive cancelled context")
	}
}

func TestShutdown_TimeoutReturnsFalse(t *testing.T) {
	Reset()
	Go("stuck-job", func(ctx context.Context) {
		<-ctx.Done()
		time.Sleep(200 * time.Millisecond)
	})
	time.Sleep(50 * time.Millisecond)
	ok := Shutdown(100 * time.Millisecond)
	if ok {
		t.Fatal("expected timeout, got success")
	}
}
