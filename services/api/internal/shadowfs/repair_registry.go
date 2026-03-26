package shadowfs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"
)

// repairRow holds a single user_files entry that needs metadata repair.
type repairRow struct {
	ID             string
	StorageBackend string
	StoragePath    string
	FileType       string
	SizeBytes      int64
	FileHash       string // may be empty (NULL)
}

// RunRepairRegistry reads every user_files entry with size_bytes=0 or file_hash IS NULL,
// fetches the file from the correct backend, computes SHA256 + size, and updates the row.
//
// Files that cannot be read (not found on backend) are logged and skipped — rows are NOT deleted.
func RunRepairRegistry(
	ctx context.Context,
	pool *pgxpool.Pool,
	local, s3 StorageBackend,
	bucketFn func(string) string,
) error {
	// Count total rows to repair for progress display.
	var total int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_files WHERE size_bytes = 0 OR file_hash IS NULL`,
	).Scan(&total); err != nil {
		return fmt.Errorf("repair-registry: count: %w", err)
	}
	fmt.Fprintf(os.Stderr, "repair-registry: %d rows need repair\n", total)
	if total == 0 {
		fmt.Fprintf(os.Stderr, "repair-registry: nothing to do\n")
		return nil
	}

	rows, err := pool.Query(ctx,
		`SELECT id, storage_backend, storage_path, file_type,
		        size_bytes, COALESCE(file_hash, '')
		 FROM user_files
		 WHERE size_bytes = 0 OR file_hash IS NULL
		 ORDER BY id`)
	if err != nil {
		return fmt.Errorf("repair-registry: query: %w", err)
	}
	defer rows.Close()

	var batch []repairRow
	for rows.Next() {
		var r repairRow
		if err := rows.Scan(&r.ID, &r.StorageBackend, &r.StoragePath, &r.FileType,
			&r.SizeBytes, &r.FileHash); err != nil {
			return fmt.Errorf("repair-registry: scan: %w", err)
		}
		batch = append(batch, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("repair-registry: rows: %w", err)
	}
	rows.Close()

	var processed, errors, skipped int64

	for _, rec := range batch {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err := repairOne(ctx, pool, rec, local, s3, bucketFn); err != nil {
			if isNotFound(err) {
				slog.Warn("repair-registry: file not found on backend — skipping",
					"id", rec.ID,
					"backend", rec.StorageBackend,
					"path", rec.StoragePath)
				skipped++
			} else {
				slog.Error("repair-registry: repair failed",
					"id", rec.ID,
					"backend", rec.StorageBackend,
					"path", rec.StoragePath,
					"error", err)
				errors++
			}
		} else {
			processed++
		}

		done := processed + errors + skipped
		if done%100 == 0 || done == total {
			fmt.Fprintf(os.Stderr, "repair-registry: %d/%d processed, %d errors, %d skipped\n",
				done, total, errors, skipped)
		}
	}

	fmt.Fprintf(os.Stderr, "\nrepair-registry: done — %d repaired, %d errors, %d skipped (not found)\n",
		processed, errors, skipped)

	if errors > 0 {
		return fmt.Errorf("repair-registry: %d files failed — check logs", errors)
	}
	return nil
}

// repairOne fetches a single file, computes its SHA256 + size, and updates user_files.
func repairOne(
	ctx context.Context,
	pool *pgxpool.Pool,
	rec repairRow,
	local, s3 StorageBackend,
	bucketFn func(string) string,
) error {
	var rc io.ReadCloser
	var err error

	// Legacy ShadowFS entries store absolute paths (e.g. /data/shadow-fs/…).
	// The LocalBackend rejects absolute paths via its path-traversal guard,
	// so we open them directly via the OS.
	if rec.StorageBackend == "local" && filepath.IsAbs(rec.StoragePath) {
		rc, err = os.Open(rec.StoragePath)
		if err != nil {
			return err // caller checks isNotFound via os.IsNotExist
		}
	} else {
		// Normal path: route through the registered backend.
		var be StorageBackend
		switch rec.StorageBackend {
		case "local":
			be = local
		case "s3":
			be = s3
		default:
			return fmt.Errorf("unknown storage_backend: %q", rec.StorageBackend)
		}
		if be == nil {
			return fmt.Errorf("backend %q is not configured", rec.StorageBackend)
		}

		opts := migrBucketOpts(bucketFn, rec.FileType)
		rc, err = be.Read(ctx, rec.StoragePath, opts)
		if err != nil {
			return err // caller checks isNotFound
		}
	}
	defer rc.Close()

	// Stream through SHA256 hasher + byte counter.
	h := sha256.New()
	n, err := io.Copy(h, rc)
	if err != nil {
		return fmt.Errorf("hash/count: %w", err)
	}

	hash := hex.EncodeToString(h.Sum(nil))

	// Determine which fields actually need updating to avoid spurious writes.
	needsSize := rec.SizeBytes == 0
	needsHash := rec.FileHash == ""

	switch {
	case needsSize && needsHash:
		_, err = pool.Exec(ctx,
			`UPDATE user_files SET size_bytes = $1, file_hash = $2, updated_at = NOW() WHERE id = $3`,
			n, hash, rec.ID)
	case needsSize:
		_, err = pool.Exec(ctx,
			`UPDATE user_files SET size_bytes = $1, updated_at = NOW() WHERE id = $2`,
			n, rec.ID)
	case needsHash:
		_, err = pool.Exec(ctx,
			`UPDATE user_files SET file_hash = $1, updated_at = NOW() WHERE id = $2`,
			hash, rec.ID)
	}
	if err != nil {
		return fmt.Errorf("db update: %w", err)
	}

	slog.Debug("repair-registry: repaired",
		"id", rec.ID,
		"size_bytes", n,
		"file_hash", hash)
	return nil
}

// isNotFound checks whether an error indicates the file does not exist on the backend.
// Both local (os.ErrNotExist) and S3 (NoSuchKey) errors are handled.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	// os.ErrNotExist covers local backend.
	if os.IsNotExist(err) {
		return true
	}
	// S3/Garage returns errors whose string contains "NoSuchKey" or "not found".
	msg := err.Error()
	for _, needle := range []string{"NoSuchKey", "not found", "no such file", "404"} {
		if containsCI(msg, needle) {
			return true
		}
	}
	return false
}

// containsCI is a simple case-sensitive substring search (strings.Contains alias kept explicit).
func containsCI(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub ||
		func() bool {
			for i := range len(s) - len(sub) + 1 {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
