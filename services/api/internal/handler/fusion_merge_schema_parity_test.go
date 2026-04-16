package handler

// fusion_merge_schema_parity_test.go — CI guard for hasOwnUUIDPK switch.
//
// hasOwnUUIDPK (in fusion_merge.go) hard-codes a set of related tables whose
// primary key is their own UUID "id" column — as opposed to tables whose PK is
// the foreign-key activity_id. copyRelatedTablesUnion uses this flag to decide
// whether to re-roll a new UUID for copied rows.
//
// If a schema change adds a new related table, or changes the PK structure of
// an existing one, without updating the switch, the copy logic silently does
// the wrong thing (either skipping a re-roll or re-rolling when it should not).
//
// This test catches drift: it queries the live schema to determine whether
// each related table has an "id" column that is part of the primary key, then
// compares the result against hasOwnUUIDPK.
//
// Requires TEST_DATABASE_URL; skipped when the env var is unset.

import (
	"context"
	"testing"
)

// TestHasOwnUUIDPK_MatchesRealSchema verifies that the hasOwnUUIDPK switch
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
			// Query whether the table has an "id" column that is part of its PK.
			var idIsPK bool
			err := pool.QueryRow(ctx, `
				SELECT EXISTS(
				  SELECT 1
				  FROM information_schema.columns c
				  WHERE c.table_schema = 'public'
				    AND c.table_name   = $1
				    AND c.column_name  = 'id'
				    AND EXISTS(
				      SELECT 1
				      FROM information_schema.table_constraints tc
				      JOIN information_schema.key_column_usage kcu
				        ON kcu.constraint_name = tc.constraint_name
				       AND kcu.table_schema    = tc.table_schema
				      WHERE tc.table_schema    = 'public'
				        AND tc.table_name      = $1
				        AND tc.constraint_type = 'PRIMARY KEY'
				        AND kcu.column_name    = 'id'
				    )
				)
			`, table).Scan(&idIsPK)
			if err != nil {
				t.Fatalf("schema query for %s: %v", table, err)
			}

			expected := hasOwnUUIDPK(table)
			if idIsPK != expected {
				t.Errorf(
					"%s: hasOwnUUIDPK=%v but schema says id-is-PK=%v — "+
						"switch is out of sync with the real schema; update hasOwnUUIDPK in fusion_merge.go",
					table, expected, idIsPK,
				)
			} else {
				t.Logf("%s: hasOwnUUIDPK=%v — matches schema", table, expected)
			}
		})
	}
}
