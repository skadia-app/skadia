package analytics

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/event"
)

// ReconcileReadiness finds active users (last_login_at within 14 days) whose
// readiness cache entry is missing or older than 24 hours, and publishes an
// analytics invalidate event for each so the ML worker recomputes their score.
//
// To prevent NATS backlog spikes on large deployments, the function:
//   - limits each run to at most cfg.MaxPerRun users (oldest stale first),
//     logging a notice when the cap is reached so callers know to continue on
//     the next tick; and
//   - sleeps cfg.PublishDelay between consecutive publishes (not after the last
//     one), making the effective publish rate ~1/delay (default ~100 msg/s).
//
// The DB cursor is closed before the rate-limit sleep loop begins — no pool
// connection is held open during the inter-publish sleeps.
//
// Per-user publish failures are logged but do not abort the batch; an aggregate
// error is returned if any user failed.
func ReconcileReadiness(ctx context.Context, pool *pgxpool.Pool, pub event.AnalyticsInvalidator) error {
	cfg := LoadReconcileConfig()

	// Select the stalest users first so progress is deterministic across ticks.
	query := fmt.Sprintf(`
		SELECT u.id::text
		  FROM users u
		 WHERE u.last_login_at > NOW() - interval '14 days'
		   AND NOT EXISTS (
		       SELECT 1 FROM analytics_cache c
		        WHERE c.user_id = u.id
		          AND c.cache_key = 'readiness'
		          AND c.computed_at > NOW() - interval '24 hours'
		   )
		 ORDER BY u.last_login_at ASC
		 LIMIT %d`, cfg.MaxPerRun)

	// R2-M1 fix: buffer all IDs into a slice and close the cursor before the
	// rate-limit sleep loop. Holding a pool connection open for the entire
	// inter-publish sleep duration (up to MaxPerRun × delay) wastes a
	// connection for the full batch duration.
	rows, err := pool.Query(ctx, query)
	if err != nil {
		return fmt.Errorf("reconcile readiness: query: %w", err)
	}

	var userIDs []string
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			rows.Close()
			return fmt.Errorf("reconcile readiness: scan: %w", err)
		}
		userIDs = append(userIDs, userID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("reconcile readiness: rows: %w", err)
	}
	rows.Close() // release the connection before any sleep

	// Publish loop with inter-publish rate limiting.
	var errs []error
	var count int
	for i, uid := range userIDs {
		if err := pub.PublishAnalyticsInvalidate(uid); err != nil {
			slog.Error("reconcile readiness: publish failed", "user_id", uid, "error", err)
			errs = append(errs, fmt.Errorf("user %s: %w", uid, err))
			// Do not sleep after a failure; proceed to the next user.
			continue
		}
		count++

		// R2-L1 fix: skip the sleep after the last publish — no next message
		// to pace, and the caller's ticker is already the real long-interval gate.
		if cfg.PublishDelay > 0 && i < len(userIDs)-1 {
			select {
			case <-ctx.Done():
				errs = append(errs, ctx.Err())
				return errors.Join(errs...)
			case <-time.After(cfg.PublishDelay):
			}
		}
	}

	if count == cfg.MaxPerRun {
		slog.Info("reconcile readiness: limited per run, will resume next tick",
			"published", count, "limit", cfg.MaxPerRun)
	} else if len(errs) > 0 {
		slog.Error("reconcile readiness: completed with errors", "published", count, "failures", len(errs))
	} else {
		slog.Info("reconcile readiness: completed", "published", count)
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
