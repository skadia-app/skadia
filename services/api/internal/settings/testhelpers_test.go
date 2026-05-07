package settings

import (
	"context"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

// settingsTestPool opens a pgx pool against TEST_DATABASE_URL.
// Mirrors the pattern from handler/testhelpers_test.go:fusionTestPool.
func settingsTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testdb.RequireSafeDSN(t)
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	return pool
}

// newTestSettingsStore returns a SettingsStore wired to a test pool.
// Uses StorageBackend "local" and no encryption key (tests only set
// non-encrypted keys).
func newTestSettingsStore(t *testing.T, pool *pgxpool.Pool) *SettingsStore {
	t.Helper()
	return NewSettingsStore(&database.DB{Pool: pool}, "", "local")
}

// setSetting writes a single key directly to server_settings, bypassing
// store.Put validation (tests need to simulate out-of-band changes like
// another replica writing).
func setSetting(t *testing.T, pool *pgxpool.Pool, key, value string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO server_settings (key, value, encrypted, updated_at) VALUES ($1, $2, false, NOW())
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
		key, value)
	if err != nil {
		t.Fatalf("setSetting %q: %v", key, err)
	}
}
