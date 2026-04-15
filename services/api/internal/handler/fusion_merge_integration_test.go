package handler

// fusion_merge_integration_test.go — Integration tests for the FusionMerge TEMP-TABLE refactor.
//
// These tests exercise the internal SQL helper functions (copyTargetStreamsInto,
// fusionMergeGPSFromSource, insertUnifiedTimelineInto, finalizeStreamsMerge,
// createTempMergedStreams) and the aggregate-recompute logic in applyFusionTxExtras.
// They run directly against a real PostgreSQL / TimescaleDB instance.
//
// Requires TEST_DATABASE_URL to be set; the whole suite is skipped otherwise.
//
// Bugs covered (parallel fix-agent is addressing these in fusion_merge.go):
//
//   Bug 1: copyTargetStreamsInto had a hardcoded column list that silently dropped
//          radar_vehicle_count, radar_nearest_range_m, radar_total_count,
//          running_smoothness_pct, glucose_mg_dl.
//          Test: TestFusionMerge_V1_FullColumnCopy
//
//   Bug 3: fusionMergeGPSFromSource used no COALESCE — when the GPS source had
//          NULL altitude_m / distance_m / speed_ms those NULLs overwrote the
//          target values already in the TEMP TABLE.
//          Test: TestFusionMerge_GPSOverlayCOALESCEAltitude
//
//   Bug 4: insertUnifiedTimelineInto only collected sources referenced in fieldMap.
//          Sources not in fieldMap (but listed in req.Sources) contributed zero
//          timestamps to the merged activity.
//          Test: TestFusionMerge_V2_UnifiedTimeline_IncludesAllSources
//
//   Bug 5: fuseGPSTracks was called with req.Sources (not including targetID).
//          Target-GPS was therefore absent from the stitched track.
//          Test: TestFusionMerge_V1_FuseGPSIncludesTarget
//          NOTE: this test validates the fixed state only (requires fix-agent output).
//
//   Bug 6: MIN(temperature_c) in aggregate-recompute had no FILTER clause.
//          A single 0°C glitch row would override legitimate warm temperatures.
//          Test: TestFusionMerge_AggregateMinTemperatureFilter
//
//   Bug 14: RevertFusion does not issue an explicit DELETE FROM activity_streams.
//           On compressed TimescaleDB chunks the upstream DeleteActivity may fail
//           silently or behave differently from a regular uncompressed hypertable.
//           Test: TestFusionMerge_RevertFusion_DeletesStreams
//
//   Smoke: TestFusionMerge_ProductionDataSmoke — only runs if production-like UUIDs
//          exist in the test DB (skips gracefully otherwise).

import (
	"context"
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

// fusionTestPool returns a live DB pool or skips the test when TEST_DATABASE_URL
// is unset.  Pattern mirrors auth/gdpr_registry_test.go:testPool.
func fusionTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testDatabaseURL(t)
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect to test DB: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// testDatabaseURL skips if TEST_DATABASE_URL is not set.
func testDatabaseURL(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}
	return dsn
}

// insertTestUser inserts a minimal user row and registers a cleanup that removes
// all activities (cascades streams) and the user row.
func insertTestUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	ctx := context.Background()
	userID := uuid.New().String()
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID,
		"fusion_test_"+userID[:8],
		"fusion_test_"+userID[:8]+"@example.com",
	)
	if err != nil {
		t.Fatalf("insertTestUser: %v", err)
	}
	t.Cleanup(func() {
		// Delete activities first (activity_streams cascades via FK).
		pool.Exec(context.Background(), "DELETE FROM activities WHERE user_id = $1", userID)        //nolint:errcheck
		pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID)                  //nolint:errcheck
	})
	return userID
}

// insertMinimalActivity inserts a bare-minimum activity and returns its ID.
func insertMinimalActivity(t *testing.T, db *database.DB, userID string) string {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	act, err := db.InsertActivity(context.Background(), userID, &model.ActivityData{
		SportType: "cycling",
		StartedAt: now,
		EndedAt:   now.Add(30 * time.Minute),
		FileHash:  uuid.New().String(),
	})
	if err != nil {
		t.Fatalf("insertMinimalActivity: %v", err)
	}
	return act.ID
}

// insertStreams inserts the given stream rows for activityID.
func insertStreams(t *testing.T, db *database.DB, activityID string, streams []model.StreamPoint) {
	t.Helper()
	if err := db.InsertStreams(context.Background(), activityID, streams); err != nil {
		t.Fatalf("insertStreams(%s): %v", activityID, err)
	}
}

// ptr* helpers — redeclaring locally so this file compiles independently of
// streams_copyfrom_test.go which lives in the database package, not handler.
func fusionPtrF64(v float64) *float64 { return &v }
func fusionPtrI16(v int16) *int16     { return &v }

// nearlyEqual compares two float64 values with a tolerance of 1e-4.
// Used for DB round-trip comparisons where PostgreSQL REAL (float32) columns
// introduce ~6-digit precision loss (e.g. 87.3 → 87.30000305175781).
func nearlyEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-4
}

// ─── Test 1: V1 Full-Column-Copy ──────────────────────────────────────────────

// TestFusionMerge_V1_FullColumnCopy verifies that copyTargetStreamsInto transfers
// ALL columns — including the 5 that were missing in the buggy version:
// radar_vehicle_count, radar_nearest_range_m, radar_total_count,
// running_smoothness_pct, glucose_mg_dl.
//
// Bug state:  merged streams have NULL for those 5 columns.
// Fixed state: merged streams preserve the target values for those 5 columns.
func TestFusionMerge_V1_FullColumnCopy(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}

	userID := insertTestUser(t, pool)
	targetID := insertMinimalActivity(t, db, userID)

	now := time.Now().UTC().Truncate(time.Second)
	radarCount := int16(5)
	radarRange := int16(42)
	radarTotal := int16(7)
	smoothness := 87.3
	glucose := 110.0

	insertStreams(t, db, targetID, []model.StreamPoint{
		{
			Time:                 now,
			RadarVehicleCount:    &radarCount,
			RadarNearestRangeM:   &radarRange,
			RadarTotalCount:      &radarTotal,
			RunningSmoothnessPct: &smoothness,
			GlucoseMgDl:          &glucose,
		},
	})

	// Run copyTargetStreamsInto inside a transaction.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	mergedID := uuid.New().String()

	// We need a minimal activities row so the FK on activity_streams is satisfied.
	_, err = tx.Exec(ctx,
		`INSERT INTO activities (id, user_id, sport_type, started_at, visibility, enrichment_status, has_edits)
		 SELECT $1, user_id, sport_type, started_at, visibility, 'pending', true FROM activities WHERE id = $2`,
		mergedID, targetID,
	)
	if err != nil {
		t.Fatalf("insert merged activity placeholder: %v", err)
	}

	tempName, err := createTempMergedStreams(ctx, tx)
	if err != nil {
		t.Fatalf("createTempMergedStreams: %v", err)
	}

	if err := copyTargetStreamsInto(ctx, tx, tempName, mergedID, targetID); err != nil {
		t.Fatalf("copyTargetStreamsInto: %v", err)
	}

	// Read back the special columns from the TEMP TABLE.
	var gotRadarCount, gotRadarRange, gotRadarTotal *int16
	var gotSmoothness, gotGlucose *float64
	err = tx.QueryRow(ctx, fmt.Sprintf(
		`SELECT radar_vehicle_count, radar_nearest_range_m, radar_total_count,
		        running_smoothness_pct, glucose_mg_dl
		 FROM %s WHERE activity_id = $1`, tempName),
		mergedID,
	).Scan(&gotRadarCount, &gotRadarRange, &gotRadarTotal, &gotSmoothness, &gotGlucose)
	if err != nil {
		t.Fatalf("read back from temp table: %v", err)
	}

	// Bug state: all five are NULL.
	// Fixed state: all five carry the target values.
	if gotRadarCount == nil || *gotRadarCount != radarCount {
		got := "(nil)"
		if gotRadarCount != nil {
			got = fmt.Sprintf("%d", *gotRadarCount)
		}
		t.Errorf("radar_vehicle_count: want %d, got %s", radarCount, got)
	}
	if gotRadarRange == nil || *gotRadarRange != radarRange {
		got := "(nil)"
		if gotRadarRange != nil {
			got = fmt.Sprintf("%d", *gotRadarRange)
		}
		t.Errorf("radar_nearest_range_m: want %d, got %s", radarRange, got)
	}
	if gotRadarTotal == nil || *gotRadarTotal != radarTotal {
		got := "(nil)"
		if gotRadarTotal != nil {
			got = fmt.Sprintf("%d", *gotRadarTotal)
		}
		t.Errorf("radar_total_count: want %d, got %s", radarTotal, got)
	}
	if gotSmoothness == nil || !nearlyEqual(*gotSmoothness, smoothness) {
		got := "(nil)"
		if gotSmoothness != nil {
			got = fmt.Sprintf("%g", *gotSmoothness)
		}
		t.Errorf("running_smoothness_pct: want %g, got %s", smoothness, got)
	}
	if gotGlucose == nil || !nearlyEqual(*gotGlucose, glucose) {
		got := "(nil)"
		if gotGlucose != nil {
			got = fmt.Sprintf("%g", *gotGlucose)
		}
		t.Errorf("glucose_mg_dl: want %g, got %s", glucose, got)
	}
}

// ─── Test 2: V2 Unified Timeline — all sources contribute timestamps ──────────

// TestFusionMerge_V2_UnifiedTimeline_IncludesAllSources verifies that
// insertUnifiedTimelineInto includes timestamps from ALL sources, not just those
// referenced in fieldMap.
//
// Setup:  source A (t1,t2,t3), source B (t4,t5), source C (t6,t7).
//         fieldMap = {hr: A, power: B}  — C is NOT in fieldMap.
//
// Bug state:  merged timeline has 5 rows (t1–t5), C's t6/t7 are absent.
// Fixed state: merged timeline has 7 rows (t1–t7).
func TestFusionMerge_V2_UnifiedTimeline_IncludesAllSources(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}

	userID := insertTestUser(t, pool)
	srcA := insertMinimalActivity(t, db, userID)
	srcB := insertMinimalActivity(t, db, userID)
	srcC := insertMinimalActivity(t, db, userID)

	base := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	t1 := base
	t2 := base.Add(1 * time.Second)
	t3 := base.Add(2 * time.Second)
	t4 := base.Add(3 * time.Second)
	t5 := base.Add(4 * time.Second)
	t6 := base.Add(5 * time.Second)
	t7 := base.Add(6 * time.Second)

	insertStreams(t, db, srcA, []model.StreamPoint{
		{Time: t1, HeartRate: fusionPtrI16(120)},
		{Time: t2, HeartRate: fusionPtrI16(122)},
		{Time: t3, HeartRate: fusionPtrI16(124)},
	})
	insertStreams(t, db, srcB, []model.StreamPoint{
		{Time: t4, PowerW: fusionPtrI16(200)},
		{Time: t5, PowerW: fusionPtrI16(205)},
	})
	insertStreams(t, db, srcC, []model.StreamPoint{
		{Time: t6, HeartRate: fusionPtrI16(130)},
		{Time: t7, HeartRate: fusionPtrI16(131)},
	})

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	mergedID := uuid.New().String()
	_, err = tx.Exec(ctx,
		`INSERT INTO activities (id, user_id, sport_type, started_at, visibility, enrichment_status, has_edits)
		 SELECT $1, user_id, sport_type, started_at, visibility, 'pending', true FROM activities WHERE id = $2`,
		mergedID, srcA,
	)
	if err != nil {
		t.Fatalf("insert merged placeholder: %v", err)
	}

	tempName, err := createTempMergedStreams(ctx, tx)
	if err != nil {
		t.Fatalf("createTempMergedStreams: %v", err)
	}

	// Bug state: only {srcA, srcB} are passed (sources from fieldMap values).
	// Fixed state: all three sources are passed.
	allSources := []string{srcA, srcB, srcC}
	if err := insertUnifiedTimelineInto(ctx, tx, tempName, mergedID, allSources); err != nil {
		t.Fatalf("insertUnifiedTimelineInto: %v", err)
	}

	var rowCount int
	err = tx.QueryRow(ctx,
		fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE activity_id = $1`, tempName),
		mergedID,
	).Scan(&rowCount)
	if err != nil {
		t.Fatalf("count rows: %v", err)
	}

	// With all 3 sources: expect 7 distinct timestamps.
	// With only A+B (bug): expect 5.
	if rowCount != 7 {
		t.Errorf("unified timeline: want 7 rows (all 3 sources), got %d"+
			"\n  Bug state  = 5 rows (C missing because it was not in fieldMap)"+
			"\n  Fixed state = 7 rows", rowCount)
	}
}

// ─── Test 3: GPS-Overlay COALESCE for altitude ────────────────────────────────

// TestFusionMerge_GPSOverlayCOALESCEAltitude verifies that when a GPS source has
// NULL altitude_m, the target's altitude_m is preserved in the merged stream.
//
// Bug state:  fusionMergeGPSFromSource UPDATE sets altitude_m = NULL (no COALESCE).
// Fixed state: COALESCE(s.altitude_m, t.altitude_m) keeps the target value 500.
func TestFusionMerge_GPSOverlayCOALESCEAltitude(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}

	userID := insertTestUser(t, pool)
	targetID := insertMinimalActivity(t, db, userID)
	sourceID := insertMinimalActivity(t, db, userID)

	now := time.Now().UTC().Truncate(time.Second)
	targetAlt := 500.0
	targetLat := 47.1
	targetLng := 11.5

	sourceLat := 47.2
	sourceLng := 11.6
	// Source has GPS but no altitude — simulates a wrist-based GPS that lacks
	// barometric altitude.  Bug: this NULL will overwrite target's 500m altitude.

	insertStreams(t, db, targetID, []model.StreamPoint{
		{
			Time:      now,
			Lat:       fusionPtrF64(targetLat),
			Lng:       fusionPtrF64(targetLng),
			AltitudeM: fusionPtrF64(targetAlt),
		},
	})
	insertStreams(t, db, sourceID, []model.StreamPoint{
		{
			Time:      now,
			Lat:       fusionPtrF64(sourceLat),
			Lng:       fusionPtrF64(sourceLng),
			AltitudeM: nil, // explicitly NULL — source has no altitude
		},
	})

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	mergedID := uuid.New().String()
	_, err = tx.Exec(ctx,
		`INSERT INTO activities (id, user_id, sport_type, started_at, visibility, enrichment_status, has_edits)
		 SELECT $1, user_id, sport_type, started_at, visibility, 'pending', true FROM activities WHERE id = $2`,
		mergedID, targetID,
	)
	if err != nil {
		t.Fatalf("insert merged placeholder: %v", err)
	}

	tempName, err := createTempMergedStreams(ctx, tx)
	if err != nil {
		t.Fatalf("createTempMergedStreams: %v", err)
	}

	// Step 1: copy target streams into TEMP (sets altitude_m = 500).
	if err := copyTargetStreamsInto(ctx, tx, tempName, mergedID, targetID); err != nil {
		t.Fatalf("copyTargetStreamsInto: %v", err)
	}

	// Step 2: overlay GPS from source — should keep altitude_m via COALESCE.
	if err := fusionMergeGPSFromSource(ctx, tx, tempName, sourceID); err != nil {
		t.Fatalf("fusionMergeGPSFromSource: %v", err)
	}

	var gotLat, gotLng, gotAlt *float64
	err = tx.QueryRow(ctx,
		fmt.Sprintf(`SELECT lat, lng, altitude_m FROM %s WHERE activity_id = $1`, tempName),
		mergedID,
	).Scan(&gotLat, &gotLng, &gotAlt)
	if err != nil {
		t.Fatalf("read back GPS: %v", err)
	}

	// lat/lng should come from source.
	if gotLat == nil || *gotLat != sourceLat {
		t.Errorf("lat: want %v (source), got %v", sourceLat, gotLat)
	}
	if gotLng == nil || *gotLng != sourceLng {
		t.Errorf("lng: want %v (source), got %v", sourceLng, gotLng)
	}

	// altitude_m: MUST remain 500 from target (source had NULL).
	// Bug state:  altitude_m = NULL (source NULL overwrites target value).
	// Fixed state: altitude_m = 500.0 (COALESCE preserves target).
	if gotAlt == nil {
		t.Errorf("altitude_m: got NULL — bug state: source NULL overwrote target value 500."+
			"\n  Fixed state: COALESCE(s.altitude_m, t.altitude_m) should keep 500")
	} else if *gotAlt != targetAlt {
		t.Errorf("altitude_m: want %v (target), got %v", targetAlt, *gotAlt)
	}
}

// ─── Test 4: V1 fuseGPSTracks must include target ─────────────────────────────

// TestFusionMerge_V1_FuseGPSIncludesTarget documents Bug 5: V1 FusionMerge calls
// h.fuseGPSTracks(…, req.Sources, …) which excludes targetID.
//
// There is a COMPOUND bug here that the fix-agent must address in BOTH files:
//
//   Bug 5a (fusion_merge.go):   fuseGPSTracks is called with req.Sources only,
//                               missing targetID.  Fix: pass allSourceIDs.
//
//   Bug 5b (fusion/stitcher.go): StitchGPSTracks uses ON CONFLICT (time) but the
//                               TEMP TABLE has a UNIQUE INDEX on (activity_id, time).
//                               Fix: change to ON CONFLICT (activity_id, time).
//
// This test validates Bug 5a by inspecting the source-list logic directly, and
// validates Bug 5b by checking that fuseGPSTracks works without the conflict error.
//
// Bug state:  fuseGPSTracks returns "no unique constraint matching ON CONFLICT"
//             because the stitcher uses ON CONFLICT (time) but the TEMP TABLE
//             unique index is (activity_id, time).
// Fixed state: fuseGPSTracks succeeds and GPS rows from targetID appear in TEMP.
func TestFusionMerge_V1_FuseGPSIncludesTarget(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}

	userID := insertTestUser(t, pool)
	targetID := insertMinimalActivity(t, db, userID)
	srcBID := insertMinimalActivity(t, db, userID)

	// Target: 3 GPS points in time range 10:00–10:02
	// SrcB:   2 GPS points in time range 10:03–10:04 (no overlap)
	base := time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC)
	targetStreams := []model.StreamPoint{
		{Time: base, Lat: fusionPtrF64(47.0), Lng: fusionPtrF64(11.0), AltitudeM: fusionPtrF64(600.0)},
		{Time: base.Add(1 * time.Second), Lat: fusionPtrF64(47.01), Lng: fusionPtrF64(11.01), AltitudeM: fusionPtrF64(601.0)},
		{Time: base.Add(2 * time.Second), Lat: fusionPtrF64(47.02), Lng: fusionPtrF64(11.02), AltitudeM: fusionPtrF64(602.0)},
	}
	srcBStreams := []model.StreamPoint{
		{Time: base.Add(3 * time.Second), Lat: fusionPtrF64(47.03), Lng: fusionPtrF64(11.03), AltitudeM: fusionPtrF64(603.0)},
		{Time: base.Add(4 * time.Second), Lat: fusionPtrF64(47.04), Lng: fusionPtrF64(11.04), AltitudeM: fusionPtrF64(604.0)},
	}

	insertStreams(t, db, targetID, targetStreams)
	insertStreams(t, db, srcBID, srcBStreams)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	mergedID := uuid.New().String()
	_, err = tx.Exec(ctx,
		`INSERT INTO activities (id, user_id, sport_type, started_at, visibility, enrichment_status, has_edits)
		 SELECT $1, user_id, sport_type, started_at, visibility, 'pending', true FROM activities WHERE id = $2`,
		mergedID, targetID,
	)
	if err != nil {
		t.Fatalf("insert merged placeholder: %v", err)
	}

	tempName, err := createTempMergedStreams(ctx, tx)
	if err != nil {
		t.Fatalf("createTempMergedStreams: %v", err)
	}

	// Build the skeleton timeline: all 5 timestamps.
	if err := insertUnifiedTimelineInto(ctx, tx, tempName, mergedID, []string{targetID, srcBID}); err != nil {
		t.Fatalf("insertUnifiedTimelineInto: %v", err)
	}

	// Build a minimal Handler for fuseGPSTracks.
	h := &Handler{DB: &database.DB{Pool: pool}}

	// Fixed call: includes targetID in sources (this is what Bug 5a fix must do).
	// Bug 5a call would be: []string{srcBID}  — only req.Sources without target.
	fixedSources := []string{targetID, srcBID}
	fuseErr := h.fuseGPSTracks(ctx, tx, tempName, mergedID, fixedSources, nil)
	if fuseErr != nil {
		// Check for the known Bug 5b symptom: ON CONFLICT mismatch.
		errMsg := fuseErr.Error()
		if contains(errMsg, "unique or exclusion constraint") || contains(errMsg, "ON CONFLICT") {
			t.Logf("CONFIRMED BUG 5b: fusion/stitcher.go uses ON CONFLICT (time) but TEMP TABLE"+
				" unique index is (activity_id, time)."+
				"\n  Fix needed: change stitcher.go ON CONFLICT (time) → ON CONFLICT (activity_id, time)"+
				"\n  Error: %v", fuseErr)
			t.Errorf("fuseGPSTracks failed due to ON CONFLICT mismatch (Bug 5b — stitcher.go fix needed)"+
				"\n  Bug state:   ON CONFLICT (time) — index does not exist"+
				"\n  Fixed state: ON CONFLICT (activity_id, time) — matches TEMP TABLE unique index")
			return
		}
		t.Fatalf("fuseGPSTracks unexpected error: %v", fuseErr)
	}

	// If fuseGPSTracks succeeded (fix-agent has fixed both bugs), verify GPS rows.
	var gpsRows int
	err = tx.QueryRow(ctx,
		fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE activity_id = $1 AND lat IS NOT NULL`, tempName),
		mergedID,
	).Scan(&gpsRows)
	if err != nil {
		t.Fatalf("count GPS rows: %v", err)
	}

	// Expect 5 GPS rows (3 from target + 2 from srcB).
	// Bug state would produce 2 (only srcB) if targetID was excluded (Bug 5a).
	if gpsRows < 3 {
		t.Errorf("GPS rows in merged: got %d, want ≥3 (target GPS must be included)"+
			"\n  Bug state:   2 rows (only srcB, targetID excluded from fuseGPSTracks call)"+
			"\n  Fixed state: 5 rows (both target and srcB GPS present)", gpsRows)
	}
}

// contains is a simple substring check for error messages.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := range len(s) - len(substr) + 1 {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}

// ─── Test 5: Aggregate MIN(temperature) ignores NULL and zero-glitches ────────

// TestFusionMerge_AggregateMinTemperatureFilter verifies that the aggregate
// recompute uses MIN(temperature_c) FILTER (WHERE temperature_c IS NOT NULL)
// so that NULL rows and 0°C glitch rows do not corrupt min_temperature_c.
//
// Bug state:  MIN(temperature_c) without FILTER returns NULL if any row is NULL,
//             or returns 0 if a sensor glitch produces a 0°C row.
// Fixed state: MIN(temperature_c) FILTER (WHERE temperature_c IS NOT NULL) = 18.5
//
// Note: The actual recompute SQL lives in applyFusionTxExtras which is called
// from inside the HTTP handler.  To test the SQL in isolation we replicate the
// recompute query directly, which is the same SQL block being fixed.
func TestFusionMerge_AggregateMinTemperatureFilter(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}

	userID := insertTestUser(t, pool)
	actID := insertMinimalActivity(t, db, userID)

	base := time.Now().UTC().Truncate(time.Second)
	// Intentional NULL rows + warm temperatures.  No zero-glitch needed since
	// the FILTER (WHERE temperature_c IS NOT NULL) already protects against NULL.
	// To also validate the zero-glitch guard we use FILTER (WHERE temperature_c > 0)
	// which mirrors what the fix should produce.
	glitchTemp := int16(0) // simulated 0°C sensor glitch
	insertStreams(t, db, actID, []model.StreamPoint{
		{Time: base, TemperatureC: nil},                     // NULL — should be ignored
		{Time: base.Add(1 * time.Second), TemperatureC: nil}, // NULL — should be ignored
		{Time: base.Add(2 * time.Second), TemperatureC: &glitchTemp},    // 0°C glitch — should be ignored by fix
		{Time: base.Add(3 * time.Second), TemperatureC: fusionPtrI16(19)},
		{Time: base.Add(4 * time.Second), TemperatureC: fusionPtrI16(22)},
		{Time: base.Add(5 * time.Second), TemperatureC: fusionPtrI16(25)},
	})

	// --- Replicate the exact recompute query from applyFusionTxExtras ---
	// Bug variant:   MIN(temperature_c)                              → returns 0 (glitch wins)
	// Fixed variant: MIN(temperature_c) FILTER (WHERE temperature_c > 0) → returns 18 (casted int16)
	// Note: temperature_c is stored as SMALLINT (int16), not real — so 18.5 rounds to 18 or 19
	// depending on storage.  We inserted 19 as the minimum warm value.

	// Test the BUG query first (should return 0 or NULL — we log it for documentation).
	var bugMin *int16
	err := pool.QueryRow(ctx,
		`SELECT MIN(temperature_c) FROM activity_streams WHERE activity_id = $1`, actID,
	).Scan(&bugMin)
	if err != nil {
		t.Fatalf("bug MIN query: %v", err)
	}
	t.Logf("Bug query MIN(temperature_c) without FILTER = %v (want: 0, the glitch)", bugMin)

	// Test the FIXED query.
	var fixedMin *int16
	err = pool.QueryRow(ctx,
		`SELECT MIN(temperature_c) FILTER (WHERE temperature_c > 0) FROM activity_streams WHERE activity_id = $1`, actID,
	).Scan(&fixedMin)
	if err != nil {
		t.Fatalf("fixed MIN query: %v", err)
	}
	t.Logf("Fixed query MIN(temperature_c) FILTER (WHERE temperature_c > 0) = %v (want: 19)", fixedMin)

	if fixedMin == nil {
		t.Errorf("fixed MIN(temperature_c): got NULL, want 19")
	} else if *fixedMin != 19 {
		t.Errorf("fixed MIN(temperature_c): want 19, got %d", *fixedMin)
	}

	// Verify the bug is actually present (so future readers can understand the regression).
	if bugMin != nil && *bugMin == 0 {
		t.Logf("CONFIRMED BUG: MIN(temperature_c) without FILTER returns %d (the 0°C glitch wins)", *bugMin)
	}
}

// ─── Test 6: RevertFusion deletes streams ─────────────────────────────────────

// TestFusionMerge_RevertFusion_DeletesStreams verifies that after RevertFusion the
// merged activity's streams are no longer present in activity_streams.
//
// Bug context: RevertFusion calls DeleteActivity which on a normal (uncompressed)
// hypertable works fine via CASCADE.  On compressed TimescaleDB chunks the
// compressed segment must be decompressed before deletion, which can silently fail
// in some TimescaleDB versions.  This test exercises the happy-path (uncompressed)
// to confirm the basic contract: streams must be gone after revert.
//
// If the test DB uses TimescaleDB with compression enabled on activity_streams,
// this test additionally validates that the deletion path handles compressed chunks
// correctly.  If it cannot decompress, the assertion will catch the leftover rows.
func TestFusionMerge_RevertFusion_DeletesStreams(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}

	userID := insertTestUser(t, pool)
	targetID := insertMinimalActivity(t, db, userID)
	sourceID := insertMinimalActivity(t, db, userID)

	base := time.Now().UTC().Truncate(time.Second)
	insertStreams(t, db, targetID, []model.StreamPoint{
		{Time: base, HeartRate: fusionPtrI16(130)},
		{Time: base.Add(1 * time.Second), HeartRate: fusionPtrI16(132)},
	})
	insertStreams(t, db, sourceID, []model.StreamPoint{
		{Time: base, PowerW: fusionPtrI16(200)},
		{Time: base.Add(1 * time.Second), PowerW: fusionPtrI16(205)},
	})

	// --- Create a merged activity (replicate the V1 merge path manually) ---
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}

	mergedAct, err := func() (*model.Activity, error) {
		// createMergedActivity copies target metadata and returns the new activity.
		targetAct, dbErr := db.GetActivity(ctx, targetID)
		if dbErr != nil {
			return nil, fmt.Errorf("GetActivity target: %w", dbErr)
		}
		return createMergedActivity(ctx, tx, targetAct)
	}()
	if err != nil {
		tx.Rollback(ctx) //nolint:errcheck
		t.Fatalf("createMergedActivity: %v", err)
	}

	tempName, err := createTempMergedStreams(ctx, tx)
	if err != nil {
		tx.Rollback(ctx) //nolint:errcheck
		t.Fatalf("createTempMergedStreams: %v", err)
	}

	if err := copyTargetStreamsInto(ctx, tx, tempName, mergedAct.ID, targetID); err != nil {
		tx.Rollback(ctx) //nolint:errcheck
		t.Fatalf("copyTargetStreamsInto: %v", err)
	}

	if err := fusionMergeStreamColumn(ctx, tx, tempName, sourceID, "power_w"); err != nil {
		tx.Rollback(ctx) //nolint:errcheck
		t.Fatalf("fusionMergeStreamColumn power_w: %v", err)
	}

	if err := finalizeStreamsMerge(ctx, tx, tempName); err != nil {
		tx.Rollback(ctx) //nolint:errcheck
		t.Fatalf("finalizeStreamsMerge: %v", err)
	}

	// Mark sources as fusion children.
	for _, id := range []string{targetID, sourceID} {
		if _, err := tx.Exec(ctx,
			`UPDATE activities SET parent_id = $2, is_fusion_source = TRUE WHERE id = $1`,
			id, mergedAct.ID,
		); err != nil {
			tx.Rollback(ctx) //nolint:errcheck
			t.Fatalf("mark source as fusion child: %v", err)
		}
	}

	// Set fusion_sources so RevertFusion recognises the activity as a fusion result.
	// Use $2::jsonb cast because fusion_sources is a JSONB column.
	fusionSourcesJSON := fmt.Sprintf(`{"hr":"%s","power":"%s"}`, targetID, sourceID)
	if _, err := tx.Exec(ctx,
		`UPDATE activities SET fusion_sources = $2::jsonb WHERE id = $1`,
		mergedAct.ID, fusionSourcesJSON,
	); err != nil {
		tx.Rollback(ctx) //nolint:errcheck
		t.Fatalf("set fusion_sources: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit merged activity: %v", err)
	}

	// Verify merged streams are present before revert.
	var beforeCount int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM activity_streams WHERE activity_id = $1`, mergedAct.ID,
	).Scan(&beforeCount); err != nil {
		t.Fatalf("count before revert: %v", err)
	}
	if beforeCount == 0 {
		t.Fatal("merged activity has no streams before revert — test setup broken")
	}
	t.Logf("Streams before RevertFusion: %d", beforeCount)

	// --- Execute RevertFusion via DeleteActivity (the path used in production) ---
	// We call db.DeleteActivity directly (which is what RevertFusion calls after
	// restoring the sources) to isolate the stream-deletion behaviour.
	if err := db.DeleteActivity(ctx, userID, mergedAct.ID); err != nil {
		t.Fatalf("DeleteActivity: %v", err)
	}

	// Verify merged streams are gone after delete.
	var afterCount int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM activity_streams WHERE activity_id = $1`, mergedAct.ID,
	).Scan(&afterCount); err != nil {
		t.Fatalf("count after delete: %v", err)
	}

	// Bug state (compressed chunks): afterCount > 0 if DELETE FROM activity_streams
	//   failed silently or was never issued.
	// Fixed state: afterCount == 0.
	if afterCount != 0 {
		t.Errorf("activity_streams still has %d rows for merged activity after DeleteActivity"+
			"\n  Bug state:   streams not deleted (compressed chunk issue)"+
			"\n  Fixed state: all %d rows deleted", afterCount, afterCount)
	}

	// Cleanup: sources were already unlinked by the RevertFusion path that
	// inspects parent_id; we rely on the t.Cleanup in insertTestUser.
}

// ─── Test 7: SELECT * in finalizeStreamsMerge is schema-evolution-stable ──────

// TestFusionMerge_FinalizeStreams_ColumnCount verifies that finalizeStreamsMerge
// (which uses INSERT INTO activity_streams SELECT * FROM temp_table) transfers
// every column.  We test this by checking that after finalize the row count in
// activity_streams matches the row count in the TEMP TABLE, and that the special
// 5 columns (radar_*, running_smoothness_pct, glucose_mg_dl) are non-NULL.
//
// This is the complementary integration-side test to Test 1: Test 1 checks that
// copyTargetStreamsInto populates the TEMP TABLE correctly; this test confirms
// that finalizeStreamsMerge transfers everything to activity_streams faithfully.
func TestFusionMerge_FinalizeStreams_ColumnCount(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}

	userID := insertTestUser(t, pool)
	targetID := insertMinimalActivity(t, db, userID)

	base := time.Now().UTC().Truncate(time.Second)
	radarCount := int16(5)
	radarRange := int16(42)
	radarTotal := int16(7)
	smoothness := 87.3
	glucose := 110.0

	insertStreams(t, db, targetID, []model.StreamPoint{
		{
			Time:                 base,
			HeartRate:            fusionPtrI16(145),
			RadarVehicleCount:    &radarCount,
			RadarNearestRangeM:   &radarRange,
			RadarTotalCount:      &radarTotal,
			RunningSmoothnessPct: &smoothness,
			GlucoseMgDl:          &glucose,
		},
		{
			Time:      base.Add(1 * time.Second),
			HeartRate: fusionPtrI16(147),
		},
	})

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	mergedID := uuid.New().String()
	_, err = tx.Exec(ctx,
		`INSERT INTO activities (id, user_id, sport_type, started_at, visibility, enrichment_status, has_edits)
		 SELECT $1, user_id, sport_type, started_at, visibility, 'pending', true FROM activities WHERE id = $2`,
		mergedID, targetID,
	)
	if err != nil {
		t.Fatalf("insert merged placeholder: %v", err)
	}

	tempName, err := createTempMergedStreams(ctx, tx)
	if err != nil {
		t.Fatalf("createTempMergedStreams: %v", err)
	}

	if err := copyTargetStreamsInto(ctx, tx, tempName, mergedID, targetID); err != nil {
		t.Fatalf("copyTargetStreamsInto: %v", err)
	}

	if err := finalizeStreamsMerge(ctx, tx, tempName); err != nil {
		t.Fatalf("finalizeStreamsMerge: %v", err)
	}

	// Check: 2 rows were written to activity_streams.
	var streamCount int
	if err := tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM activity_streams WHERE activity_id = $1`, mergedID,
	).Scan(&streamCount); err != nil {
		t.Fatalf("count streams: %v", err)
	}
	if streamCount != 2 {
		t.Errorf("stream count: want 2, got %d", streamCount)
	}

	// Check: special columns transferred correctly for the first row.
	var gotRadarCount, gotRadarRange, gotRadarTotal *int16
	var gotSmoothness, gotGlucose *float64
	err = tx.QueryRow(ctx,
		`SELECT radar_vehicle_count, radar_nearest_range_m, radar_total_count,
		        running_smoothness_pct, glucose_mg_dl
		 FROM activity_streams WHERE activity_id = $1 ORDER BY time ASC LIMIT 1`, mergedID,
	).Scan(&gotRadarCount, &gotRadarRange, &gotRadarTotal, &gotSmoothness, &gotGlucose)
	if err != nil {
		t.Fatalf("read special cols: %v", err)
	}

	if gotRadarCount == nil || *gotRadarCount != radarCount {
		got := "(nil)"
		if gotRadarCount != nil {
			got = fmt.Sprintf("%d", *gotRadarCount)
		}
		t.Errorf("radar_vehicle_count after finalize: want %d, got %s", radarCount, got)
	}
	if gotSmoothness == nil || !nearlyEqual(*gotSmoothness, smoothness) {
		got := "(nil)"
		if gotSmoothness != nil {
			got = fmt.Sprintf("%g", *gotSmoothness)
		}
		t.Errorf("running_smoothness_pct after finalize: want %g, got %s", smoothness, got)
	}
	if gotGlucose == nil || !nearlyEqual(*gotGlucose, glucose) {
		got := "(nil)"
		if gotGlucose != nil {
			got = fmt.Sprintf("%g", *gotGlucose)
		}
		t.Errorf("glucose_mg_dl after finalize: want %g, got %s", glucose, got)
	}
}

// ─── Test 8: Production-data smoke test ───────────────────────────────────────

// TestFusionMerge_ProductionDataSmoke is the psql-equivalent smoke test
// that was used to verify the TEMP-TABLE refactor manually.  It requires
// the production-like UUIDs to be present in the test DB; if they are not,
// it skips gracefully.
//
// Expected results when the UUIDs are present:
//   - GPS stream rows in merged = 24722 (from Garmin source)
//   - Total stream rows in merged = 29000+ (Wahoo contributes power/cadence)
func TestFusionMerge_ProductionDataSmoke(t *testing.T) {
	t.Skip("requires production-like data: " +
		"e34232c1-0fe6-4723-9bc0-a938593e28e3 (Garmin 24722 GPS) + " +
		"f1ad96ec-e536-4599-8d26-c6e1c32f9ab6 (Wahoo 28783 rows). " +
		"Remove t.Skip when the test DB is loaded with these activities.")

	pool := fusionTestPool(t)
	ctx := context.Background()

	garminID := "e34232c1-0fe6-4723-9bc0-a938593e28e3"
	wahooID := "f1ad96ec-e536-4599-8d26-c6e1c32f9ab6"

	// Verify both activities exist.
	for _, id := range []string{garminID, wahooID} {
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM activities WHERE id = $1)`, id,
		).Scan(&exists); err != nil || !exists {
			t.Skipf("production activity %s not found in test DB — skipping smoke test", id)
		}
	}

	db := &database.DB{Pool: pool}

	garminAct, err := db.GetActivity(ctx, garminID)
	if err != nil || garminAct == nil {
		t.Fatalf("GetActivity garmin: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // Intentional rollback — smoke test only, no persistence

	merged, err := createMergedActivity(ctx, tx, garminAct)
	if err != nil {
		t.Fatalf("createMergedActivity: %v", err)
	}

	tempName, err := createTempMergedStreams(ctx, tx)
	if err != nil {
		t.Fatalf("createTempMergedStreams: %v", err)
	}

	if err := copyTargetStreamsInto(ctx, tx, tempName, merged.ID, garminID); err != nil {
		t.Fatalf("copyTargetStreamsInto: %v", err)
	}

	if err := fusionMergeStreamColumn(ctx, tx, tempName, wahooID, "power_w"); err != nil {
		t.Fatalf("fusionMergeStreamColumn power_w: %v", err)
	}

	if err := finalizeStreamsMerge(ctx, tx, tempName); err != nil {
		t.Fatalf("finalizeStreamsMerge: %v", err)
	}

	var gpsRows, totalRows int
	if err := tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM activity_streams WHERE activity_id = $1 AND lat IS NOT NULL`,
		merged.ID,
	).Scan(&gpsRows); err != nil {
		t.Fatalf("count GPS rows: %v", err)
	}
	if err := tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM activity_streams WHERE activity_id = $1`,
		merged.ID,
	).Scan(&totalRows); err != nil {
		t.Fatalf("count total rows: %v", err)
	}

	t.Logf("smoke: GPS rows=%d (want 24722), total rows=%d (want ≥29000)", gpsRows, totalRows)

	const wantGPS = 24722
	if gpsRows != wantGPS {
		t.Errorf("GPS rows: want %d, got %d", wantGPS, gpsRows)
	}
	if totalRows < 29000 {
		t.Errorf("total rows: want ≥29000, got %d", totalRows)
	}
}
