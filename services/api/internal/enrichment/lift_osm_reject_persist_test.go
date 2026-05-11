package enrichment

// Phase-4 Adversarial-Critic finding F2 (Variante B) — OSM-Reject persists
// across re-enrichment.
//
// Pre-fix: rejecting an OSM audit-row only flipped rejected_by_user=true on
// the audit row but on re-enrichment the snap pipeline produced a fresh
// segment_effort + a new (non-rejected) audit-row, restoring lift_count and
// silently undoing the user's reject.
//
// Post-fix: enrichStreamLifts pre-checks IsOSMRejectedInWindow before
// inserting a Stage-1 / Stage-2 / Stage-3 result and skips the insert when
// a rejected OSM audit-row covers the same window (boundary deltas
// ≤ 5 s on both ends).

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/enrichment/lifts"
)

// projectXYToLatLngOSMReject mirrors the same helper in
// lift_stage2_persist_test.go — separate name to avoid duplicate-symbol
// errors in the same package's test binary.
func projectXYToLatLngOSMReject(xM, yM float64) (lat, lng float64) {
	const lat0 = 47.0
	const lng0 = 13.0
	lat = lat0 + yM/111320.0
	lng = lng0 + xM/(111320.0*math.Cos(lat0*math.Pi/180.0))
	return lat, lng
}

// TestEnrichStreamLifts_RejectedOSMSnap_NotReinserted runs a full Stage-1
// snap, rejects the resulting audit-row, then re-runs enrichStreamLifts
// and asserts:
//  1. The rejected audit-row still exists (rejected_by_user=true).
//  2. NO new segment_effort is created (the pre-check suppressed the
//     insert).
//  3. activities.lift_count remains 0.
func TestEnrichStreamLifts_RejectedOSMSnap_NotReinserted(t *testing.T) {
	pool := liftTestPool(t)
	ctx := context.Background()

	const syntheticOSMID = int64(9_000_000_001_001)
	userID := liftTestUser(t, pool)
	activityID := uuid.New().String()
	startedAt := time.Now().UTC().Truncate(time.Second)

	// 1500m east-going lift, 300s ride at 5 m/s, 0.7 m/s ascent.
	const liftLengthM = 1500.0
	const altStart = 1000.0
	const speedMs = 5.0
	const stepM = 5.0

	type sample struct {
		t   time.Time
		lat float64
		lng float64
		alt float64
		spd float64
		dst float64
	}
	var samples []sample
	steps := int(liftLengthM / stepM) // 300 steps → 300s
	for i := 0; i <= steps; i++ {
		x := stepM * float64(i)
		alt := altStart + 0.7*float64(i)
		lat, lng := projectXYToLatLngOSMReject(x, 0)
		samples = append(samples, sample{
			t:   startedAt.Add(time.Duration(i) * time.Second),
			lat: lat,
			lng: lng,
			alt: alt,
			spd: speedMs,
			dst: x,
		})
	}

	t.Cleanup(func() {
		bg := context.Background()
		pool.Exec(bg, "DELETE FROM activity_lift_candidates WHERE activity_id = $1", activityID) //nolint:errcheck
		pool.Exec(bg, "DELETE FROM segment_efforts WHERE activity_id = $1", activityID)          //nolint:errcheck
		pool.Exec(bg, "DELETE FROM activity_streams WHERE activity_id = $1", activityID)         //nolint:errcheck
		pool.Exec(bg, "DELETE FROM activity_tracks WHERE activity_id = $1", activityID)          //nolint:errcheck
		pool.Exec(bg, "DELETE FROM activities WHERE id = $1", activityID)                        //nolint:errcheck
		pool.Exec(bg, "DELETE FROM segments WHERE osm_lift_id = $1", syntheticOSMID)             //nolint:errcheck
		pool.Exec(bg, "DELETE FROM osm_lifts WHERE osm_id = $1", syntheticOSMID)                 //nolint:errcheck
	})

	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, sport_type, started_at, ended_at, file_hash)
		VALUES ($1, $2, 'alpine_skiing', $3, $4, $5)
	`, activityID, userID, samples[0].t, samples[len(samples)-1].t, uuid.New().String()); err != nil {
		t.Fatalf("insert activity: %v", err)
	}

	var wktParts []string
	for _, s := range samples {
		wktParts = append(wktParts, fmt.Sprintf("%.7f %.7f", s.lng, s.lat))
	}
	trackWKT := "LINESTRING(" + strings.Join(wktParts, ",") + ")"
	startLat, startLng := projectXYToLatLngOSMReject(0, 0)
	endLat, endLng := projectXYToLatLngOSMReject(liftLengthM, 0)
	liftWKT := fmt.Sprintf("LINESTRING(%.7f %.7f, %.7f %.7f)", startLng, startLat, endLng, endLat)

	if _, err := pool.Exec(ctx, `
		INSERT INTO activity_tracks (activity_id, track_full, point_count)
		VALUES ($1, ST_GeomFromText($2, 4326), $3)
	`, activityID, trackWKT, len(samples)); err != nil {
		t.Fatalf("insert track: %v", err)
	}

	batch := strings.Builder{}
	args := []any{activityID}
	batch.WriteString(`INSERT INTO activity_streams (activity_id, time, lat, lng, altitude_m, speed_ms, distance_m) VALUES `)
	for i, s := range samples {
		if i > 0 {
			batch.WriteString(",")
		}
		off := len(args) + 1
		batch.WriteString(fmt.Sprintf("($1,$%d,$%d,$%d,$%d,$%d,$%d)", off, off+1, off+2, off+3, off+4, off+5))
		args = append(args, s.t, s.lat, s.lng, s.alt, s.spd, s.dst)
	}
	if _, err := pool.Exec(ctx, batch.String(), args...); err != nil {
		t.Fatalf("insert streams: %v", err)
	}

	// Real osm_lifts schema (migrations 000068 + 000164): id (BIGSERIAL),
	// osm_id, name, geom, source_region. way_id and aerialway never existed
	// — the production loader (lift_enrich.go) only reads osm_id/name/geom.
	if _, err := pool.Exec(ctx, `
		INSERT INTO osm_lifts (osm_id, name, geom)
		VALUES ($1, $2, ST_GeomFromText($3, 4326))
	`, syntheticOSMID, "synthetic-osm-reject-test-lift", liftWKT); err != nil {
		t.Fatalf("insert osm_lifts: %v", err)
	}

	segmentID := uuid.New().String()
	if _, err := pool.Exec(ctx, `
		INSERT INTO segments (id, created_by, name, segment_type, osm_lift_id, distance_m)
		VALUES ($1, $2, $3, 'lift', $4, $5)
	`, segmentID, userID, "synthetic osm-reject lift", syntheticOSMID, liftLengthM); err != nil {
		t.Fatalf("insert segments: %v", err)
	}
	segmentVersionID := uuid.New().String()
	if _, err := pool.Exec(ctx, `
		INSERT INTO segment_versions (id, segment_id, version_number, track, valid_from, created_by)
		VALUES ($1, $2, 1, ST_GeomFromText($3, 4326), CURRENT_DATE, $4)
	`, segmentVersionID, segmentID, liftWKT, userID); err != nil {
		t.Fatalf("insert segment_versions: %v", err)
	}

	stream := make([]EnrichmentStreamPoint, len(samples))
	for i, s := range samples {
		lat, lng, alt, spd, dst := s.lat, s.lng, s.alt, s.spd, s.dst
		stream[i] = EnrichmentStreamPoint{
			Time:      s.t,
			Lat:       &lat,
			Lng:       &lng,
			AltitudeM: &alt,
			SpeedMs:   &spd,
			DistanceM: &dst,
		}
	}

	// Sanity gate: the heuristic detector must emit exactly one ride for
	// this synthetic stream, otherwise the snap pipeline does not exercise
	// the Stage-1 path the test depends on.
	{
		streamPts := make([]lifts.StreamPoint, len(samples))
		for i, s := range samples {
			lat, lng, alt, spd, dst := s.lat, s.lng, s.alt, s.spd, s.dst
			streamPts[i] = lifts.StreamPoint{
				Time: s.t, Lat: &lat, Lng: &lng, AltitudeM: &alt, SpeedMs: &spd, DistanceM: &dst,
			}
		}
		rides := lifts.DetectHeuristicWithIndices(streamPts, lifts.DefaultDetectorConfig("alpine_skiing"))
		if len(rides) != 1 {
			t.Fatalf("detector emitted %d rides, want 1 — fixture no longer triggers Stage 1 snap", len(rides))
		}
	}

	db := &database.DB{Pool: pool}

	// First enrich pass: produces 1 segment_effort + 1 OSM audit-row.
	if err := enrichStreamLifts(ctx, db, userID, activityID, stream); err != nil {
		t.Fatalf("first enrichStreamLifts: %v", err)
	}

	var firstEffortCount int
	if err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM segment_efforts WHERE activity_id = $1 AND segment_id = $2",
		activityID, segmentID,
	).Scan(&firstEffortCount); err != nil {
		t.Fatalf("count efforts after first enrich: %v", err)
	}
	if firstEffortCount != 1 {
		t.Fatalf("first enrich produced %d segment_efforts, want 1 — fixture broken", firstEffortCount)
	}

	// Fetch the audit-row id and reject it.
	var auditID, originalEffortID string
	if err := pool.QueryRow(ctx, `
		SELECT id::text, segment_effort_id::text FROM activity_lift_candidates
		WHERE activity_id = $1 AND source = 'osm'
		LIMIT 1
	`, activityID).Scan(&auditID, &originalEffortID); err != nil {
		t.Fatalf("fetch audit row: %v", err)
	}
	if err := db.RejectLiftCandidate(ctx, userID, auditID, activityID); err != nil {
		t.Fatalf("RejectLiftCandidate: %v", err)
	}

	// Second enrich pass — the pre-check must suppress the Stage-1 insert.
	if err := enrichStreamLifts(ctx, db, userID, activityID, stream); err != nil {
		t.Fatalf("second enrichStreamLifts: %v", err)
	}

	// Assertion 1: rejected audit-row still present and still rejected.
	var stillRejected bool
	if err := pool.QueryRow(ctx,
		"SELECT rejected_by_user FROM activity_lift_candidates WHERE id = $1", auditID,
	).Scan(&stillRejected); err != nil {
		t.Fatalf("query rejected audit-row: %v", err)
	}
	if !stillRejected {
		t.Error("rejected_by_user = false after re-enrich — soft-reject was cleared")
	}

	// Assertion 2: no new segment_effort was created. The pre-existing one
	// is deleted by the idempotent DELETE in enrichStreamLifts; the
	// post-DELETE count should be 0 (pre-check blocked the re-insert).
	var effortCountAfter int
	if err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM segment_efforts WHERE activity_id = $1 AND segment_id = $2",
		activityID, segmentID,
	).Scan(&effortCountAfter); err != nil {
		t.Fatalf("count efforts after second enrich: %v", err)
	}
	if effortCountAfter != 0 {
		t.Errorf("segment_efforts count = %d, want 0 (pre-check must suppress re-insert of rejected window)", effortCountAfter)
	}

	// Assertion 3: lift_count is 0.
	var liftCount int
	if err := pool.QueryRow(ctx,
		"SELECT lift_count FROM activities WHERE id = $1", activityID,
	).Scan(&liftCount); err != nil {
		t.Fatalf("query lift_count: %v", err)
	}
	if liftCount != 0 {
		t.Errorf("lift_count = %d, want 0 (rejected OSM lift must not be counted)", liftCount)
	}
}

// TestEnrichStreamLifts_RejectedOSMSnap_DriftWithinTolerance_Suppressed:
// the rejected audit-row covers [t, t+300s]. A re-detection produces a
// snap whose window is [t+3s, t+303s] (both boundaries shifted +3s, well
// within ±5s). The pre-check must suppress the insert.
//
// We exercise the boundary-delta predicate at the unit-level via
// IsOSMRejectedInWindow (the integration test above already covers the
// end-to-end pipeline; replicating the full Stage-1 fixture here for a
// 3s shift would not add behavioural coverage beyond what
// IsOSMRejectedInWindow already proves).
func TestEnrichStreamLifts_RejectedOSMSnap_DriftWithinTolerance_Suppressed(t *testing.T) {
	pool := liftTestPool(t)
	ctx := context.Background()

	userID := liftTestUser(t, pool)
	activityID := liftTestActivity(t, pool, userID)

	now := time.Now().UTC().Truncate(time.Second)
	if _, err := pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m,
			 confidence, confirmed, source, rejected_by_user)
		VALUES ($1, $2, $3, 300, 100, 0, 600, 'high', true, 'osm', true)
	`, activityID, now, now.Add(300*time.Second)); err != nil {
		t.Fatalf("insert rejected OSM audit-row: %v", err)
	}

	db := &database.DB{Pool: pool}
	// Drift +3s on both boundaries — must be suppressed (≤5s tolerance).
	suppressed, err := db.IsOSMRejectedInWindow(ctx, activityID,
		now.Add(3*time.Second), now.Add(303*time.Second))
	if err != nil {
		t.Fatalf("IsOSMRejectedInWindow: %v", err)
	}
	if !suppressed {
		t.Error("suppressed = false, want true: ±3s drift on both boundaries must trigger pre-check")
	}
}

// TestEnrichStreamLifts_RejectedOSMSnap_LargeDriftBeyondTolerance_NotSuppressed:
// drift of 30s on both boundaries → treated as a distinct lift. The
// pre-check must NOT suppress; the snap may proceed.
func TestEnrichStreamLifts_RejectedOSMSnap_LargeDriftBeyondTolerance_NotSuppressed(t *testing.T) {
	pool := liftTestPool(t)
	ctx := context.Background()

	userID := liftTestUser(t, pool)
	activityID := liftTestActivity(t, pool, userID)

	now := time.Now().UTC().Truncate(time.Second)
	if _, err := pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m,
			 confidence, confirmed, source, rejected_by_user)
		VALUES ($1, $2, $3, 300, 100, 0, 600, 'high', true, 'osm', true)
	`, activityID, now, now.Add(300*time.Second)); err != nil {
		t.Fatalf("insert rejected OSM audit-row: %v", err)
	}

	db := &database.DB{Pool: pool}
	suppressed, err := db.IsOSMRejectedInWindow(ctx, activityID,
		now.Add(30*time.Second), now.Add(330*time.Second))
	if err != nil {
		t.Fatalf("IsOSMRejectedInWindow: %v", err)
	}
	if suppressed {
		t.Error("suppressed = true, want false: ±30s drift exceeds tolerance — distinct lift must not be suppressed")
	}
}
