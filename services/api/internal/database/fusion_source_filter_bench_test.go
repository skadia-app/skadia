//go:build integration

// fusion_source_filter_bench_test.go compares membership-absence query shapes
// (NOT EXISTS vs LEFT JOIN IS NULL) at 1k/5k/10k fixture sizes. Variant A
// (legacy is_fusion_source column check) was removed after migration 000304
// dropped the column — the historical Appendix A result ("NOT EXISTS chosen,
// within 10% of column-check") remains in the design spec for context.

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

// benchFusionPool connects to TEST_DATABASE_URL and returns a pool.
// Uses b.Skip (not b.Fatal) when the env var is absent so the bench is a no-op
// in environments that don't have the integration DB.
func benchFusionPool(b *testing.B) *pgxpool.Pool {
	b.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		b.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		b.Fatalf("benchFusionPool: connect: %v", err)
	}
	b.Cleanup(pool.Close)
	return pool
}

// benchSeedUser inserts a fresh user and returns its UUID string.
func benchSeedUser(b *testing.B, ctx context.Context, pool *pgxpool.Pool) string {
	b.Helper()
	suffix := fmt.Sprintf("%d-%s", time.Now().UnixNano(), uuid.New().String()[:8])
	var uid string
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username, email, registration_source)
		 VALUES ($1, $2, 'test')
		 RETURNING id`,
		"ci-bench-"+suffix,
		"ci-bench-"+suffix+"@test.localhost",
	).Scan(&uid); err != nil {
		b.Fatalf("benchSeedUser: %v", err)
	}
	return uid
}

// fusionFixture holds the seeded state for one benchmark run.
type fusionFixture struct {
	userID string
	pool   *pgxpool.Pool
}

// cleanup removes all seeded rows for this fixture.
func (f *fusionFixture) cleanup() {
	ctx := context.Background()
	_, _ = f.pool.Exec(ctx,
		`DELETE FROM activity_fusion_memberships WHERE user_id = $1`, f.userID)
	_, _ = f.pool.Exec(ctx,
		`DELETE FROM activities WHERE user_id = $1`, f.userID)
	_, _ = f.pool.Exec(ctx,
		`DELETE FROM users WHERE id = $1`, f.userID)
}

// seedFusionFixture inserts n activities for a fresh user.
// ~10% of them (ceil(n/10)) are fusion sources: each gets a matching row in
// activity_fusion_memberships. For each source activity Src, a distinct
// "merged" activity Mrg (also belonging to the same user) is chosen as
// merged_id — the merged_id activities are NOT themselves sources.
//
// Consistency guarantee: every source activity has exactly one membership row,
// and no membership row points at a non-source activity.
// Variants B/C (memberships-based absence queries) must therefore return the
// same set of non-source activity IDs.
func seedFusionFixture(b *testing.B, ctx context.Context, pool *pgxpool.Pool, n int) fusionFixture {
	b.Helper()

	userID := benchSeedUser(b, ctx, pool)
	fix := fusionFixture{userID: userID, pool: pool}

	suffix := fmt.Sprintf("%d-%s", time.Now().UnixNano(), uuid.New().String()[:8])
	insertAct := `INSERT INTO activities
	              (id, user_id, sport_type, title, started_at, file_hash, visibility, enrichment_status)
	              VALUES ($1, $2, 'running', $3, $4, $5, 'private', 'complete')`

	ids := make([]string, n)
	for i := 0; i < n; i++ {
		ids[i] = uuid.New().String()
	}

	numSources := (n + 9) / 10 // ceil(n/10) ≈ 10%

	// We need numSources "merged" activities as FK targets for memberships.
	// We'll reuse the LAST numSources IDs as merged activities (not sources),
	// and the FIRST numSources IDs as source activities.
	// The remaining IDs in the middle are plain non-source activities.
	// This keeps the fixture self-contained within the n allocated IDs.
	// Guard: n must be > 2*numSources.
	if n <= 2*numSources {
		b.Fatalf("seedFusionFixture: n=%d too small for numSources=%d", n, numSources)
	}

	t0 := time.Now().UTC()
	for i, id := range ids {
		ts := t0.Add(time.Duration(i) * time.Second)
		if _, err := pool.Exec(ctx, insertAct,
			id, userID,
			fmt.Sprintf("bench-act-%d-%s", i, suffix),
			ts,
			fmt.Sprintf("bench-hash-%d-%s", i, suffix),
		); err != nil {
			fix.cleanup()
			b.Fatalf("seedFusionFixture: insert activity %d: %v", i, err)
		}
	}

	// Insert memberships: source IDs[0..numSources-1], merged IDs[n-numSources..n-1]
	insertMembership := `INSERT INTO activity_fusion_memberships
	                     (merged_id, source_id, field_key, user_id)
	                     VALUES ($1, $2, 'gps', $3)`
	for i := 0; i < numSources; i++ {
		srcID := ids[i]
		mrgedID := ids[n-numSources+i]
		if _, err := pool.Exec(ctx, insertMembership, mrgedID, srcID, userID); err != nil {
			fix.cleanup()
			b.Fatalf("seedFusionFixture: insert membership %d: %v", i, err)
		}
	}

	// Sanity-check: Variants B and C must agree on the count of non-source activities.
	var countB, countC int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM activities a
		 WHERE a.user_id = $1
		   AND NOT EXISTS (
		       SELECT 1 FROM activity_fusion_memberships m WHERE m.source_id = a.id
		   )`, userID,
	).Scan(&countB); err != nil {
		fix.cleanup()
		b.Fatalf("seedFusionFixture: countB: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM activities a
		 LEFT JOIN activity_fusion_memberships m ON m.source_id = a.id
		 WHERE a.user_id = $1 AND m.source_id IS NULL`, userID,
	).Scan(&countC); err != nil {
		fix.cleanup()
		b.Fatalf("seedFusionFixture: countC: %v", err)
	}

	if countB != countC {
		fix.cleanup()
		b.Fatalf("seedFusionFixture: inconsistent row counts: B=%d C=%d (n=%d, numSources=%d)",
			countB, countC, n, numSources)
	}

	return fix
}

// drainRows reads all rows in the result set to prevent the query-prepare
// optimizer from masking the real cost of stream-fetching.
func drainRows(b *testing.B, rows interface{ Next() bool; Err() error; Close() }) {
	b.Helper()
	defer rows.Close()
	var scanned int
	for rows.Next() {
		scanned++
	}
	if err := rows.Err(); err != nil {
		b.Fatalf("drainRows: %v", err)
	}
	_ = scanned
}

// runVariantB executes the NOT EXISTS sub-select query and drains all rows.
func runVariantB(b *testing.B, ctx context.Context, pool *pgxpool.Pool, userID string) {
	b.Helper()
	rows, err := pool.Query(ctx,
		`SELECT a.id FROM activities a
		 WHERE a.user_id = $1
		   AND NOT EXISTS (
		       SELECT 1 FROM activity_fusion_memberships m WHERE m.source_id = a.id
		   )`,
		userID,
	)
	if err != nil {
		b.Fatalf("variantB: %v", err)
	}
	drainRows(b, rows)
}

// runVariantC executes the LEFT JOIN IS NULL query and drains all rows.
func runVariantC(b *testing.B, ctx context.Context, pool *pgxpool.Pool, userID string) {
	b.Helper()
	rows, err := pool.Query(ctx,
		`SELECT a.id FROM activities a
		 LEFT JOIN activity_fusion_memberships m ON m.source_id = a.id
		 WHERE a.user_id = $1 AND m.source_id IS NULL`,
		userID,
	)
	if err != nil {
		b.Fatalf("variantC: %v", err)
	}
	drainRows(b, rows)
}

// --- 1k benchmarks ---

func BenchmarkFusionSourceFilter_NotExists_1k(b *testing.B) {
	ctx := context.Background()
	pool := benchFusionPool(b)
	b.StopTimer()
	fix := seedFusionFixture(b, ctx, pool, 1000)
	b.Cleanup(fix.cleanup)
	b.ResetTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		runVariantB(b, ctx, pool, fix.userID)
	}
}

func BenchmarkFusionSourceFilter_LeftJoin_1k(b *testing.B) {
	ctx := context.Background()
	pool := benchFusionPool(b)
	b.StopTimer()
	fix := seedFusionFixture(b, ctx, pool, 1000)
	b.Cleanup(fix.cleanup)
	b.ResetTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		runVariantC(b, ctx, pool, fix.userID)
	}
}

// --- 5k benchmarks ---

func BenchmarkFusionSourceFilter_NotExists_5k(b *testing.B) {
	ctx := context.Background()
	pool := benchFusionPool(b)
	b.StopTimer()
	fix := seedFusionFixture(b, ctx, pool, 5000)
	b.Cleanup(fix.cleanup)
	b.ResetTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		runVariantB(b, ctx, pool, fix.userID)
	}
}

func BenchmarkFusionSourceFilter_LeftJoin_5k(b *testing.B) {
	ctx := context.Background()
	pool := benchFusionPool(b)
	b.StopTimer()
	fix := seedFusionFixture(b, ctx, pool, 5000)
	b.Cleanup(fix.cleanup)
	b.ResetTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		runVariantC(b, ctx, pool, fix.userID)
	}
}

// --- 10k benchmarks (gate) ---

func BenchmarkFusionSourceFilter_NotExists_10k(b *testing.B) {
	ctx := context.Background()
	pool := benchFusionPool(b)
	b.StopTimer()
	fix := seedFusionFixture(b, ctx, pool, 10000)
	b.Cleanup(fix.cleanup)
	b.ResetTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		runVariantB(b, ctx, pool, fix.userID)
	}
}

func BenchmarkFusionSourceFilter_LeftJoin_10k(b *testing.B) {
	ctx := context.Background()
	pool := benchFusionPool(b)
	b.StopTimer()
	fix := seedFusionFixture(b, ctx, pool, 10000)
	b.Cleanup(fix.cleanup)
	b.ResetTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		runVariantC(b, ctx, pool, fix.userID)
	}
}
