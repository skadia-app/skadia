package migrations_test

// Integration tests for migration 000333 (map_layers).
//
// Requires TEST_DATABASE_URL to point to a safe Skadia test database.
// Historical replay tests are skipped when that database is already migrated
// past 000333; latest-DB CI still runs the notice/safety coverage below.
//
// Strategy: each test loads the migration SQL from disk and executes it inside
// a transaction that is rolled back on completion, so the test schema is unaffected.
// The idempotent test exercises up → down → up to validate both directions.

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// captureNotices opens a fresh pgx.Conn (not pool) with an OnNotice hook so that
// RAISE NOTICE messages emitted by the migration SQL can be captured. This is
// required because OnNotice must be configured before the connection is established;
// it cannot be retrofitted to an existing pool connection.
func captureNotices(t *testing.T, fn func(conn *pgx.Conn)) string {
	t.Helper()
	dsn := requireSafeTestDSN(t)
	var sb strings.Builder
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("captureNotices: parse config: %v", err)
	}
	cfg.OnNotice = func(_ *pgconn.PgConn, n *pgconn.Notice) {
		sb.WriteString(n.Message + "\n")
	}
	conn, err := pgx.ConnectConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("captureNotices: connect: %v", err)
	}
	defer conn.Close(context.Background())
	fn(conn)
	return sb.String()
}

func TestMigration000333Seeds(t *testing.T) {
	upSQL, err := os.ReadFile("000333_map_layers.up.sql")
	if err != nil {
		t.Fatalf("read up.sql: %v", err)
	}

	pool := migrationTestPool(t)
	ctx := context.Background()
	skipIfMigrationAlreadyApplied(t, ctx, pool, 333, "000333 up.sql creates map_layers tables and must replay against a pre-000333 schema")

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
			t.Logf("rollback: %v", err)
		}
	})

	if _, err := tx.Exec(ctx, string(upSQL)); err != nil {
		t.Fatalf("apply up.sql: %v", err)
	}

	var mapLayers int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM map_layers WHERE is_system = true`).Scan(&mapLayers); err != nil {
		t.Fatalf("query map_layers count: %v", err)
	}
	if mapLayers != 7 {
		t.Errorf("map_layers seed: want 7 system rows, got %d", mapLayers)
	}

	var sysVis int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM system_layer_visibility`).Scan(&sysVis); err != nil {
		t.Fatalf("query system_layer_visibility count: %v", err)
	}
	if sysVis != 14 {
		t.Errorf("system_layer_visibility seed: want 14 rows, got %d", sysVis)
	}
}

func TestMigration000333UpdatedAtTriggers(t *testing.T) {
	upSQL, err := os.ReadFile("000333_map_layers.up.sql")
	if err != nil {
		t.Fatalf("read up.sql: %v", err)
	}

	pool := migrationTestPool(t)
	ctx := context.Background()
	skipIfMigrationAlreadyApplied(t, ctx, pool, 333, "000333 up.sql creates map_layers tables and must replay against a pre-000333 schema")

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
			t.Logf("rollback: %v", err)
		}
	})

	if _, err := tx.Exec(ctx, string(upSQL)); err != nil {
		t.Fatalf("apply up.sql: %v", err)
	}

	for _, table := range []string{"map_layers", "system_layer_visibility", "user_map_layers"} {
		var hasTrigger bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM pg_trigger
				WHERE tgname = 'trg_' || $1 || '_updated_at' AND NOT tgisinternal
			)
		`, table).Scan(&hasTrigger); err != nil {
			t.Fatalf("query trigger for %s: %v", table, err)
		}
		if !hasTrigger {
			t.Errorf("trigger trg_%s_updated_at must exist after migration", table)
		}
	}
}

func TestMigration000333UrlConsistencyConstraint(t *testing.T) {
	upSQL, err := os.ReadFile("000333_map_layers.up.sql")
	if err != nil {
		t.Fatalf("read up.sql: %v", err)
	}

	pool := migrationTestPool(t)
	ctx := context.Background()
	skipIfMigrationAlreadyApplied(t, ctx, pool, 333, "000333 up.sql creates map_layers tables and must replay against a pre-000333 schema")

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
			t.Logf("rollback: %v", err)
		}
	})

	if _, err := tx.Exec(ctx, string(upSQL)); err != nil {
		t.Fatalf("apply up.sql: %v", err)
	}

	// raster_tiles with both url_template AND style_url must be rejected.
	_, err = tx.Exec(ctx, `
		INSERT INTO map_layers (key, role, name, source_kind, url_template, style_url)
		VALUES ('bad-raster', 'base', 'Bad', 'raster_tiles',
			'https://example.com/{z}/{x}/{y}.png', 'https://example.com/style.json')
	`)
	if err == nil {
		t.Error("expected constraint violation for raster_tiles with style_url set, got nil")
	}

	// vector_style without style_url must be rejected.
	_, err = tx.Exec(ctx, `
		INSERT INTO map_layers (key, role, name, source_kind, url_template)
		VALUES ('bad-vector', 'base', 'Bad', 'vector_style', 'https://example.com/{z}/{x}/{y}.pbf')
	`)
	if err == nil {
		t.Error("expected constraint violation for vector_style without style_url, got nil")
	}
}

// TestMigration000333RoundTrip verifies that up → down → up produces the same
// row counts as a single up run (no double-seeding) and that the audit-log
// DELETE in down.sql prevents audit rows from accumulating across re-cycles.
func TestMigration000333RoundTrip(t *testing.T) {
	upSQL, err := os.ReadFile("000333_map_layers.up.sql")
	if err != nil {
		t.Fatalf("read up.sql: %v", err)
	}
	downSQL, err := os.ReadFile("000333_map_layers.down.sql")
	if err != nil {
		t.Fatalf("read down.sql: %v", err)
	}

	pool := migrationTestPool(t)
	ctx := context.Background()
	skipIfMigrationAlreadyApplied(t, ctx, pool, 333, "000333 up/down round-trip must start before map_layers exists")

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin outer tx: %v", err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
			t.Logf("rollback outer tx: %v", err)
		}
	})

	// First up.
	if _, err := tx.Exec(ctx, string(upSQL)); err != nil {
		t.Fatalf("first up.sql: %v", err)
	}

	// Savepoint so we can undo the down migration cleanly within the same tx.
	if _, err := tx.Exec(ctx, "SAVEPOINT before_down"); err != nil {
		t.Fatalf("savepoint: %v", err)
	}

	// Down migration — ignoring RAISE NOTICE (no pgx.Conn with OnNotice here).
	if _, err := tx.Exec(ctx, string(downSQL)); err != nil {
		t.Fatalf("down.sql: %v", err)
	}

	// Second up.
	if _, err := tx.Exec(ctx, string(upSQL)); err != nil {
		t.Fatalf("second up.sql after down: %v", err)
	}

	// map_layers seed must still be exactly 7 (not 14).
	var mapLayers int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM map_layers WHERE is_system = true`).Scan(&mapLayers); err != nil {
		t.Fatalf("query map_layers count after round-trip: %v", err)
	}
	if mapLayers != 7 {
		t.Errorf("map_layers after up→down→up: want 7, got %d (double-seed?)", mapLayers)
	}

	// system_layer_visibility seed must still be exactly 14 (not 28).
	var sysVis int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM system_layer_visibility`).Scan(&sysVis); err != nil {
		t.Fatalf("query system_layer_visibility count after round-trip: %v", err)
	}
	if sysVis != 14 {
		t.Errorf("system_layer_visibility after up→down→up: want 14, got %d (double-seed?)", sysVis)
	}

	// Audit-log must contain exactly 1 row for migration '000333' (down.sql
	// DELETE + second up INSERT = net 1, not 2). Validates F14 fix.
	var auditRows int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM auth_audit_log WHERE details->>'migration' = '000333'`).Scan(&auditRows); err != nil {
		t.Fatalf("query audit_log count after round-trip: %v", err)
	}
	if auditRows != 1 {
		t.Errorf("auth_audit_log for migration '000333' after up→down→up: want 1, got %d", auditRows)
	}
}

func TestMigration000333DownRaisesNotice(t *testing.T) {
	downSQL, err := os.ReadFile("000333_map_layers.down.sql")
	if err != nil {
		t.Fatalf("read down.sql: %v", err)
	}

	// Wrap in a tx that gets rolled back so the DROP statements never persist.
	// RAISE NOTICE fires during statement execution (before commit), so the notice
	// is captured even though the DROPs are reverted on Rollback.
	//
	// History: an earlier version ran downSQL directly via conn.Exec without a tx.
	// When TEST_DATABASE_URL pointed at the production DB, that path silently
	// dropped map_layers / system_layer_visibility / user_map_layers — see the
	// 2026-05-03 incident. The tx wrapper + the safety guard in captureNotices
	// (refuses production DSNs) are the two layers that prevent recurrence.
	noticed := captureNotices(t, func(conn *pgx.Conn) {
		ctx := context.Background()
		tx, err := conn.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			t.Fatalf("begin tx for down.sql: %v", err)
		}
		defer tx.Rollback(ctx) //nolint:errcheck
		if _, err := tx.Exec(ctx, string(downSQL)); err != nil {
			t.Logf("down.sql exec inside tx returned error (non-fatal, NOTICE still captured): %v", err)
		}
	})

	if !strings.Contains(noticed, "Run pg_dump") {
		t.Errorf("down.sql must RAISE NOTICE containing 'Run pg_dump' before dropping user data; got: %q", noticed)
	}
}
