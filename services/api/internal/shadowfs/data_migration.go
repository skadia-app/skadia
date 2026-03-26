package shadowfs

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CanonicalizeParentTypes fixes legacy parent_type values.
// Idempotent — safe to run on every startup.
func CanonicalizeParentTypes(ctx context.Context, pool *pgxpool.Pool) error {
	tag, err := pool.Exec(ctx,
		`UPDATE user_files SET parent_type = 'photo' WHERE parent_type = 'activity_photo'`)
	if err != nil {
		return fmt.Errorf("canonicalize parent_type: %w", err)
	}
	if tag.RowsAffected() > 0 {
		slog.Info("canonicalized parent_type values", "rows", tag.RowsAffected())
	}
	return nil
}
