// services/api/internal/settings/seed.go
package settings

import (
	"context"
	"fmt"
	"log/slog"
)

// ApplySeedsIfMissing ensures the given defaults are present in server_settings.
// Missing keys are inserted with ON CONFLICT DO NOTHING — existing values are
// never overwritten (admin may have customized them).
//
// The caller (main.go) constructs `defaults` from shadowfs.NewSchemaRenderer()
// — this function intentionally takes a plain map to avoid an import cycle
// (shadowfs already imports settings via legacy_hotfolder.go:14).
//
// Idempotent. Safe to call on every startup.
func ApplySeedsIfMissing(ctx context.Context, store *SettingsStore, defaults map[string]string) error {
	for key, value := range defaults {
		res, err := store.db.Pool.Exec(ctx,
			`INSERT INTO server_settings (key, value, encrypted, updated_at)
			 VALUES ($1, $2, false, NOW())
			 ON CONFLICT (key) DO NOTHING`,
			key, value)
		if err != nil {
			return fmt.Errorf("seed %q: %w", key, err)
		}
		if res.RowsAffected() > 0 {
			slog.Info("settings: seeded key", "key", key, "value", value)
		}
	}

	if err := store.loadCache(ctx); err != nil {
		return fmt.Errorf("seed: reload cache: %w", err)
	}
	return nil
}
