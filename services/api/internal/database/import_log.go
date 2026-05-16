package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// InsertImportLog inserts a new import_log entry and returns the generated ID.
func (db *DB) InsertImportLog(ctx context.Context, entry model.ImportLogEntry) (string, error) {
	var id string
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO import_log
			(user_id, source_platform, filename, file_hash, file_size_bytes,
			 status, activity_id, error_msg, duration_ms, metadata, batch_id, skip_reason,
			 import_type)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id`,
		entry.UserID, entry.SourcePlatform, entry.Filename, entry.FileHash,
		entry.FileSizeBytes, entry.Status, entry.ActivityID, entry.ErrorMsg,
		entry.DurationMs, entry.Metadata, entry.BatchID, entry.SkipReason,
		entry.ImportType,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert import log: %w", err)
	}
	return id, nil
}

// UpdateImportLogStatus updates the status and related fields of an import_log entry.
func (db *DB) UpdateImportLogStatus(ctx context.Context, id, status string, errorMsg *string, activityID *string, durationMs *int) error {
	result, err := db.Pool.Exec(ctx, `
		UPDATE import_log
		SET status = $2, error_msg = $3, activity_id = $4, duration_ms = $5
		WHERE id = $1`,
		id, status, errorMsg, activityID, durationMs,
	)
	if err != nil {
		return fmt.Errorf("update import log status: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("update import log status: row %s not found", id)
	}
	return nil
}

// UpdateImportLogFileHash writes a computed file hash back to the DB.
func (db *DB) UpdateImportLogFileHash(ctx context.Context, id, fileHash string) error {
	result, err := db.Pool.Exec(ctx, `
		UPDATE import_log SET file_hash = $2 WHERE id = $1`,
		id, fileHash,
	)
	if err != nil {
		return fmt.Errorf("update import log file_hash: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("update import log file_hash: row %s not found", id)
	}
	return nil
}

// UpdateImportLogSkipReason sets the skip_reason field of an import_log entry.
func (db *DB) UpdateImportLogSkipReason(ctx context.Context, id, skipReason string) error {
	result, err := db.Pool.Exec(ctx, `
		UPDATE import_log SET skip_reason = $2 WHERE id = $1`,
		id, skipReason,
	)
	if err != nil {
		return fmt.Errorf("update import log skip_reason: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("update import log skip_reason: row %s not found", id)
	}
	return nil
}

// UpdateImportLogMetadata sets the metadata field of an import_log entry.
func (db *DB) UpdateImportLogMetadata(ctx context.Context, id string, metadata []byte) error {
	result, err := db.Pool.Exec(ctx, `
		UPDATE import_log SET metadata = $2 WHERE id = $1`,
		id, metadata,
	)
	if err != nil {
		return fmt.Errorf("update import log metadata: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("update import log metadata: row %s not found", id)
	}
	return nil
}

func (db *DB) CompleteDocumentImportLog(ctx context.Context, userID, docID, fileID string, success bool, markdownStatus string) error {
	if userID == "" || docID == "" {
		return nil
	}
	status := "completed"
	stepStatus := "completed"
	var errorMsg *string
	if !success {
		status = "error"
		stepStatus = "failed"
		msg := "document processing failed"
		errorMsg = &msg
	}
	if markdownStatus != "" && markdownStatus != "completed" && markdownStatus != "failed" && markdownStatus != "skipped" {
		markdownStatus = ""
	}
	result, err := db.Pool.Exec(ctx, `
			UPDATE import_log AS l
			SET status = $4,
			    error_msg = $5,
			    duration_ms = GREATEST(0, (EXTRACT(EPOCH FROM (NOW() - l.created_at)) * 1000)::int),
			    metadata = COALESCE(l.metadata, '{}'::jsonb) || jsonb_build_object(
			      'document_processed', jsonb_build_object('success', $3::boolean, 'file_id', NULLIF($2, ''), 'markdown_status', NULLIF($8, '')),
			      'steps', COALESCE((
			        SELECT jsonb_agg(
			          CASE
			            WHEN step->>'key' = 'markdown' AND $8 <> '' THEN jsonb_set(step, '{status}', to_jsonb($8::text), true)
			            WHEN step->>'status' = 'pending' THEN jsonb_set(step, '{status}', to_jsonb($6::text), true)
			            ELSE step
			          END ORDER BY ordinality
			        )
			        FROM jsonb_array_elements(COALESCE(l.metadata->'steps', '[]'::jsonb)) WITH ORDINALITY AS s(step, ordinality)
			      ), '[]'::jsonb)
			    )
			WHERE l.user_id = $7
			  AND l.import_type = 'document'
			  AND l.status IN ('pending', 'processing')
			  AND l.metadata->>'target_id' = $1
			  AND ($2 = '' OR l.metadata->>'file_id' = $2)`,
		docID, fileID, success, status, errorMsg, stepStatus, userID, markdownStatus,
	)
	if err != nil {
		return fmt.Errorf("complete document import log: %w", err)
	}
	if result.RowsAffected() == 0 {
		return nil
	}
	return nil
}

func (db *DB) SetQueuedDocumentImportProcessing(ctx context.Context, userID, queueID string, metadata json.RawMessage) error {
	if userID == "" || queueID == "" {
		return nil
	}
	result, err := db.Pool.Exec(ctx, `
		UPDATE import_log
		SET status = 'processing',
		    error_msg = NULL,
		    metadata = COALESCE(metadata, '{}'::jsonb) || $3::jsonb
		WHERE user_id = $1
		  AND metadata->>'queue_id' = $2
		  AND status IN ('queued', 'completed')`,
		userID, queueID, metadata,
	)
	if err != nil {
		return fmt.Errorf("set queued document import processing: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("set queued document import processing: queue row %s not found", queueID)
	}
	return nil
}

func (db *DB) SetQueuedDocumentImportError(ctx context.Context, userID, queueID string, errMsg string) error {
	if userID == "" || queueID == "" {
		return nil
	}
	result, err := db.Pool.Exec(ctx, `
		UPDATE import_log
		SET status = 'error',
		    error_msg = $3
		WHERE user_id = $1
		  AND metadata->>'queue_id' = $2
		  AND status IN ('queued', 'completed', 'processing')`,
		userID, queueID, errMsg,
	)
	if err != nil {
		return fmt.Errorf("set queued document import error: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("set queued document import error: queue row %s not found", queueID)
	}
	return nil
}

// ListImportLog returns paginated import_log entries for a user, with optional filters.
// importType: "" = no filter; "unknown" = import_type IS NULL; other = import_type = value.
// Returns (entries, total, error).
func (db *DB) ListImportLog(ctx context.Context, userID string, statusFilter []string, sourcePlatform string, importType string, limit, offset int) ([]model.ImportLogEntry, int, error) {
	args := []any{userID}
	paramIdx := 2

	whereClause := "WHERE user_id = $1"

	if len(statusFilter) > 0 {
		placeholders := make([]string, len(statusFilter))
		for i, s := range statusFilter {
			placeholders[i] = fmt.Sprintf("$%d", paramIdx)
			args = append(args, s)
			paramIdx++
		}
		whereClause += " AND status IN (" + strings.Join(placeholders, ", ") + ")"
	}

	if sourcePlatform != "" {
		whereClause += fmt.Sprintf(" AND source_platform = $%d", paramIdx)
		args = append(args, sourcePlatform)
		paramIdx++
	}

	if importType == "unknown" {
		whereClause += " AND import_type IS NULL"
	} else if importType != "" {
		whereClause += fmt.Sprintf(" AND import_type = $%d", paramIdx)
		args = append(args, importType)
		paramIdx++
	}

	var total int
	countSQL := "SELECT COUNT(*) FROM import_log " + whereClause
	if err := db.Pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count import log: %w", err)
	}

	dataSQL := fmt.Sprintf(`
		SELECT id, user_id, source_platform, filename, file_hash, file_size_bytes,
		       status, activity_id, error_msg, duration_ms, metadata,
		       batch_id, skip_reason, import_type, created_at, updated_at
		FROM import_log
		%s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`,
		whereClause, paramIdx, paramIdx+1,
	)
	args = append(args, limit, offset)

	rows, err := db.Pool.Query(ctx, dataSQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list import log: %w", err)
	}
	defer rows.Close()

	entries := make([]model.ImportLogEntry, 0)
	for rows.Next() {
		var entry model.ImportLogEntry
		var metaBytes []byte
		if err := rows.Scan(
			&entry.ID, &entry.UserID, &entry.SourcePlatform, &entry.Filename,
			&entry.FileHash, &entry.FileSizeBytes, &entry.Status, &entry.ActivityID,
			&entry.ErrorMsg, &entry.DurationMs, &metaBytes, &entry.BatchID,
			&entry.SkipReason, &entry.ImportType, &entry.CreatedAt, &entry.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan import log: %w", err)
		}
		if metaBytes != nil {
			entry.Metadata = json.RawMessage(metaBytes)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("list import log rows: %w", err)
	}
	return entries, total, nil
}

// CleanupStaleProcessing marks timed-out 'processing' entries as 'error'.
// Returns the number of rows affected.
func (db *DB) CleanupStaleProcessing(ctx context.Context, olderThan time.Duration) (int, error) {
	cutoff := time.Now().Add(-olderThan)
	result, err := db.Pool.Exec(ctx, `
		UPDATE import_log
		SET status = 'error',
		    error_msg = 'processing timeout — server may have restarted'
		WHERE status = 'processing' AND updated_at < $1`,
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("cleanup stale processing: %w", err)
	}
	return int(result.RowsAffected()), nil
}
