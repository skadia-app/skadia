package analytics

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/event"
)

// reconcilePublishDelay is the pause between consecutive NATS publishes to
// avoid flooding the broker and the ML service on large user bases.
// Override with env var RECONCILE_PUBLISH_DELAY_MS (integer milliseconds).
// Default: 10 ms → ~100 msg/s.
const reconcilePublishDelayDefault = 10 * time.Millisecond

// reconcileMaxPerRunDefault caps the number of invalidations issued in a
// single job tick. Users beyond the cap are processed on the next tick.
// Override with env var RECONCILE_MAX_PER_RUN (integer > 0).
// Default: 1000.
const reconcileMaxPerRunDefault = 1000

func reconcilePublishDelay() time.Duration {
	if raw := os.Getenv("RECONCILE_PUBLISH_DELAY_MS"); raw != "" {
		if ms, err := strconv.Atoi(raw); err == nil && ms >= 0 {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return reconcilePublishDelayDefault
}

func reconcileMaxPerRun() int {
	if raw := os.Getenv("RECONCILE_MAX_PER_RUN"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			return n
		}
	}
	return reconcileMaxPerRunDefault
}

// ReconcileReadiness finds active users (last_login_at within 14 days) whose
// readiness cache entry is missing or older than 24 hours, and publishes an
// analytics invalidate event for each so the ML worker recomputes their score.
//
// To prevent NATS backlog spikes on large deployments, the function:
//   - limits each run to at most reconcileMaxPerRun users (oldest stale first),
//     logging a notice when the cap is reached so callers know to continue on
//     the next tick; and
//   - sleeps reconcilePublishDelay between consecutive publishes, making the
//     effective publish rate ~1/delay (default ~100 msg/s).
//
// Per-user publish failures are logged but do not abort the batch; an aggregate
// error is returned if any user failed.
func ReconcileReadiness(ctx context.Context, pool *pgxpool.Pool, pub event.AnalyticsInvalidator) error {
	delay := reconcilePublishDelay()
	limit := reconcileMaxPerRun()

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
		 LIMIT %d`, limit)

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

		// Rate-limit: sleep between publishes; honour context cancellation.
		if delay > 0 {
			select {
			case <-ctx.Done():
				errs = append(errs, ctx.Err())
				return errors.Join(errs...)
			case <-time.After(delay):
			}
		}
	}
	if err := rows.Err(); err != nil {
		errs = append(errs, fmt.Errorf("rows: %w", err))
	}

	if count == limit {
		slog.Info("reconcile readiness: limited per run, will resume next tick",
			"published", count, "limit", limit)
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
