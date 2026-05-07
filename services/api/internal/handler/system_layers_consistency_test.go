package handler_test

import (
	"context"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

// systemLayersTestPool returns a pgxpool connected to TEST_DATABASE_URL or skips.
// This pool is used for lightweight consistency checks that only read the seed data.
func systemLayersTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testdb.RequireSafeDSN(t)
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect to test DB: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// TestSystemLayersCodeMatchesDB verifies bidirectional equality between
// model.SystemLayers (code source-of-truth) and the system_layer_visibility seed
// inserted by migration 000333.
//
// Fails when:
//   - A key is added to model.SystemLayers but not to the migration seed.
//   - A key is inserted by the migration but not declared in model.SystemLayers.
//   - The migration is rolled back or the DB is missing (test is skipped).
func TestSystemLayersCodeMatchesDB(t *testing.T) {
	pool := systemLayersTestPool(t)
	ctx := context.Background()

	rows, err := pool.Query(ctx, `SELECT layer_key FROM system_layer_visibility`)
	if err != nil {
		t.Fatalf("query system_layer_visibility: %v", err)
	}
	defer rows.Close()

	dbKeys := make(map[string]struct{})
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			t.Fatalf("scan layer_key: %v", err)
		}
		dbKeys[k] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}

	codeKeys := model.SystemLayerKeySet()

	// Every DB key must have a code constant.
	for k := range dbKeys {
		if _, ok := codeKeys[k]; !ok {
			t.Errorf("DB key %q has no code constant in model.SystemLayers — add it to system_layers.go", k)
		}
	}
	// Every code constant must have a DB seed row.
	for k := range codeKeys {
		if _, ok := dbKeys[k]; !ok {
			t.Errorf("code constant %q has no DB row in system_layer_visibility — add it to migration 000333 seed", k)
		}
	}

	if len(dbKeys) != len(model.SystemLayers) {
		t.Errorf("count mismatch: DB has %d rows, model.SystemLayers has %d entries",
			len(dbKeys), len(model.SystemLayers))
	}
}
