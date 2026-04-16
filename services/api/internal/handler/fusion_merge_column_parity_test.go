package handler

// fusion_merge_column_parity_test.go — CI guard for Phase 4b TEMP column-parity.
//
// Verifies that CREATE TEMP TABLE (LIKE ... INCLUDING ...) replicates the exact
// column set of every Phase 4b related table. Guards against schema drift where a
// new column is added to the live table but not picked up by the TEMP pattern
// (which would silently drop the column from the merged activity's copy).
//
// Requires TEST_DATABASE_URL; the whole suite is skipped when the env var is unset.

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// TestFusionRelatedTablesCopyAllColumns verifies that CREATE TEMP TABLE (LIKE ...
// INCLUDING DEFAULTS INCLUDING CONSTRAINTS INCLUDING GENERATED) replicates the
// column set of every Phase 4b related table. Schema drift (a new column silently
// absent from the TEMP copy) would cause INSERT INTO target SELECT * FROM temp to
// fail or silently omit data.
func TestFusionRelatedTablesCopyAllColumns(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()

	// Acquire a single connection: TEMP tables are session-scoped.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal("acquire connection:", err)
	}
	defer conn.Release()

	tables := []string{
		"activity_photos",
		"activity_geo_tags",
		"activity_privacy_sections",
		"activity_tag_removals",
		"activity_developer_apps",
		"best_effort_exclusions",
	}

	for _, table := range tables {
		table := table // capture
		t.Run(table, func(t *testing.T) {
			// Count columns in the live (public schema) table.
			var sourceCount int
			if err := conn.QueryRow(ctx,
				`SELECT COUNT(*) FROM information_schema.columns
				 WHERE table_schema = 'public' AND table_name = $1`, table,
			).Scan(&sourceCount); err != nil {
				t.Fatal("count source columns:", err)
			}
			if sourceCount == 0 {
				t.Fatalf("table %s not found in public schema — check the table name", table)
			}

			suffix := fmt.Sprintf("%016x", rand.Uint64())
			tempName := fmt.Sprintf("parity_%s_%s", strings.ReplaceAll(table, "_", ""), suffix)

			tx, err := conn.Begin(ctx)
			if err != nil {
				t.Fatal("begin tx:", err)
			}

			if _, err := tx.Exec(ctx, fmt.Sprintf(
				`CREATE TEMP TABLE %s
				 (LIKE %s INCLUDING DEFAULTS INCLUDING CONSTRAINTS INCLUDING GENERATED)
				 ON COMMIT DROP`,
				tempName, table,
			)); err != nil {
				tx.Rollback(ctx) //nolint:errcheck
				t.Fatal("create temp table:", err)
			}

			// Count columns in the temp table (pg_temp schema).
			// information_schema.columns shows pg_temp_* schema names for session-local temps.
			var tempCount int
			if err := tx.QueryRow(ctx,
				`SELECT COUNT(*) FROM information_schema.columns
				 WHERE table_name = $1
				   AND table_schema LIKE 'pg_temp%'`, tempName,
			).Scan(&tempCount); err != nil {
				tx.Rollback(ctx) //nolint:errcheck
				t.Fatal("count temp columns:", err)
			}

			// ON COMMIT DROP removes the temp; commit to trigger it.
			if err := tx.Commit(ctx); err != nil {
				t.Fatal("commit:", err)
			}

			if sourceCount != tempCount {
				t.Errorf("%s: public schema has %d columns, TEMP has %d — schema drift risk; update Phase 4b copy logic",
					table, sourceCount, tempCount)
			} else {
				t.Logf("%s: %d columns — parity OK", table, sourceCount)
			}
		})
	}
}
