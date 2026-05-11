// CLI tool: re-enrich lift candidates for all eligible activities.
//
// Usage:
//
//	lift_recompute_all --user-id <uuid>    re-enriches one user; also runs
//	                                       scoped ack-backfill for that user
//	lift_recompute_all --all-users         re-enriches all users with EnableLiftDetection=true
//	lift_recompute_all --ack-backfill-only sets lifts_acknowledged_at=COALESCE(updated_at,
//	                                       started_at, NOW()) for activities whose
//	                                       heuristic candidates are all resolved;
//	                                       combinable with --user-id for a scoped run
//	lift_recompute_all --dry-run           list what would be processed without mutating
//	                                       any state (exit 0 always)
//
// Soft-rejects (rejected_by_user=true) survive re-enrichment: enrichStreamLifts
// filters on NOT rejected_by_user, so they are never deleted.
//
// Gear-stats strategy:
// RunStreamEnrichment is called with SkipGearRefresh=true so that the
// per-activity gear-stats refresh inside recomputeLiftStatsTx is suppressed.
// A single gear-usage refresh per affected gear runs after all activities are
// processed. This avoids N redundant per-activity refresh calls while still
// keeping gear totals, component usage, chain-rotation usage, and usage rules
// consistent.
//
// Privacy:
// By default, user/activity IDs are hashed in log output. Use --verbose for
// raw IDs (debug-only; may expose PII to operator logs).
package main

import (
	"context"
	"flag"
	"fmt"
	"hash/fnv"
	"log/slog"
	"os"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/enrichment"
)

// runConfig holds the parsed command-line options for run(). It is separate
// from flag.FlagSet so tests can construct it directly without parsing args.
type runConfig struct {
	userID      string // empty = all-users mode
	allUsers    bool
	ackOnly     bool
	dryRun      bool
	maxParallel int
	verbose     bool
	dbURL       string
}

func main() {
	userIDFlag := flag.String("user-id", "", "re-enrich a single user (UUID)")
	allUsers := flag.Bool("all-users", false, "re-enrich all users with EnableLiftDetection=true")
	ackOnly := flag.Bool("ack-backfill-only", false, "only run the acknowledged-at backfill UPDATE, no re-enrichment")
	dryRun := flag.Bool("dry-run", false, "list what would be processed without mutating any state")
	maxParallel := flag.Int("max-parallel", 1, "max concurrent activity enrichments per user (1-8)")
	verbose := flag.Bool("verbose", false, "log raw user/activity IDs (debug-only; may expose PII to operator logs)")
	dbURL := flag.String("db-url", os.Getenv("DATABASE_URL"), "PostgreSQL connection string")
	flag.Parse()

	if *dbURL == "" {
		fmt.Fprintln(os.Stderr, "need --db-url or DATABASE_URL")
		os.Exit(2)
	}
	if !*ackOnly && !*dryRun && *userIDFlag == "" && !*allUsers {
		fmt.Fprintln(os.Stderr, "need --user-id, --all-users, --ack-backfill-only, or --dry-run")
		flag.Usage()
		os.Exit(2)
	}
	if *userIDFlag != "" && *allUsers {
		fmt.Fprintln(os.Stderr, "--user-id and --all-users are mutually exclusive")
		os.Exit(2)
	}

	cfg := runConfig{
		userID:      *userIDFlag,
		allUsers:    *allUsers,
		ackOnly:     *ackOnly,
		dryRun:      *dryRun,
		maxParallel: *maxParallel,
		verbose:     *verbose,
		dbURL:       *dbURL,
	}

	ctx := context.Background()
	code := run(ctx, cfg)
	if code != 0 {
		os.Exit(code)
	}
}

// run is the testable core of the CLI. It returns 0 on success, 1 on
// recoverable errors (some activities failed), and 2 on startup/config errors.
func run(ctx context.Context, cfg runConfig) int {
	if cfg.maxParallel < 1 {
		cfg.maxParallel = 1
	}
	if cfg.maxParallel > 8 {
		cfg.maxParallel = 8
	}

	logID := func(id string) string {
		if cfg.verbose {
			return id
		}
		return hashID(id)
	}

	db, err := database.New(ctx, cfg.dbURL)
	if err != nil {
		slog.Error("connect to database", "error", err)
		return 1
	}
	defer db.Pool.Close()

	// --ack-backfill-only: run ack for one user or all users, then exit.
	if cfg.ackOnly {
		var userID *string
		if cfg.userID != "" {
			userID = &cfg.userID
		}
		n, err := runAckBackfill(ctx, db.Pool, userID)
		if err != nil {
			slog.Error("ack backfill failed", "error", err)
			return 1
		}
		slog.Info("ack backfill complete", "updated", n)
		return 0
	}

	var userIDs []string
	if cfg.userID != "" {
		userIDs = []string{cfg.userID}
	} else {
		ids, err := listUsersWithLiftDetection(ctx, db.Pool)
		if err != nil {
			slog.Error("list users with lift detection", "error", err)
			return 1
		}
		userIDs = ids
		slog.Info("found users with lift detection enabled", "count", len(userIDs))
	}

	// --dry-run: log what would be processed and exit cleanly.
	if cfg.dryRun {
		for _, uid := range userIDs {
			acts, err := listLiftEligibleActivities(ctx, db.Pool, uid, 0, 1000)
			if err != nil {
				slog.Warn("dry-run: list activities failed", "user_hash", logID(uid), "error", err)
				continue
			}
			slog.Info("dry-run: would process user",
				"user_hash", logID(uid),
				"activity_count", len(acts),
			)
		}
		slog.Info("dry-run: no mutations performed")
		return 0
	}

	var (
		totalEnriched int
		totalErrors   int
		totalDeleted  int
		// affectedGears collects gear IDs across all users. Protected by mu
		// when --max-parallel > 1; the single-threaded path uses it directly.
		mu            sync.Mutex
		affectedGears = make(map[string]struct{})
	)

	for _, userID := range userIDs {
		// Per-user advisory lock (two-argument form, distinct namespace from
		// the per-activity lock) prevents two concurrent CLI runs from
		// double-processing the same user. A second process that cannot
		// acquire the lock logs and skips rather than blocking.
		userLocked, userLockConn, err := tryAcquireUserLock(ctx, db.Pool, userID)
		if err != nil {
			slog.Warn("per-user lock acquire failed",
				"user_hash", logID(userID),
				"error", err,
			)
			totalErrors++
			continue
		}
		if !userLocked {
			slog.Info("skipping user — concurrent CLI run in progress",
				"user_hash", logID(userID),
			)
			userLockConn.Release()
			continue
		}

		// Process activities in 500-row pages to avoid loading the full
		// corpus into memory when a user has thousands of lift-sport activities.
		offset := 0
		const pageSize = 500
		userEnriched := 0
		userDeleted := 0
		userErrors := 0
		// Per-user gear map: reset each user iteration so gear IDs from other
		// users don't persist across the loop.
		userGears := make(map[string]struct{})

		for {
			page, err := listLiftEligibleActivities(ctx, db.Pool, userID, offset, pageSize)
			if err != nil {
				slog.Error("list activities",
					"user_hash", logID(userID),
					"offset", offset,
					"error", err,
				)
				totalErrors++
				break
			}
			if len(page) == 0 {
				break
			}
			offset += len(page)

			// Process page activities, optionally in parallel.
			type result struct {
				deleted int
				gearIDs []string
				err     error
				actID   string
			}
			results := make([]result, len(page))

			if cfg.maxParallel <= 1 {
				for i, act := range page {
					deleted, gearIDs, err := processActivity(ctx, db, act, userID, logID)
					results[i] = result{deleted: deleted, gearIDs: gearIDs, err: err, actID: act.ID}
				}
			} else {
				g, gctx := errgroup.WithContext(ctx)
				g.SetLimit(cfg.maxParallel)
				for i, act := range page {
					i, act := i, act
					g.Go(func() error {
						deleted, gearIDs, err := processActivity(gctx, db, act, userID, logID)
						results[i] = result{deleted: deleted, gearIDs: gearIDs, err: err, actID: act.ID}
						return nil // collect errors in result, not via errgroup
					})
				}
				_ = g.Wait()
			}

			for _, r := range results {
				if r.err != nil {
					slog.Warn("re-enrich failed",
						"activity_hash", logID(r.actID),
						"user_hash", logID(userID),
						"error", r.err,
					)
					userErrors++
					continue
				}
				userDeleted += r.deleted
				userEnriched++
				for _, gearID := range r.gearIDs {
					userGears[gearID] = struct{}{}
				}
			}

			if len(page) < pageSize {
				// Last page — no need to query again.
				break
			}
		}

		// Merge per-user gears into the global set.
		if cfg.maxParallel > 1 {
			mu.Lock()
		}
		for gid := range userGears {
			affectedGears[gid] = struct{}{}
		}
		if cfg.maxParallel > 1 {
			mu.Unlock()
		}

		// Release the per-user advisory lock explicitly (session-scoped lock).
		if _, err := userLockConn.Exec(ctx, "SELECT pg_advisory_unlock_shared($1, $2)",
			liftBackfillUserLockNs, hashUserIDToInt32(userID)); err != nil {
			slog.Warn("per-user advisory unlock failed", "user_hash", logID(userID), "error", err)
		}
		userLockConn.Release()

		totalEnriched += userEnriched
		totalDeleted += userDeleted
		totalErrors += userErrors

		slog.Info("user processed",
			"user_hash", logID(userID),
			"enriched", userEnriched,
			"deleted", userDeleted,
			"errors", userErrors,
		)
	}

	// Refresh gear stats once per affected gear (single path for --all-users
	// and --user-id; avoids N redundant per-activity calls because
	// RunStreamEnrichment was called with SkipGearRefresh=true).
	gearErrors := 0
	for gearID := range affectedGears {
		if err := db.RefreshGearAndDependentUsageStatsOwnerVerified(ctx, gearID); err != nil {
			slog.Warn("gear usage refresh failed", "gear_id", gearID, "error", err)
			gearErrors++
		}
	}

	// Run the acknowledged-at backfill after re-enrichment for both
	// --all-users and --user-id paths.
	var ackUserID *string
	if cfg.userID != "" {
		ackUserID = &cfg.userID
	}
	n, err := runAckBackfill(ctx, db.Pool, ackUserID)
	if err != nil {
		slog.Warn("ack backfill failed", "error", err)
	} else {
		slog.Info("ack backfill complete", "updated", n)
	}

	slog.Info("backfill complete",
		"re_enriched", totalEnriched,
		"pre_deleted_candidates", totalDeleted,
		"errors", totalErrors,
		"gear_updates", len(affectedGears),
		"gear_errors", gearErrors,
	)

	if totalErrors > 0 || gearErrors > 0 {
		return 1
	}
	return 0
}

// liftBackfillUserLockNs is the high-half namespace for the per-user
// advisory lock that prevents two concurrent CLI runs from processing
// the same user simultaneously. Uses the two-argument pg_advisory_lock
// form so it is disjoint from the per-activity LiftEnrichAdvisoryLockNs
// key space (which uses the single-argument, 64-bit form).
//
// Mnemonic 0x4C425546 spells "LBUF" (Lift Backfill User) in ASCII.
const liftBackfillUserLockNs = int32(0x4C425546)

// hashUserIDToInt32 hashes a user UUID string to an int32 for the second
// argument of pg_advisory_lock_shared (two-argument form). The same fnv32a
// pattern as LiftEnrichLockKey keeps implementation consistent.
func hashUserIDToInt32(userID string) int32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(userID))
	return int32(h.Sum32())
}

// tryAcquireUserLock attempts to acquire a shared advisory lock for the given
// user. Returns (true, conn) on success, (false, conn) if another process
// holds it, or (false, nil) on error. The caller must release conn on both
// success and !locked (but NOT on error when conn is nil).
func tryAcquireUserLock(ctx context.Context, pool *pgxpool.Pool, userID string) (bool, *pgxpool.Conn, error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return false, nil, fmt.Errorf("tryAcquireUserLock: acquire conn: %w", err)
	}
	var locked bool
	if err := conn.QueryRow(ctx,
		"SELECT pg_try_advisory_lock_shared($1, $2)",
		liftBackfillUserLockNs,
		hashUserIDToInt32(userID),
	).Scan(&locked); err != nil {
		conn.Release()
		return false, nil, fmt.Errorf("tryAcquireUserLock: %w", err)
	}
	return locked, conn, nil
}

// processActivity performs lift re-enrichment for one activity.
// Returns (deletedCount, gearIDs, err). deletedCount is kept for legacy log
// compatibility and is always zero; cleanup is performed by the enrichment
// transaction. On error the caller increments errors and continues; on success
// the caller records affected gear IDs for one deferred usage refresh.
func processActivity(
	ctx context.Context,
	db *database.DB,
	act liftEligibleActivity,
	userID string,
	logID func(string) string,
) (int, []string, error) {
	// SkipGearRefresh=true: gear stats are refreshed once per affected gear
	// in the post-loop gear usage refresh rather than per activity. This
	// avoids N redundant refreshes across the bulk run.
	settings := enrichment.StreamEnrichmentSettings{
		EnableLiftDetection: true,
		SkipGearRefresh:     true,
	}
	if err := enrichment.RunStreamEnrichment(ctx, db, userID, act.ID, settings); err != nil {
		return 0, nil, fmt.Errorf("run stream enrichment: %w", err)
	}
	gearIDs, err := listActivityGearIDs(ctx, db.Pool, act.ID, userID)
	if err != nil {
		return 0, nil, err
	}

	slog.Info("re-enriched",
		"activity_hash", logID(act.ID),
		"user_hash", logID(userID),
	)

	return 0, gearIDs, nil
}

// hashID returns an 8-hex-char FNV-1a hash of the given ID for privacy-safe
// logging. Use --verbose to log raw IDs.
func hashID(id string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(id))
	return fmt.Sprintf("%08x", h.Sum32())
}

// liftEligibleActivity is the minimal activity projection needed by the backfill.
type liftEligibleActivity struct {
	ID string
}

// listUsersWithLiftDetection returns user IDs that have enable_lift_detection=true
// in their enrichment_settings row. Users that have been soft-deleted
// (deleted_at IS NOT NULL) are excluded — their activities should not be
// re-processed.
func listUsersWithLiftDetection(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	rows, err := pool.Query(ctx, `
		SELECT u.id::text
		FROM user_enrichment_settings ues
		JOIN users u ON u.id = ues.user_id
		WHERE ues.enable_lift_detection = true
		  AND u.deleted_at IS NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("listUsersWithLiftDetection: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("listUsersWithLiftDetection scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// listLiftEligibleActivities returns a page of activities for a user that are
// either lift-sport activities or have lift_count > 0 (were previously
// classified as containing lifts). The page is bounded by offset and limit
// so callers can stream through large corpora without loading everything into
// memory at once.
func listLiftEligibleActivities(ctx context.Context, pool *pgxpool.Pool, userID string, offset, limit int) ([]liftEligibleActivity, error) {
	rows, err := pool.Query(ctx, `
		SELECT id::text
		FROM activities
		WHERE user_id = $1
		  AND (
		        (
		          NOT COALESCE(is_indoor, false)
		          AND NOT COALESCE(is_virtual, false)
		          AND (
		            sport_type = ANY($2)
		            OR (
		              sport_type = 'cycling'
		              AND COALESCE(sub_type, '') <> ALL($3)
		            )
		          )
		        )
		        OR lift_count > 0
		      )
		ORDER BY started_at DESC
		LIMIT $4 OFFSET $5
	`, userID,
		[]string{"alpine_skiing", "hiking", "mountaineering", "cross_country_skiing", "snowboarding"},
		[]string{"indoor_cycling", "spin"},
		limit,
		offset,
	)
	if err != nil {
		return nil, fmt.Errorf("listLiftEligibleActivities: %w", err)
	}
	defer rows.Close()

	var acts []liftEligibleActivity
	for rows.Next() {
		var a liftEligibleActivity
		if err := rows.Scan(&a.ID); err != nil {
			return nil, fmt.Errorf("listLiftEligibleActivities scan: %w", err)
		}
		acts = append(acts, a)
	}
	return acts, rows.Err()
}

func listActivityGearIDs(ctx context.Context, pool *pgxpool.Pool, activityID, userID string) ([]string, error) {
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT aga.gear_id::text
		FROM activity_gear_assignments aga
		JOIN activities a ON a.id = aga.activity_id
		WHERE aga.activity_id = $1
		  AND aga.user_id = $2
		  AND a.user_id = $2
	`, activityID, userID)
	if err != nil {
		return nil, fmt.Errorf("listActivityGearIDs: %w", err)
	}
	defer rows.Close()
	var gearIDs []string
	for rows.Next() {
		var gearID string
		if err := rows.Scan(&gearID); err != nil {
			return nil, fmt.Errorf("listActivityGearIDs scan: %w", err)
		}
		gearIDs = append(gearIDs, gearID)
	}
	return gearIDs, rows.Err()
}

// runAckBackfill sets activities.lifts_acknowledged_at = COALESCE(updated_at,
// started_at, NOW()) for every activity whose heuristic candidates are all
// already resolved (confirmed or rejected). Activities with pending heuristic
// candidates keep lifts_acknowledged_at = NULL so the review modal continues
// to appear.
//
// When userID is non-nil the UPDATE is scoped to that user only; otherwise
// it runs across all users.
//
// This prevents the review modal from popping up on all historic activities
// the first time a user visits them after the lift-detection migration
// (Adversarial-Review-Finding #22).
func runAckBackfill(ctx context.Context, pool *pgxpool.Pool, userID *string) (int64, error) {
	if userID != nil {
		tag, err := pool.Exec(ctx, `
			UPDATE activities
			SET lifts_acknowledged_at = COALESCE(updated_at, started_at, NOW())
			WHERE user_id = $1
			  AND id IN (
			    SELECT activity_id
			    FROM activity_lift_candidates
			    GROUP BY activity_id
			    HAVING COUNT(*) FILTER (
			      WHERE source = 'heuristic'
			        AND confirmed = false
			        AND NOT rejected_by_user
			    ) = 0
			  )
			  AND lifts_acknowledged_at IS NULL
		`, *userID)
		if err != nil {
			return 0, fmt.Errorf("runAckBackfill (scoped): %w", err)
		}
		return tag.RowsAffected(), nil
	}

	tag, err := pool.Exec(ctx, `
		UPDATE activities
		SET lifts_acknowledged_at = COALESCE(updated_at, started_at, NOW())
		WHERE id IN (
		  SELECT activity_id
		  FROM activity_lift_candidates
		  GROUP BY activity_id
		  HAVING COUNT(*) FILTER (
		    WHERE source = 'heuristic'
		      AND confirmed = false
		      AND NOT rejected_by_user
		  ) = 0
		)
		AND lifts_acknowledged_at IS NULL
	`)
	if err != nil {
		return 0, fmt.Errorf("runAckBackfill: %w", err)
	}
	return tag.RowsAffected(), nil
}
