package handler

// Phase 6.1.3.3 — tests for the PATCH .../unconfirm endpoint.
//
// Coverage:
//   1. Success: confirmed heuristic candidate → 204, confirmed=false, lift_count=0.
//   2. OSM source → 204, same review semantics as heuristic rows.
//   3. Already rejected (rejected_by_user=true) → 409 ALREADY_REJECTED (F12: structured error).
//   4. Different user → 404 (multi-user isolation, activity-ownership check fires first).
//   5. Rule retraction: after unconfirm the lift_detected system tag is removed.
//   6. Non-existent candidate ID → 404 NOT_FOUND (F12: structured error with code field).
//   7. Wrong activity ID → 404 NOT_FOUND (F12: collapses to NOT_FOUND to avoid owner-leak).

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// callUnconfirmHandler fires PATCH on the unconfirm endpoint via chi mux +
// fake-auth context mirroring middleware.AuthMiddleware's UserIDKey injection.
func callUnconfirmHandler(t *testing.T, h *Handler, userID, activityID, candID string) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	r.Patch("/api/v1/activities/{id}/lift-candidates/{cid}/unconfirm", h.UnconfirmLiftCandidate)
	req := httptest.NewRequest(http.MethodPatch,
		"/api/v1/activities/"+activityID+"/lift-candidates/"+candID+"/unconfirm", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// insertConfirmedHeuristicCandidate inserts a confirmed heuristic candidate
// and returns its ID.
func insertConfirmedHeuristicCandidate(t *testing.T, db *database.DB, activityID string) string {
	t.Helper()
	start := time.Now().UTC().Truncate(time.Second)
	var id string
	if err := db.Pool.QueryRow(context.Background(), `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m,
			 confidence, confirmed, source)
		VALUES ($1, $2, $3, 180, 120, 0, 700, 'high', true, 'heuristic')
		RETURNING id::text
	`, activityID, start, start.Add(3*time.Minute)).Scan(&id); err != nil {
		t.Fatalf("insertConfirmedHeuristicCandidate: %v", err)
	}
	return id
}

// liftCount returns activities.lift_count for the given activity.
func liftCount(t *testing.T, db *database.DB, activityID string) int {
	t.Helper()
	var count int
	if err := db.Pool.QueryRow(context.Background(),
		`SELECT COALESCE(lift_count, 0) FROM activities WHERE id = $1`, activityID,
	).Scan(&count); err != nil {
		t.Fatalf("liftCount query: %v", err)
	}
	return count
}

// isConfirmed returns the confirmed flag for the given candidate row.
func isConfirmed(t *testing.T, db *database.DB, candID string) bool {
	t.Helper()
	var confirmed bool
	if err := db.Pool.QueryRow(context.Background(),
		`SELECT confirmed FROM activity_lift_candidates WHERE id = $1`, candID,
	).Scan(&confirmed); err != nil {
		t.Fatalf("isConfirmed query: %v", err)
	}
	return confirmed
}

// TestUnconfirmLiftCandidate_Success: a confirmed heuristic candidate is
// flipped back, lift_count drops to 0, and the row is not deleted.
func TestUnconfirmLiftCandidate_Success(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	// Manually set lift_count=1 to simulate a prior recompute that counted
	// this confirmed candidate. The unconfirm path calls recomputeLiftStatsTx
	// which will set it back to 0.
	candID := insertConfirmedHeuristicCandidate(t, db, activityID)
	if _, err := db.Pool.Exec(context.Background(),
		`UPDATE activities SET lift_count = 1 WHERE id = $1`, activityID,
	); err != nil {
		t.Fatalf("set lift_count=1: %v", err)
	}

	rec := callUnconfirmHandler(t, h, userID, activityID, candID)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("UnconfirmLiftCandidate status=%d, want 204; body=%s", rec.Code, rec.Body.String())
	}

	if isConfirmed(t, db, candID) {
		t.Error("confirmed = true after unconfirm, want false")
	}
	if got := liftCount(t, db, activityID); got != 0 {
		t.Errorf("lift_count = %d after unconfirm, want 0", got)
	}
}

// TestUnconfirmLiftCandidate_OSMSourceSucceeds: OSM-sourced candidates use the
// same review semantics as heuristic rows.
func TestUnconfirmLiftCandidate_OSMSourceSucceeds(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	// source='osm', confirmed=true
	start := time.Now().UTC().Truncate(time.Second)
	candID := insertUnifiedLiftCandidate(t, db, activityID, start, "osm", true, nil)

	rec := callUnconfirmHandler(t, h, userID, activityID, candID)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("OSM unconfirm status=%d, want 204; body=%s", rec.Code, rec.Body.String())
	}

	if isConfirmed(t, db, candID) {
		t.Error("OSM candidate confirmed flag is still true after unconfirm")
	}
}

// TestUnconfirmLiftCandidate_AlreadyRejectedReturns409: a soft-rejected row
// (rejected_by_user=true) must not be unconfirmable — return 409 ALREADY_REJECTED (F12).
func TestUnconfirmLiftCandidate_AlreadyRejectedReturns409(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	start := time.Now().UTC().Truncate(time.Second)
	candID := insertRejectedLiftCandidate(t, db, activityID, start)

	rec := callUnconfirmHandler(t, h, userID, activityID, candID)
	if rec.Code != http.StatusConflict {
		t.Fatalf("rejected candidate unconfirm status=%d, want 409; body=%s", rec.Code, rec.Body.String())
	}
	assertLiftProblemCode(t, rec, "ALREADY_REJECTED")

	// Row must be untouched (rejected_by_user still true).
	var rejectedByUser bool
	if err := db.Pool.QueryRow(context.Background(),
		`SELECT rejected_by_user FROM activity_lift_candidates WHERE id = $1`, candID,
	).Scan(&rejectedByUser); err != nil {
		t.Fatalf("query rejected_by_user: %v", err)
	}
	if !rejectedByUser {
		t.Error("rejected_by_user was cleared — WHERE restriction failed")
	}
}

// TestUnconfirmLiftCandidate_DifferentUserReturns404: user B must not be able
// to unconfirm a candidate belonging to user A's activity.
func TestUnconfirmLiftCandidate_DifferentUserReturns404(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userA := insertTestUser(t, db.Pool)
	userB := insertTestUser(t, db.Pool)
	activityA := insertMinimalActivity(t, db, userA)

	candID := insertConfirmedHeuristicCandidate(t, db, activityA)

	// userB attempts to unconfirm userA's candidate — must be rejected at the
	// activity-ownership check (activity.UserID != userID → 404 before the DB
	// method is even called).
	rec := callUnconfirmHandler(t, h, userB, activityA, candID)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-user unconfirm status=%d, want 404; body=%s", rec.Code, rec.Body.String())
	}

	// Candidate confirmed flag must be untouched.
	if !isConfirmed(t, db, candID) {
		t.Error("confirmed flag was altered by cross-user unconfirm — isolation failed")
	}
}

// TestUnconfirmLiftCandidate_RetractsLiftTag: after unconfirm the
// lift_detected system rule re-fires and removes the Lift tag when
// lift_count drops to 0.
func TestUnconfirmLiftCandidate_RetractsLiftTag(t *testing.T) {
	h, db, userID, activityID, candID := liftRuleTriggerTestSetup(t)

	// Confirm first so Lift tag is attached.
	if rec := callConfirmHandler(t, h, userID, activityID, candID); rec.Code != http.StatusNoContent {
		t.Fatalf("setup confirm: status %d, body=%s", rec.Code, rec.Body.String())
	}
	if !hasLiftTag(t, db, activityID) {
		t.Fatalf("setup: Lift tag must be present after Confirm")
	}

	// Now unconfirm the same (only) candidate. lift_count drops to 0 →
	// retraction semantics remove the Lift tag.
	rec := callUnconfirmHandler(t, h, userID, activityID, candID)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("UnconfirmLiftCandidate status=%d, want 204; body=%s", rec.Code, rec.Body.String())
	}

	if hasLiftTag(t, db, activityID) {
		t.Error("Lift tag still present after unconfirm — rule retraction did not fire")
	}
}

// ----- F12 structured-error tests ----------------------------------------

// assertLiftProblemCode decodes the response body as a model.ProblemDetail
// (via its structured `code` field — see httputil.WriteProblem) and asserts
// the code matches wantCode. Fails the test on decode error.
func assertLiftProblemCode(t *testing.T, rec *httptest.ResponseRecorder, wantCode string) {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("assertLiftProblemCode: decode body: %v; raw=%s", err, rec.Body.String())
	}
	if body.Code != wantCode {
		t.Errorf("problem code=%q, want %q", body.Code, wantCode)
	}
}

// TestUnconfirmLiftCandidate_NonExistentCandidateReturns404WithCode: a
// completely unknown candidate UUID must return 404 NOT_FOUND with code (F12).
func TestUnconfirmLiftCandidate_NonExistentCandidateReturns404WithCode(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	ghostID := uuid.NewString()
	rec := callUnconfirmHandler(t, h, userID, activityID, ghostID)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("non-existent candidate status=%d, want 404; body=%s", rec.Code, rec.Body.String())
	}
	assertLiftProblemCode(t, rec, "NOT_FOUND")
}

func TestConfirmLiftCandidate_NonExistentCandidateReturns404WithCode(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	ghostID := uuid.NewString()
	rec := callConfirmHandler(t, h, userID, activityID, ghostID)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("non-existent candidate confirm status=%d, want 404; body=%s", rec.Code, rec.Body.String())
	}
	assertLiftProblemCode(t, rec, "NOT_FOUND")
}

func TestRejectLiftCandidate_NonExistentCandidateReturns404WithCode(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	ghostID := uuid.NewString()
	rec := callRejectHandler(t, h, userID, activityID, ghostID)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("non-existent candidate reject status=%d, want 404; body=%s", rec.Code, rec.Body.String())
	}
	assertLiftProblemCode(t, rec, "NOT_FOUND")
}

// TestUnconfirmLiftCandidate_WrongActivityIDReturns404WithCode: a candidate
// that belongs to a different activity must return 404 NOT_FOUND (collapses
// cross-activity case to avoid candidate-existence leak, F12).
func TestUnconfirmLiftCandidate_WrongActivityIDReturns404WithCode(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityA := insertMinimalActivity(t, db, userID)
	activityB := insertMinimalActivity(t, db, userID)

	// Candidate belongs to activityA; request uses activityB as the URL param.
	candID := insertConfirmedHeuristicCandidate(t, db, activityA)

	rec := callUnconfirmHandler(t, h, userID, activityB, candID)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("wrong-activity unconfirm status=%d, want 404; body=%s", rec.Code, rec.Body.String())
	}
	assertLiftProblemCode(t, rec, "NOT_FOUND")

	// Original candidate must be untouched.
	if !isConfirmed(t, db, candID) {
		t.Error("confirmed flag altered by wrong-activity unconfirm — isolation failed")
	}
}
