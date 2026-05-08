package migrations_test

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestMigration000341PatchesHeatmapTupleFilters(t *testing.T) {
	upBytes, err := os.ReadFile("000341_heatmap_tuple_filters.up.sql")
	if err != nil {
		t.Fatalf("read 000341 up.sql: %v", err)
	}
	downBytes, err := os.ReadFile("000341_heatmap_tuple_filters.down.sql")
	if err != nil {
		t.Fatalf("read 000341 down.sql: %v", err)
	}
	upSQL := string(upBytes)
	downSQL := string(downBytes)

	for _, needle := range []string{
		"CREATE OR REPLACE FUNCTION skadia_sport_tuple_filter_matches",
		"position(':' in raw_token)",
		"COALESCE(row_sub_type, '')",
		"heatmap_lines_mvt(integer,integer,integer,json)",
		"heatmap_points_mvt(integer,integer,integer,json)",
		"skadia_sport_tuple_filter_matches(sport_filter, hl.sport_type, hl.sub_type)",
		"skadia_sport_tuple_filter_matches(sport_filter, hp.sport_type, hp.sub_type)",
		"SET search_path = public",
		"could not patch heatmap_lines_mvt",
		"could not patch heatmap_points_mvt",
	} {
		if !strings.Contains(upSQL, needle) {
			t.Fatalf("000341 up migration missing %q", needle)
		}
	}
	for _, needle := range []string{
		"could not unpatch heatmap_lines_mvt",
		"could not unpatch heatmap_points_mvt",
	} {
		if !strings.Contains(downSQL, needle) {
			t.Fatalf("000341 down migration missing %q", needle)
		}
	}
	if !strings.Contains(downSQL, "DROP FUNCTION IF EXISTS skadia_sport_tuple_filter_matches") {
		t.Fatalf("000341 down migration must drop tuple filter helper")
	}
}

func TestMigration000341HeatmapFunctionsContainTupleFilterInDatabase(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	for _, fn := range []string{
		"heatmap_lines_mvt(integer,integer,integer,json)",
		"heatmap_points_mvt(integer,integer,integer,json)",
	} {
		var def string
		if err := pool.QueryRow(ctx, `SELECT pg_get_functiondef($1::regprocedure)`, fn).Scan(&def); err != nil {
			t.Fatalf("read %s definition: %v", fn, err)
		}
		if !strings.Contains(def, "skadia_sport_tuple_filter_matches") {
			t.Fatalf("%s does not call skadia_sport_tuple_filter_matches:\n%s", fn, def)
		}
	}
}
