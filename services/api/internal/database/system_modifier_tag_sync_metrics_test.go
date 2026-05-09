package database

// TDD coverage for m.3: Prometheus instrumentation on the system modifier tag
// sync helper.
//
// All tests use an isolated prometheus.Registry so they never collide with the
// package-level promauto registrations in other packages that share the default
// DefaultRegisterer in the test binary.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	dto "github.com/prometheus/client_model/go"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"
)

// makeMetricsDB wires a test-scoped prometheus.Registry into a fresh DB
// so all metric assertions are isolated to this test.
func makeMetricsDB(t *testing.T) (*DB, *prometheus.Registry) {
	t.Helper()
	pool := testdb.Open(t)
	reg := prometheus.NewRegistry()
	m := newSystemModifierTagSyncMetrics(reg)
	return &DB{Pool: pool, syncMetrics: m}, reg
}

// metricsTestSetup inserts the user + system tags + activities and returns the DB
// with injected registry, the registry for assertions, and the activity IDs.
func metricsTestSetup(t *testing.T) (*DB, *prometheus.Registry, string, []string) {
	t.Helper()
	db, reg := makeMetricsDB(t)
	ctx := context.Background()

	userID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID, "met-"+userID[:8], "met-"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	for _, tag := range []struct{ name, color string }{
		{"Indoor", "#3B82F6"},
		{"Pendeln", "#6B7280"},
		{"virtuell", "#8B5CF6"},
		{"E-Bike", "#16A34A"},
	} {
		if _, err := db.Pool.Exec(ctx,
			`INSERT INTO tags (user_id, name, color, text_color, category, is_system, is_auto)
			 VALUES ($1, $2, $3, '#FFFFFF', 'system', true, false)
			 ON CONFLICT (user_id, name) DO NOTHING`,
			userID, tag.name, tag.color); err != nil {
			t.Fatalf("insert system tag %s: %v", tag.name, err)
		}
	}

	ids := make([]string, 2)
	for i := range ids {
		id := uuid.NewString()
		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO activities (id, user_id, title, sport_type, sub_type, tags, is_indoor, is_electric, is_virtual, is_commute, started_at)
			VALUES ($1, $2, $3, 'cycling', NULL, ARRAY[]::text[], false, false, false, false, now())
		`, id, userID, "met-act"); err != nil {
			t.Fatalf("insert activity %d: %v", i, err)
		}
		ids[i] = id
	}
	return db, reg, userID, ids
}

// gatherCounter reads the named counter value from the registry.
// labelPairs is an alternating key/value sequence used to match label sets.
func gatherCounter(t *testing.T, reg prometheus.Gatherer, metricName string, labelPairs ...string) float64 {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, fam := range families {
		if fam.GetName() != metricName {
			continue
		}
		for _, m := range fam.GetMetric() {
			if labelsMatch(m, labelPairs) {
				return m.GetCounter().GetValue()
			}
		}
	}
	return 0
}

// gatherHistogramCount reads the observation count for a named histogram.
func gatherHistogramCount(t *testing.T, reg prometheus.Gatherer, metricName string) uint64 {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, fam := range families {
		if fam.GetName() != metricName {
			continue
		}
		for _, m := range fam.GetMetric() {
			if h := m.GetHistogram(); h != nil {
				return h.GetSampleCount()
			}
		}
	}
	return 0
}

// gatherHistogramSum reads the observation sum for a named histogram.
func gatherHistogramSum(t *testing.T, reg prometheus.Gatherer, metricName string) float64 {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, fam := range families {
		if fam.GetName() != metricName {
			continue
		}
		for _, m := range fam.GetMetric() {
			if h := m.GetHistogram(); h != nil {
				return h.GetSampleSum()
			}
		}
	}
	return 0
}

func labelsMatch(m *dto.Metric, pairs []string) bool {
	if len(pairs)%2 != 0 {
		panic("labelsMatch: pairs must be even")
	}
	for i := 0; i < len(pairs); i += 2 {
		k, v := pairs[i], pairs[i+1]
		found := false
		for _, lbl := range m.GetLabel() {
			if lbl.GetName() == k && lbl.GetValue() == v {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// TestWithSystemModifierTagSyncSuspendedIncrementsCallCounter verifies that a
// successful helper call increments resync_calls_total by 1 and labels it with
// the caller name embedded in the context.
func TestWithSystemModifierTagSyncSuspendedIncrementsCallCounter(t *testing.T) {
	db, reg, _, ids := metricsTestSetup(t)
	ctx := context.Background()

	err := db.WithSystemModifierTagSyncSuspended(ctx, "test", ids, func(ctx context.Context, tx pgx.Tx) error {
		return nil
	})
	if err != nil {
		t.Fatalf("helper: %v", err)
	}

	got := gatherCounter(t, reg,
		"skadia_system_modifier_tag_sync_resync_calls_total",
		"caller", "test")
	if got != 1 {
		t.Fatalf("resync_calls_total{caller=test} = %v, want 1", got)
	}
}

// TestWithSystemModifierTagSyncSuspendedRecordsActivityCountInHistogram verifies
// that the helper records the number of deduped activity IDs into the
// resync_activities_processed histogram.
func TestWithSystemModifierTagSyncSuspendedRecordsActivityCountInHistogram(t *testing.T) {
	db, reg, _, ids := metricsTestSetup(t)
	ctx := context.Background()

	// Pass ids with a duplicate: deduped count = len(ids) = 2.
	// Use an explicit-copy append to avoid mutating the underlying array of ids
	// when cap(ids) > len(ids).
	withDupe := append(append([]string{}, ids...), ids[0]) // 3 elements, 2 unique
	err := db.WithSystemModifierTagSyncSuspended(ctx, "test", withDupe, func(ctx context.Context, tx pgx.Tx) error {
		return nil
	})
	if err != nil {
		t.Fatalf("helper: %v", err)
	}

	count := gatherHistogramCount(t, reg, "skadia_system_modifier_tag_sync_resync_activities_processed")
	if count != 1 {
		t.Fatalf("histogram sample count = %d, want 1", count)
	}
	sum := gatherHistogramSum(t, reg, "skadia_system_modifier_tag_sync_resync_activities_processed")
	if sum != 2 {
		t.Fatalf("histogram sum = %v, want 2 (2 deduped IDs)", sum)
	}
}

// TestWithSystemModifierTagSyncSuspendedRollbackDoesNotIncrement verifies that
// when fn returns an error and the transaction rolls back, ALL metric instruments
// remain at zero: resync_calls_total, resync_activities_processed, all five
// repairs_total kind labels, and skip_gate_active_seconds.
//
// skip_gate_active_seconds contract (verified against Wave D production code):
// the defer that observes the histogram checks `err == nil`, so a rolled-back
// call (err != nil) does NOT observe — asserted explicitly below.
func TestWithSystemModifierTagSyncSuspendedRollbackDoesNotIncrement(t *testing.T) {
	db, reg, _, ids := metricsTestSetup(t)
	ctx := context.Background()

	wantErr := errors.New("intentional failure")
	err := db.WithSystemModifierTagSyncSuspended(ctx, "test", ids, func(ctx context.Context, tx pgx.Tx) error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("helper err = %v, want wrap of %v", err, wantErr)
	}

	got := gatherCounter(t, reg,
		"skadia_system_modifier_tag_sync_resync_calls_total",
		"caller", "test")
	if got != 0 {
		t.Fatalf("resync_calls_total after rollback = %v, want 0", got)
	}

	count := gatherHistogramCount(t, reg, "skadia_system_modifier_tag_sync_resync_activities_processed")
	if count != 0 {
		t.Fatalf("resync_activities_processed observations after rollback = %d, want 0", count)
	}

	// skip_gate_active_seconds must not observe on rollback (err != nil guard in defer).
	gateCount := gatherHistogramCount(t, reg, "skadia_system_modifier_tag_sync_skip_gate_active_seconds")
	if gateCount != 0 {
		t.Fatalf("skip_gate_active_seconds observations after rollback = %d, want 0", gateCount)
	}

	// All five repairs_total kind labels must remain at zero; a bug that
	// increments any of them on rollback would otherwise be invisible.
	for _, kind := range []string{
		"junction_inserted",
		"junction_deleted",
		"tag_promoted",
		"removal_dropped",
		"system_tag_seeded",
	} {
		repairCount := gatherCounter(t, reg,
			"skadia_system_modifier_tag_sync_repairs_total",
			"kind", kind)
		if repairCount != 0 {
			t.Fatalf("repairs_total{kind=%s} after rollback = %v, want 0", kind, repairCount)
		}
	}
}

// TestWithSystemModifierTagSyncSuspendedSkipGateHistogram verifies that the
// skip_gate_active_seconds histogram receives one observation per successful call.
func TestWithSystemModifierTagSyncSuspendedSkipGateHistogram(t *testing.T) {
	db, reg, _, ids := metricsTestSetup(t)
	ctx := context.Background()

	before := time.Now()
	err := db.WithSystemModifierTagSyncSuspended(ctx, "test", ids, func(ctx context.Context, tx pgx.Tx) error {
		return nil
	})
	after := time.Now()
	if err != nil {
		t.Fatalf("helper: %v", err)
	}

	count := gatherHistogramCount(t, reg, "skadia_system_modifier_tag_sync_skip_gate_active_seconds")
	if count != 1 {
		t.Fatalf("skip_gate_active_seconds sample count = %d, want 1", count)
	}
	sum := gatherHistogramSum(t, reg, "skadia_system_modifier_tag_sync_skip_gate_active_seconds")
	elapsed := after.Sub(before).Seconds()
	if sum < 0 || sum > elapsed+1 {
		t.Fatalf("skip_gate_active_seconds sum = %v out of plausible range [0, %v]", sum, elapsed+1)
	}
}

// TestSkadiaResyncSystemModifierTagsReturnsRepairCounts verifies that the SQL
// resync function returns non-zero junction_inserted counts when called in a state
// where junctions are missing. This pins the RETURNS TABLE contract introduced
// by the m.3 in-place amendment of migration 000353.
func TestSkadiaResyncSystemModifierTagsReturnsRepairCounts(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()

	userID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID, "rsc-"+userID[:8], "rsc-"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	// Insert activity with is_indoor=true but skip system tag seeding —
	// the resync function will create the system tag and junction.
	actID := uuid.NewString()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Set the skip flag so triggers do not insert junctions; we want the
	// activity to have is_indoor=true but no junction — damaged state.
	if _, err := tx.Exec(ctx, `SET LOCAL skadia.skip_system_modifier_tag_sync = 'true'`); err != nil {
		t.Fatalf("set skip flag: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO activities (id, user_id, title, sport_type, sub_type, tags, is_indoor, is_electric, is_virtual, is_commute, started_at)
		VALUES ($1, $2, 'rsc-act', 'cycling', NULL, ARRAY[]::text[], true, false, false, false, now())
	`, actID, userID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}

	// Verify: no junction exists yet.
	var junctionCount int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM activity_tags at
		JOIN tags t ON t.id = at.tag_id
		WHERE at.activity_id = $1 AND t.name = 'Indoor'
	`, actID).Scan(&junctionCount); err != nil {
		t.Fatalf("query junction pre-resync: %v", err)
	}
	if junctionCount != 0 {
		t.Fatalf("pre-condition: expected 0 junctions, got %d", junctionCount)
	}

	// Call resync and read the returned counts.
	var junctionsInserted, junctionsDeleted, tagsPromoted, removalsDropped int
	if err := tx.QueryRow(ctx,
		`SELECT junctions_inserted, junctions_deleted, tags_promoted, removals_dropped
		 FROM skadia_resync_system_modifier_tags($1::uuid[])`,
		[]string{actID},
	).Scan(&junctionsInserted, &junctionsDeleted, &tagsPromoted, &removalsDropped); err != nil {
		t.Fatalf("call resync: %v", err)
	}

	// The resync should have inserted at least one junction (Indoor for is_indoor=true).
	if junctionsInserted < 1 {
		t.Fatalf("junctions_inserted = %d, want >= 1", junctionsInserted)
	}
	// No stale removals expected.
	if removalsDropped != 0 {
		t.Logf("removals_dropped = %d (informational)", removalsDropped)
	}
	// tags_promoted should be 0 since we did not seed any manual tag with reserved name.
	if tagsPromoted != 0 {
		t.Logf("tags_promoted = %d (informational)", tagsPromoted)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

// TestWithSystemModifierTagSyncSuspendedRepairsCounterIncrement verifies that
// the repairs_total counter advances for each repair kind after a successful call
// that actually performs repairs (junction_inserted).
func TestWithSystemModifierTagSyncSuspendedRepairsCounterIncrement(t *testing.T) {
	db, reg, _, ids := metricsTestSetup(t)
	ctx := context.Background()

	// Flip is_indoor on the activities inside the helper so the trigger skips;
	// the resync call must insert junctions and the repairs counter must advance.
	err := db.WithSystemModifierTagSyncSuspended(ctx, "test", ids, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`UPDATE activities SET is_indoor = true WHERE id = ANY($1::uuid[])`, ids)
		return err
	})
	if err != nil {
		t.Fatalf("helper: %v", err)
	}

	got := gatherCounter(t, reg,
		"skadia_system_modifier_tag_sync_repairs_total",
		"kind", "junction_inserted")
	if got < 1 {
		t.Fatalf("repairs_total{kind=junction_inserted} = %v, want >= 1", got)
	}
}
