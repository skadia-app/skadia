//go:build integration

package database

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

// The three INSERT statements from migration 000302's backfill block,
// verbatim except that BEGIN/COMMIT are omitted (caller manages the tx).
const backfillSQL = `
INSERT INTO activity_fusion_memberships (merged_id, source_id, field_key, user_id)
SELECT
    a.id AS merged_id,
    (kv.value #>> '{}')::uuid AS source_id,
    kv.key AS field_key,
    a.user_id AS user_id
FROM activities a,
     LATERAL jsonb_each(a.fusion_sources) AS kv
WHERE a.fusion_sources IS NOT NULL
  AND kv.value::text != '"fuse"'
  AND kv.value #>> '{}' ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
ON CONFLICT DO NOTHING;

INSERT INTO activity_fusion_memberships (merged_id, source_id, field_key, user_id)
SELECT DISTINCT
    fuse_a.id AS merged_id,
    other_src::uuid AS source_id,
    'gps' AS field_key,
    fuse_a.user_id AS user_id
FROM activities fuse_a,
     LATERAL jsonb_each(fuse_a.fusion_sources) AS fuse_kv,
     LATERAL (
         SELECT kv2.value #>> '{}' AS other_src
         FROM jsonb_each(fuse_a.fusion_sources) AS kv2
         WHERE kv2.value::text != '"fuse"'
           AND kv2.value #>> '{}' ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
     ) others
WHERE fuse_a.fusion_sources IS NOT NULL
  AND fuse_kv.key = 'gps'
  AND fuse_kv.value::text = '"fuse"'
ON CONFLICT DO NOTHING;

INSERT INTO activity_fusion_memberships (merged_id, source_id, field_key, user_id)
SELECT
    merged.id AS merged_id,
    child.id AS source_id,
    'gps' AS field_key,
    merged.user_id AS user_id
FROM activities merged
JOIN activities child ON child.parent_id = merged.id
WHERE merged.fusion_sources IS NOT NULL
  AND (merged.fusion_sources->>'gps') = 'fuse'
ON CONFLICT DO NOTHING;
`

// runBackfill runs the three INSERT statements from the 000302 backfill block
// inside an existing connection (acquired from pool) that already holds the seeded rows.
// The backfill SQL is run directly via pool because the seeded rows were committed
// (or are visible in the same connection's implicit transaction).
// We use pool.Exec here; callers manage cleanup via t.Cleanup.
func runBackfillOnPool(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	// pgx pool.Exec supports multiple statements separated by semicolons.
	if _, err := pool.Exec(ctx, backfillSQL); err != nil {
		t.Fatalf("runBackfill: %v", err)
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

// TestBackfill_OneSourcePerField seeds: merged M with fusion_sources = {gps: A, hr: B}.
// Expects 2 membership rows: (M, A, 'gps') and (M, B, 'hr').
func TestBackfill_OneSourcePerField(t *testing.T) {
	pool := backfillTestPool(t)
	ctx := context.Background()

	userID := backfillSeedUser(t, ctx, pool)
	acts := backfillSeedActivities(t, ctx, pool, userID, 3) // [M, A, B]
	M, A, B := acts[0], acts[1], acts[2]

	t.Cleanup(func() { cleanupBackfillFixture(pool, userID, acts) })

	fusionSources := fmt.Sprintf(`{"gps": "%s", "hr": "%s"}`, A, B)
	if _, err := pool.Exec(ctx,
		`UPDATE activities SET fusion_sources = $1::jsonb WHERE id = $2`,
		fusionSources, M,
	); err != nil {
		t.Fatalf("set fusion_sources: %v", err)
	}

	runBackfillOnPool(t, ctx, pool)

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

// TestBackfill_SameSourceMultiField seeds: merged M with fusion_sources = {gps: A, hr: A, power: A}.
// Expects 3 membership rows — all with source_id=A, distinct field_keys.
func TestBackfill_SameSourceMultiField(t *testing.T) {
	pool := backfillTestPool(t)
	ctx := context.Background()

	userID := backfillSeedUser(t, ctx, pool)
	acts := backfillSeedActivities(t, ctx, pool, userID, 2) // [M, A]
	M, A := acts[0], acts[1]

	t.Cleanup(func() { cleanupBackfillFixture(pool, userID, acts) })

	fusionSources := fmt.Sprintf(`{"gps": "%s", "hr": "%s", "power": "%s"}`, A, A, A)
	if _, err := pool.Exec(ctx,
		`UPDATE activities SET fusion_sources = $1::jsonb WHERE id = $2`,
		fusionSources, M,
	); err != nil {
		t.Fatalf("set fusion_sources: %v", err)
	}

	runBackfillOnPool(t, ctx, pool)

	if got := countMembershipsForMerged(t, ctx, pool, M); got != 3 {
		t.Errorf("expected 3 memberships for M, got %d", got)
	}
	for _, fk := range []string{"gps", "hr", "power"} {
		if !membershipExists(t, ctx, pool, M, A, fk) {
			t.Errorf("expected (M, A, %q) membership to exist", fk)
		}
	}
}

// TestBackfill_FuseLiteralExpandsAll seeds: merged M with fusion_sources = {gps: "fuse", hr: A, power: B}.
// The "fuse" literal on 'gps' triggers INSERT #2: one row per OTHER concrete source_id under 'gps'.
// Expected 4 memberships total:
//   - (M, A, 'hr')   from INSERT #1
//   - (M, B, 'power') from INSERT #1
//   - (M, A, 'gps')  from INSERT #2 (expansion)
//   - (M, B, 'gps')  from INSERT #2 (expansion)
func TestBackfill_FuseLiteralExpandsAll(t *testing.T) {
	pool := backfillTestPool(t)
	ctx := context.Background()

	userID := backfillSeedUser(t, ctx, pool)
	acts := backfillSeedActivities(t, ctx, pool, userID, 3) // [M, A, B]
	M, A, B := acts[0], acts[1], acts[2]

	t.Cleanup(func() { cleanupBackfillFixture(pool, userID, acts) })

	fusionSources := fmt.Sprintf(`{"gps": "fuse", "hr": "%s", "power": "%s"}`, A, B)
	if _, err := pool.Exec(ctx,
		`UPDATE activities SET fusion_sources = $1::jsonb WHERE id = $2`,
		fusionSources, M,
	); err != nil {
		t.Fatalf("set fusion_sources: %v", err)
	}

	runBackfillOnPool(t, ctx, pool)

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

// TestBackfill_FusionOfFusion seeds three activities:
//   - A, B are plain sources
//   - M1 is a fusion of A+B (fusion_sources = {gps: A, hr: B})
//   - C is another plain source
//   - M2 is a fusion of M1+C (fusion_sources = {gps: M1, power: C})
//
// After backfill:
//   - M1 gets 2 rows: (M1,A,'gps'), (M1,B,'hr')
//   - M2 gets 2 rows: (M2,M1,'gps'), (M2,C,'power')
//
// Lineage correctly truncates at M1 — no (M2, A, *) or (M2, B, *) rows.
func TestBackfill_FusionOfFusion(t *testing.T) {
	pool := backfillTestPool(t)
	ctx := context.Background()

	userID := backfillSeedUser(t, ctx, pool)
	acts := backfillSeedActivities(t, ctx, pool, userID, 5) // [A, B, M1, C, M2]
	A, B, M1, C, M2 := acts[0], acts[1], acts[2], acts[3], acts[4]

	t.Cleanup(func() { cleanupBackfillFixture(pool, userID, acts) })

	// M1 sources A (gps) and B (hr)
	fsM1 := fmt.Sprintf(`{"gps": "%s", "hr": "%s"}`, A, B)
	if _, err := pool.Exec(ctx,
		`UPDATE activities SET fusion_sources = $1::jsonb WHERE id = $2`, fsM1, M1,
	); err != nil {
		t.Fatalf("set fusion_sources M1: %v", err)
	}

	// M2 sources M1 (gps) and C (power)
	fsM2 := fmt.Sprintf(`{"gps": "%s", "power": "%s"}`, M1, C)
	if _, err := pool.Exec(ctx,
		`UPDATE activities SET fusion_sources = $1::jsonb WHERE id = $2`, fsM2, M2,
	); err != nil {
		t.Fatalf("set fusion_sources M2: %v", err)
	}

	runBackfillOnPool(t, ctx, pool)

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

// TestBackfill_CrossFusion seeds: activity A is a source of both M1 (field 'gps') and M2 (field 'hr').
// After backfill, countMembershipsForSource(A) should be 2 — distinct merged_ids.
func TestBackfill_CrossFusion(t *testing.T) {
	pool := backfillTestPool(t)
	ctx := context.Background()

	userID := backfillSeedUser(t, ctx, pool)
	acts := backfillSeedActivities(t, ctx, pool, userID, 3) // [A, M1, M2]
	A, M1, M2 := acts[0], acts[1], acts[2]

	t.Cleanup(func() { cleanupBackfillFixture(pool, userID, acts) })

	// M1 sources A via gps
	fsM1 := fmt.Sprintf(`{"gps": "%s"}`, A)
	if _, err := pool.Exec(ctx,
		`UPDATE activities SET fusion_sources = $1::jsonb WHERE id = $2`, fsM1, M1,
	); err != nil {
		t.Fatalf("set fusion_sources M1: %v", err)
	}

	// M2 sources A via hr
	fsM2 := fmt.Sprintf(`{"hr": "%s"}`, A)
	if _, err := pool.Exec(ctx,
		`UPDATE activities SET fusion_sources = $1::jsonb WHERE id = $2`, fsM2, M2,
	); err != nil {
		t.Fatalf("set fusion_sources M2: %v", err)
	}

	runBackfillOnPool(t, ctx, pool)

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
