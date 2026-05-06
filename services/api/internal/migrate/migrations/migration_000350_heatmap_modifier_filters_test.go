package migrations_test

import (
	"os"
	"strings"
	"testing"
)

func TestMigration000350PatchesHeatmapModifierFilters(t *testing.T) {
	upBytes, err := os.ReadFile("000350_heatmap_modifier_filters.up.sql")
	if err != nil {
		t.Fatalf("read 000350 up.sql: %v", err)
	}
	downBytes, err := os.ReadFile("000350_heatmap_modifier_filters.down.sql")
	if err != nil {
		t.Fatalf("read 000350 down.sql: %v", err)
	}
	upSQL := string(upBytes)
	downSQL := string(downBytes)

	for _, needle := range []string{
		"CREATE OR REPLACE FUNCTION skadia_heatmap_modifier_filter_matches",
		"SET search_path = public",
		"query_params->>'is_electric'",
		"query_params->>'is_commute'",
		"activity_id UUID",
		"skadia_sport_tuple_filter_matches(sport_filter, hl.sport_type, hl.sub_type)",
		"skadia_sport_tuple_filter_matches(sport_filter, hp.sport_type, hp.sub_type)",
		"skadia_heatmap_modifier_filter_matches(query_params, hl.activity_id)",
		"skadia_heatmap_modifier_filter_matches(query_params, hp.activity_id)",
		"could not patch heatmap_lines_mvt",
		"could not patch heatmap_points_mvt",
	} {
		if !strings.Contains(upSQL, needle) {
			t.Fatalf("000350 up migration missing %q", needle)
		}
	}
	for _, needle := range []string{
		"could not unpatch heatmap_lines_mvt",
		"could not unpatch heatmap_points_mvt",
		"DROP FUNCTION IF EXISTS skadia_heatmap_modifier_filter_matches",
	} {
		if !strings.Contains(downSQL, needle) {
			t.Fatalf("000350 down migration missing %q", needle)
		}
	}
}
