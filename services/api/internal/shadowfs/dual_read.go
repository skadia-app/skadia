package shadowfs

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ResolveActivityFile returns a reader for the activity's original file via ShadowFS.
func ResolveActivityFile(ctx context.Context, fs ShadowFS, pool *pgxpool.Pool, activityID, userID string) (io.ReadCloser, error) {
	var fileID *string
	err := pool.QueryRow(ctx,
		`SELECT file_id FROM activities WHERE id = $1 AND user_id = $2`,
		activityID, userID,
	).Scan(&fileID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("activity not found")
		}
		return nil, fmt.Errorf("query activity: %w", err)
	}

	if fileID == nil || *fileID == "" {
		return nil, fmt.Errorf("no file_id for activity %s", activityID)
	}

	return fs.Get(ctx, userID, *fileID)
}

// allowedMediaTables is the set of tables permitted in ResolveMediaFile queries.
var allowedMediaTables = map[string]bool{
	"activity_photos":    true,
	"gear_documents":     true,
	"route_photos":       true,
	"tags":               true,
	"custom_map_markers": true,
}

// allowedFileIDColumns is the set of file_id column names permitted in ResolveMediaFile queries.
var allowedFileIDColumns = map[string]bool{
	"file_id":          true,
	"document_file_id": true,
	"photo_file_id":    true,
	"icon_file_id":     true,
}

// ResolveMediaFile returns a reader for a media file (photo, document, etc.)
// by querying the given table for its file_id column.
//
// tableName and fileIDColumn must be static, trusted values —
// never pass user-supplied strings here.
func ResolveMediaFile(ctx context.Context, fs ShadowFS, pool *pgxpool.Pool, tableName, fileIDColumn, rowID, userID string) (io.ReadCloser, error) {
	if !allowedMediaTables[tableName] {
		panic("invariant violation: invalid table " + tableName)
	}
	if !allowedFileIDColumns[fileIDColumn] {
		panic("invariant violation: invalid column " + fileIDColumn)
	}

	var fileID *string
	query := fmt.Sprintf(`SELECT %s FROM %s WHERE id = $1 AND user_id = $2`, fileIDColumn, tableName)
	err := pool.QueryRow(ctx, query, rowID, userID).Scan(&fileID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("record not found")
		}
		return nil, fmt.Errorf("query %s: %w", tableName, err)
	}

	if fileID == nil || *fileID == "" {
		return nil, fmt.Errorf("no file_id for %s %s", tableName, rowID)
	}

	return fs.Get(ctx, userID, *fileID)
}
