package filestore

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ResolveActivityFile returns a reader for the activity's original file.
// Tries file_id first, falls back to legacy columns (local_file_path, S3 key).
//
// Sentinel error prefix "legacy_s3:<key>" is returned when only an S3 key is
// available; callers that still have an S3Client should check for this prefix
// and fall back to their own S3 read path. This keeps the helper free from an
// S3Client dependency.
func ResolveActivityFile(ctx context.Context, fs FileStore, pool *pgxpool.Pool, activityID, userID string) (io.ReadCloser, error) {
	var fileID *string
	var localPath *string
	var s3Key *string
	err := pool.QueryRow(ctx,
		`SELECT file_id, local_file_path, original_file_key
		 FROM activities WHERE id = $1 AND user_id = $2`,
		activityID, userID,
	).Scan(&fileID, &localPath, &s3Key)
	if err != nil {
		return nil, fmt.Errorf("activity not found")
	}

	// New path: file_id → FileStore
	if fileID != nil && *fileID != "" {
		return fs.Get(ctx, userID, *fileID)
	}

	// Legacy fallback 1: local file
	if localPath != nil && *localPath != "" {
		f, err := os.Open(*localPath)
		if err == nil {
			return f, nil
		}
	}

	// Legacy fallback 2: S3 — return sentinel so caller can use its S3Client
	if s3Key != nil && *s3Key != "" {
		return nil, fmt.Errorf("legacy_s3:%s", *s3Key)
	}

	return nil, fmt.Errorf("no file found for activity %s", activityID)
}

// ResolveMediaFile returns a reader for a media file (photo, document, etc.)
// by querying the given table for its file_id and legacy S3 key columns.
//
// tableName, fileIDColumn, and s3KeyColumn must be static, trusted values —
// never pass user-supplied strings here.
//
// Same "legacy_s3:<key>" sentinel convention as ResolveActivityFile applies.
func ResolveMediaFile(ctx context.Context, fs FileStore, pool *pgxpool.Pool, tableName, fileIDColumn, s3KeyColumn, rowID, userID string) (io.ReadCloser, error) {
	var fileID *string
	var s3Key *string
	query := fmt.Sprintf(
		`SELECT %s, %s FROM %s WHERE id = $1 AND user_id = $2`,
		fileIDColumn, s3KeyColumn, tableName,
	)
	err := pool.QueryRow(ctx, query, rowID, userID).Scan(&fileID, &s3Key)
	if err != nil {
		return nil, fmt.Errorf("record not found")
	}

	if fileID != nil && *fileID != "" {
		return fs.Get(ctx, userID, *fileID)
	}

	if s3Key != nil && *s3Key != "" {
		return nil, fmt.Errorf("legacy_s3:%s", *s3Key)
	}

	return nil, fmt.Errorf("no file found")
}
