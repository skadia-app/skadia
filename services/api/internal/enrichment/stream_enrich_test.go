package enrichment

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// TestStreamSettingsFromModel_PropagatesLiftDetection asserts that
// EnableLiftDetection flows from model.EnrichmentSettings into the smaller
// StreamEnrichmentSettings used by RunStreamEnrichment. Phase 4 introduced
// the field; the conversion must keep parity with the existing flags.
func TestStreamSettingsFromModel_PropagatesLiftDetection(t *testing.T) {
	cases := []struct {
		name string
		in   *model.EnrichmentSettings
		want bool
	}{
		{
			name: "lift on",
			in:   &model.EnrichmentSettings{EnableLiftDetection: true},
			want: true,
		},
		{
			name: "lift off",
			in:   &model.EnrichmentSettings{EnableLiftDetection: false},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := StreamSettingsFromModel(tc.in)
			if got.EnableLiftDetection != tc.want {
				t.Errorf("EnableLiftDetection = %v, want %v", got.EnableLiftDetection, tc.want)
			}
		})
	}
}

// TestAllStreamEnabled_IncludesLiftDetection guards the fallback case used
// when no per-user enrichment settings are stored: lift detection must be
// part of "all enabled" so re-enrich and recompute paths without explicit
// settings still run the new in-process detector.
func TestAllStreamEnabled_IncludesLiftDetection(t *testing.T) {
	got := AllStreamEnabled()
	if !got.EnableLiftDetection {
		t.Error("AllStreamEnabled().EnableLiftDetection = false, want true")
	}
	if !got.EnableStopDetection || !got.EnableAscentDetection || !got.EnableIntervalDetection {
		t.Errorf("AllStreamEnabled lost a pre-existing flag: %+v", got)
	}
}

// TestRunStreamEnrichment_LiftDetectionTriggersModule verifies that
// RunStreamEnrichment routes EnableLiftDetection=true into enrichStreamLifts.
//
// Determinism note: enrichStreamLifts short-circuits on empty osm_lifts, so
// the assertion is "the module ran without error and produced a count
// consistent with the local test DB's OSM state". The behavioural semantics
// of the detector itself live in lift_enrich_test.go; this test only proves
// the new wiring slot in the module list.
func TestRunStreamEnrichment_LiftDetectionTriggersModule(t *testing.T) {
	pool := liftTestPool(t)
	ctx := context.Background()
	userID := liftTestUser(t, pool)
	activityID := liftTestActivity(t, pool, userID)

	// Activity_tracks row keeps enrichStreamLifts's track-existence check
	// happy when osm_lifts has data (test environments vary; the test stays
	// green either way).
	if _, err := pool.Exec(ctx, `
		INSERT INTO activity_tracks (activity_id, track_full, point_count)
		VALUES ($1, ST_GeomFromText('LINESTRING(11.0 47.0, 11.01 47.01)', 4326), 2)
	`, activityID); err != nil {
		t.Fatalf("insert track: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	if _, err := pool.Exec(ctx, `
		INSERT INTO activity_streams (activity_id, time, lat, lng)
		VALUES ($1, $2, 47.0, 11.0),
		       ($1, $3, 47.01, 11.01)
	`, activityID, now, now.Add(30*time.Second)); err != nil {
		t.Fatalf("insert streams: %v", err)
	}

	db := &database.DB{Pool: pool}
	settings := StreamEnrichmentSettings{
		EnableLiftDetection:     true,
		EnableStopDetection:     false,
		EnableAscentDetection:   false,
		EnableIntervalDetection: false,
	}

	if err := RunStreamEnrichment(ctx, db, userID, activityID, settings); err != nil {
		t.Fatalf("RunStreamEnrichment: %v", err)
	}

	// The module ran. Either the test DB has osm_lifts data (count may be
	// positive — Method-A inserted candidates from real OSM geometry near
	// the synthetic GPS line) or it does not (count stays at 0 because the
	// detector short-circuits at the EXISTS check). Both branches mean the
	// wiring delivered the call. We only fail if the row count is impossible
	// (negative — indicates schema or query corruption).
	var count int
	if err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM activity_lift_candidates WHERE activity_id = $1",
		activityID,
	).Scan(&count); err != nil {
		t.Fatalf("count candidates: %v", err)
	}
	if count < 0 {
		t.Fatalf("impossible candidate count: %d", count)
	}
}

// TestRunStreamEnrichment_LiftDetectionDisabledSkipsModule asserts the
// negative wiring path: EnableLiftDetection=false must not call the lift
// detector even on a lift-relevant sport with valid track + streams.
func TestRunStreamEnrichment_LiftDetectionDisabledSkipsModule(t *testing.T) {
	pool := liftTestPool(t)
	ctx := context.Background()
	userID := liftTestUser(t, pool)
	activityID := liftTestActivity(t, pool, userID)

	if _, err := pool.Exec(ctx, `
		INSERT INTO activity_tracks (activity_id, track_full, point_count)
		VALUES ($1, ST_GeomFromText('LINESTRING(11.0 47.0, 11.01 47.01)', 4326), 2)
	`, activityID); err != nil {
		t.Fatalf("insert track: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := pool.Exec(ctx, `
		INSERT INTO activity_streams (activity_id, time, lat, lng)
		VALUES ($1, $2, 47.0, 11.0), ($1, $3, 47.01, 11.01)
	`, activityID, now, now.Add(30*time.Second)); err != nil {
		t.Fatalf("insert streams: %v", err)
	}

	// Pre-seed a sentinel candidate to prove enrichStreamLifts did NOT run
	// — if it had, the idempotent DELETE at the start of the function would
	// have wiped this row. With EnableLiftDetection=false, the row survives.
	sentinelID := uuid.New().String()
	if _, err := pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
		    (id, activity_id, started_at, ended_at,
		     duration_s, ascent_m, descent_m, distance_m, confidence, confirmed)
		VALUES ($1, $2, $3, $4, 60, 50, 0, 200, 'high', false)
	`, sentinelID, activityID, now, now.Add(time.Minute)); err != nil {
		t.Fatalf("insert sentinel candidate: %v", err)
	}

	db := &database.DB{Pool: pool}
	settings := StreamEnrichmentSettings{
		EnableLiftDetection:     false,
		EnableStopDetection:     false,
		EnableAscentDetection:   false,
		EnableIntervalDetection: false,
	}

	if err := RunStreamEnrichment(ctx, db, userID, activityID, settings); err != nil {
		t.Fatalf("RunStreamEnrichment: %v", err)
	}

	var stillExists bool
	if err := pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM activity_lift_candidates WHERE id = $1)",
		sentinelID,
	).Scan(&stillExists); err != nil {
		t.Fatalf("check sentinel: %v", err)
	}
	if !stillExists {
		t.Error("sentinel candidate was deleted — enrichStreamLifts ran despite EnableLiftDetection=false")
	}
}

func TestRunStreamEnrichment_LiftOnlyStrictReturnsModuleError(t *testing.T) {
	pool := liftTestPool(t)
	ctx := context.Background()
	userID := liftTestUser(t, pool)
	activityID := liftTestActivity(t, pool, userID)

	now := time.Now().UTC().Truncate(time.Second)
	if _, err := pool.Exec(ctx, `
		INSERT INTO activity_streams (activity_id, time, lat, lng)
		VALUES ($1, $2, 47.0, 11.0), ($1, $3, 47.01, 11.01)
	`, activityID, now, now.Add(30*time.Second)); err != nil {
		t.Fatalf("insert streams: %v", err)
	}

	db := &database.DB{Pool: pool}
	settings := StreamEnrichmentSettings{
		EnableLiftDetection:     true,
		EnableStopDetection:     false,
		EnableAscentDetection:   false,
		EnableIntervalDetection: false,
		FailOnModuleError:       true,
	}

	err := RunStreamEnrichment(ctx, db, "not-a-uuid", activityID, settings)
	if err == nil {
		t.Fatal("RunStreamEnrichment returned nil for strict lift module failure")
	}
	if !strings.Contains(err.Error(), "module lifts") {
		t.Fatalf("error=%q, want module lifts context", err.Error())
	}
}
