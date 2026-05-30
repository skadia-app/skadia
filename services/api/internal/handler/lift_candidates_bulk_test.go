package handler

// Phase 6.3.1 — bulk endpoints + acknowledge + recalculate + restore.
//
// Endpoints under test:
//   POST /api/v1/activities/{id}/lifts/confirm-all       → ConfirmAllLiftCandidates
//   POST /api/v1/activities/{id}/lifts/reject-all        → RejectAllLiftCandidates
//   POST /api/v1/activities/{id}/lifts/acknowledge       → AcknowledgeLifts
//   POST /api/v1/activities/{id}/lifts/recalculate-all   → RecalculateActivityLifts
//   POST /api/v1/lift-candidates/{id}/restore            → RestoreLiftCandidate
//
// Co-existence note: the existing user-wide POST /api/v1/lifts/recalculate-all
// (handler.RecalculateLifts) is NOT replaced — it has different semantics
// (clear rejected_by_user flag, no DELETE, no re-enrichment). The new
// activity-scoped recalculate-all endpoint (RecalculateActivityLifts) hard-
// resets one activity: DELETE all candidates (including rejected),
// DELETE coupled lift segment_efforts, clear lifts_acknowledged_at, then
// trigger re-enrichment AFTER the transaction commits.
//
// Test isolation: every test uses testdb.RequireSafeDSN(t) via fusionTestPool,
// so each parallel worktree runs against its own skadia_test_<slug> database.
//
// Recalculate-test scope (Option A, user-confirmed 2026-05-08): this file's
// recalculate-all test verifies the reset-state semantics (DELETEs,
// acknowledged_at clearing, lift_count = 0) but NOT the re-detection output —
// re-enrichment of an empty test stream is a no-op (stream_enrich.go:139
// short-circuits on len(stream) == 0). End-to-end detection coverage is
// in Phase 8 smoke against real activity streams.

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
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// ----- helpers -----------------------------------------------------------

// callBulkLifts fires a POST against /api/v1/activities/{id}/lifts/<action>
// via a minimal chi mux + fake-auth context. The handler-under-test is
// looked up via the action string.
func callBulkLifts(
	t *testing.T,
	h *Handler,
	userID, activityID, action string,
) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	switch action {
	case "confirm-all":
		r.Post("/api/v1/activities/{id}/lifts/confirm-all", h.ConfirmAllLiftCandidates)
	case "reject-all":
		r.Post("/api/v1/activities/{id}/lifts/reject-all", h.RejectAllLiftCandidates)
	case "acknowledge":
		r.Post("/api/v1/activities/{id}/lifts/acknowledge", h.AcknowledgeLifts)
	case "recalculate-all":
		r.Post("/api/v1/activities/{id}/lifts/recalculate-all", h.RecalculateActivityLifts)
	default:
		t.Fatalf("callBulkLifts: unknown action %q", action)
	}
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/activities/"+activityID+"/lifts/"+action, nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// callRestoreLiftCandidate fires POST against the new restore endpoint.
// The route is /api/v1/lift-candidates/{id}/restore — note that there is
// no activity-id in the URL (regression case chose this
// form so the inspector can hit it without re-deriving the activity_id
// from the candidate row).
func callRestoreLiftCandidate(
	t *testing.T,
	h *Handler,
	userID, candidateID string,
) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	r.Post("/api/v1/lift-candidates/{id}/restore", h.RestoreLiftCandidate)
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/lift-candidates/"+candidateID+"/restore", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// liftAcknowledgedAt reads activities.lifts_acknowledged_at as a nullable
// timestamp. Returns nil when the column is NULL.
func liftAcknowledgedAt(t *testing.T, db *database.DB, activityID string) *time.Time {
	t.Helper()
	var ts *time.Time
	if err := db.Pool.QueryRow(context.Background(),
		`SELECT lifts_acknowledged_at FROM activities WHERE id = $1`, activityID,
	).Scan(&ts); err != nil {
		t.Fatalf("query lifts_acknowledged_at: %v", err)
	}
	return ts
}

// candidateState is a compact projection of one row in
// activity_lift_candidates used by the bulk-endpoint tests.
type candidateState struct {
	ID             string
	Source         string
	Confirmed      bool
	RejectedByUser bool
}

// loadCandidate reads one candidate row by id.
func loadCandidate(t *testing.T, db *database.DB, candidateID string) candidateState {
	t.Helper()
	var s candidateState
	s.ID = candidateID
	if err := db.Pool.QueryRow(context.Background(),
		`SELECT source, confirmed, rejected_by_user
		 FROM activity_lift_candidates WHERE id = $1`,
		candidateID,
	).Scan(&s.Source, &s.Confirmed, &s.RejectedByUser); err != nil {
		t.Fatalf("loadCandidate %s: %v", candidateID, err)
	}
	return s
}

// candidateExists reports whether a candidate row with the given id is still
// present (not deleted).
func candidateExists(t *testing.T, db *database.DB, candidateID string) bool {
	t.Helper()
	var n int
	if err := db.Pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM activity_lift_candidates WHERE id = $1`, candidateID,
	).Scan(&n); err != nil {
		t.Fatalf("candidateExists: %v", err)
	}
	return n > 0
}

// insertHeuristicUnconfirmed inserts a heuristic candidate with
// confirmed=false, rejected_by_user=false. Returns its id.
func insertHeuristicUnconfirmed(t *testing.T, db *database.DB, activityID string, startedAt time.Time) string {
	t.Helper()
	return insertUnifiedLiftCandidate(t, db, activityID, startedAt, "heuristic", false, nil)
}

// insertOSMConfirmed inserts an OSM-source candidate confirmed=true and
// (optionally) linked to a segment_effort row. effortID may be nil.
func insertOSMConfirmed(t *testing.T, db *database.DB, activityID string, startedAt time.Time, effortID *string) string {
	t.Helper()
	return insertUnifiedLiftCandidate(t, db, activityID, startedAt, "osm", true, effortID)
}

// setRejected marks an existing candidate as rejected_by_user=true (test
// fixture only; the handler does this via the soft-reject path).
func setRejected(t *testing.T, db *database.DB, candidateID string) {
	t.Helper()
	if _, err := db.Pool.Exec(context.Background(),
		`UPDATE activity_lift_candidates SET rejected_by_user = true WHERE id = $1`,
		candidateID,
	); err != nil {
		t.Fatalf("setRejected: %v", err)
	}
}

// insertLiftSegmentEffort sets up a lift-segment + segment_effort pair
// referenced by an OSM-source candidate. Mirrors the production schema
// shape used in lift_osm_reject_test.go.
//
// Returns the segment_effort id so the caller can link it via the
// segment_effort_id column on activity_lift_candidates.
func insertLiftSegmentEffort(t *testing.T, db *database.DB, userID, activityID string, startedAt time.Time) string {
	t.Helper()
	ctx := context.Background()
	segID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO segments
			(id, name, segment_type, distance_m, elevation_gain_m, sport_group, created_by)
		VALUES ($1, 'Lift Bulk Fixture', 'lift', 600, 100, 'skiing', $2)
	`, segID, userID); err != nil {
		t.Fatalf("insert lift segment: %v", err)
	}
	versionID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO segment_versions
			(id, segment_id, version_number, track, valid_from, created_by)
		VALUES ($1, $2, 1,
		        ST_GeomFromText('LINESTRING(11.0 47.0, 11.005 47.0)', 4326),
		        CURRENT_DATE, $3)
	`, versionID, segID, userID); err != nil {
		t.Fatalf("insert segment_version: %v", err)
	}
	effortID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO segment_efforts
			(id, activity_id, segment_id, segment_version_id, user_id,
			 started_at, elapsed_time_s, distance_m, elevation_gain_m,
			 is_reverse, sport_type, sub_type, in_leaderboard, is_lift_segment_match)
		VALUES ($1, $2, $3, $4, $5, $6, 300, 600, 100, false,
		        'skiing', 'generic', false, true)
	`, effortID, activityID, segID, versionID, userID, startedAt); err != nil {
		t.Fatalf("insert segment_effort: %v", err)
	}
	return effortID
}

// segmentEffortExists reports whether a segment_efforts row with the given
// id is still present. Used by the Method-A reject test to assert that the
// soft-reject path preserves the audit row physically.
func segmentEffortExists(t *testing.T, db *database.DB, effortID string) bool {
	t.Helper()
	var n int
	if err := db.Pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM segment_efforts WHERE id = $1`, effortID,
	).Scan(&n); err != nil {
		t.Fatalf("segmentEffortExists: %v", err)
	}
	return n > 0
}

// ----- Confirm-All -------------------------------------------------------

// TestConfirmAllLiftCandidates_BulkConfirmsAndAcknowledges: 5 unconfirmed
// heuristic candidates → POST flips all to confirmed=true, sets
// lifts_acknowledged_at to now-ish, lift_count = 5.
//
// Method-A is intentionally untouched: the test inserts one extra OSM
// candidate that is already confirmed=true; after the bulk it must remain
// at confirmed=true with rejected_by_user=false.
//
// Idempotency: a second POST is a no-op (counts unchanged).
func TestConfirmAllLiftCandidates_BulkConfirmsAndAcknowledges(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	now := time.Now().UTC().Truncate(time.Second)
	heuristicIDs := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		heuristicIDs = append(heuristicIDs,
			insertHeuristicUnconfirmed(t, db, activityID, now.Add(time.Duration(i*10)*time.Minute)))
	}
	osmID := insertOSMConfirmed(t, db, activityID, now.Add(2*time.Hour), nil)

	rec := callBulkLifts(t, h, userID, activityID, "confirm-all")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("confirm-all status=%d, want 204; body=%s", rec.Code, rec.Body.String())
	}

	for _, id := range heuristicIDs {
		s := loadCandidate(t, db, id)
		if !s.Confirmed {
			t.Errorf("heuristic candidate %s confirmed=false after bulk confirm", id)
		}
		if s.RejectedByUser {
			t.Errorf("heuristic candidate %s rejected_by_user=true after bulk confirm", id)
		}
	}
	// Method-A row must be untouched.
	if got := loadCandidate(t, db, osmID); !got.Confirmed || got.RejectedByUser {
		t.Errorf("OSM candidate altered by confirm-all: %+v", got)
	}

	if got := liftAcknowledgedAt(t, db, activityID); got == nil {
		t.Errorf("lifts_acknowledged_at = nil after confirm-all, want non-nil")
	}
	if got := liftCount(t, db, activityID); got != 5 {
		t.Errorf("lift_count = %d after confirm-all, want 5", got)
	}

	// Idempotency: a second confirm-all must not change counts.
	ackBefore := liftAcknowledgedAt(t, db, activityID)
	rec = callBulkLifts(t, h, userID, activityID, "confirm-all")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("second confirm-all status=%d, want 204", rec.Code)
	}
	if got := liftCount(t, db, activityID); got != 5 {
		t.Errorf("lift_count = %d after second confirm-all, want 5", got)
	}
	// acknowledged_at MAY refresh on a re-acknowledge (per-call NOW()),
	// but must still be non-nil. We do not assert it stays equal — the
	// handler's contract is "set when missing, refresh is acceptable".
	if got := liftAcknowledgedAt(t, db, activityID); got == nil {
		t.Errorf("lifts_acknowledged_at cleared by second confirm-all (was %v)", ackBefore)
	}
}

// TestConfirmAllLiftCandidates_DifferentUserReturns404: cross-user POST
// must be rejected at the activity-ownership check.
func TestConfirmAllLiftCandidates_DifferentUserReturns404(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userA := insertTestUser(t, db.Pool)
	userB := insertTestUser(t, db.Pool)
	activityA := insertMinimalActivity(t, db, userA)

	candID := insertHeuristicUnconfirmed(t, db, activityA, time.Now().UTC().Truncate(time.Second))

	rec := callBulkLifts(t, h, userB, activityA, "confirm-all")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-user confirm-all status=%d, want 404; body=%s",
			rec.Code, rec.Body.String())
	}
	if s := loadCandidate(t, db, candID); s.Confirmed {
		t.Error("candidate confirmed by cross-user request — isolation failed")
	}
}

// ----- Reject-All --------------------------------------------------------

// TestRejectAllLiftCandidates_SoftRejectsAndAcknowledges: 5 unconfirmed
// heuristic → POST sets all rejected_by_user=true, confirmed stays false,
// lift_count=0, acknowledged_at non-nil. Method-A is untouched.
func TestRejectAllLiftCandidates_SoftRejectsAndAcknowledges(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	now := time.Now().UTC().Truncate(time.Second)
	heuristicIDs := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		heuristicIDs = append(heuristicIDs,
			insertHeuristicUnconfirmed(t, db, activityID, now.Add(time.Duration(i*10)*time.Minute)))
	}
	osmID := insertOSMConfirmed(t, db, activityID, now.Add(2*time.Hour), nil)

	rec := callBulkLifts(t, h, userID, activityID, "reject-all")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("reject-all status=%d, want 204; body=%s", rec.Code, rec.Body.String())
	}

	for _, id := range heuristicIDs {
		s := loadCandidate(t, db, id)
		if !s.RejectedByUser {
			t.Errorf("heuristic candidate %s rejected_by_user=false after bulk reject", id)
		}
		if s.Confirmed {
			t.Errorf("heuristic candidate %s confirmed=true after bulk reject — must stay false", id)
		}
	}
	if got := loadCandidate(t, db, osmID); !got.Confirmed || got.RejectedByUser {
		t.Errorf("OSM candidate altered by reject-all: %+v", got)
	}

	if got := liftAcknowledgedAt(t, db, activityID); got == nil {
		t.Errorf("lifts_acknowledged_at = nil after reject-all, want non-nil")
	}
	// Heuristic candidates are now rejected, OSM (without segment_effort link)
	// has no segment_efforts row → lift_count must be 0.
	if got := liftCount(t, db, activityID); got != 0 {
		t.Errorf("lift_count = %d after reject-all, want 0", got)
	}

	// Idempotency.
	rec = callBulkLifts(t, h, userID, activityID, "reject-all")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("second reject-all status=%d, want 204", rec.Code)
	}
	if got := liftCount(t, db, activityID); got != 0 {
		t.Errorf("lift_count = %d after second reject-all, want 0", got)
	}
}

// TestRejectAllLiftCandidates_DifferentUserReturns404: cross-user.
func TestRejectAllLiftCandidates_DifferentUserReturns404(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userA := insertTestUser(t, db.Pool)
	userB := insertTestUser(t, db.Pool)
	activityA := insertMinimalActivity(t, db, userA)

	candID := insertHeuristicUnconfirmed(t, db, activityA, time.Now().UTC().Truncate(time.Second))

	rec := callBulkLifts(t, h, userB, activityA, "reject-all")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-user reject-all status=%d, want 404; body=%s",
			rec.Code, rec.Body.String())
	}
	if s := loadCandidate(t, db, candID); s.RejectedByUser {
		t.Error("candidate rejected by cross-user request — isolation failed")
	}
}

// ----- Acknowledge -------------------------------------------------------

// TestAcknowledgeLifts_DoesNotChangeCandidates: 3 mixed-state candidates →
// POST acknowledge → lifts_acknowledged_at non-nil, all candidates exactly
// as before.
func TestAcknowledgeLifts_DoesNotChangeCandidates(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	now := time.Now().UTC().Truncate(time.Second)
	openID := insertHeuristicUnconfirmed(t, db, activityID, now)
	rejectedID := insertHeuristicUnconfirmed(t, db, activityID, now.Add(10*time.Minute))
	setRejected(t, db, rejectedID)
	osmID := insertOSMConfirmed(t, db, activityID, now.Add(20*time.Minute), nil)

	before := []candidateState{
		loadCandidate(t, db, openID),
		loadCandidate(t, db, rejectedID),
		loadCandidate(t, db, osmID),
	}

	rec := callBulkLifts(t, h, userID, activityID, "acknowledge")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("acknowledge status=%d, want 204; body=%s", rec.Code, rec.Body.String())
	}

	if got := liftAcknowledgedAt(t, db, activityID); got == nil {
		t.Errorf("lifts_acknowledged_at = nil after acknowledge, want non-nil")
	}

	after := []candidateState{
		loadCandidate(t, db, openID),
		loadCandidate(t, db, rejectedID),
		loadCandidate(t, db, osmID),
	}
	for i := range before {
		if before[i] != after[i] {
			t.Errorf("candidate %d changed by acknowledge: before=%+v after=%+v",
				i, before[i], after[i])
		}
	}
}

// TestAcknowledgeLifts_DifferentUserReturns404: cross-user.
func TestAcknowledgeLifts_DifferentUserReturns404(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userA := insertTestUser(t, db.Pool)
	userB := insertTestUser(t, db.Pool)
	activityA := insertMinimalActivity(t, db, userA)

	rec := callBulkLifts(t, h, userB, activityA, "acknowledge")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-user acknowledge status=%d, want 404; body=%s",
			rec.Code, rec.Body.String())
	}
	if got := liftAcknowledgedAt(t, db, activityA); got != nil {
		t.Errorf("lifts_acknowledged_at set by cross-user request: %v", got)
	}
}

// ----- Restore -----------------------------------------------------------

// TestRestoreLiftCandidate_HeuristicClearsRejectedAndUnconfirms:
// rejected heuristic → restore → rejected_by_user=false, confirmed=false
// (user must re-confirm or re-reject).
func TestRestoreLiftCandidate_HeuristicClearsRejectedAndUnconfirms(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	now := time.Now().UTC().Truncate(time.Second)
	candID := insertHeuristicUnconfirmed(t, db, activityID, now)
	setRejected(t, db, candID)

	rec := callRestoreLiftCandidate(t, h, userID, candID)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("restore heuristic status=%d, want 204; body=%s", rec.Code, rec.Body.String())
	}

	got := loadCandidate(t, db, candID)
	if got.RejectedByUser {
		t.Errorf("heuristic restore left rejected_by_user=true")
	}
	if got.Confirmed {
		t.Errorf("heuristic restore set confirmed=true; want confirmed=false")
	}
	if got.Source != "heuristic" {
		t.Errorf("heuristic restore changed source to %q", got.Source)
	}
	// Restored heuristic without confirmation must NOT count.
	if c := liftCount(t, db, activityID); c != 0 {
		t.Errorf("lift_count = %d after heuristic restore, want 0 (unconfirmed)", c)
	}
}

// TestRestoreLiftCandidate_OSMClearsRejectedAndUnconfirms:
// rejected OSM → restore → rejected_by_user=false, confirmed=false.
func TestRestoreLiftCandidate_OSMClearsRejectedAndUnconfirms(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	now := time.Now().UTC().Truncate(time.Second)
	candID := insertOSMConfirmed(t, db, activityID, now, nil)
	setRejected(t, db, candID)

	rec := callRestoreLiftCandidate(t, h, userID, candID)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("restore osm status=%d, want 204; body=%s", rec.Code, rec.Body.String())
	}

	got := loadCandidate(t, db, candID)
	if got.RejectedByUser {
		t.Errorf("osm restore left rejected_by_user=true")
	}
	if got.Confirmed {
		t.Errorf("osm restore left confirmed=true; want confirmed=false for review")
	}
	if got.Source != "osm" {
		t.Errorf("osm restore changed source to %q", got.Source)
	}
}

// TestRestoreLiftCandidate_DifferentUserReturns404: cross-user.
func TestRestoreLiftCandidate_DifferentUserReturns404(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userA := insertTestUser(t, db.Pool)
	userB := insertTestUser(t, db.Pool)
	activityA := insertMinimalActivity(t, db, userA)

	candID := insertHeuristicUnconfirmed(t, db, activityA, time.Now().UTC().Truncate(time.Second))
	setRejected(t, db, candID)

	rec := callRestoreLiftCandidate(t, h, userB, candID)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-user restore status=%d, want 404; body=%s",
			rec.Code, rec.Body.String())
	}
	if !loadCandidate(t, db, candID).RejectedByUser {
		t.Error("rejected_by_user cleared by cross-user restore — isolation failed")
	}
}

// TestRestoreLiftCandidate_NonRejectedReturns404: a candidate that is not
// in the rejected state has nothing to restore — must return 404.
func TestRestoreLiftCandidate_NonRejectedReturns404(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	candID := insertHeuristicUnconfirmed(t, db, activityID, time.Now().UTC().Truncate(time.Second))

	rec := callRestoreLiftCandidate(t, h, userID, candID)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("restore non-rejected status=%d, want 404; body=%s",
			rec.Code, rec.Body.String())
	}
	got := loadCandidate(t, db, candID)
	if got.RejectedByUser || got.Confirmed {
		t.Errorf("non-rejected restore mutated row: %+v", got)
	}
}

// ----- Recalculate-All ---------------------------------------------------

// TestRecalculateAllLifts_ResetsState: 3 mixed-state heuristic candidates
// (open, confirmed, rejected) + 1 OSM-confirmed → POST recalculate-all →
// all rows DELETE'd, lifts_acknowledged_at = NULL, lift_count = 0.
//
// Re-detection output is NOT asserted — see file-level comment.
func TestRecalculateAllLifts_ResetsState(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	// Pre-set acknowledged_at so we can prove the endpoint clears it.
	if _, err := db.Pool.Exec(context.Background(),
		`UPDATE activities SET lifts_acknowledged_at = NOW() WHERE id = $1`,
		activityID,
	); err != nil {
		t.Fatalf("pre-set acknowledged_at: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	openID := insertHeuristicUnconfirmed(t, db, activityID, now)

	confirmedID := insertHeuristicUnconfirmed(t, db, activityID, now.Add(10*time.Minute))
	if _, err := db.Pool.Exec(context.Background(),
		`UPDATE activity_lift_candidates SET confirmed = true WHERE id = $1`,
		confirmedID,
	); err != nil {
		t.Fatalf("set confirmed: %v", err)
	}

	rejectedID := insertHeuristicUnconfirmed(t, db, activityID, now.Add(20*time.Minute))
	setRejected(t, db, rejectedID)

	osmID := insertOSMConfirmed(t, db, activityID, now.Add(30*time.Minute), nil)

	// Set lift_count to a non-zero value so we can prove the endpoint clears it.
	if _, err := db.Pool.Exec(context.Background(),
		`UPDATE activities SET lift_count = 99 WHERE id = $1`, activityID,
	); err != nil {
		t.Fatalf("pre-set lift_count: %v", err)
	}

	rec := callBulkLifts(t, h, userID, activityID, "recalculate-all")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("recalculate-all status=%d, want 204; body=%s", rec.Code, rec.Body.String())
	}

	// All 4 rows must be gone (Hard-Reset includes the rejected row).
	for _, id := range []string{openID, confirmedID, rejectedID, osmID} {
		if candidateExists(t, db, id) {
			t.Errorf("candidate %s still present after recalculate-all", id)
		}
	}

	if got := liftAcknowledgedAt(t, db, activityID); got != nil {
		t.Errorf("lifts_acknowledged_at = %v after recalculate-all, want nil", got)
	}
	if got := liftCount(t, db, activityID); got != 0 {
		t.Errorf("lift_count = %d after recalculate-all, want 0", got)
	}
}

// TestRecalculateAllLifts_DeletesCoupledSegmentEfforts: an OSM candidate
// whose segment_effort_id points at a lift segment_effort row → recalculate
// must DELETE both the candidate AND the coupled segment_effort.
func TestRecalculateAllLifts_DeletesCoupledSegmentEfforts(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	now := time.Now().UTC().Truncate(time.Second)
	effortID := insertLiftSegmentEffort(t, db, userID, activityID, now)
	candID := insertOSMConfirmed(t, db, activityID, now, &effortID)

	rec := callBulkLifts(t, h, userID, activityID, "recalculate-all")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("recalculate-all status=%d, want 204; body=%s", rec.Code, rec.Body.String())
	}

	if candidateExists(t, db, candID) {
		t.Error("OSM candidate still present after recalculate-all")
	}
	if segmentEffortExists(t, db, effortID) {
		t.Error("coupled segment_effort still present after recalculate-all")
	}
}

// TestRecalculateAllLifts_DifferentUserReturns404: cross-user.
func TestRecalculateAllLifts_DifferentUserReturns404(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userA := insertTestUser(t, db.Pool)
	userB := insertTestUser(t, db.Pool)
	activityA := insertMinimalActivity(t, db, userA)

	candID := insertHeuristicUnconfirmed(t, db, activityA, time.Now().UTC().Truncate(time.Second))

	rec := callBulkLifts(t, h, userB, activityA, "recalculate-all")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-user recalculate-all status=%d, want 404; body=%s",
			rec.Code, rec.Body.String())
	}
	if !candidateExists(t, db, candID) {
		t.Error("candidate deleted by cross-user recalculate — isolation failed")
	}
}

// ----- Method-A reject + recompute exclude (Test #7, Adv-Review #21) ---

// TestRejectMethodALift_ExcludesFromAggregates: an OSM candidate with
// segment_effort_id linked → DELETE /api/v1/activities/{id}/lift-candidates/{cid}
// (existing endpoint, RejectLiftCandidate). Post: rejected_by_user=true,
// segment_effort row PHYSICALLY preserved (soft-reject), and recomputeLiftStats
// excludes the lift via the NOT EXISTS audit-row clause (lift_count=0).
//
// Complements TestRejectLiftCandidate_OSMSource_204 (which only verifies the
// flag flip) by also asserting the recompute-exclusion side.
func TestRejectMethodALift_ExcludesFromAggregates(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	now := time.Now().UTC().Truncate(time.Second)
	effortID := insertLiftSegmentEffort(t, db, userID, activityID, now)
	candID := insertOSMConfirmed(t, db, activityID, now, &effortID)

	// Establish baseline: with the OSM candidate not rejected, lift_count = 1
	// (segment_effort + non-rejected audit-row → counted via the segment_efforts
	// arm of recomputeLiftStats).
	if err := db.RecomputeLiftStats(context.Background(), userID, activityID); err != nil {
		t.Fatalf("baseline recompute: %v", err)
	}
	if got := liftCount(t, db, activityID); got != 1 {
		t.Fatalf("baseline lift_count = %d, want 1", got)
	}

	rec := callRejectLiftCandidate(t, h, userID, activityID, candID)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("reject status=%d, want 204; body=%s", rec.Code, rec.Body.String())
	}

	if got := loadCandidate(t, db, candID); !got.RejectedByUser {
		t.Errorf("rejected_by_user = false after reject, want true")
	}
	if !segmentEffortExists(t, db, effortID) {
		t.Error("segment_effort row deleted by soft-reject — must be preserved physically")
	}
	if got := liftCount(t, db, activityID); got != 0 {
		t.Errorf("lift_count = %d after Method-A reject, want 0 (NOT-EXISTS audit clause)", got)
	}
}

// ----- DTO field ----------------------------------------------------------

// TestActivityResponseExposesLiftsAcknowledgedAt: the activity DTO
// returned by GetActivity must include the lifts_acknowledged_at field
// so the frontend modal-trigger can read it. Plain-shape probe via the
// DB layer (not the HTTP layer) — the GetActivity SELECT is what feeds
// every activity-detail response in the API.
func TestActivityResponseExposesLiftsAcknowledgedAt(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)
	activityID := insertMinimalActivity(t, db, userID)

	ctx := context.Background()

	// Initial state: column is NULL.
	a, err := db.GetActivity(ctx, activityID)
	if err != nil {
		t.Fatalf("GetActivity: %v", err)
	}
	if a == nil {
		t.Fatalf("GetActivity returned nil for inserted activity")
	}
	if a.LiftsAcknowledgedAt != nil {
		t.Errorf("LiftsAcknowledgedAt = %v on fresh activity, want nil", a.LiftsAcknowledgedAt)
	}

	// Set acknowledged_at, re-read, verify the field is propagated.
	if _, err := pool.Exec(ctx,
		`UPDATE activities SET lifts_acknowledged_at = NOW() WHERE id = $1`,
		activityID,
	); err != nil {
		t.Fatalf("set acknowledged_at: %v", err)
	}
	a, err = db.GetActivity(ctx, activityID)
	if err != nil {
		t.Fatalf("GetActivity after update: %v", err)
	}
	if a.LiftsAcknowledgedAt == nil {
		t.Errorf("LiftsAcknowledgedAt = nil after UPDATE, want non-nil")
	}
}

// ----- F2: async dispatch -------------------------------------------------

// insertActivityWithSport inserts a minimal activity with a given sport_type
// and returns its ID. Used by F25 tests to exercise the sport-eligibility guard.
func insertActivityWithSport(t *testing.T, db *database.DB, userID, sportType string) string {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	act, err := db.InsertActivity(context.Background(), userID, &model.ActivityData{
		SportType: sportType,
		StartedAt: now,
		EndedAt:   now.Add(30 * time.Minute),
		FileHash:  uuid.New().String(),
	})
	if err != nil {
		t.Fatalf("insertActivityWithSport(%q): %v", sportType, err)
	}
	return act.ID
}

// TestRecalculateAllLifts_Returns204Quickly: RecalculateActivityLifts must
// return 204 within a generous wall-clock budget (F2: async dispatch).
// Re-enrichment now runs in a background goroutine — the handler response
// must arrive long before the goroutine completes.
func TestRecalculateAllLifts_Returns204Quickly(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userID := insertTestUser(t, db.Pool)
	// cycling is lift-eligible (F25: guard must not fire here).
	activityID := insertMinimalActivity(t, db, userID)

	now := time.Now().UTC().Truncate(time.Second)
	insertHeuristicUnconfirmed(t, db, activityID, now)

	deadline := time.Now().Add(5 * time.Second)
	rec := callBulkLifts(t, h, userID, activityID, "recalculate-all")
	elapsed := time.Since(deadline.Add(-5 * time.Second))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("recalculate-all status=%d, want 204; body=%s", rec.Code, rec.Body.String())
	}
	// The handler must respond within 5 s on any reasonable test machine.
	// (Real budget: the DB reset is synchronous; only the re-enrichment goroutine
	// is detached. For a minimal activity the reset completes in milliseconds.)
	if elapsed > 5*time.Second {
		t.Errorf("handler took %v — async dispatch likely not working", elapsed)
	}

	// DB reset must still be synchronous: all candidates are gone before 204 is sent.
	var count int
	if err := db.Pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM activity_lift_candidates WHERE activity_id = $1`, activityID,
	).Scan(&count); err != nil {
		t.Fatalf("count candidates: %v", err)
	}
	if count != 0 {
		t.Errorf("candidates count = %d after recalculate-all (want 0 — reset must be synchronous)", count)
	}
}

// ----- F25: sport-eligibility guard --------------------------------------

// TestRecalculateAllLifts_NonLiftEligibleSportReturns400: sending
// recalculate-all for a running activity must return 400 NOT_LIFT_ELIGIBLE
// without touching any DB rows.
func TestRecalculateAllLifts_NonLiftEligibleSportReturns400(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertActivityWithSport(t, db, userID, "running")

	now := time.Now().UTC().Truncate(time.Second)
	candID := insertHeuristicUnconfirmed(t, db, activityID, now)

	// Set lift_count to a non-zero value to prove the endpoint does NOT clear it.
	if _, err := db.Pool.Exec(context.Background(),
		`UPDATE activities SET lift_count = 3 WHERE id = $1`, activityID,
	); err != nil {
		t.Fatalf("set lift_count: %v", err)
	}

	rec := callBulkLifts(t, h, userID, activityID, "recalculate-all")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("running activity recalculate-all status=%d, want 400; body=%s",
			rec.Code, rec.Body.String())
	}

	// Assert structured error code.
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode 400 body: %v; raw=%s", err, rec.Body.String())
	}
	if body.Code != "NOT_LIFT_ELIGIBLE" {
		t.Errorf("problem code=%q, want NOT_LIFT_ELIGIBLE", body.Code)
	}

	// Candidate row must be untouched.
	if !candidateExists(t, db, candID) {
		t.Error("candidate deleted by running-activity recalculate-all — eligibility guard failed")
	}
	// lift_count must be unchanged (DELETE was skipped).
	if got := liftCount(t, db, activityID); got != 3 {
		t.Errorf("lift_count = %d after rejected recalculate-all, want 3 (unchanged)", got)
	}
}
