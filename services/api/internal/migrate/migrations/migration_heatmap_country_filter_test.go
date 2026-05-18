package migrations_test

import (
	"os"
	"strings"
	"testing"
)

func TestMigration_HeatmapCountryFilter_PatchesMVTFunctions(t *testing.T) {
	upBytes, err := os.ReadFile("000374_heatmap_country_filter.up.sql")
	if err != nil {
		t.Fatalf("read up.sql: %v", err)
	}
	downBytes, err := os.ReadFile("000374_heatmap_country_filter.down.sql")
	if err != nil {
		t.Fatalf("read down.sql: %v", err)
	}
	upSQL := string(upBytes)
	downSQL := string(downBytes)

	for _, needle := range []string{
		"pg_get_functiondef('heatmap_lines_mvt(integer,integer,integer,json)'::regprocedure)",
		"pg_get_functiondef('heatmap_points_mvt(integer,integer,integer,json)'::regprocedure)",
		"query_params->>'country_codes'",
		"visited_country_codes && country_codes_arr",
		"skadia_heatmap_country_filter_matches(query_params, hl.activity_id)",
		"skadia_heatmap_country_filter_matches(query_params, hp.activity_id)",
	} {
		if !strings.Contains(upSQL, needle) {
			t.Fatalf("up.sql missing %q", needle)
		}
	}
	if !strings.Contains(downSQL, "skadia_heatmap_country_filter_matches(query_params, hl.activity_id)") ||
		!strings.Contains(downSQL, "skadia_heatmap_country_filter_matches(query_params, hp.activity_id)") {
		t.Fatalf("down.sql must remove the country filter patch")
	}
}
