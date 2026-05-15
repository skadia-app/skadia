package migrations_test

import (
	"context"
	"testing"
)

func TestMigration_OSMReadonlyGrants(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	for _, table := range []string{"osm_pois", "osm_ways"} {
		var allowed bool
		if err := pool.QueryRow(ctx, `SELECT has_table_privilege('skadia_readonly', $1, 'SELECT')`, table).Scan(&allowed); err != nil {
			t.Fatalf("query readonly privilege for %s: %v", table, err)
		}
		if !allowed {
			t.Fatalf("skadia_readonly lacks SELECT on %s", table)
		}
	}
}
