package migrations_test

// GIN index performance test for migration 000355.
//
// Verifies that a filter on applicable_sport_predicates using the @> operator
// triggers a GIN index scan rather than a sequential scan, confirming the
// jsonb_path_ops operator class is compatible with the query pattern used by
// Phase 4 handler code.

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestGINIndexUsedForApplicableSportPredicatesFilter inserts 1000 gear rows
// with varied applicable_sport_predicates values, then runs EXPLAIN on a @>
// filter and asserts a GIN index scan is chosen.
func TestGINIndexUsedForApplicableSportPredicatesFilter(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
			t.Logf("rollback: %v", err)
		}
	})

	userID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source, is_admin)
		VALUES ($1, $2, $3, true, 'test', false)
	`, userID, "gin_"+userID[:8], "gin_"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	// Insert 1000 gear rows with varied predicates so the planner has enough
	// rows to consider an index scan.
	sportTypes := []string{"cycling", "running", "swimming", "hiking", "skiing"}
	for i := 0; i < 1000; i++ {
		sport := sportTypes[i%len(sportTypes)]
		var predicates string
		if i%3 == 0 {
			predicates = `[{"sport_type":"` + sport + `"}]`
		} else if i%3 == 1 {
			predicates = `[]`
		} else {
			predicates = `[{"sport_type":"running"},{"sport_type":"hiking"}]`
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO gear (user_id, name, type, scope, applicable_sport_predicates)
			VALUES ($1, $2, 'other', 'other', $3::jsonb)
		`, userID, "gin_gear_"+uuid.NewString()[:8], predicates); err != nil {
			t.Fatalf("insert gear %d: %v", i, err)
		}
	}

	// Force statistics refresh so the planner sees the row count.
	if _, err := tx.Exec(ctx, `ANALYZE gear`); err != nil {
		t.Logf("ANALYZE skipped (no-op in transaction is expected): %v", err)
	}

	// EXPLAIN the @> query; the plan must mention the GIN index.
	rows, err := tx.Query(ctx, `
		EXPLAIN SELECT id FROM gear
		WHERE applicable_sport_predicates @> '[{"sport_type":"cycling"}]'::jsonb
	`)
	if err != nil {
		t.Fatalf("EXPLAIN: %v", err)
	}
	defer rows.Close()

	var planLines []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan EXPLAIN row: %v", err)
		}
		planLines = append(planLines, line)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}

	planText := strings.Join(planLines, "\n")
	t.Logf("query plan:\n%s", planText)

	// The planner must choose the GIN index, not a sequential scan.
	// We accept either a Bitmap Index Scan or an Index Scan on the GIN index.
	hasGINIndexScan := strings.Contains(planText, "gear_applicable_sport_predicates_gin")
	if !hasGINIndexScan {
		t.Errorf("expected GIN index scan on gear_applicable_sport_predicates_gin, got:\n%s", planText)
	}
}
