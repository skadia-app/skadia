package migrations_test

import (
	"context"
	"testing"
)

func TestMigration_StorageMaintenanceModeAllowsDurableFlag(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`INSERT INTO system_flags (key, value, expires_at)
		 VALUES ('maintenance_mode', 'storage migration failed', 'infinity'::timestamptz)
		 ON CONFLICT (key) DO UPDATE
		 SET value = EXCLUDED.value, expires_at = EXCLUDED.expires_at`,
	); err != nil {
		t.Fatalf("maintenance_mode infinity insert failed: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM system_flags WHERE key IN ('maintenance_mode', 'storage_migration_probe')`)
	})

	if _, err := pool.Exec(ctx,
		`INSERT INTO system_flags (key, value, expires_at)
		 VALUES ('storage_migration_probe', 'too long', 'infinity'::timestamptz)`,
	); err == nil {
		t.Fatal("non-maintenance system flag accepted infinity expires_at")
	}
}
