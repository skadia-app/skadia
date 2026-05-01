package shutdown

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// Manager owns its own WaitGroup, cancel-map, and closed flag.
// All methods are safe for concurrent use.
//
// Design invariants:
//
//  1. mu (RWMutex) gates every wg.Add call (RLock in Go) and every wg.Wait
//     call (Lock in Reset), so wg.Add and wg.Wait are never concurrent.
//
//  2. Shutdown spawns a goroutine to call wg.Wait. That goroutine captures
//     the *wgHolder at call time. Reset swaps in a fresh *wgHolder, so
//     subsequent Go/Reset calls operate on an independent WaitGroup; the
//     Shutdown goroutine may still be draining the old one without any data
//     race.
//
//  3. The package-level functions delegate to defaultManager, preserving the
//     existing API for production callers. Tests can create isolated Managers
//     with NewManager().
type Manager struct {
	mu      sync.RWMutex
	cancels sync.Map
	wg      atomic.Pointer[sync.WaitGroup]
	closed  atomic.Bool
}

// NewManager returns a ready-to-use Manager.
func NewManager() *Manager {
	m := &Manager{}
	m.wg.Store(&sync.WaitGroup{})
	return m
}

// currentWG returns the active WaitGroup pointer (for callers that need to
// capture it before a potential Reset swap).
func (m *Manager) currentWG() *sync.WaitGroup {
	return m.wg.Load()
}

// Go registers and launches fn as a background goroutine tracked by the
// Manager. The context passed to fn is cancelled when Cancel(id), Shutdown,
// or Reset is called.
//
// If the Manager is shut down, fn still runs but receives an already-cancelled
// context.
func (m *Manager) Go(id string, fn func(ctx context.Context)) {
	ctx, cancel := context.WithCancel(context.Background())

	// RLock prevents Reset from swapping the WaitGroup and returning from
	// wg.Wait while we have already incremented the old WaitGroup counter.
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed.Load() {
		cancel()
	}
	m.cancels.Store(id, cancel)

	wg := m.currentWG()
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer m.cancels.Delete(id)
		defer cancel()
		fn(ctx)
	}()
}

// Cancel cancels the goroutine registered under id, if any.
func (m *Manager) Cancel(id string) {
	if val, ok := m.cancels.Load(id); ok {
		val.(context.CancelFunc)()
	}
}

// Shutdown cancels all goroutines and waits up to timeout for them to finish.
// Returns true if all goroutines exited within the timeout.
//
// When the timeout fires, a background goroutine may still be draining the
// WaitGroup. That goroutine captures the WaitGroup pointer at call time and
// is therefore not affected by a subsequent Reset.
func (m *Manager) Shutdown(timeout time.Duration) bool {
	m.closed.Store(true)
	m.cancels.Range(func(_, value any) bool {
		value.(context.CancelFunc)()
		return true
	})

	// Capture current WaitGroup; Reset may swap it out before this goroutine
	// finishes, and that is fine — they operate on independent objects.
	wg := m.currentWG()
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

// Reset cancels all running goroutines, waits for them to finish, clears the
// cancel map, and resets the Manager for reuse (e.g. in tests).
//
// Race-safety: the write lock is held across wg.Wait + WaitGroup swap +
// closed.Store(false), so no concurrent Go call can call wg.Add on the old
// WaitGroup after Wait has returned.
func (m *Manager) Reset() {
	// Cancel all goroutines before acquiring the write lock so they can make
	// progress and finish promptly.
	m.cancels.Range(func(_, value any) bool {
		value.(context.CancelFunc)()
		return true
	})

	// Write-lock: blocks concurrent Go calls from calling wg.Add until we are
	// done swapping the WaitGroup.
	m.mu.Lock()
	defer m.mu.Unlock()

	// Drain the current WaitGroup.
	m.currentWG().Wait()

	// Clear the cancel map.
	m.cancels.Range(func(key, _ any) bool {
		m.cancels.Delete(key)
		return true
	})

	// Swap in a fresh WaitGroup so that any lingering Shutdown goroutine
	// (still blocked on the old WaitGroup after a timeout) operates on an
	// independent object and cannot race with future Go/Reset calls.
	m.wg.Store(&sync.WaitGroup{})

	m.closed.Store(false)
}

// ---------------------------------------------------------------------------
// Package-level default — backward-compatible API
// ---------------------------------------------------------------------------

var defaultManager = NewManager()

// Go launches fn as a background goroutine in the default Manager.
func Go(id string, fn func(ctx context.Context)) {
	defaultManager.Go(id, fn)
}

// Cancel cancels the goroutine registered under id in the default Manager.
func Cancel(id string) {
	defaultManager.Cancel(id)
}

// Shutdown cancels all goroutines in the default Manager and waits up to
// timeout for them to finish.
func Shutdown(timeout time.Duration) bool {
	return defaultManager.Shutdown(timeout)
}

// Reset cancels all goroutines in the default Manager, waits for them to
// finish, and resets the state for reuse (e.g. in tests).
func Reset() {
	defaultManager.Reset()
}
