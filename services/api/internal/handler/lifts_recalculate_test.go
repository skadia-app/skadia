package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

func callRecalculateLifts(t *testing.T, h *Handler, userID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/lifts/recalculate-all", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rec := httptest.NewRecorder()
	h.RecalculateLifts(rec, req)
	return rec
}

// TestRecalculateLifts_Unauthenticated verifies that the endpoint returns 401
// when no user ID is present in the request context.
func TestRecalculateLifts_Unauthenticated(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	h := &Handler{DB: db}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/lifts/recalculate-all", nil)
	rec := httptest.NewRecorder()
	h.RecalculateLifts(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", rec.Code)
	}
}

// TestRecalculateLifts_ZeroRows verifies that 204 is returned when no
// candidates are in soft-rejected state for the authenticated user.
func TestRecalculateLifts_ZeroRows(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	h := &Handler{DB: db}

	userID := insertTestUser(t, pool)

	rec := callRecalculateLifts(t, h, userID)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d, want 204; body=%s", rec.Code, rec.Body.String())
	}
}

// TestRecalculateLifts_ClearsRejectedCandidates verifies that soft-rejected
// candidates are reset and the response reports correct counts.
func TestRecalculateLifts_ClearsRejectedCandidates(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	h := &Handler{DB: db}

	userID := insertTestUser(t, pool)
	activityID := insertMinimalActivity(t, db, userID)

	start := time.Now().UTC().Truncate(time.Second)
	insertRejectedLiftCandidate(t, db, activityID, start)
	insertRejectedLiftCandidate(t, db, activityID, start.Add(10*time.Minute))

	rec := callRecalculateLifts(t, h, userID)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		ClearedCount       int `json:"cleared_count"`
		AffectedActivities int `json:"affected_activities"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
	}
	if resp.ClearedCount != 2 {
		t.Errorf("cleared_count=%d, want 2", resp.ClearedCount)
	}
	if resp.AffectedActivities != 1 {
		t.Errorf("affected_activities=%d, want 1", resp.AffectedActivities)
	}

	// Verify the DB rows were actually cleared.
	var stillRejected int
	err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM activity_lift_candidates
		 WHERE activity_id = $1 AND rejected_by_user = true`, activityID,
	).Scan(&stillRejected)
	if err != nil {
		t.Fatalf("query still-rejected count: %v", err)
	}
	if stillRejected != 0 {
		t.Errorf("still-rejected count=%d after reset, want 0", stillRejected)
	}
}

// TestRecalculateLifts_MultiTenant verifies that only the requesting user's
// candidates are cleared; another user's rejected candidates remain untouched.
func TestRecalculateLifts_MultiTenant(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	h := &Handler{DB: db}

	userA := insertTestUser(t, pool)
	userB := insertTestUser(t, pool)

	actA := insertMinimalActivity(t, db, userA)
	actB := insertMinimalActivity(t, db, userB)

	start := time.Now().UTC().Truncate(time.Second)
	insertRejectedLiftCandidate(t, db, actA, start)
	insertRejectedLiftCandidate(t, db, actB, start)

	// userA resets — only their candidate should clear.
	rec := callRecalculateLifts(t, h, userA)
	if rec.Code != http.StatusOK {
		t.Fatalf("userA reset status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	// userB's candidate must remain rejected.
	var bRejected bool
	err := pool.QueryRow(context.Background(),
		`SELECT rejected_by_user FROM activity_lift_candidates
		 WHERE activity_id = $1`, actB,
	).Scan(&bRejected)
	if err != nil {
		t.Fatalf("query userB candidate: %v", err)
	}
	if !bRejected {
		t.Error("userB rejected_by_user was cleared by userA's request — multi-tenant isolation failure")
	}
}
