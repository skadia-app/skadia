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
			 status, activity_id, error_msg, duration_ms, metadata, batch_id, skip_reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id`,
		entry.UserID,
		entry.SourcePlatform,
		entry.Filename,
		entry.FileHash,
		entry.FileSizeBytes,
		entry.Status,
		entry.ActivityID,
		entry.ErrorMsg,
		entry.DurationMs,
		entry.Metadata,
		entry.BatchID,
		entry.SkipReason,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert import log: %w", err)
	}
	return id, nil
}

// UpdateImportLogStatus updates the status and related fields of an import_log entry.
func (db *DB) UpdateImportLogStatus(ctx context.Context, id, status string, errorMsg *string, activityID *string, durationMs *int) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE import_log
		SET status = $2, error_msg = $3, activity_id = $4, duration_ms = $5, updated_at = NOW()
		WHERE id = $1`,
		id, status, errorMsg, activityID, durationMs,
	)
	if err != nil {
		return fmt.Errorf("update import log status: %w", err)
	}
	return nil
}

// UpdateImportLogFileHash writes a computed file hash back to the DB.
func (db *DB) UpdateImportLogFileHash(ctx context.Context, id, fileHash string) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE import_log SET file_hash = $2, updated_at = NOW() WHERE id = $1`,
		id, fileHash,
	)
	return err
}

// UpdateImportLogSkipReason sets the skip_reason field of an import_log entry.
func (db *DB) UpdateImportLogSkipReason(ctx context.Context, id, skipReason string) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE import_log SET skip_reason = $2, updated_at = NOW() WHERE id = $1`,
		id, skipReason,
	)
	return err
}

// UpdateImportLogMetadata sets the metadata field of an import_log entry.
func (db *DB) UpdateImportLogMetadata(ctx context.Context, id string, metadata []byte) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE import_log SET metadata = $2, updated_at = NOW() WHERE id = $1`,
		id, metadata,
	)
	if err != nil {
		return fmt.Errorf("update import log metadata: %w", err)
	}
	return nil
}

// ListImportLog returns paginated import_log entries for a user, with optional filters.
// Returns (entries, total, error).
func (db *DB) ListImportLog(ctx context.Context, userID string, statusFilter []string, sourcePlatform string, limit, offset int) ([]model.ImportLogEntry, int, error) {
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

	// COUNT query
	var total int
	countSQL := "SELECT COUNT(*) FROM import_log " + whereClause
	if err := db.Pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count import log: %w", err)
	}

	// Data query
	dataSQL := fmt.Sprintf(`
		SELECT id, user_id, source_platform, filename, file_hash, file_size_bytes,
		       status, activity_id, error_msg, duration_ms, metadata,
		       batch_id, skip_reason, created_at, updated_at
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
			&entry.ID,
			&entry.UserID,
			&entry.SourcePlatform,
			&entry.Filename,
			&entry.FileHash,
			&entry.FileSizeBytes,
			&entry.Status,
			&entry.ActivityID,
			&entry.ErrorMsg,
			&entry.DurationMs,
			&metaBytes,
			&entry.BatchID,
			&entry.SkipReason,
			&entry.CreatedAt,
			&entry.UpdatedAt,
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
		    error_msg = 'processing timeout — server may have restarted',
		    updated_at = NOW()
		WHERE status = 'processing' AND updated_at < $1`,
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("cleanup stale processing: %w", err)
	}
	return int(result.RowsAffected()), nil
}
