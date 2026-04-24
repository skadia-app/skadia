// services/api/internal/settings/seed_test.go
package settings

import (
	"context"
	"testing"
)

func TestApplySeedsIfMissingEmptyDB(t *testing.T) {
	pool := settingsTestPool(t)
	defer pool.Close()
	ctx := context.Background()

	_, err := pool.Exec(ctx, `DELETE FROM server_settings WHERE key LIKE 'library.default_schema.%'`)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	store := newTestSettingsStore(t, pool)
	defaults := map[string]string{
		"library.default_schema.activity": "{year}/{month}/{day}_{time}_{sport}",
		"library.default_schema.media":    "{year}/{month}/{day}_{title}",
		"library.default_schema.document": "{year}/{category}_{title}",
		"library.default_schema.health":   "{year}/{month}/{day}_{type}",
		"library.default_schema.route":    "{sport}/{title}",
	}

	if err := ApplySeedsIfMissing(ctx, store, defaults); err != nil {
		t.Fatalf("ApplySeedsIfMissing: %v", err)
	}

	var count int
	pool.QueryRow(ctx,
		`SELECT count(*) FROM server_settings WHERE key LIKE 'library.default_schema.%'`).Scan(&count)

	if count != 5 {
		t.Errorf("expected 5 seeded schema rows, got %d", count)
	}
}

func TestApplySeedsIfMissingPreservesExisting(t *testing.T) {
	pool := settingsTestPool(t)
	defer pool.Close()
	ctx := context.Background()

	_, _ = pool.Exec(ctx,
		`INSERT INTO server_settings (key, value, encrypted, updated_at) VALUES ($1, $2, false, NOW())
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`,
		"library.default_schema.activity", "custom/schema/{year}")

	store := newTestSettingsStore(t, pool)
	defaults := map[string]string{
		"library.default_schema.activity": "{year}/{month}/{day}_{time}_{sport}",
	}

	if err := ApplySeedsIfMissing(ctx, store, defaults); err != nil {
		t.Fatalf("apply: %v", err)
	}

	var got string
	pool.QueryRow(ctx,
		`SELECT value FROM server_settings WHERE key = 'library.default_schema.activity'`).Scan(&got)

	if got != "custom/schema/{year}" {
		t.Errorf("existing value was overwritten: got %q, want %q", got, "custom/schema/{year}")
	}
}
