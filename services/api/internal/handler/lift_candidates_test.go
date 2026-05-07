package handler

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

type unifiedLiftCandidateResponse struct {
	ID              string    `json:"id"`
	ActivityID      string    `json:"activity_id"`
	StartedAt       time.Time `json:"started_at"`
	EndedAt         time.Time `json:"ended_at"`
	DurationS       int       `json:"duration_s"`
	AscentM         float64   `json:"ascent_m"`
	DescentM        *float64  `json:"descent_m"`
	DistanceM       *float64  `json:"distance_m"`
	Confidence      string    `json:"confidence"`
	Confirmed       bool      `json:"confirmed"`
	RejectedByUser  bool      `json:"rejected_by_user"`
	Source          string    `json:"source"`
	SegmentEffortID *string   `json:"segment_effort_id"`
}

func liftCandidatesHandlerSetup(t *testing.T) (*Handler, *database.DB) {
	t.Helper()
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	return &Handler{DB: db}, db
}

func callUnifiedLiftCandidates(
	t *testing.T,
	h *Handler,
	userID string,
	activityID string,
	query string,
) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	r.Get("/api/v1/activities/{id}/lifts/candidates", h.GetLiftCandidates)
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/activities/"+activityID+"/lifts/candidates"+query, nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func decodeUnifiedLiftCandidates(t *testing.T, rec *httptest.ResponseRecorder) []unifiedLiftCandidateResponse {
	t.Helper()
	var candidates []unifiedLiftCandidateResponse
	if err := json.NewDecoder(rec.Body).Decode(&candidates); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
	}
	return candidates
}

func insertUnifiedLiftCandidate(
	t *testing.T,
	db *database.DB,
	activityID string,
	startedAt time.Time,
	source string,
	confirmed bool,
	segmentEffortID *string,
) string {
	t.Helper()
	var id string
	err := db.Pool.QueryRow(context.Background(), `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m,
			 confidence, confirmed, source, segment_effort_id)
		VALUES ($1, $2, $3, 180, 120, 0, 700, 'high', $4, $5, $6)
		RETURNING id::text
	`, activityID, startedAt, startedAt.Add(3*time.Minute), confirmed, source, segmentEffortID).Scan(&id)
	if err != nil {
		t.Fatalf("insert lift candidate: %v", err)
	}
	return id
}

func TestGetUnifiedLiftCandidates_ReturnsBothMethods(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	start := time.Now().UTC().Truncate(time.Second)
	segmentEffortID := uuid.NewString()
	insertUnifiedLiftCandidate(t, db, activityID, start, "osm", true, &segmentEffortID)
	insertUnifiedLiftCandidate(t, db, activityID, start.Add(10*time.Minute), "heuristic", false, nil)

	rec := callUnifiedLiftCandidates(t, h, userID, activityID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	candidates := decodeUnifiedLiftCandidates(t, rec)
	if len(candidates) != 2 {
		t.Fatalf("candidate count=%d, want 2", len(candidates))
	}
	if candidates[0].Source != "osm" || candidates[0].SegmentEffortID == nil {
		t.Fatalf("first candidate source/segment_effort_id = %q/%v, want osm/non-nil",
			candidates[0].Source, candidates[0].SegmentEffortID)
	}
	if candidates[1].Source != "heuristic" || candidates[1].SegmentEffortID != nil {
		t.Fatalf("second candidate source/segment_effort_id = %q/%v, want heuristic/nil",
			candidates[1].Source, candidates[1].SegmentEffortID)
	}
}

func TestGetUnifiedLiftCandidates_FiltersBySource(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	start := time.Now().UTC().Truncate(time.Second)
	segmentEffortID := uuid.NewString()
	insertUnifiedLiftCandidate(t, db, activityID, start, "osm", true, &segmentEffortID)
	insertUnifiedLiftCandidate(t, db, activityID, start.Add(10*time.Minute), "heuristic", false, nil)

	rec := callUnifiedLiftCandidates(t, h, userID, activityID, "?source=heuristic")
	if rec.Code != http.StatusOK {
		t.Fatalf("heuristic filter status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	heuristic := decodeUnifiedLiftCandidates(t, rec)
	if len(heuristic) != 1 || heuristic[0].Source != "heuristic" {
		t.Fatalf("heuristic filter returned %+v, want only heuristic", heuristic)
	}

	rec = callUnifiedLiftCandidates(t, h, userID, activityID, "?source=osm")
	if rec.Code != http.StatusOK {
		t.Fatalf("osm filter status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	osm := decodeUnifiedLiftCandidates(t, rec)
	if len(osm) != 1 || osm[0].Source != "osm" {
		t.Fatalf("osm filter returned %+v, want only osm", osm)
	}
}

func TestGetUnifiedLiftCandidates_DefaultReturnsAll(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	start := time.Now().UTC().Truncate(time.Second)
	segmentEffortID := uuid.NewString()
	insertUnifiedLiftCandidate(t, db, activityID, start, "osm", true, &segmentEffortID)
	insertUnifiedLiftCandidate(t, db, activityID, start.Add(10*time.Minute), "heuristic", false, nil)

	rec := callUnifiedLiftCandidates(t, h, userID, activityID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if got := len(decodeUnifiedLiftCandidates(t, rec)); got != 2 {
		t.Fatalf("default candidate count=%d, want 2", got)
	}
}

func TestGetUnifiedLiftCandidates_AcceptsIncludeRejectedFlag(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	insertUnifiedLiftCandidate(t, db, activityID, time.Now().UTC().Truncate(time.Second), "heuristic", false, nil)

	rec := callUnifiedLiftCandidates(t, h, userID, activityID, "?include_rejected=true")
	if rec.Code != http.StatusOK {
		t.Fatalf("include_rejected=true status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	rec = callUnifiedLiftCandidates(t, h, userID, activityID, "?include_rejected=maybe")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("include_rejected=maybe status=%d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetUnifiedLiftCandidates_RLSIsolationBetweenUsers(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userA := insertTestUser(t, db.Pool)
	userB := insertTestUser(t, db.Pool)
	activityA := insertMinimalActivity(t, db, userA)

	insertUnifiedLiftCandidate(t, db, activityA, time.Now().UTC().Truncate(time.Second), "heuristic", false, nil)

	rec := callUnifiedLiftCandidates(t, h, userB, activityA, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-user status=%d, want 404; body=%s", rec.Code, rec.Body.String())
	}

	rec = callUnifiedLiftCandidates(t, h, userA, activityA, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("owner status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if got := len(decodeUnifiedLiftCandidates(t, rec)); got != 1 {
		t.Fatalf("owner candidate count=%d, want 1", got)
	}
}

// insertRejectedLiftCandidate inserts a lift candidate with rejected_by_user=true.
func insertRejectedLiftCandidate(
	t *testing.T,
	db *database.DB,
	activityID string,
	startedAt time.Time,
) string {
	t.Helper()
	var id string
	err := db.Pool.QueryRow(context.Background(), `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m,
			 confidence, confirmed, source, rejected_by_user)
		VALUES ($1, $2, $3, 180, 120, 0, 700, 'high', false, 'heuristic', true)
		RETURNING id::text
	`, activityID, startedAt, startedAt.Add(3*time.Minute)).Scan(&id)
	if err != nil {
		t.Fatalf("insert rejected lift candidate: %v", err)
	}
	return id
}

// TestGetLiftCandidates_DefaultHidesRejected: a rejected candidate must not
// appear in the default response (include_rejected defaults to false).
func TestGetLiftCandidates_DefaultHidesRejected(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	start := time.Now().UTC().Truncate(time.Second)
	insertRejectedLiftCandidate(t, db, activityID, start)

	rec := callUnifiedLiftCandidates(t, h, userID, activityID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	candidates := decodeUnifiedLiftCandidates(t, rec)
	if len(candidates) != 0 {
		t.Fatalf("default response returned %d candidates with rejected row, want 0", len(candidates))
	}
}

// TestGetLiftCandidates_IncludeRejectedReturnsAll: with include_rejected=true the
// rejected candidate must be included.
func TestGetLiftCandidates_IncludeRejectedReturnsAll(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	start := time.Now().UTC().Truncate(time.Second)
	insertRejectedLiftCandidate(t, db, activityID, start)
	// Also insert one non-rejected candidate.
	insertUnifiedLiftCandidate(t, db, activityID, start.Add(10*time.Minute), "heuristic", false, nil)

	rec := callUnifiedLiftCandidates(t, h, userID, activityID, "?include_rejected=true")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	candidates := decodeUnifiedLiftCandidates(t, rec)
	if len(candidates) != 2 {
		t.Fatalf("include_rejected=true returned %d candidates, want 2", len(candidates))
	}
}

// TestGetLiftCandidates_DTOExposesRejectedFlag: the rejected_by_user field must
// be present and correctly set in the JSON response.
func TestGetLiftCandidates_DTOExposesRejectedFlag(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	start := time.Now().UTC().Truncate(time.Second)
	insertRejectedLiftCandidate(t, db, activityID, start)
	// Insert a normal (non-rejected) candidate for comparison.
	insertUnifiedLiftCandidate(t, db, activityID, start.Add(10*time.Minute), "heuristic", false, nil)

	rec := callUnifiedLiftCandidates(t, h, userID, activityID, "?include_rejected=true")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	candidates := decodeUnifiedLiftCandidates(t, rec)
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
	// candidates are ordered by started_at: rejected first, normal second.
	if !candidates[0].RejectedByUser {
		t.Errorf("candidates[0].RejectedByUser = false, want true")
	}
	if candidates[1].RejectedByUser {
		t.Errorf("candidates[1].RejectedByUser = true, want false")
	}
}

// callRejectLiftCandidate fires DELETE on the lift-candidate endpoint.
func callRejectLiftCandidate(
	t *testing.T,
	h *Handler,
	userID, activityID, candidateID string,
) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	r.Delete("/api/v1/activities/{id}/lift-candidates/{cid}", h.RejectLiftCandidate)
	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/activities/"+activityID+"/lift-candidates/"+candidateID, nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// TestRejectLiftCandidate_OSMSource_204: rejecting an audit-row with
// source='osm' must succeed (204 No Content) and flip rejected_by_user to
// true. Phase-4 critic F2 Variante B: the OSM reject path must be open so
// the user can recover from a wrong snap (e.g. paths running parallel to
// the lift line that the snap heuristic mis-attributes).
func TestRejectLiftCandidate_OSMSource_204(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	// Insert an OSM audit-row (segment_effort_id pointer is intentionally
	// nil — the reject path does not depend on the segment_effort linkage,
	// only on the audit-row PK).
	start := time.Now().UTC().Truncate(time.Second)
	candID := insertUnifiedLiftCandidate(t, db, activityID, start, "osm", true, nil)

	rec := callRejectLiftCandidate(t, h, userID, activityID, candID)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d, want 204; body=%s", rec.Code, rec.Body.String())
	}

	var rejected bool
	if err := db.Pool.QueryRow(context.Background(),
		"SELECT rejected_by_user FROM activity_lift_candidates WHERE id = $1", candID,
	).Scan(&rejected); err != nil {
		t.Fatalf("query rejected_by_user: %v", err)
	}
	if !rejected {
		t.Error("rejected_by_user = false after OSM reject — soft-reject did not write")
	}
}

// TestGetUnifiedLiftCandidates_DefaultReturnsOnlyNonRejected: non-rejected
// candidates are returned while rejected ones remain hidden by default.
func TestGetUnifiedLiftCandidates_DefaultReturnsOnlyNonRejected(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	start := time.Now().UTC().Truncate(time.Second)
	insertRejectedLiftCandidate(t, db, activityID, start)
	insertUnifiedLiftCandidate(t, db, activityID, start.Add(10*time.Minute), "heuristic", false, nil)

	rec := callUnifiedLiftCandidates(t, h, userID, activityID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	candidates := decodeUnifiedLiftCandidates(t, rec)
	// Default must return only the non-rejected one.
	if len(candidates) != 1 {
		t.Fatalf("default response returned %d candidates (rejected+non-rejected mix), want 1", len(candidates))
	}
	if candidates[0].RejectedByUser {
		t.Errorf("default response returned a rejected candidate")
	}
}
