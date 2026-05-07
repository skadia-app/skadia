package enrichment

// Phase 4 Critic finding F1 (critical): Stage-2 cable-hold merge persistence
// path. MergeMultiRideForOSM marks the non-canonical group members as
// SuppressedByMerge but does not propagate MergedRideIndices onto them. The
// persistence loop in lift_enrich.go indexes res.MergedRideIndices[0] and
// panics index-out-of-range before the audit row can be written. This test
// drives a real Stage-2 merge end-to-end through enrichStreamLifts and
// asserts the panic-free post-fix contract:
//
//  1. enrichStreamLifts returns nil (no panic, no error).
//  2. Exactly ONE segment_efforts row exists for the merged group.
//  3. Exactly N audit-rows in activity_lift_candidates with source='osm'
//     all referencing the same canonical segment_effort_id.
//  4. recomputeLiftStats counts the merged group as 1 lift (not N).
//
// The synthetic activity is a two-ride sequence on the same OSM lift with
// every cable-hold signal positive (inter-ride distance, gap-pattern,
// no bearing reversal) and union-coverage >= 0.70.

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

// projectXYToLatLng mirrors projectToWGS84 in lifts/test_helpers_test.go,
// duplicated here because that helper is in a *_test.go file in a different
// package.
func projectXYToLatLng(xM, yM float64) (lat, lng float64) {
	const lat0 = 47.0
	const lng0 = 12.0
	lat = lat0 + yM/111320.0
	lng = lng0 + xM/(111320.0*math.Cos(lat0*math.Pi/180.0))
	return lat, lng
}

// TestEnrichStreamLifts_Stage2CableHoldMerge_Persists drives a real Stage-2
// cable-hold merge through the persistence path. Pre-fix this panics with
// "runtime error: index out of range [0] with length 0" inside the
// SuppressedByMerge branch of the persist loop. Post-fix it must complete
// without panic and produce 1 segment_effort + 2 audit candidates linked to
// it.
func TestEnrichStreamLifts_Stage2CableHoldMerge_Persists(t *testing.T) {
	pool := liftTestPool(t)
	ctx := context.Background()

	// The synthetic OSM lift uses an osm_id far above any real-world way id
	// to avoid colliding with imported regions in shared dev databases.
	const syntheticOSMID = int64(9_000_000_000_001)

	userID := liftTestUser(t, pool)
	activityID := uuid.New().String()
	startedAt := time.Now().UTC().Truncate(time.Second)

	// Two rides on the same 1500m east-going lift. Each ride covers ~40% of
	// the lift; the cable-hold gap is a smart-recording pause (no samples
	// between ride A end at xM=600 and ride B start at xM=610). Ride
	// parameters mirror TestEnrichStreamLifts_NoOSMRunsHeuristicOnly so the
	// heuristic detector emits one ride per chunk: 5 m/s horizontal,
	// 0.7 m/s vertical (gradient 0.14 > MinGradient 0.10), straight east
	// (sinuosity 1.0 < MaxSinuosity 1.08), CV ~0.
	const liftLengthM = 1500.0
	const altStart = 1000.0
	const speedMs = 5.0
	const stepM = 5.0 // one sample per second at 5 m/s
	// Per-ride length 600m → 120s → 84m altitude per ride at 0.7 m/s.
	const rideAEndX = 600.0
	const rideBStartX = 610.0
	const rideBEndX = 1200.0

	type sample struct {
		t   time.Time
		lat float64
		lng float64
		alt float64
		spd float64
		dst float64
	}
	var samples []sample

	// Emit one ride: stepM-spaced samples from fromX to toX with constant
	// horizontal speed (1 Hz at 5 m/s) and altitude rising at 0.7 m/s.
	emitRide := func(rideStart time.Time, startDistM, fromX, toX, fromAlt float64) {
		steps := int((toX - fromX) / stepM)
		for i := 0; i <= steps; i++ {
			frac := float64(i) / float64(steps)
			x := fromX + (toX-fromX)*frac
			elapsedS := float64(i)
			alt := fromAlt + 0.7*elapsedS
			lat, lng := projectXYToLatLng(x, 0)
			samples = append(samples, sample{
				t:   rideStart.Add(time.Duration(i) * time.Second),
				lat: lat,
				lng: lng,
				alt: alt,
				spd: speedMs,
				dst: startDistM + (x - fromX),
			})
		}
	}

	// Ride A: 0..600m over 120s, alt 1000→1084 (rate 0.7 m/s, gradient 0.14).
	emitRide(startedAt, 0, 0, rideAEndX, altStart)
	// Cable-hold gap: smart-recording pause for 5 minutes (no samples in the
	// gap → triggers the gap-sample-pattern smart-recording branch).
	rideBStart := samples[len(samples)-1].t.Add(5 * time.Minute)
	rideBStartAlt := samples[len(samples)-1].alt + 2.0 // 2m alt drift across the cable hold
	rideBStartDist := samples[len(samples)-1].dst + (rideBStartX - rideAEndX)
	emitRide(rideBStart, rideBStartDist, rideBStartX, rideBEndX, rideBStartAlt)

	t.Cleanup(func() {
		bg := context.Background()
		// Order matters: candidates → segment_efforts → segments
		// (segment_versions cascades) → osm_lifts → activity tracks/streams.
		pool.Exec(bg, "DELETE FROM activity_lift_candidates WHERE activity_id = $1", activityID)            //nolint:errcheck
		pool.Exec(bg, "DELETE FROM segment_efforts WHERE activity_id = $1", activityID)                    //nolint:errcheck
		pool.Exec(bg, "DELETE FROM activity_streams WHERE activity_id = $1", activityID)                   //nolint:errcheck
		pool.Exec(bg, "DELETE FROM activity_tracks WHERE activity_id = $1", activityID)                    //nolint:errcheck
		pool.Exec(bg, "DELETE FROM activities WHERE id = $1", activityID)                                  //nolint:errcheck
		pool.Exec(bg, "DELETE FROM segments WHERE osm_lift_id = $1", syntheticOSMID)                       //nolint:errcheck
		pool.Exec(bg, "DELETE FROM osm_lifts WHERE osm_id = $1", syntheticOSMID)                           //nolint:errcheck
	})

	// Insert activity (alpine_skiing → IsLiftRelevant=true).
	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, sport_type, started_at, ended_at, file_hash)
		VALUES ($1, $2, 'alpine_skiing', $3, $4, $5)
	`, activityID, userID, samples[0].t, samples[len(samples)-1].t, uuid.New().String()); err != nil {
		t.Fatalf("insert activity: %v", err)
	}

	// Build a LineString WKT from all samples for activity_tracks.track_full
	// and a synthetic OSM lift LineString from xM=0 to xM=liftLengthM at y=0.
	var wktParts []string
	for _, s := range samples {
		wktParts = append(wktParts, fmt.Sprintf("%.7f %.7f", s.lng, s.lat))
	}
	trackWKT := "LINESTRING(" + strings.Join(wktParts, ",") + ")"

	startLat, startLng := projectXYToLatLng(0, 0)
	endLat, endLng := projectXYToLatLng(liftLengthM, 0)
	liftWKT := fmt.Sprintf("LINESTRING(%.7f %.7f, %.7f %.7f)", startLng, startLat, endLng, endLat)

	if _, err := pool.Exec(ctx, `
		INSERT INTO activity_tracks (activity_id, track_full, point_count)
		VALUES ($1, ST_GeomFromText($2, 4326), $3)
	`, activityID, trackWKT, len(samples)); err != nil {
		t.Fatalf("insert track: %v", err)
	}

	// Insert activity_streams in one batch.
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

	// Insert synthetic osm_lifts row + matching segment + segment_version
	// so loadOSMLiftCandidates resolves a candidate.
	if _, err := pool.Exec(ctx, `
		INSERT INTO osm_lifts (way_id, osm_id, name, aerialway, geom)
		VALUES ($1, $1, $2, 'gondola', ST_GeomFromText($3, 4326))
	`, syntheticOSMID, "synthetic-stage2-test-lift", liftWKT); err != nil {
		t.Fatalf("insert osm_lifts: %v", err)
	}

	segmentID := uuid.New().String()
	if _, err := pool.Exec(ctx, `
		INSERT INTO segments (id, created_by, name, segment_type, osm_lift_id, distance_m)
		VALUES ($1, $2, $3, 'lift', $4, $5)
	`, segmentID, userID, "synthetic stage-2 lift", syntheticOSMID, liftLengthM); err != nil {
		t.Fatalf("insert segments: %v", err)
	}
	segmentVersionID := uuid.New().String()
	if _, err := pool.Exec(ctx, `
		INSERT INTO segment_versions (id, segment_id, version_number, track, valid_from, created_by)
		VALUES ($1, $2, 1, ST_GeomFromText($3, 4326), CURRENT_DATE, $4)
	`, segmentVersionID, segmentID, liftWKT, userID); err != nil {
		t.Fatalf("insert segment_versions: %v", err)
	}

	// Build EnrichmentStreamPoint slice for enrichStreamLifts.
	stream := make([]EnrichmentStreamPoint, len(samples))
	for i, s := range samples {
		lat := s.lat
		lng := s.lng
		alt := s.alt
		spd := s.spd
		dst := s.dst
		stream[i] = EnrichmentStreamPoint{
			Time:      s.t,
			Lat:       &lat,
			Lng:       &lng,
			AltitudeM: &alt,
			SpeedMs:   &spd,
			DistanceM: &dst,
		}
	}

	// Sanity gate: confirm the heuristic detector splits the synthetic
	// stream into exactly the two rides Stage 2 requires. If a future
	// detector calibration change makes the fixture stop producing two
	// rides this gate fails fast with a clear message instead of letting
	// the assertions below misfire.
	{
		streamPts := make([]lifts.StreamPoint, len(samples))
		for i, s := range samples {
			lat := s.lat
			lng := s.lng
			alt := s.alt
			spd := s.spd
			dst := s.dst
			streamPts[i] = lifts.StreamPoint{
				Time: s.t, Lat: &lat, Lng: &lng, AltitudeM: &alt, SpeedMs: &spd, DistanceM: &dst,
			}
		}
		rides := lifts.DetectHeuristicWithIndices(streamPts, lifts.DefaultDetectorConfig("alpine_skiing"))
		if len(rides) != 2 {
			t.Fatalf("detector emitted %d rides, want 2 — fixture no longer triggers Stage 2 merge", len(rides))
		}
	}

	db := &database.DB{Pool: pool}

	// Pre-fix: this panics with "runtime error: index out of range [0] with
	// length 0" inside the SuppressedByMerge branch. Post-fix: returns nil.
	if err := enrichStreamLifts(ctx, db, userID, activityID, stream); err != nil {
		t.Fatalf("enrichStreamLifts: %v (Phase-4 critic F1: persistence must not error)", err)
	}

	// Assertion 2: exactly one segment_efforts row for the merged group.
	var effortCount int
	var canonicalEffortID string
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM segment_efforts
		WHERE activity_id = $1 AND segment_id = $2
	`, activityID, segmentID).Scan(&effortCount); err != nil {
		t.Fatalf("count segment_efforts: %v", err)
	}
	if effortCount != 1 {
		t.Fatalf("segment_efforts count = %d, want 1 (canonical merge row only)", effortCount)
	}
	if err := pool.QueryRow(ctx, `
		SELECT id FROM segment_efforts
		WHERE activity_id = $1 AND segment_id = $2 AND is_cable_hold_merge = true
		LIMIT 1
	`, activityID, segmentID).Scan(&canonicalEffortID); err != nil {
		t.Fatalf("fetch canonical effort id: %v", err)
	}

	// Assertion 3: N audit candidates with source='osm' all referencing the
	// canonical segment_effort_id. The synthetic stream produces exactly two
	// rides; if the detector behaves as the unit-level corpus expects we
	// see N=2 audit rows.
	rows, err := pool.Query(ctx, `
		SELECT source, segment_effort_id, confirmed
		FROM activity_lift_candidates
		WHERE activity_id = $1
		ORDER BY started_at
	`, activityID)
	if err != nil {
		t.Fatalf("query candidates: %v", err)
	}
	type candRow struct {
		source    string
		effortID  *string
		confirmed bool
	}
	var cands []candRow
	for rows.Next() {
		var c candRow
		if err := rows.Scan(&c.source, &c.effortID, &c.confirmed); err != nil {
			rows.Close()
			t.Fatalf("scan candidate: %v", err)
		}
		cands = append(cands, c)
	}
	rows.Close()
	if len(cands) < 2 {
		t.Fatalf("activity_lift_candidates count = %d, want >= 2 (canonical + suppressed sibling)", len(cands))
	}
	for i, c := range cands {
		if c.source != "osm" {
			t.Errorf("candidate[%d].source = %q, want 'osm'", i, c.source)
		}
		if c.effortID == nil {
			t.Errorf("candidate[%d].segment_effort_id IS NULL, want canonical effort id", i)
			continue
		}
		if *c.effortID != canonicalEffortID {
			t.Errorf("candidate[%d].segment_effort_id = %q, want canonical %q",
				i, *c.effortID, canonicalEffortID)
		}
		if !c.confirmed {
			t.Errorf("candidate[%d].confirmed = false, want true (audit rows are confirmed)", i)
		}
	}

	// Assertion 4: recomputeLiftStats counts the merged group as one lift.
	var liftCount int
	if err := pool.QueryRow(ctx, `
		SELECT lift_count FROM activities WHERE id = $1
	`, activityID).Scan(&liftCount); err != nil {
		t.Fatalf("query lift_count: %v", err)
	}
	if liftCount != 1 {
		t.Errorf("activities.lift_count = %d, want 1 (merged group counts as one)", liftCount)
	}
}
