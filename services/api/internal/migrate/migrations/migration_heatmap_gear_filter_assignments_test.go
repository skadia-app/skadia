package migrations_test

import (
	"context"
	"strings"
	"testing"
)

func TestMigration_HeatmapGearFilterUsesActivityGearAssignments(t *testing.T) {
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
		if strings.Contains(def, "FROM activities WHERE gear_id") {
			t.Fatalf("%s still filters gear through removed activities.gear_id column:\n%s", fn, def)
		}
		if !strings.Contains(def, "FROM activity_gear_assignments aga") {
			t.Fatalf("%s does not filter gear through activity_gear_assignments:\n%s", fn, def)
		}
		if !strings.Contains(def, "aga.role_key = 'primary'") {
			t.Fatalf("%s gear filter must use primary activity gear assignments:\n%s", fn, def)
		}
	}
}
