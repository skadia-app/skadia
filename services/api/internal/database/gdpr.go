package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// HealthHypertables lists health tables backed by TimescaleDB hypertables.
// These are deleted explicitly in Phase 1 of DeleteUserData because
// TimescaleDB hypertables can have issues with cascading deletes.
var HealthHypertables = []string{
	"health_daily_summary", "health_sleep", "health_snapshots",
	"health_hrv_night_values", "health_monitoring_samples",
}

// HealthRegularTables lists health tables stored as regular PostgreSQL tables.
var HealthRegularTables = []string{
	"health_weight_entries", "health_blood_pressure", "health_fitness_age",
	"health_stress_snapshots", "health_spo2_snapshots",
	"health_training_load", "health_readiness_scores",
}

// AllHealthTables returns a combined slice of all health tables (hypertables + regular).
func AllHealthTables() []string {
	return append(append([]string{}, HealthHypertables...), HealthRegularTables...)
}

// PurgeDeletedUsers finds users whose deleted_at is >30 days ago and purges them.
// Intended to be called by a daily cleanup job.
// storageCleanup is an optional callback that deletes all stored files for a user;
// pass nil to skip external storage cleanup.
func (db *DB) PurgeDeletedUsers(ctx context.Context, storageCleanup func(ctx context.Context, userID string) error) error {
	rows, err := db.Pool.Query(ctx,
		`SELECT id FROM users WHERE deleted_at IS NOT NULL AND deleted_at < NOW() - INTERVAL '30 days'`)
	if err != nil {
		return fmt.Errorf("query expired deleted users: %w", err)
	}
	defer rows.Close()

	var userIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			slog.Error("GDPR purge: scan user id failed", "error", err)
			continue
		}
		userIDs = append(userIDs, id)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate expired deleted users: %w", err)
	}

	for _, uid := range userIDs {
		if err := db.DeleteUserData(ctx, uid, storageCleanup); err != nil {
			if errors.Is(err, ErrPurgeSkipped) {
				slog.Info("GDPR purge skipped (user revoked deletion)", "user_id", uid)
			} else {
				slog.Error("GDPR purge failed for user", "user_id", uid, "error", err)
			}
			// Continue with other users — don't let one failure block the rest.
		}
	}
	return nil
}

// ErrPurgeSkipped is returned when a purge is requested but the user is no
// longer marked for deletion (or another worker holds the per-user advisory
// lock). Callers (notably PurgeDeletedUsers) should treat this as a non-error
// and continue with the next user.
var ErrPurgeSkipped = errors.New("GDPR: purge skipped: user not marked for deletion or already in progress")

// isTransientPgError reports whether a postgres error is the kind that may
// succeed on a retry: serialization failures, deadlocks, lock-not-available
// races. These are typically caused by concurrent activity on the same rows
// and resolve on their own.
//
// Permanent errors (constraint violations, missing columns, syntax errors,
// permission denied, etc.) return false and are not retried.
func isTransientPgError(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	switch pgErr.Code {
	case "40001", // serialization_failure
		"40P01", // deadlock_detected
		"55P03": // lock_not_available
		return true
	}
	return false
}

// DeleteUserData performs a multi-phase GDPR data deletion for a single user.
//
// Audit-Trail (Option H — Audit-First architecture):
//
//	A persistent audit trail is written to auth_audit_log via LogAuthEvent.
//	auth_audit_log.user_id is ON DELETE SET NULL by design, so the audit
//	entries survive the user-row deletion. The events are:
//	  - gdpr_purge_started   — at the very beginning
//	  - gdpr_purge_skipped   — when the user is no longer marked for deletion
//	  - gdpr_purge_failed    — on any hard failure (with phase + error details)
//	  - gdpr_purge_completed — on success
//	background_jobs is used only as ephemeral live-status for the admin UI;
//	it is NOT the source of truth for audit (background_jobs rows are
//	disposable and may be cleaned up later).
//
// Phases:
//
//	Phase 1 (outside TX):
//	  Delete TimescaleDB hypertable rows + activity_streams + segment_efforts
//	  (own-segment + own-activity efforts) + health regular tables. Errors are
//	  AGGREGATED and trigger fail-fast: a single Phase 1 failure aborts the
//	  whole purge BEFORE Phase 1.5 / Phase 2 run, so the user row stays
//	  intact and a retry can pick up where this attempt left off (Phase 1
//	  deletes are idempotent — DELETE WHERE on already-empty table is a no-op).
//
//	Phase 1.5 (best-effort, outside TX):
//	  External storage cleanup via storageCleanup callback. Failures are
//	  logged as WARN but do NOT block the purge (best-effort by spec).
//
//	Phase 2 (TX):
//	  Transactional delete of everything else. The final DELETE FROM users
//	  cascades to ~45 dependent tables via ON DELETE CASCADE.
//
// storageCleanup is an optional callback that deletes all stored files for
// a user; pass nil to skip external storage cleanup.
func (db *DB) DeleteUserData(ctx context.Context, userID string, storageCleanup func(ctx context.Context, userID string) error) error {
	startedAt := time.Now()
	slog.Info("GDPR purge starting", "user_id", userID)

	// ---- Acquire dedicated connection + per-user advisory lock ----
	//
	// The advisory lock makes the entire purge per-user serialised across the
	// whole cluster: two pods (or two goroutines in the same pod) cannot purge
	// the same user concurrently. Without this, they would race on:
	//   - the unique partial index `idx_bg_jobs_one_running` (would reject the
	//     second INSERT with 23505 silently if we still ignored the error)
	//   - Phase 1 deletes (idempotent but doubly wasteful)
	//   - Phase 2 TX on the user row (FOR UPDATE serialises but only AFTER both
	//     pods have already done all of Phase 1's heavy lifting)
	//
	// The lock is held on a dedicated connection checked out from the pool for
	// the entire purge, then released on conn.Release() at the end. We use a
	// session-level lock (pg_try_advisory_lock) rather than transaction-level
	// because Phase 1 runs OUTSIDE any TX, so the lock has to outlive multiple
	// statements.
	conn, err := db.Pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("GDPR: acquire connection: %w", err)
	}
	defer conn.Release()

	var locked bool
	if err := conn.QueryRow(ctx,
		`SELECT pg_try_advisory_lock(hashtextextended('gdpr_purge:' || $1, 0))`, userID).Scan(&locked); err != nil {
		return fmt.Errorf("GDPR: acquire advisory lock: %w", err)
	}
	if !locked {
		slog.Warn("GDPR: another purge for this user is in progress, skipping", "user_id", userID)
		return ErrPurgeSkipped
	}
	defer func() {
		if _, err := conn.Exec(ctx,
			`SELECT pg_advisory_unlock(hashtextextended('gdpr_purge:' || $1, 0))`, userID); err != nil {
			slog.Warn("GDPR: release advisory lock failed", "user_id", userID, "error", err)
		}
	}()

	// ---- Audit: gdpr_purge_started (HARD-FAIL if the audit insert errors) ----
	//
	// The audit trail MUST exist for compliance. If we cannot write the
	// "started" event, abort BEFORE doing any data work — the absence of an
	// audit trail is itself a GDPR violation. LogAuthEventStrict returns the
	// error so we can fail fast here.
	if err := db.LogAuthEventStrict(ctx, userID, "gdpr_purge_started", "", "system", map[string]string{
		"user_id": userID, // also in details so it stays visible after FK SET NULL
	}); err != nil {
		slog.Error("GDPR_AUDIT_LOST: gdpr_purge_started insert failed, aborting purge",
			"user_id", userID, "error", err)
		return fmt.Errorf("GDPR: audit gdpr_purge_started: %w", err)
	}

	// ---- Insert tracking row (advisory-lock-protected, no race possible) ----
	//
	// The lock guarantees no concurrent INSERT, so we can safely DELETE any
	// leftover 'running' row from a crashed previous attempt and INSERT-RETURNING
	// our own. We capture the id explicitly so all subsequent UPDATEs target
	// THIS purge attempt and never accidentally hit a stale row.
	conn.Exec(ctx, `DELETE FROM background_jobs
		WHERE job_type = 'gdpr_purge' AND user_id = $1 AND status = 'running'`, userID)
	var purgeJobID string
	if err := conn.QueryRow(ctx, `INSERT INTO background_jobs (id, job_type, user_id, status)
		VALUES (gen_random_uuid(), 'gdpr_purge', $1, 'running')
		RETURNING id`, userID).Scan(&purgeJobID); err != nil {
		// Tracking row failed — purge cannot proceed safely.
		if auditErr := db.LogAuthEventStrict(ctx, "", "gdpr_purge_failed", "", "system", map[string]string{
			"user_id": userID,
			"phase":   "init",
			"error":   err.Error(),
		}); auditErr != nil {
			slog.Error("GDPR_AUDIT_LOST: gdpr_purge_failed (init) insert failed",
				"user_id", userID, "error", auditErr)
		}
		return fmt.Errorf("GDPR: insert tracking job: %w", err)
	}

	// recordPurgeFailure marks the background_jobs row as failed AND writes
	// a persistent gdpr_purge_failed event to auth_audit_log with phase + error
	// details. Both writes happen on the locked connection so they remain
	// per-user serialised. If the audit insert ALSO fails, log at ERROR level
	// with a special "GDPR_AUDIT_LOST" tag so a centralised log alert can fire
	// — the caller is already in failure state and cannot meaningfully recover.
	//
	// Outside-TX background_jobs update: spec Phase 0.3 explicitly permits
	// this — a failed purge MUST be visible to admins even if the TX is
	// rolled back. Phase 0.3 forbids markJobDone (success) outside TX, NOT
	// the failure-status outside TX.
	//
	// Audit event uses user_id="" (NULL in DB) and stores the actual purged
	// user-id under details.user_id. This avoids FK violations when the failure
	// happens AFTER `DELETE FROM users` (the user row is gone but the audit
	// insert would otherwise FK-violate).
	recordPurgeFailure := func(phase, errMsg string) {
		if _, err := conn.Exec(ctx, `UPDATE background_jobs SET status = 'failed', error = $2, finished_at = NOW()
			WHERE id = $1`, purgeJobID, errMsg); err != nil {
			slog.Error("GDPR: recordPurgeFailure update failed", "user_id", userID, "error", err)
		}
		if err := db.LogAuthEventStrict(ctx, "", "gdpr_purge_failed", "", "system", map[string]string{
			"user_id": userID,
			"phase":   phase,
			"error":   errMsg,
		}); err != nil {
			slog.Error("GDPR_AUDIT_LOST: gdpr_purge_failed event could not be written",
				"user_id", userID, "phase", phase, "audit_error", err)
		}
	}

	// ---- Phase 1: Hypertables + non-cascading dependents (with retry) ----
	//
	// Phase-1 errors are aggregated. Per-table transient errors (serialization
	// failures, deadlocks, lock-not-available) are retried up to 3× with linear
	// backoff. Permanent errors (constraint violations, missing columns) are
	// not retried — they will not succeed on a retry and would just delay the
	// failure report.
	//
	// If ANY delete still fails after retries, the purge aborts BEFORE Phase 1.5
	// and Phase 2 — the user row stays intact and a future cron-run can pick up
	// where this attempt left off (Phase 1 deletes are idempotent: DELETE WHERE
	// on an already-empty table is a no-op).
	var phase1Errors []string

	retryDelete := func(name, sql string, args ...any) {
		const maxAttempts = 3
		var lastErr error
		for attempt := 1; attempt <= maxAttempts; attempt++ {
			if _, err := conn.Exec(ctx, sql, args...); err != nil {
				lastErr = err
				if !isTransientPgError(err) || attempt == maxAttempts {
					break
				}
				backoff := time.Duration(attempt) * 250 * time.Millisecond
				slog.Warn("GDPR: phase1 delete transient error, retrying",
					"table", name, "attempt", attempt, "backoff_ms", backoff.Milliseconds(), "error", err)
				time.Sleep(backoff)
				continue
			}
			return // success
		}
		slog.Error("GDPR: delete phase1 table failed permanently",
			"table", name, "error", lastErr)
		phase1Errors = append(phase1Errors, fmt.Sprintf("%s: %s", name, lastErr))
	}

	actIDs := db.getUserActivityIDs(ctx, userID)
	if len(actIDs) > 0 {
		retryDelete("activity_streams",
			`DELETE FROM activity_streams WHERE activity_id = ANY($1)`, actIDs)
	}

	segIDs := db.getUserSegmentIDs(ctx, userID)
	if len(segIDs) > 0 {
		retryDelete("segment_efforts(by_segment)",
			`DELETE FROM segment_efforts WHERE segment_id = ANY($1)`, segIDs)
	}
	if len(actIDs) > 0 {
		retryDelete("segment_efforts(by_activity)",
			`DELETE FROM segment_efforts WHERE activity_id = ANY($1)`, actIDs)
	}

	for _, table := range HealthHypertables {
		retryDelete(table,
			fmt.Sprintf(`DELETE FROM %s WHERE user_id = $1`, pgx.Identifier{table}.Sanitize()), userID)
	}
	for _, table := range HealthRegularTables {
		retryDelete(table,
			fmt.Sprintf(`DELETE FROM %s WHERE user_id = $1`, pgx.Identifier{table}.Sanitize()), userID)
	}

	if len(phase1Errors) > 0 {
		joined := strings.Join(phase1Errors, "; ")
		msg := fmt.Sprintf("phase1 failed: %d table(s) errored: %s", len(phase1Errors), joined)
		recordPurgeFailure("phase1_hypertables", msg)
		return fmt.Errorf("GDPR: %s", msg)
	}

	// ---- Phase 1.5: External storage (best-effort, outside TX) ----
	// ShadowFS.DeleteAllForUser queries user_files rows, which CASCADE-delete
	// with the user row. Run cleanup while those rows still exist.
	if storageCleanup != nil {
		if err := storageCleanup(ctx, userID); err != nil {
			slog.Warn("GDPR: file store cleanup failed", "user_id", userID, "error", err)
		} else {
			slog.Info("GDPR: file store cleanup complete", "user_id", userID)
		}
	}

	// ---- Phase 2: Transaction for everything else (on the locked connection) ----

	tx, err := conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		recordPurgeFailure("phase2_begin_tx", err.Error())
		return fmt.Errorf("GDPR begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Safety net — verify user is still marked for deletion.
	var deletedAt *time.Time
	if err := tx.QueryRow(ctx,
		`SELECT deleted_at FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&deletedAt); err != nil {
		recordPurgeFailure("phase2_user_lookup", err.Error())
		return fmt.Errorf("GDPR: user %s not found: %w", userID, err)
	}
	if deletedAt == nil {
		// Skipped is NOT a failure — the user revoked their deletion. The
		// user still exists at this point, so we can keep the FK on user_id
		// intact for the audit event.
		if _, err := conn.Exec(ctx,
			`UPDATE background_jobs SET status = 'completed', error = 'skipped: user not marked for deletion', finished_at = NOW()
			 WHERE id = $1`, purgeJobID); err != nil {
			slog.Warn("GDPR: mark skipped purge failed", "user_id", userID, "error", err)
		}
		if err := db.LogAuthEventStrict(ctx, userID, "gdpr_purge_skipped", "", "system", map[string]string{
			"user_id": userID,
			"reason":  "user no longer marked for deletion",
		}); err != nil {
			slog.Error("GDPR_AUDIT_LOST: gdpr_purge_skipped event could not be written",
				"user_id", userID, "error", err)
		}
		return ErrPurgeSkipped
	}

	// Tables without CASCADE on users that still need explicit deletion.
	// (best_efforts, best_effort_exclusions, activity_edits removed — they
	// have transitive CASCADE chains via users/activities.)

	// user_daily_features: no FK constraint at all (TEXT user_id).
	if _, err := tx.Exec(ctx, `DELETE FROM user_daily_features WHERE user_id = $1`, userID); err != nil {
		slog.Warn("GDPR: delete user_daily_features failed", "error", err)
	}

	// llm_user_config: no FK constraint at all.
	if _, err := tx.Exec(ctx, `DELETE FROM llm_user_config WHERE user_id = $1`, userID); err != nil {
		slog.Warn("GDPR: delete llm_user_config failed", "error", err)
	}

	// segment_efforts: user_id REFERENCES users(id) — NO ACTION (no CASCADE).
	// Covers efforts by this user on OTHER users' segments (own-segment efforts
	// already deleted in Phase 1, but the user_id FK still blocks user deletion).
	if _, err := tx.Exec(ctx, `DELETE FROM segment_efforts WHERE user_id = $1`, userID); err != nil {
		slog.Warn("GDPR: delete segment_efforts by user_id failed", "error", err)
	}

	// segment_versions: created_by REFERENCES users(id) — NO ACTION.
	if _, err := tx.Exec(ctx, `DELETE FROM segment_versions WHERE created_by = $1`, userID); err != nil {
		slog.Warn("GDPR: delete segment_versions failed", "error", err)
	}

	// segments: created_by REFERENCES users(id) — NO ACTION. Hard fail if
	// this DELETE errors out — without it, the final DELETE FROM users will
	// fail with FK violation.
	if _, err := tx.Exec(ctx, `DELETE FROM segments WHERE created_by = $1`, userID); err != nil {
		recordPurgeFailure("phase2_segments", err.Error())
		return fmt.Errorf("GDPR: delete segments: %w", err)
	}

	// Delete route_photos user_files entries BEFORE deleting planned_routes
	// (which cascades to route_photos). Runs inside the TX for atomicity.
	db.deleteRoutePhotoFiles(ctx, tx, userID)

	// route_photos: user_id REFERENCES users(id) — NO ACTION.
	if _, err := tx.Exec(ctx, `DELETE FROM route_photos WHERE user_id = $1`, userID); err != nil {
		slog.Warn("GDPR: delete route_photos failed", "error", err)
	}

	// planned_routes: user_id REFERENCES users(id) — NO ACTION.
	if _, err := tx.Exec(ctx, `DELETE FROM planned_routes WHERE user_id = $1`, userID); err != nil {
		slog.Warn("GDPR: delete planned_routes failed", "error", err)
	}

	// heatmap_trail_matches: user_id REFERENCES users(id) — NO ACTION.
	if _, err := tx.Exec(ctx, `DELETE FROM heatmap_trail_matches WHERE user_id = $1`, userID); err != nil {
		slog.Warn("GDPR: delete heatmap_trail_matches failed", "error", err)
	}

	// terra_incognita_cache: user_id TEXT, no FK constraint.
	if _, err := tx.Exec(ctx, `DELETE FROM terra_incognita_cache WHERE user_id = $1`, userID); err != nil {
		slog.Warn("GDPR: delete terra_incognita_cache failed", "error", err)
	}

	// background_jobs: clear all OTHER background_jobs for this user. The
	// gdpr_purge row is intentionally KEPT and marked 'completed' below
	// (still inside this TX) so the admin UI can show its final state.
	if _, err := tx.Exec(ctx, `DELETE FROM background_jobs WHERE user_id = $1 AND job_type != 'gdpr_purge'`, userID); err != nil {
		slog.Warn("GDPR: delete background_jobs failed", "error", err)
	}

	// Mark the gdpr_purge tracking row as 'completed' INSIDE the TX so it
	// commits atomically with DELETE FROM users. HARD-FAIL on UPDATE error:
	// without a valid completed-status row, admin UI / monitoring cannot
	// distinguish a real success from a crashed-mid-purge.
	durationMs := time.Since(startedAt).Milliseconds()
	if _, err := tx.Exec(ctx,
		`UPDATE background_jobs SET status = 'completed', finished_at = NOW(),
			metadata = jsonb_build_object(
				'activities', $2::int,
				'segments', $3::int,
				'duration_ms', $4::bigint
			)
		 WHERE id = $1`, purgeJobID, len(actIDs), len(segIDs), durationMs); err != nil {
		recordPurgeFailure("phase2_mark_completed", err.Error())
		return fmt.Errorf("GDPR: mark purge completed: %w", err)
	}

	// Write the gdpr_purge_completed audit event INSIDE the TX (BEFORE the
	// DELETE FROM users) so that:
	//   1. Either the audit event commits atomically with the user deletion,
	//      OR neither commits (rollback) — no half-state.
	//   2. The user_id FK is still valid here, so the audit row is inserted
	//      with the correct top-level user_id. After commit, ON DELETE SET NULL
	//      will null it (FK-link removed), but the row stays in auth_audit_log
	//      with details.user_id preserving the link.
	if _, err := tx.Exec(ctx,
		`INSERT INTO auth_audit_log (user_id, event_type, ip_address, user_agent, details)
		 VALUES ($1::uuid, 'gdpr_purge_completed', NULL, 'system', $2::jsonb)`,
		userID, fmt.Sprintf(`{"user_id":%q,"activities":%d,"segments":%d,"duration_ms":%d}`,
			userID, len(actIDs), len(segIDs), durationMs)); err != nil {
		recordPurgeFailure("phase2_audit_completed", err.Error())
		return fmt.Errorf("GDPR: insert completed audit event: %w", err)
	}

	// DELETE user — CASCADE handles all remaining dependent tables. The
	// auth_audit_log row inserted above is preserved via ON DELETE SET NULL.
	if _, err := tx.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID); err != nil {
		recordPurgeFailure("phase2_delete_user", err.Error())
		return fmt.Errorf("GDPR: delete user: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		recordPurgeFailure("phase2_commit", err.Error())
		return fmt.Errorf("GDPR: commit: %w", err)
	}

	slog.Info("GDPR purge complete", "user_id", userID,
		"activities", len(actIDs), "segments", len(segIDs), "duration_ms", durationMs)
	return nil
}

func (db *DB) getUserActivityIDs(ctx context.Context, userID string) []string {
	rows, err := db.Pool.Query(ctx,
		`SELECT id FROM activities WHERE user_id = $1`, userID)
	if err != nil {
		slog.Warn("GDPR: query activity IDs failed", "user_id", userID, "error", err)
		return nil
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

func (db *DB) getUserSegmentIDs(ctx context.Context, userID string) []string {
	rows, err := db.Pool.Query(ctx,
		`SELECT id FROM segments WHERE created_by = $1`, userID)
	if err != nil {
		slog.Warn("GDPR: query segment IDs failed", "user_id", userID, "error", err)
		return nil
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

// deleteRoutePhotoFiles collects file IDs from route_photos for the user and
// deletes the corresponding user_files entries. This MUST be called
// BEFORE planned_routes are deleted (which would CASCADE-delete route_photos).
// M46: Runs inside the GDPR transaction for atomicity.
// L38: Queries route_photos first, then deletes user_files (correct ordering).
func (db *DB) deleteRoutePhotoFiles(ctx context.Context, tx pgx.Tx, userID string) {
	rows, err := tx.Query(ctx,
		`SELECT COALESCE(rp.file_id::text, '')
		 FROM route_photos rp
		 JOIN planned_routes pr ON rp.route_id = pr.id
		 WHERE pr.user_id = $1 AND rp.file_id IS NOT NULL`, userID)
	if err != nil {
		slog.Warn("GDPR: query route_photos file IDs failed", "user_id", userID, "error", err)
		return
	}
	defer rows.Close()

	var fileIDs []string
	for rows.Next() {
		var fileID string
		if err := rows.Scan(&fileID); err != nil {
			continue
		}
		if fileID != "" {
			fileIDs = append(fileIDs, fileID)
		}
	}
	if len(fileIDs) > 0 {
		slog.Info("GDPR: route_photos file IDs collected for deletion",
			"user_id", userID, "count", len(fileIDs))
		for _, fid := range fileIDs {
			if _, err := tx.Exec(ctx,
				`DELETE FROM user_files WHERE user_id = $1 AND id = $2`,
				userID, fid); err != nil {
				slog.Warn("GDPR: delete route_photo user_file failed",
					"user_id", userID, "file_id", fid, "error", err)
			}
		}
	}
}
