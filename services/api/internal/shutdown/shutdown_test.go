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

// TestManager_GoDuringShutdown_NotLeaked exercises the TOCTOU race between
// Go and Shutdown (R2-M5). 100 goroutines are launched concurrently with a
// Shutdown call. Every goroutine must receive a cancelled context — either
// because it saw closed=true in Go, or because Shutdown's cancels.Range
// reached its entry. The WaitGroup must drain within the timeout, proving
// that no context was permanently uncancelled.
func TestManager_GoDuringShutdown_NotLeaked(t *testing.T) {
	const (
		numGoroutines = 100
		timeout       = 2 * time.Second
	)

	m := NewManager()

	// Signal to start all goroutines and Shutdown simultaneously.
	start := make(chan struct{})

	// Count how many goroutines completed (context cancelled + returned).
	var completed atomic.Int32

	// Launch goroutines that race with Shutdown.
	for i := range numGoroutines {
		id := "racing-job-" + strconv.Itoa(i)
		go func() {
			<-start // wait for the gun
			m.Go(id, func(ctx context.Context) {
				// Block until context is cancelled (which must happen —
				// either via the closed-path in Go or via Shutdown's Range).
				<-ctx.Done()
				completed.Add(1)
			})
		}()
	}

	// Trigger Shutdown concurrently with all the Go() calls above.
	go func() {
		<-start
		m.Shutdown(timeout)
	}()

	close(start) // release everything at once

	// Allow time for all goroutines and Shutdown to run.
	deadline := time.After(timeout)
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out: only %d/%d goroutines completed — leaked contexts detected", completed.Load(), numGoroutines)
		case <-tick.C:
			if completed.Load() == numGoroutines {
				return // all goroutines received their cancellation signal
			}
		}
	}
}
