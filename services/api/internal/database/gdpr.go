package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
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
// longer marked for deletion. Callers (notably PurgeDeletedUsers) should treat
// this as a non-error and continue with the next user.
var ErrPurgeSkipped = errors.New("GDPR: purge skipped: user not marked for deletion")

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

	// Audit: gdpr_purge_started — written before any data is touched.
	// user_id is set so admins can correlate this with the user; ON DELETE
	// SET NULL ensures the row survives the user-row deletion (the FK link
	// is severed but the audit entry stays in auth_audit_log).
	db.LogAuthEvent(ctx, userID, "gdpr_purge_started", "", "system", map[string]string{
		"user_id": userID, // also in details so it stays visible after FK SET NULL
	})

	// Track purge in background_jobs for live admin-UI visibility.
	// This is ephemeral; auth_audit_log is the persistent audit trail.
	//
	// We capture the inserted row's id explicitly so all subsequent UPDATEs
	// target THIS purge attempt — not a stale 'running' row that a previous
	// crashed run left behind. Without the explicit id, a crashed-then-resumed
	// purge would race with the startup job_resume sweeper, which marks stale
	// 'running' rows as 'failed' before our INSERT can land. The unique partial
	// index `idx_bg_jobs_one_running` would also reject our INSERT during the
	// race window.
	//
	// Cleanup-then-insert pattern: drop any leftover 'running' gdpr_purge for
	// this user (the row is just a tracking artefact, not an audit record),
	// THEN insert ours, capture the id with RETURNING.
	db.Pool.Exec(ctx, `DELETE FROM background_jobs
		WHERE job_type = 'gdpr_purge' AND user_id = $1 AND status = 'running'`, userID)
	var purgeJobID string
	if err := db.Pool.QueryRow(ctx, `INSERT INTO background_jobs (id, job_type, user_id, status)
		VALUES (gen_random_uuid(), 'gdpr_purge', $1, 'running')
		RETURNING id`, userID).Scan(&purgeJobID); err != nil {
		// Hard fail — without a tracking row we cannot mark progress reliably.
		slog.Error("GDPR: insert tracking job failed", "user_id", userID, "error", err)
		db.LogAuthEvent(ctx, "", "gdpr_purge_failed", "", "system", map[string]string{
			"user_id": userID,
			"phase":   "init",
			"error":   err.Error(),
		})
		return fmt.Errorf("GDPR: insert tracking job: %w", err)
	}

	// recordPurgeFailure marks the background_jobs row as failed (outside the
	// Phase 2 TX so it survives a TX rollback) AND writes a persistent
	// gdpr_purge_failed event to auth_audit_log with phase + error details.
	//
	// Outside-TX background_jobs update: spec Phase 0.3 explicitly permits
	// this — a failed purge MUST be visible to admins even if the TX is
	// rolled back. Phase 0.3 forbids the markJobDone (success) outside TX
	// path, NOT the failure-status outside TX path.
	//
	// The audit event is written with user_id="" (NULL in DB) and the actual
	// purged user-id stored under details.user_id. This avoids FK violations
	// when the failure happens AFTER `DELETE FROM users` but before commit
	// (the user row is gone but the audit insert would fail FK check).
	recordPurgeFailure := func(phase, errMsg string) {
		if _, err := db.Pool.Exec(ctx, `UPDATE background_jobs SET status = 'failed', error = $2, finished_at = NOW()
			WHERE id = $1`, purgeJobID, errMsg); err != nil {
			slog.Warn("GDPR: recordPurgeFailure update failed", "user_id", userID, "error", err)
		}
		db.LogAuthEvent(ctx, "", "gdpr_purge_failed", "", "system", map[string]string{
			"user_id": userID,
			"phase":   phase,
			"error":   errMsg,
		})
	}

	// ---- Phase 1: Hypertables + non-cascading dependents (outside TX) ----
	//
	// Phase-1 errors are aggregated. If ANY delete fails, the purge aborts
	// before Phase 1.5/Phase 2, leaving the user row intact for a retry.
	// This prevents the silent-data-loss scenario where a hypertable delete
	// fails but Phase 2 commits anyway, leaving orphaned GDPR data behind.
	var phase1Errors []string

	// Get activity IDs for stream cleanup.
	actIDs := db.getUserActivityIDs(ctx, userID)

	// activity_streams: cascaded from activities, but TimescaleDB hypertables
	// occasionally fail to cascade reliably — delete explicitly.
	if len(actIDs) > 0 {
		if _, err := db.Pool.Exec(ctx,
			`DELETE FROM activity_streams WHERE activity_id = ANY($1)`, actIDs); err != nil {
			slog.Error("GDPR: delete activity_streams failed", "error", err)
			phase1Errors = append(phase1Errors, fmt.Sprintf("activity_streams: %s", err))
		}
	}

	// Get segment IDs owned by this user.
	segIDs := db.getUserSegmentIDs(ctx, userID)

	// segment_efforts on user's own segments.
	if len(segIDs) > 0 {
		if _, err := db.Pool.Exec(ctx,
			`DELETE FROM segment_efforts WHERE segment_id = ANY($1)`, segIDs); err != nil {
			slog.Error("GDPR: delete segment_efforts for segment failed", "error", err)
			phase1Errors = append(phase1Errors, fmt.Sprintf("segment_efforts(by segment): %s", err))
		}
	}

	// segment_efforts by this user on OTHER people's segments.
	if len(actIDs) > 0 {
		if _, err := db.Pool.Exec(ctx,
			`DELETE FROM segment_efforts WHERE activity_id = ANY($1)`, actIDs); err != nil {
			slog.Error("GDPR: delete segment_efforts by activity failed", "error", err)
			phase1Errors = append(phase1Errors, fmt.Sprintf("segment_efforts(by activity): %s", err))
		}
	}

	// Health hypertables (CASCADE unreliable on hypertables — delete explicitly).
	for _, table := range HealthHypertables {
		if _, err := db.Pool.Exec(ctx,
			fmt.Sprintf(`DELETE FROM %s WHERE user_id = $1`, pgx.Identifier{table}.Sanitize()), userID); err != nil {
			slog.Error("GDPR: delete health hypertable failed", "table", table, "error", err)
			phase1Errors = append(phase1Errors, fmt.Sprintf("%s: %s", table, err))
		}
	}

	// Health regular tables (no CASCADE from users).
	for _, table := range HealthRegularTables {
		if _, err := db.Pool.Exec(ctx,
			fmt.Sprintf(`DELETE FROM %s WHERE user_id = $1`, pgx.Identifier{table}.Sanitize()), userID); err != nil {
			slog.Error("GDPR: delete health table failed", "table", table, "error", err)
			phase1Errors = append(phase1Errors, fmt.Sprintf("%s: %s", table, err))
		}
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

	// ---- Phase 2: Transaction for everything else ----

	tx, err := db.Pool.Begin(ctx)
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
		// Skipped is NOT a failure — the user revoked their deletion.
		// Mark the background_jobs row as 'completed' with a "skipped" error
		// hint, and emit gdpr_purge_skipped (NOT gdpr_purge_failed) so admin
		// monitoring can filter these out. The user still exists at this
		// point, so we can keep the FK on user_id intact for the audit event.
		if _, err := db.Pool.Exec(ctx,
			`UPDATE background_jobs SET status = 'completed', error = 'skipped: user not marked for deletion', finished_at = NOW()
			 WHERE id = $1`, purgeJobID); err != nil {
			slog.Warn("GDPR: mark skipped purge failed", "user_id", userID, "error", err)
		}
		db.LogAuthEvent(ctx, userID, "gdpr_purge_skipped", "", "system", map[string]string{
			"user_id": userID,
			"reason":  "user no longer marked for deletion",
		})
		return ErrPurgeSkipped
	}

	// Tables without CASCADE on users that still need explicit deletion.
	// (best_efforts, best_effort_exclusions, activity_edits removed — they
	// have transitive CASCADE chains via users/activities and are cleaned
	// up by the final DELETE FROM users.)

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
	// background_jobs has no FK on users, so the surviving row does not
	// block DELETE FROM users.
	if _, err := tx.Exec(ctx, `DELETE FROM background_jobs WHERE user_id = $1 AND job_type != 'gdpr_purge'`, userID); err != nil {
		slog.Warn("GDPR: delete background_jobs failed", "error", err)
	}

	// Mark the gdpr_purge tracking row as 'completed' INSIDE the TX so it
	// commits atomically with DELETE FROM users. If we did this post-commit
	// the WHERE filter could miss the row (race with the startup job_resume
	// sweeper or with deferred metadata updates).
	durationMs := time.Since(startedAt).Milliseconds()
	if _, err := tx.Exec(ctx,
		`UPDATE background_jobs SET status = 'completed', finished_at = NOW(),
			metadata = jsonb_build_object(
				'activities', $2::int,
				'segments', $3::int,
				'duration_ms', $4::bigint
			)
		 WHERE id = $1`, purgeJobID, len(actIDs), len(segIDs), durationMs); err != nil {
		slog.Warn("GDPR: mark purge completed failed", "user_id", userID, "error", err)
	}

	// DELETE user — CASCADE handles all remaining dependent tables.
	if _, err := tx.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID); err != nil {
		recordPurgeFailure("phase2_delete_user", err.Error())
		return fmt.Errorf("GDPR: delete user: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		recordPurgeFailure("phase2_commit", err.Error())
		return fmt.Errorf("GDPR: commit: %w", err)
	}

	// ---- Success audit event (post-commit) ----
	//
	// user_id is intentionally empty here: the user row is gone, so an INSERT
	// with a non-null user_id would FK-violate. The purged user-id is preserved
	// in details.user_id so admins can correlate.
	db.LogAuthEvent(ctx, "", "gdpr_purge_completed", "", "system", map[string]string{
		"user_id":     userID,
		"activities":  fmt.Sprintf("%d", len(actIDs)),
		"segments":    fmt.Sprintf("%d", len(segIDs)),
		"duration_ms": fmt.Sprintf("%d", durationMs),
	})

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
