package analytics

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/event"
)

// ReconcileReadiness finds active users (last_login_at within 14 days) whose
// readiness cache entry is missing or older than 24 hours, and publishes an
// analytics invalidate event for each so the ML worker recomputes their score.
// Per-user publish failures are logged but do not abort the batch; an aggregate
// error is returned if any user failed.
func ReconcileReadiness(ctx context.Context, pool *pgxpool.Pool, pub event.AnalyticsInvalidator) error {
	const query = `
		SELECT u.id::text FROM users u
		 WHERE u.last_login_at > NOW() - interval '14 days'
		   AND NOT EXISTS (
		       SELECT 1 FROM analytics_cache c
		        WHERE c.user_id = u.id
		          AND c.cache_key = 'readiness'
		          AND c.computed_at > NOW() - interval '24 hours'
		   )`

	rows, err := pool.Query(ctx, query)
	if err != nil {
		return fmt.Errorf("reconcile readiness: query: %w", err)
	}
	defer rows.Close()

	var errs []error
	var count int
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			errs = append(errs, fmt.Errorf("scan: %w", err))
			continue
		}
		if err := pub.PublishAnalyticsInvalidate(userID); err != nil {
			slog.Error("reconcile readiness: publish failed", "user_id", userID, "error", err)
			errs = append(errs, fmt.Errorf("user %s: %w", userID, err))
			continue
		}
		count++
	}
	if err := rows.Err(); err != nil {
		errs = append(errs, fmt.Errorf("rows: %w", err))
	}

	if len(errs) > 0 {
		slog.Error("reconcile readiness: completed with errors", "published", count, "failures", len(errs))
		return errors.Join(errs...)
	}
	slog.Info("reconcile readiness: completed", "published", count)
	return nil
}
