package analytics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// TestRefreshInsights_TriggersInvalidate verifies that RefreshInsights
// publishes exactly one analytics-invalidate event per request (Phase 6.3).
// The Go-analytics goroutine is short-circuited by pre-acquiring the per-user
// refreshLock, so the test does not need a live DB.
func TestRefreshInsights_TriggersInvalidate(t *testing.T) {
	const uid = "00000000-0000-0000-0000-0000000000aa"

	// Pre-acquire the per-user refresh lock so RefreshInsights skips the
	// goroutine that would otherwise call goanalytics.* with a nil DB.
	refreshMu.Lock()
	refreshLock[uid] = struct{}{}
	refreshMu.Unlock()
	t.Cleanup(func() {
		refreshMu.Lock()
		delete(refreshLock, uid)
		refreshMu.Unlock()
	})

	fake := &event.FakeInvalidator{}
	h := &Handler{Invalidator: fake}

	r := httptest.NewRequest(http.MethodPost, "/analytics/insights/refresh", nil)
	r = r.WithContext(context.WithValue(r.Context(), middleware.UserIDKey, uid))
	rr := httptest.NewRecorder()
	h.RefreshInsights(rr, r)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status %d, want 202; body: %s", rr.Code, rr.Body.String())
	}
	if got, want := len(fake.Calls), 1; got != want {
		t.Fatalf("FakeInvalidator.Calls = %d, want %d", got, want)
	}
	if fake.Calls[0] != uid {
		t.Errorf("FakeInvalidator.Calls[0] = %q, want %q", fake.Calls[0], uid)
	}
}

// TestRefreshInsights_NilInvalidatorOK ensures the handler does not panic when
// no invalidator is configured (NATS unavailable scenario).
func TestRefreshInsights_NilInvalidatorOK(t *testing.T) {
	const uid = "00000000-0000-0000-0000-0000000000bb"

	refreshMu.Lock()
	refreshLock[uid] = struct{}{}
	refreshMu.Unlock()
	t.Cleanup(func() {
		refreshMu.Lock()
		delete(refreshLock, uid)
		refreshMu.Unlock()
	})

	h := &Handler{Invalidator: nil}

	r := httptest.NewRequest(http.MethodPost, "/analytics/insights/refresh", nil)
	r = r.WithContext(context.WithValue(r.Context(), middleware.UserIDKey, uid))
	rr := httptest.NewRecorder()
	h.RefreshInsights(rr, r)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status %d, want 202; body: %s", rr.Code, rr.Body.String())
	}
}
