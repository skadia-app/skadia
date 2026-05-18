package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

type ReverseResponse struct {
	EffortsUpdated int `json:"efforts_updated"`
}

// TestAdvisoryLockKeyForSegmentReverse derives a stable int64 from the segment UUID,
// namespaced with "segment.reverse." so it cannot collide with advisory-locks
// used elsewhere.
func TestAdvisoryLockKeyForSegmentReverse(t *testing.T) {
	k1 := advisoryLockKeyForSegmentReverse("00000000-0000-0000-0000-000000000001")
	k2 := advisoryLockKeyForSegmentReverse("00000000-0000-0000-0000-000000000001")
	if k1 != k2 {
		t.Fatal("lock-key should be deterministic for same input")
	}
}

func TestErrLapSegmentCannotBeReversed(t *testing.T) {
	err := errLapSegmentCannotBeReversed
	if err == nil {
		t.Fatal("errLapSegmentCannotBeReversed must not be nil")
	}
}

// --- DB-integration tests ---

func TestReverseSegmentFlipsGeometryAndEfforts_DB(t *testing.T) {
	pool := fusionTestPool(t)

	userID := insertTestUser(t, pool)
	segmentID := insertTestSegment(t, pool, userID, TestSegmentOpts{
		Track: "LINESTRING(0 0, 1 1, 2 2)",
	})
	effort1ID := insertTestEffort(t, pool, segmentID, userID, false)
	effort2ID := insertTestEffort(t, pool, segmentID, userID, true)

	h := newTestHandler(t, pool)
	r := chi.NewRouter()
	r.Post("/api/v1/segments/{id}/reverse", h.ReverseSegment)

	req := httptest.NewRequest("POST", "/api/v1/segments/"+segmentID+"/reverse", nil)
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Code = %d, want 200", rec.Code)
	}
	var resp ReverseResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.EffortsUpdated != 2 {
		t.Errorf("EffortsUpdated = %d, want 2", resp.EffortsUpdated)
	}

	// Verify track is reversed
	var track string
	err := pool.QueryRow(context.Background(),
		`SELECT ST_AsText(sv.track) FROM segment_versions sv
         WHERE sv.segment_id = $1 ORDER BY sv.version_number DESC LIMIT 1`,
		segmentID).Scan(&track)
	if err != nil {
		t.Fatal(err)
	}
	if track != "LINESTRING(2 2,1 1,0 0)" {
		t.Errorf("track = %q, want LINESTRING(2 2,1 1,0 0)", track)
	}

	// Verify efforts are flipped
	var e1Rev, e2Rev bool
	pool.QueryRow(context.Background(), "SELECT is_reverse FROM segment_efforts WHERE id = $1", effort1ID).Scan(&e1Rev)
	pool.QueryRow(context.Background(), "SELECT is_reverse FROM segment_efforts WHERE id = $1", effort2ID).Scan(&e2Rev)
	if !e1Rev {
		t.Error("effort 1 should be reversed")
	}
	if e2Rev {
		t.Error("effort 2 should NOT be reversed")
	}
	var e1Direction, e1Identity, e2Direction, e2Identity string
	if err := pool.QueryRow(context.Background(),
		`SELECT leaderboard_direction, match_identity_key FROM segment_efforts WHERE id = $1`, effort1ID).Scan(&e1Direction, &e1Identity); err != nil {
		t.Fatalf("query effort 1 metadata: %v", err)
	}
	if err := pool.QueryRow(context.Background(),
		`SELECT leaderboard_direction, match_identity_key FROM segment_efforts WHERE id = $1`, effort2ID).Scan(&e2Direction, &e2Identity); err != nil {
		t.Fatalf("query effort 2 metadata: %v", err)
	}
	if e1Direction != "reverse" || !strings.Contains(e1Identity, ":direction:reverse:") {
		t.Fatalf("effort 1 metadata = %s/%s, want reverse direction", e1Direction, e1Identity)
	}
	if e2Direction != "forward" || !strings.Contains(e2Identity, ":direction:forward:") {
		t.Fatalf("effort 2 metadata = %s/%s, want forward direction", e2Direction, e2Identity)
	}
}

func TestReverseSegmentUsesActiveVersionBeforeFutureVersion_DB(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()

	userID := insertTestUser(t, pool)
	segmentID := insertTestSegment(t, pool, userID, TestSegmentOpts{
		Track: "LINESTRING(0 0, 1 1, 2 2)",
	})
	futureAt := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Microsecond)
	if _, err := pool.Exec(ctx, `
		INSERT INTO segment_versions (
			id, segment_id, version_number, track, start_point, end_point, valid_from, created_by,
			valid_from_at, validity_basis, matching_contract_key, matching_contract_version, published_at
		)
		SELECT gen_random_uuid(), $1, 2,
		       ST_GeomFromText('LINESTRING(9 9, 10 10)', 4326),
		       ST_StartPoint(ST_GeomFromText('LINESTRING(9 9, 10 10)', 4326)),
		       ST_EndPoint(ST_GeomFromText('LINESTRING(9 9, 10 10)', 4326)),
		       CURRENT_DATE, $2, $3, 'editor_asserted', 'course_gps_v1', 1, NOW()`,
		segmentID, userID, futureAt); err != nil {
		t.Fatalf("insert future version: %v", err)
	}

	h := newTestHandler(t, pool)
	r := chi.NewRouter()
	r.Post("/api/v1/segments/{id}/reverse", h.ReverseSegment)

	req := httptest.NewRequest("POST", "/api/v1/segments/"+segmentID+"/reverse", nil)
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Code = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var versionNumber int
	var validUntil time.Time
	var track string
	err := pool.QueryRow(ctx, `
		SELECT version_number, valid_until_at, ST_AsText(track)
		FROM segment_versions
		WHERE segment_id = $1
		ORDER BY version_number DESC
		LIMIT 1`, segmentID).Scan(&versionNumber, &validUntil, &track)
	if err != nil {
		t.Fatal(err)
	}
	if versionNumber != 3 {
		t.Fatalf("new version_number = %d, want 3", versionNumber)
	}
	if !validUntil.Equal(futureAt) {
		t.Fatalf("new valid_until_at = %v, want future boundary %v", validUntil, futureAt)
	}
	if track != "LINESTRING(2 2,1 1,0 0)" {
		t.Fatalf("new track = %q, want reversed currently active v1 track", track)
	}
}

func TestReverseSegmentForbiddenForNonOwner_DB(t *testing.T) {
	pool := fusionTestPool(t)

	ownerID := insertTestUser(t, pool)
	otherID := insertTestUser(t, pool)
	// Bidirectional/Track irrelevant für Permission-Test — Default-Segment reicht.
	segmentID := insertTestSegment(t, pool, ownerID, TestSegmentOpts{})

	h := newTestHandler(t, pool)
	r := chi.NewRouter()
	r.Post("/api/v1/segments/{id}/reverse", h.ReverseSegment)

	req := httptest.NewRequest("POST", "/api/v1/segments/"+segmentID+"/reverse", nil)
	req = req.WithContext(withUserAndAdmin(req.Context(), otherID, false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("Code = %d, want 403", rec.Code)
	}
}

func TestReverseSegmentAllowsAdminOnForeignSegment_DB(t *testing.T) {
	pool := fusionTestPool(t)

	ownerID := insertTestUser(t, pool)
	adminID := insertTestUser(t, pool, WithIsAdmin(true))
	// Bidirectional/Track irrelevant für Permission-Test — Default-Segment reicht.
	segmentID := insertTestSegment(t, pool, ownerID, TestSegmentOpts{})

	h := newTestHandler(t, pool)
	r := chi.NewRouter()
	r.Post("/api/v1/segments/{id}/reverse", h.ReverseSegment)

	req := httptest.NewRequest("POST", "/api/v1/segments/"+segmentID+"/reverse", nil)
	req = req.WithContext(withUserAndAdmin(req.Context(), adminID, true))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Code = %d, want 200", rec.Code)
	}
}

func TestReverseSegmentConcurrentCallsSerialize_DB(t *testing.T) {
	pool := fusionTestPool(t)

	userID := insertTestUser(t, pool)
	segmentID := insertTestSegment(t, pool, userID, TestSegmentOpts{
		Track: "LINESTRING(0 0, 1 1)",
	})

	h := newTestHandler(t, pool)
	r := chi.NewRouter()
	r.Post("/api/v1/segments/{id}/reverse", h.ReverseSegment)

	// 2 concurrent requests
	var wg sync.WaitGroup
	results := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("POST", "/api/v1/segments/"+segmentID+"/reverse", nil)
			req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			results <- rec.Code
		}()
	}
	wg.Wait()
	close(results)

	for code := range results {
		if code != http.StatusOK {
			t.Errorf("Concurrent reverse code = %d, want 200", code)
		}
	}

	// Net effect of 2 reverses = original track
	var track string
	err := pool.QueryRow(context.Background(),
		`SELECT ST_AsText(sv.track) FROM segment_versions sv
         WHERE sv.segment_id = $1 ORDER BY sv.version_number DESC LIMIT 1`,
		segmentID).Scan(&track)
	if err != nil {
		t.Fatal(err)
	}
	if track != "LINESTRING(0 0,1 1)" {
		t.Errorf("track = %q, want LINESTRING(0 0,1 1)", track)
	}
}
