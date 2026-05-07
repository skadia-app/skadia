// Package testdb provides the single test entry point for opening the
// per-worktree isolated test database.
package testdb

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RequireSafeDSN returns TEST_DATABASE_URL after asserting that it points at a
// dedicated test database. It skips when TEST_DATABASE_URL is unset.
func RequireSafeDSN(t testing.TB) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; run via ./scripts/test_db.sh run -- go test, or set it manually")
	}
	if err := ValidateDSN(dsn, os.Getenv("DATABASE_URL")); err != nil {
		t.Fatalf("%v", err)
	}
	return dsn
}

// ValidateDSN rejects production, shared, or ambiguous database names.
// DATABASE_URL may equal TEST_DATABASE_URL only when the parsed database name is
// a per-worktree skadia_test_<slug> database, which is how scripts/test_db.sh
// run isolates app code.
func ValidateDSN(testDSN, databaseURL string) error {
	cfg, err := pgx.ParseConfig(testDSN)
	if err != nil {
		return fmt.Errorf("TEST_DATABASE_URL is not a valid Postgres DSN: %w", err)
	}
	if databaseURL != "" && databaseURL != testDSN {
		return fmt.Errorf("DATABASE_URL must be unset or equal TEST_DATABASE_URL during DB-backed tests")
	}
	if cfg.Database == "skadia" {
		return fmt.Errorf("TEST_DATABASE_URL points at production database 'skadia'; use a dedicated skadia_test database")
	}
	if !isTestDatabaseName(cfg.Database) {
		return fmt.Errorf("TEST_DATABASE_URL must point at a skadia_test_<12-hex-slug> database, got %q", cfg.Database)
	}
	return nil
}

func isTestDatabaseName(name string) bool {
	const prefix = "skadia_test_"
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	slug := strings.TrimPrefix(name, prefix)
	if len(slug) != 12 {
		return false
	}
	for _, r := range slug {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// Open returns a connected pool to the per-worktree test database or skips the
// test if TEST_DATABASE_URL is unset.
func Open(t testing.TB) *pgxpool.Pool {
	t.Helper()
	dsn := RequireSafeDSN(t)
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("testdb.Open: pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}
