package migrations_test

// Safety guard for migration integration tests. Refuses to run when
// TEST_DATABASE_URL points at the production database, so a buggy test like
// the historical migration_000333_test.go::TestMigration000333DownRaisesNotice
// (which executed down.sql via conn.Exec without a transaction) cannot drop
// real schema again.
//
// The guard rejects three configurations:
//   1. TEST_DATABASE_URL unset → t.Skip (existing behavior; CI without DB)
//   2. TEST_DATABASE_URL parsed DB name == "skadia"  → t.Fatal
//   3. TEST_DATABASE_URL byte-equal to DATABASE_URL    → t.Fatal
//
// Use a separate database (default name skadia_test). See .env.example.

import (
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

// requireSafeTestDSN returns TEST_DATABASE_URL after asserting it does not
// reference the production database. Skips the test if unset.
func requireSafeTestDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping migration integration test")
	}

	if prod := os.Getenv("DATABASE_URL"); prod != "" && dsn == prod {
		t.Fatalf("TEST_DATABASE_URL must not equal DATABASE_URL (production). " +
			"Use a separate database such as skadia_test — see .env.example.")
	}

	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("TEST_DATABASE_URL is not a valid Postgres DSN: %v", err)
	}
	if cfg.Database == "skadia" {
		t.Fatalf("TEST_DATABASE_URL points at production database 'skadia'. " +
			"Migration tests must use a dedicated DB (default: skadia_test). " +
			"See .env.example for the recommended DSN.")
	}

	return dsn
}
