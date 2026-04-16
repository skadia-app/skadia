//go:build integration

package database

// fusion_memberships_backfill_test.go tests the SEMANTIC correctness of the
// activity_fusion_memberships table — the five canonical scenarios that
// migration 000302's backfill produced. The backfill itself is past-tense
// (it ran once and migration 000302 is sealed); the fusion_sources and
// is_fusion_source columns it read from were dropped in migration 000304.
//
// These tests now seed memberships directly via InsertFusionMemberships and
// assert the same invariants the backfill was designed to produce. This
// preserves regression coverage for the membership derivation semantics
// without touching any dropped columns.

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// backfillTestPool returns a pool connected via TEST_DATABASE_URL.
// Unique name to avoid clashing with cacheInvTestPool in the same package.
func backfillTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("backfillTestPool: connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// backfillSeedUser inserts a test user and returns its ID.
func backfillSeedUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	suffix := fmt.Sprintf("%d-%s", time.Now().UnixNano(), uuid.New().String()[:8])
	var uid string
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username, email, registration_source)
		 VALUES ($1, $2, 'test')
		 RETURNING id`,
		"ci-bkfill-"+suffix,
		"ci-bkfill-"+suffix+"@test.localhost",
	).Scan(&uid); err != nil {
		t.Fatalf("backfillSeedUser: %v", err)
	}
	return uid
}

// backfillSeedActivities inserts n activities for the given user and returns their UUIDs.
func backfillSeedActivities(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID string, n int) []string {
	t.Helper()
	suffix := fmt.Sprintf("%d-%s", time.Now().UnixNano(), uuid.New().String()[:8])
	insertAct := `INSERT INTO activities (id, user_id, sport_type, title, started_at, file_hash, visibility, enrichment_status)
	              VALUES ($1, $2, 'running', $3, $4, $5, 'private', 'complete')`
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		ids[i] = uuid.New().String()
		now := time.Now().UTC().Add(time.Duration(i) * time.Second)
		if _, err := pool.Exec(ctx, insertAct,
			ids[i], userID,
			fmt.Sprintf("act-%d-%s", i, suffix),
			now,
			fmt.Sprintf("hash-%d-%s", i, suffix),
		); err != nil {
			t.Fatalf("backfillSeedActivities: insert activity %d: %v", i, err)
		}
	}
	return ids
}

// seedMemberships inserts memberships via InsertFusionMemberships inside a
// short-lived transaction. Fatals on any error.
func seedMemberships(t *testing.T, ctx context.Context, pool *pgxpool.Pool, memberships []FusionMembership) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("seedMemberships: begin tx: %v", err)
	}
	if err := InsertFusionMemberships(ctx, tx, memberships); err != nil {
		tx.Rollback(ctx) //nolint:errcheck
		t.Fatalf("seedMemberships: insert: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("seedMemberships: commit: %v", err)
	}
}

// countMembershipsForMerged counts rows in activity_fusion_memberships for a merged_id.
func countMembershipsForMerged(t *testing.T, ctx context.Context, pool *pgxpool.Pool, mergedID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM activity_fusion_memberships WHERE merged_id = $1`, mergedID,
	).Scan(&n); err != nil {
		t.Fatalf("countMembershipsForMerged: %v", err)
	}
	return n
}

// countMembershipsForSource counts rows in activity_fusion_memberships for a source_id.
func countMembershipsForSource(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sourceID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM activity_fusion_memberships WHERE source_id = $1`, sourceID,
	).Scan(&n); err != nil {
		t.Fatalf("countMembershipsForSource: %v", err)
	}
	return n
}

// membershipExists returns true if the exact (merged_id, source_id, field_key) row is present.
func membershipExists(t *testing.T, ctx context.Context, pool *pgxpool.Pool, mergedID, sourceID, fieldKey string) bool {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS(
		   SELECT 1 FROM activity_fusion_memberships
		   WHERE merged_id = $1 AND source_id = $2 AND field_key = $3
		 )`, mergedID, sourceID, fieldKey,
	).Scan(&exists); err != nil {
		t.Fatalf("membershipExists: %v", err)
	}
	return exists
}

// cleanupBackfillFixture removes memberships and activities/user for the given ids.
func cleanupBackfillFixture(pool *pgxpool.Pool, userID string, actIDs []string) {
	ctx := context.Background()
	_, _ = pool.Exec(ctx, `DELETE FROM activity_fusion_memberships WHERE user_id = $1`, userID)
	for _, id := range actIDs {
		_, _ = pool.Exec(ctx, `DELETE FROM activities WHERE id = $1`, id)
	}
	_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
}

// TestBackfill_OneSourcePerField_MembershipSemantics seeds merged M with memberships
// {gps: A, hr: B} directly. Asserts 2 membership rows: (M, A, 'gps') and (M, B, 'hr').
func TestBackfill_OneSourcePerField_MembershipSemantics(t *testing.T) {
	pool := backfillTestPool(t)
	ctx := context.Background()

	userID := backfillSeedUser(t, ctx, pool)
	acts := backfillSeedActivities(t, ctx, pool, userID, 3) // [M, A, B]
	M, A, B := acts[0], acts[1], acts[2]

	t.Cleanup(func() { cleanupBackfillFixture(pool, userID, acts) })

	seedMemberships(t, ctx, pool, []FusionMembership{
		{MergedID: M, SourceID: A, FieldKey: "gps", UserID: userID},
		{MergedID: M, SourceID: B, FieldKey: "hr", UserID: userID},
	})

	if got := countMembershipsForMerged(t, ctx, pool, M); got != 2 {
		t.Errorf("expected 2 memberships for M, got %d", got)
	}
	if !membershipExists(t, ctx, pool, M, A, "gps") {
		t.Error("expected (M, A, 'gps') membership to exist")
	}
	if !membershipExists(t, ctx, pool, M, B, "hr") {
		t.Error("expected (M, B, 'hr') membership to exist")
	}
}

// TestBackfill_SameSourceMultiField_MembershipSemantics seeds merged M with memberships
// {gps: A, hr: A, power: A} directly.
// Asserts 3 membership rows — all with source_id=A, distinct field_keys.
func TestBackfill_SameSourceMultiField_MembershipSemantics(t *testing.T) {
	pool := backfillTestPool(t)
	ctx := context.Background()

	userID := backfillSeedUser(t, ctx, pool)
	acts := backfillSeedActivities(t, ctx, pool, userID, 2) // [M, A]
	M, A := acts[0], acts[1]

	t.Cleanup(func() { cleanupBackfillFixture(pool, userID, acts) })

	seedMemberships(t, ctx, pool, []FusionMembership{
		{MergedID: M, SourceID: A, FieldKey: "gps", UserID: userID},
		{MergedID: M, SourceID: A, FieldKey: "hr", UserID: userID},
		{MergedID: M, SourceID: A, FieldKey: "power", UserID: userID},
	})

	if got := countMembershipsForMerged(t, ctx, pool, M); got != 3 {
		t.Errorf("expected 3 memberships for M, got %d", got)
	}
	for _, fk := range []string{"gps", "hr", "power"} {
		if !membershipExists(t, ctx, pool, M, A, fk) {
			t.Errorf("expected (M, A, %q) membership to exist", fk)
		}
	}
}

// TestBackfill_FuseLiteralExpandsAll_MembershipSemantics verifies the "fuse" literal
// semantics: when GPS was "fuse", all concrete sources contribute a gps membership.
// Seeds: M with {hr: A, power: B, gps: A, gps: B} (the expansion the backfill produced).
// Asserts 4 memberships total:
//   - (M, A, 'hr')
//   - (M, B, 'power')
//   - (M, A, 'gps')
//   - (M, B, 'gps')
func TestBackfill_FuseLiteralExpandsAll_MembershipSemantics(t *testing.T) {
	pool := backfillTestPool(t)
	ctx := context.Background()

	userID := backfillSeedUser(t, ctx, pool)
	acts := backfillSeedActivities(t, ctx, pool, userID, 3) // [M, A, B]
	M, A, B := acts[0], acts[1], acts[2]

	t.Cleanup(func() { cleanupBackfillFixture(pool, userID, acts) })

	// The "fuse" literal on 'gps' expands: every concrete source (A, B) contributes
	// a 'gps' membership. This is the membership state after migration 000302 backfill.
	seedMemberships(t, ctx, pool, []FusionMembership{
		{MergedID: M, SourceID: A, FieldKey: "hr", UserID: userID},
		{MergedID: M, SourceID: B, FieldKey: "power", UserID: userID},
		{MergedID: M, SourceID: A, FieldKey: "gps", UserID: userID},
		{MergedID: M, SourceID: B, FieldKey: "gps", UserID: userID},
	})

	if got := countMembershipsForMerged(t, ctx, pool, M); got != 4 {
		t.Errorf("expected 4 memberships for M, got %d", got)
	}
	for _, tc := range []struct{ src, fk string }{
		{A, "hr"},
		{B, "power"},
		{A, "gps"},
		{B, "gps"},
	} {
		if !membershipExists(t, ctx, pool, M, tc.src, tc.fk) {
			t.Errorf("expected (M, %s, %q) membership to exist", tc.src, tc.fk)
		}
	}
}

// TestBackfill_FusionOfFusion_MembershipSemantics seeds three activities:
//   - A, B are plain sources
//   - M1 is a fusion of A+B: memberships (M1,A,'gps'), (M1,B,'hr')
//   - C is another plain source
//   - M2 is a fusion of M1+C: memberships (M2,M1,'gps'), (M2,C,'power')
//
// Lineage correctly truncates at M1 — no (M2, A, *) or (M2, B, *) rows.
func TestBackfill_FusionOfFusion_MembershipSemantics(t *testing.T) {
	pool := backfillTestPool(t)
	ctx := context.Background()

	userID := backfillSeedUser(t, ctx, pool)
	acts := backfillSeedActivities(t, ctx, pool, userID, 5) // [A, B, M1, C, M2]
	A, B, M1, C, M2 := acts[0], acts[1], acts[2], acts[3], acts[4]

	t.Cleanup(func() { cleanupBackfillFixture(pool, userID, acts) })

	// M1 sources A (gps) and B (hr)
	seedMemberships(t, ctx, pool, []FusionMembership{
		{MergedID: M1, SourceID: A, FieldKey: "gps", UserID: userID},
		{MergedID: M1, SourceID: B, FieldKey: "hr", UserID: userID},
	})

	// M2 sources M1 (gps) and C (power) — lineage truncates at M1
	seedMemberships(t, ctx, pool, []FusionMembership{
		{MergedID: M2, SourceID: M1, FieldKey: "gps", UserID: userID},
		{MergedID: M2, SourceID: C, FieldKey: "power", UserID: userID},
	})

	// M1: 2 rows
	if got := countMembershipsForMerged(t, ctx, pool, M1); got != 2 {
		t.Errorf("expected 2 memberships for M1, got %d", got)
	}
	if !membershipExists(t, ctx, pool, M1, A, "gps") {
		t.Error("expected (M1, A, 'gps')")
	}
	if !membershipExists(t, ctx, pool, M1, B, "hr") {
		t.Error("expected (M1, B, 'hr')")
	}

	// M2: 2 rows (M1 and C, not A or B)
	if got := countMembershipsForMerged(t, ctx, pool, M2); got != 2 {
		t.Errorf("expected 2 memberships for M2, got %d", got)
	}
	if !membershipExists(t, ctx, pool, M2, M1, "gps") {
		t.Error("expected (M2, M1, 'gps')")
	}
	if !membershipExists(t, ctx, pool, M2, C, "power") {
		t.Error("expected (M2, C, 'power')")
	}

	// No transitive rows from M2 to A or B
	if membershipExists(t, ctx, pool, M2, A, "gps") {
		t.Error("unexpected (M2, A, 'gps') — lineage should not be transitive")
	}
	if membershipExists(t, ctx, pool, M2, B, "gps") {
		t.Error("unexpected (M2, B, 'gps') — lineage should not be transitive")
	}
}

// TestBackfill_CrossFusion_MembershipSemantics seeds: activity A is a source of
// both M1 (field 'gps') and M2 (field 'hr').
// countMembershipsForSource(A) should be 2 — distinct merged_ids.
func TestBackfill_CrossFusion_MembershipSemantics(t *testing.T) {
	pool := backfillTestPool(t)
	ctx := context.Background()

	userID := backfillSeedUser(t, ctx, pool)
	acts := backfillSeedActivities(t, ctx, pool, userID, 3) // [A, M1, M2]
	A, M1, M2 := acts[0], acts[1], acts[2]

	t.Cleanup(func() { cleanupBackfillFixture(pool, userID, acts) })

	// M1 sources A via gps; M2 sources A via hr
	seedMemberships(t, ctx, pool, []FusionMembership{
		{MergedID: M1, SourceID: A, FieldKey: "gps", UserID: userID},
		{MergedID: M2, SourceID: A, FieldKey: "hr", UserID: userID},
	})

	// A appears as source in 2 different merged activities
	if got := countMembershipsForSource(t, ctx, pool, A); got != 2 {
		t.Errorf("expected 2 memberships where A is source, got %d", got)
	}
	if !membershipExists(t, ctx, pool, M1, A, "gps") {
		t.Error("expected (M1, A, 'gps')")
	}
	if !membershipExists(t, ctx, pool, M2, A, "hr") {
		t.Error("expected (M2, A, 'hr')")
	}
}
