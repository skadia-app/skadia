package shadowfs

import (
	"context"
	"hash/fnv"

	"github.com/jackc/pgx/v5/pgxpool"
)

// advisoryLockKey computes a 64-bit FNV-1a hash for use as a pg_advisory_lock key.
// Replaces PostgreSQL's hashtext() which returns only 32 bits (collision-prone with UUIDs).
func advisoryLockKey(s string) int64 {
	h := fnv.New64a()
	h.Write([]byte(s)) //nolint:errcheck
	return int64(h.Sum64())
}

// listRecords queries user_files from the DB. Used by both backends.
func listRecords(ctx context.Context, pool *pgxpool.Pool, userID string, fileType *string) ([]FileRecord, error) {
	query := `SELECT id, user_id, file_type, storage_backend, storage_path, content_type,
		file_name, size_bytes, file_hash, parent_type, parent_id, has_sidecar,
		is_renaming, status, replaced_by, created_at, updated_at
		FROM user_files WHERE user_id = $1`
	args := []any{userID}
	if fileType != nil {
		query += " AND file_type = $2"
		args = append(args, *fileType)
	}
	query += " ORDER BY created_at DESC"

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []FileRecord
	for rows.Next() {
		var r FileRecord
		if err := rows.Scan(&r.ID, &r.UserID, &r.FileType, &r.StorageBackend, &r.StoragePath,
			&r.ContentType, &r.FileName, &r.SizeBytes, &r.FileHash, &r.ParentType, &r.ParentID,
			&r.HasSidecar, &r.IsRenaming, &r.Status, &r.ReplacedBy, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// diskUsage sums size_bytes from active user_files only. Used by both backends.
// Archived and purging files are excluded from quota accounting.
func diskUsage(ctx context.Context, pool *pgxpool.Pool, userID string) (int64, error) {
	var total int64
	err := pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(size_bytes), 0) FROM user_files
		 WHERE user_id = $1 AND status = 'active'
		 AND file_type NOT IN ('thumb', 'preview')`, userID).Scan(&total)
	return total, err
}
