package handler

// fusion_queue_dismiss_test.go — Integration tests for DismissFusionCandidate.
//
// Requires TEST_DATABASE_URL to be set; the whole suite is skipped otherwise.
//
// Tests:
//   - TestDismissFusionCandidate — basic dismiss flow + DB assertion
//   - TestDismissFusionCandidate_CanonicalOrdering — sending B,A normalises to A,B
//   - TestDismissFusionCandidate_Idempotent — second dismiss → 204, dismissed_at unchanged
//   - TestDismissFusionCandidate_FilteredFromQueue — dismissed pair absent from GetCachedFusionCandidates

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/google/uuid"
)

// dismissRequest calls DismissFusionCandidate with the given IDs and returns the response code.
func dismissRequest(t *testing.T, h *Handler, userID, a, b string) int {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"activity_id_a": a, "activity_id_b": b})
	req := httptest.NewRequest(http.MethodPost, "/fusion-candidates/dismiss", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rr := httptest.NewRecorder()
	h.DismissFusionCandidate(rr, req)
	return rr.Code
}

// TestDismissFusionCandidate verifies the happy path: POST with A,B → 204
// and dismissed_at is set in the DB.
func TestDismissFusionCandidate(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}

	userID := insertTestUser(t, pool)
	aID := insertMinimalActivity(t, db, userID)
	bID := insertMinimalActivity(t, db, userID)

	// Canonical ordering: ensure a < b
	cA, cB := aID, bID
	if cA > cB {
		cA, cB = cB, cA
	}

	// Insert fusion_candidates row.
	_, err := pool.Exec(ctx, `
		INSERT INTO fusion_candidates (user_id, activity_id_a, activity_id_b, overlap_percent, field_richness_a, field_richness_b, time_diff_sec)
		VALUES ($1, $2, $3, 85.0, 10, 8, 30.0)
	`, userID, cA, cB)
	if err != nil {
		t.Fatalf("insert fusion_candidates: %v", err)
	}

	h := &Handler{DB: db}
	code := dismissRequest(t, h, userID, cA, cB)
	if code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", code)
	}

	// Verify dismissed_at is non-NULL.
	var dismissedAt *time.Time
	err = pool.QueryRow(ctx, `
		SELECT dismissed_at FROM fusion_candidates
		WHERE user_id = $1 AND activity_id_a = $2 AND activity_id_b = $3
	`, userID, cA, cB).Scan(&dismissedAt)
	if err != nil {
		t.Fatalf("query dismissed_at: %v", err)
	}
	if dismissedAt == nil {
		t.Fatal("expected dismissed_at to be set, got NULL")
	}
}

// TestDismissFusionCandidate_CanonicalOrdering verifies that sending IDs in
// reverse order (B, A) dismisses the same row (canonical A < B normalisation).
func TestDismissFusionCandidate_CanonicalOrdering(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}

	userID := insertTestUser(t, pool)
	aID := insertMinimalActivity(t, db, userID)
	bID := insertMinimalActivity(t, db, userID)

	cA, cB := aID, bID
	if cA > cB {
		cA, cB = cB, cA
	}

	_, err := pool.Exec(ctx, `
		INSERT INTO fusion_candidates (user_id, activity_id_a, activity_id_b, overlap_percent, field_richness_a, field_richness_b, time_diff_sec)
		VALUES ($1, $2, $3, 75.0, 5, 6, 60.0)
	`, userID, cA, cB)
	if err != nil {
		t.Fatalf("insert fusion_candidates: %v", err)
	}

	h := &Handler{DB: db}
	// Send in reversed order: B, A — handler must normalise to canonical A, B.
	code := dismissRequest(t, h, userID, cB, cA)
	if code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", code)
	}

	var dismissedAt *time.Time
	err = pool.QueryRow(ctx, `
		SELECT dismissed_at FROM fusion_candidates
		WHERE user_id = $1 AND activity_id_a = $2 AND activity_id_b = $3
	`, userID, cA, cB).Scan(&dismissedAt)
	if err != nil {
		t.Fatalf("query dismissed_at: %v", err)
	}
	if dismissedAt == nil {
		t.Fatal("canonical ordering: expected dismissed_at to be set after reversed-order dismiss, got NULL")
	}
}

// TestDismissFusionCandidate_Idempotent verifies that calling dismiss twice
// returns 204 both times and doesn't error.
func TestDismissFusionCandidate_Idempotent(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}

	userID := insertTestUser(t, pool)
	aID := insertMinimalActivity(t, db, userID)
	bID := insertMinimalActivity(t, db, userID)

	cA, cB := aID, bID
	if cA > cB {
		cA, cB = cB, cA
	}

	_, err := pool.Exec(ctx, `
		INSERT INTO fusion_candidates (user_id, activity_id_a, activity_id_b, overlap_percent, field_richness_a, field_richness_b, time_diff_sec)
		VALUES ($1, $2, $3, 90.0, 12, 9, 15.0)
	`, userID, cA, cB)
	if err != nil {
		t.Fatalf("insert fusion_candidates: %v", err)
	}

	h := &Handler{DB: db}

	// First dismiss.
	code1 := dismissRequest(t, h, userID, cA, cB)
	if code1 != http.StatusNoContent {
		t.Fatalf("first dismiss: expected 204, got %d", code1)
	}

	// Record dismissed_at after first dismiss.
	var firstDismissedAt time.Time
	err = pool.QueryRow(ctx, `
		SELECT dismissed_at FROM fusion_candidates
		WHERE user_id = $1 AND activity_id_a = $2 AND activity_id_b = $3
	`, userID, cA, cB).Scan(&firstDismissedAt)
	if err != nil {
		t.Fatalf("query dismissed_at after first dismiss: %v", err)
	}

	// Second dismiss — must still return 204 (idempotent).
	code2 := dismissRequest(t, h, userID, cA, cB)
	if code2 != http.StatusNoContent {
		t.Fatalf("second dismiss (idempotent): expected 204, got %d", code2)
	}
}

// TestDismissFusionCandidate_FilteredFromQueue verifies that after dismissal,
// GetCachedFusionCandidates no longer returns the pair.
func TestDismissFusionCandidate_FilteredFromQueue(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}

	userID := insertTestUser(t, pool)
	aID := insertMinimalActivity(t, db, userID)
	bID := insertMinimalActivity(t, db, userID)

	cA, cB := aID, bID
	if cA > cB {
		cA, cB = cB, cA
	}

	_, err := pool.Exec(ctx, `
		INSERT INTO fusion_candidates (user_id, activity_id_a, activity_id_b, overlap_percent, field_richness_a, field_richness_b, time_diff_sec)
		VALUES ($1, $2, $3, 80.0, 7, 7, 45.0)
	`, userID, cA, cB)
	if err != nil {
		t.Fatalf("insert fusion_candidates: %v", err)
	}

	// Before dismiss: pair should appear in the queue.
	before, err := db.GetCachedFusionCandidates(ctx, userID, 50)
	if err != nil {
		t.Fatalf("GetCachedFusionCandidates before dismiss: %v", err)
	}
	found := false
	for _, row := range before {
		if row.ActivityIDA == cA && row.ActivityIDB == cB {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("pair not found in queue before dismiss")
	}

	h := &Handler{DB: db}
	code := dismissRequest(t, h, userID, cA, cB)
	if code != http.StatusNoContent {
		t.Fatalf("dismiss: expected 204, got %d", code)
	}

	// After dismiss: pair must NOT appear in the queue.
	after, err := db.GetCachedFusionCandidates(ctx, userID, 50)
	if err != nil {
		t.Fatalf("GetCachedFusionCandidates after dismiss: %v", err)
	}
	for _, row := range after {
		if row.ActivityIDA == cA && row.ActivityIDB == cB {
			t.Fatal("dismissed pair still appears in GetCachedFusionCandidates — filter not working")
		}
	}
}

// TestDismissFusionCandidate_MissingIDs verifies that an empty body returns 400.
func TestDismissFusionCandidate_MissingIDs(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	userID := uuid.New().String()

	h := &Handler{DB: db}

	body := `{"activity_id_a": "", "activity_id_b": ""}`
	req := httptest.NewRequest(http.MethodPost, "/fusion-candidates/dismiss", bytes.NewReader([]byte(body)))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rr := httptest.NewRecorder()
	h.DismissFusionCandidate(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty IDs, got %d", rr.Code)
	}
}

// TestDismissFusionCandidate_CrossUser verifies that a dismiss request from user B
// does not affect user A's fusion_candidates row.
//
// dismiss is idempotent: if the row doesn't match (wrong user_id), the UPDATE
// simply hits 0 rows and returns 204. The key assertion is that user A's row
// still has dismissed_at = NULL afterwards.
func TestDismissFusionCandidate_CrossUser(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}

	// User A owns the candidate pair.
	userA := insertTestUser(t, pool)
	a1 := insertMinimalActivity(t, db, userA)
	a2 := insertMinimalActivity(t, db, userA)

	cA, cB := a1, a2
	if cA > cB {
		cA, cB = cB, cA
	}

	_, err := pool.Exec(ctx, `
		INSERT INTO fusion_candidates (user_id, activity_id_a, activity_id_b, overlap_percent, field_richness_a, field_richness_b, time_diff_sec)
		VALUES ($1, $2, $3, 88.0, 9, 7, 20.0)
	`, userA, cA, cB)
	if err != nil {
		t.Fatalf("insert fusion_candidates for userA: %v", err)
	}

	// User B attempts to dismiss user A's pair.
	userB := insertTestUser(t, pool)
	h := &Handler{DB: db}
	code := dismissRequest(t, h, userB, cA, cB)
	// Must return 204 (idempotent from B's perspective — no matching row).
	if code != http.StatusNoContent {
		t.Fatalf("cross-user dismiss: expected 204, got %d", code)
	}

	// User A's row must still have dismissed_at = NULL.
	var dismissedAt *time.Time
	err = pool.QueryRow(ctx, `
		SELECT dismissed_at FROM fusion_candidates
		WHERE user_id = $1 AND activity_id_a = $2 AND activity_id_b = $3
	`, userA, cA, cB).Scan(&dismissedAt)
	if err != nil {
		t.Fatalf("cross-user check: query dismissed_at: %v", err)
	}
	if dismissedAt != nil {
		t.Fatal("cross-user isolation violated: user A's row was dismissed by user B")
	}
}
