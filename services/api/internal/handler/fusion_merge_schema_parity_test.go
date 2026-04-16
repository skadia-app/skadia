package handler

// fusion_merge_schema_parity_test.go — CI guard for relatedTableHasOwnUUIDPK switch.
//
// relatedTableHasOwnUUIDPK (in fusion_merge.go) hard-codes a set of related tables whose
// own UUID "id" column must be re-rolled when a row is copied — to avoid duplicate-key
// violations on the UNIQUE or PRIMARY-KEY constraint covering "id". Tables whose PK
// is only the foreign-key activity_id (no own id column at all, or an "id" that
// is neither PK nor UNIQUE) do not need re-rolling.
//
// Note: the detection is UNIQUE-or-PK, not PK-only. activity_developer_apps has
// compound PK (activity_id, application_id) plus a separate UNIQUE index on id —
// copied rows would collide on id without a re-roll.
//
// If a schema change adds a new related table, or changes the uniqueness of id
// on an existing one, without updating the switch, the copy logic silently does
// the wrong thing (either skipping a re-roll or re-rolling when it should not).
//
// Requires TEST_DATABASE_URL; skipped when the env var is unset.

import (
	"context"
	"testing"
)

// TestHasOwnUUIDPK_MatchesRealSchema verifies that the relatedTableHasOwnUUIDPK switch
// agrees with the live database schema for every Phase 4b related table.
func TestHasOwnUUIDPK_MatchesRealSchema(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()

	// All six related tables handled by copyRelatedTablesUnion.
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
			// Query whether the table has an "id" column that participates in
			// ANY unique index (PK index, UNIQUE constraint, or standalone UNIQUE INDEX) —
			// all three cases require re-rolling on copy to avoid duplicate-key
			// violations. pg_index catches UNIQUE-INDEX-without-constraint which
			// information_schema.table_constraints misses (e.g. activity_developer_apps
			// has a CREATE UNIQUE INDEX on id alongside a compound PK).
			var idNeedsReroll bool
			err := pool.QueryRow(ctx, `
				SELECT EXISTS(
				  SELECT 1
				  FROM pg_index i
				  JOIN pg_attribute a
				    ON a.attrelid = i.indrelid
				   AND a.attnum   = ANY(i.indkey)
				  WHERE i.indrelid = ($1 || '')::regclass
				    AND i.indisunique = TRUE
				    AND a.attname = 'id'
				)
			`, table).Scan(&idNeedsReroll)
			if err != nil {
				t.Fatalf("schema query for %s: %v", table, err)
			}

			expected := relatedTableHasOwnUUIDPK(table)
			if idNeedsReroll != expected {
				t.Errorf(
					"%s: relatedTableHasOwnUUIDPK=%v but schema says id-needs-reroll=%v "+
						"(id participates in PK or UNIQUE) — switch out of sync; update relatedTableHasOwnUUIDPK in fusion_merge.go",
					table, expected, idNeedsReroll,
				)
			} else {
				t.Logf("%s: relatedTableHasOwnUUIDPK=%v — matches schema", table, expected)
			}
		})
	}
}
