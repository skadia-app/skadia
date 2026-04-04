package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

// HealthHypertables lists health tables backed by TimescaleDB hypertables.
var HealthHypertables = []string{
	"health_daily_summary", "health_sleep", "health_snapshots",
	"health_hrv_night_values", "health_hrv_snapshots",
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
			slog.Error("GDPR purge failed for user", "user_id", uid, "error", err)
			// Continue with other users — don't let one failure block the rest.
		}
	}
	return nil
}

// DeleteUserData performs a 3-phase GDPR data deletion for a single user.
//
// Phase 1: Delete hypertable rows that reference the user (outside TX —
//
//	TimescaleDB hypertables can't safely mix DDL with transactions).
//
// Phase 2: Transactional delete of everything else. The final DELETE FROM users
//
//	cascades to ~45+ dependent tables via ON DELETE CASCADE, including:
//	activities, gear, tags, activity_tags, activity_photos, privacy_zones,
//	enrichment_settings, custom_maps, map_layers, heatmap_lines, pmc_daily,
//	gear_rules, gear_auto_assign_rules, gear_documents, meta_files,
//	garmin_import_jobs, garmin_import_mappings, strava_imports,
//	import_queue, analytics_cache, segment_favorites, ml_settings,
//	device_tokens, notifications, chat_sessions, chat_messages,
//	oauth_connections, refresh_tokens, email_tokens, auth_codes,
//	auth_audit_log (SET NULL), activity_weather, activity_events,
//	activity_sets, activity_laps, workouts, workout_steps, devices,
//	route_photos, planned_routes (explicit), user_files, user_file_schemas,
//	file_attachments, activity_dive_settings, activity_dive_gases,
//	routing_profiles, etc.
//
// Phase 1.5: Best-effort external storage cleanup via ShadowFS.
// storageCleanup is an optional callback that deletes all stored files for a user;
// pass nil to skip external storage cleanup.
func (db *DB) DeleteUserData(ctx context.Context, userID string, storageCleanup func(ctx context.Context, userID string) error) error {
	slog.Info("GDPR purge starting", "user_id", userID)

	// Track purge in background_jobs for UI visibility.
	db.Pool.Exec(ctx, `INSERT INTO background_jobs (id, job_type, user_id, status, created_at)
		VALUES (gen_random_uuid(), 'gdpr_purge', $1, 'running', NOW())`, userID)
	markJobDone := func(status, errMsg string) {
		// L66: Log errors from job status updates so silent failures are visible.
		var execErr error
		if status == "failed" {
			_, execErr = db.Pool.Exec(ctx, `UPDATE background_jobs SET status = 'failed', error = $2, completed_at = NOW()
				WHERE job_type = 'gdpr_purge' AND user_id = $1 AND status = 'running'`, userID, errMsg)
		} else {
			_, execErr = db.Pool.Exec(ctx, `UPDATE background_jobs SET status = 'completed', completed_at = NOW()
				WHERE job_type = 'gdpr_purge' AND user_id = $1 AND status = 'running'`, userID)
		}
		if execErr != nil {
			slog.Warn("GDPR: markJobDone update failed", "status", status, "user_id", userID, "error", execErr)
		}
	}

	// ---- Phase 1: Hypertables (outside TX) ----

	// activity_streams and segment_efforts are cascaded from activities/segments,
	// but hypertables sometimes have issues with cascading deletes.
	// Delete explicitly to be safe.

	// Get activity IDs for stream cleanup.
	actIDs := db.getUserActivityIDs(ctx, userID)

	// Batch-delete activity streams.
	if len(actIDs) > 0 {
		if _, err := db.Pool.Exec(ctx,
			`DELETE FROM activity_streams WHERE activity_id = ANY($1)`, actIDs); err != nil {
			slog.Warn("GDPR: delete activity_streams failed", "error", err)
		}
	}

	// Get segment IDs owned by this user.
	segIDs := db.getUserSegmentIDs(ctx, userID)

	// Batch-delete efforts on user's own segments.
	if len(segIDs) > 0 {
		if _, err := db.Pool.Exec(ctx,
			`DELETE FROM segment_efforts WHERE segment_id = ANY($1)`, segIDs); err != nil {
			slog.Warn("GDPR: delete segment_efforts for segment failed", "error", err)
		}
	}

	// Also delete efforts by this user on OTHER people's segments.
	if len(actIDs) > 0 {
		if _, err := db.Pool.Exec(ctx,
			`DELETE FROM segment_efforts WHERE activity_id = ANY($1)`, actIDs); err != nil {
			slog.Warn("GDPR: delete segment_efforts by activity failed", "error", err)
		}
	}

	// Health hypertables (no CASCADE from users).
	for _, table := range HealthHypertables {
		if _, err := db.Pool.Exec(ctx,
			fmt.Sprintf(`DELETE FROM %s WHERE user_id = $1`, table), userID); err != nil {
			// L37: Phase 1 errors logged at ERROR level — these are data-deletion failures, not warnings.
			slog.Error("GDPR: delete health hypertable failed", "table", table, "error", err)
		}
	}

	// Health regular tables (no CASCADE from users).
	for _, table := range HealthRegularTables {
		if _, err := db.Pool.Exec(ctx,
			fmt.Sprintf(`DELETE FROM %s WHERE user_id = $1`, table), userID); err != nil {
			slog.Error("GDPR: delete health table failed", "table", table, "error", err)
		}
	}

	// ---- Phase 1.5: External storage (must run BEFORE user row deletion) ----
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
		markJobDone("failed", err.Error())
		return fmt.Errorf("GDPR begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Safety net — verify user is still marked for deletion.
	var deletedAt *time.Time
	if err := tx.QueryRow(ctx,
		`SELECT deleted_at FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&deletedAt); err != nil {
		markJobDone("failed", err.Error())
		return fmt.Errorf("GDPR: user %s not found: %w", userID, err)
	}
	if deletedAt == nil {
		markJobDone("failed", "user no longer marked for deletion")
		return fmt.Errorf("GDPR: user %s no longer marked for deletion", userID)
	}

	// Tables without CASCADE that need explicit deletion.

	// user_daily_features: user_id without CASCADE.
	if _, err := tx.Exec(ctx, `DELETE FROM user_daily_features WHERE user_id = $1`, userID); err != nil {
		slog.Warn("GDPR: delete user_daily_features failed", "error", err)
	}

	// llm_user_config: user_id without CASCADE.
	if _, err := tx.Exec(ctx, `DELETE FROM llm_user_config WHERE user_id = $1`, userID); err != nil {
		slog.Warn("GDPR: delete llm_user_config failed", "error", err)
	}

	// segment_efforts: user_id REFERENCES users(id) — no CASCADE.
	// Covers efforts by this user on OTHER users' segments (own-segment efforts
	// already deleted in Phase 1, but the user_id FK still blocks user deletion).
	if _, err := tx.Exec(ctx, `DELETE FROM segment_efforts WHERE user_id = $1`, userID); err != nil {
		slog.Warn("GDPR: delete segment_efforts by user_id failed", "error", err)
	}

	// segment_versions: created_by REFERENCES users(id) — no CASCADE.
	if _, err := tx.Exec(ctx, `DELETE FROM segment_versions WHERE created_by = $1`, userID); err != nil {
		slog.Warn("GDPR: delete segment_versions failed", "error", err)
	}

	// segments: created_by REFERENCES users(id) — no CASCADE.
	if _, err := tx.Exec(ctx, `DELETE FROM segments WHERE created_by = $1`, userID); err != nil {
		markJobDone("failed", err.Error())
		return fmt.Errorf("GDPR: delete segments: %w", err)
	}

	// activity_edits: user_id without CASCADE FK — explicit delete required.
	if _, err := tx.Exec(ctx, `DELETE FROM activity_edits WHERE user_id = $1`, userID); err != nil {
		slog.Warn("GDPR: delete activity_edits failed", "error", err)
	}

	// Delete route_photos user_files entries BEFORE deleting planned_routes (which cascades to route_photos).
	// M46: Run inside the TX so a rollback also reverts the DB deletes. The actual S3 objects
	// are deleted by the Phase 1.5 storage cleanup callback anyway; this is DB cleanup only.
	db.deleteRoutePhotoFiles(ctx, tx, userID)

	// route_photos: user_id REFERENCES users(id) — no CASCADE. Delete explicitly as defense-in-depth
	// (normally cascaded from planned_routes via route_id, but orphaned rows would block user deletion).
	if _, err := tx.Exec(ctx, `DELETE FROM route_photos WHERE user_id = $1`, userID); err != nil {
		slog.Warn("GDPR: delete route_photos failed", "error", err)
	}

	// planned_routes: user_id REFERENCES users(id) — no CASCADE.
	if _, err := tx.Exec(ctx, `DELETE FROM planned_routes WHERE user_id = $1`, userID); err != nil {
		slog.Warn("GDPR: delete planned_routes failed", "error", err)
	}

	// heatmap_trail_matches: user_id REFERENCES users(id) — no CASCADE.
	if _, err := tx.Exec(ctx, `DELETE FROM heatmap_trail_matches WHERE user_id = $1`, userID); err != nil {
		slog.Warn("GDPR: delete heatmap_trail_matches failed", "error", err)
	}

	// terra_incognita_cache: user_id TEXT PK, no FK constraint.
	if _, err := tx.Exec(ctx, `DELETE FROM terra_incognita_cache WHERE user_id = $1`, userID); err != nil {
		slog.Warn("GDPR: delete terra_incognita_cache failed", "error", err)
	}

	// background_jobs: user_id TEXT, no FK constraint.
	// M26: Mark the GDPR tracking job as completed INSIDE the TX so a rollback also
	// reverts the status update. The row is deleted in the same TX immediately after.
	if _, err := tx.Exec(ctx,
		`UPDATE background_jobs SET status = 'completed', completed_at = NOW()
		 WHERE job_type = 'gdpr_purge' AND user_id = $1 AND status = 'running'`, userID); err != nil {
		slog.Warn("GDPR: mark job done in TX failed", "error", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM background_jobs WHERE user_id = $1`, userID); err != nil {
		slog.Warn("GDPR: delete background_jobs failed", "error", err)
	}

	// DELETE user — CASCADE handles ~45+ dependent tables (see func doc comment).
	if _, err := tx.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID); err != nil {
		return fmt.Errorf("GDPR: delete user: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("GDPR: commit: %w", err)
	}

	slog.Info("GDPR purge complete", "user_id", userID,
		"activities", len(actIDs), "segments", len(segIDs))
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
