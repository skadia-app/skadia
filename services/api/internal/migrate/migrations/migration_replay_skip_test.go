package migrations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func skipIfMigrationAlreadyApplied(t testing.TB, ctx context.Context, pool *pgxpool.Pool, targetVersion int, reason string) {
	t.Helper()
	var applied bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM schema_migrations
			WHERE version >= $1 AND dirty = false
		)
	`, targetVersion).Scan(&applied); err != nil {
		t.Fatalf("check migration version before historical replay test: %v", err)
	}
	if applied {
		t.Skipf("historical migration replay requires a database before migration %06d: %s", targetVersion, reason)
	}
}
