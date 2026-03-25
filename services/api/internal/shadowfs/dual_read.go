package shadowfs

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

// LegacyS3Error is returned when only a legacy S3 key is available for a file.
// Callers that still have an S3Client should check for this error type using
// errors.As and fall back to their own S3 read path.
type LegacyS3Error struct {
	Key string
}

func (e *LegacyS3Error) Error() string { return "legacy s3: " + e.Key }

// ResolveActivityFile returns a reader for the activity's original file.
// Tries file_id first, falls back to legacy columns (local_file_path, S3 key).
//
// A *LegacyS3Error is returned when only an S3 key is available; callers that
// still have an S3Client should check for this error type using errors.As and
// fall back to their own S3 read path. This keeps the helper free from an
// S3Client dependency.
func ResolveActivityFile(ctx context.Context, fs ShadowFS, pool *pgxpool.Pool, activityID, userID string) (io.ReadCloser, error) {
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

	// New path: file_id → ShadowFS
	if fileID != nil && *fileID != "" {
		return fs.Get(ctx, userID, *fileID)
	}

	// Legacy fallback: direct filesystem path from pre-ShadowFS era.
	// No validateLocalPath check — this column is dropped in migration 000233.
	// Until then, the path was written by trusted server code (ShadowFS).
	if localPath != nil && *localPath != "" {
		f, err := os.Open(*localPath)
		if err == nil {
			return f, nil
		}
	}

	// Legacy fallback 2: S3 — return typed error so caller can use its S3Client
	if s3Key != nil && *s3Key != "" {
		return nil, &LegacyS3Error{Key: *s3Key}
	}

	return nil, fmt.Errorf("no file found for activity %s", activityID)
}

// allowedMediaTables is the set of tables permitted in ResolveMediaFile queries.
var allowedMediaTables = map[string]bool{
	"activity_photos":    true,
	"gear_documents":     true,
	"route_photos":       true,
	"tags":               true,
	"custom_map_markers": true,
}

// allowedColumns is the set of column names permitted in ResolveMediaFile queries.
var allowedColumns = map[string]bool{
	"file_id":            true,
	"s3_key":             true,
	"original_file_key":  true,
	"thumbnail_file_id":  true,
	"thumbnail_s3_key":   true,
	"preview_file_id":    true,
	"preview_s3_key":     true,
	"document_file_id":   true,
	"document_s3_key":    true,
	"photo_file_id":      true,
	"photo_s3_key":       true,
	"icon_file_id":       true,
	"icon_s3_key":        true,
}

// ResolveMediaFile returns a reader for a media file (photo, document, etc.)
// by querying the given table for its file_id and legacy S3 key columns.
//
// tableName, fileIDColumn, and s3KeyColumn must be static, trusted values —
// never pass user-supplied strings here.
//
// Same *LegacyS3Error convention as ResolveActivityFile applies.
func ResolveMediaFile(ctx context.Context, fs ShadowFS, pool *pgxpool.Pool, tableName, fileIDColumn, s3KeyColumn, rowID, userID string) (io.ReadCloser, error) {
	if !allowedMediaTables[tableName] {
		return nil, fmt.Errorf("invalid table name")
	}
	if !allowedColumns[fileIDColumn] {
		return nil, fmt.Errorf("invalid file_id column: %q", fileIDColumn)
	}
	if !allowedColumns[s3KeyColumn] {
		return nil, fmt.Errorf("invalid s3_key column: %q", s3KeyColumn)
	}

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
		return nil, &LegacyS3Error{Key: *s3Key}
	}

	return nil, fmt.Errorf("no file found")
}
