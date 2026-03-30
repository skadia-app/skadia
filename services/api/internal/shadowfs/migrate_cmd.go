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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// migrateRecord holds a row from user_files that needs migration.
type migrateRecord struct {
	ID             string
	UserID         string
	FileType       string
	StorageBackend string
	StoragePath    string
	ContentType    string
	FileName       string
	SizeBytes      int64
	FileHash       string // may be empty (NULL)
	ParentType     string
	ParentID       *string
	HasSidecar     bool
}

// RunMigrateStorage is the entry point for the migrate-storage subcommand.
// It migrates all user_files rows whose storage_backend differs from targetName
// from source to target backend, verifying checksums after copy.
func RunMigrateStorage(
	ctx context.Context,
	pool *pgxpool.Pool,
	source, target StorageBackend,
	targetName string,
	bucketFn func(string) string,
	basePath string,
	renderer *SchemaRenderer,
	dryRun bool,
	batchSize, workers int,
	cleanupSource bool,
) error {
	// Determine source backend name.
	sourceName := "local"
	if targetName == "local" {
		sourceName = "s3"
	}

	if workers > 1 {
		fmt.Fprintf(os.Stderr, "WARNING: each worker buffers up to 500 MB in memory (S3 write). workers=%d → up to %d MB RAM\n", workers, workers*500)
	}

	// ── Dry-Run ──────────────────────────────────────────────────────────────
	if dryRun {
		return runDryRun(ctx, pool, source, target, sourceName, targetName)
	}

	// ── Set maintenance mode ──────────────────────────────────────────────────
	reason := fmt.Sprintf("migrate-storage %s→%s started at %s", sourceName, targetName, time.Now().UTC().Format(time.RFC3339))
	if _, err := pool.Exec(ctx,
		`INSERT INTO system_flags (key, value, expires_at)
		 VALUES ('maintenance_mode', $1, NOW() + INTERVAL '24 hours')
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW(), expires_at = EXCLUDED.expires_at`,
		reason); err != nil {
		return fmt.Errorf("migrate-storage: set maintenance_mode: %w", err)
	}
	slog.Info("migrate-storage: maintenance mode set", "reason", reason)

	// Clear maintenance mode on success (deferred, only runs on clean exit).
	success := false
	defer func() {
		if success {
			if _, err := pool.Exec(context.Background(),
				`DELETE FROM system_flags WHERE key = 'maintenance_mode'`); err != nil {
				slog.Error("migrate-storage: failed to clear maintenance_mode", "error", err)
			} else {
				slog.Info("migrate-storage: maintenance mode cleared")
			}
		} else {
			slog.Warn("migrate-storage: maintenance mode NOT cleared — migration did not complete successfully")
		}
	}()

	// ── Pre-Pass: fill NULL file_hash entries ─────────────────────────────────
	if err := fillNullHashes(ctx, pool, source, sourceName, bucketFn, batchSize); err != nil {
		return fmt.Errorf("migrate-storage: pre-pass hash fill: %w", err)
	}

	// ── Main migration loop ────────────────────────────────────────────────────
	var (
		totalMigrated int64
		totalBytes    int64
		totalErrors   int64
	)

	// Get grand total for progress display.
	var grandTotal int64
	_ = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_files WHERE storage_backend = $1 AND status != 'purging'`,
		sourceName).Scan(&grandTotal)

	start := time.Now()

	for {
		rows, err := fetchMigrateBatch(ctx, pool, sourceName, batchSize)
		if err != nil {
			return fmt.Errorf("migrate-storage: fetch batch: %w", err)
		}
		if len(rows) == 0 {
			break // Done.
		}

		if err := processBatch(ctx, pool, rows, source, target, targetName, sourceName,
			bucketFn, basePath, renderer, workers,
			&totalMigrated, &totalBytes, &totalErrors, grandTotal, start); err != nil {
			return fmt.Errorf("migrate-storage: process batch: %w", err)
		}
	}

	fmt.Fprintf(os.Stderr, "\nmigrate-storage: migration complete — %d files, %s, %d errors\n",
		totalMigrated, formatMigrBytes(totalBytes), totalErrors)

	if totalErrors > 0 {
		return fmt.Errorf("migrate-storage: %d files failed — check logs", totalErrors)
	}

	// ── Cleanup-Source pass ────────────────────────────────────────────────────
	if cleanupSource {
		fmt.Fprintf(os.Stderr, "migrate-storage: starting source cleanup pass...\n")
		if err := cleanupSourceFiles(ctx, pool, sourceName); err != nil {
			return fmt.Errorf("migrate-storage: cleanup-source: %w", err)
		}
	}

	success = true
	return nil
}

// runDryRun prints statistics about files that would be migrated.
func runDryRun(
	ctx context.Context,
	pool *pgxpool.Pool,
	source, target StorageBackend,
	sourceName, targetName string,
) error {
	var count int64
	var totalBytes int64
	err := pool.QueryRow(ctx,
		`SELECT COUNT(*), COALESCE(SUM(size_bytes), 0)
		 FROM user_files WHERE storage_backend = $1 AND status != 'purging'`,
		sourceName).Scan(&count, &totalBytes)
	if err != nil {
		return fmt.Errorf("dry-run: count: %w", err)
	}

	fmt.Fprintf(os.Stderr, "dry-run: %d files to migrate (%s) from %s → %s\n",
		count, formatMigrBytes(totalBytes), sourceName, targetName)

	// Check target reachability with a probe Exists call.
	if exists, err := target.Exists(ctx, "migrate-storage-probe-do-not-create", nil); err != nil {
		fmt.Fprintf(os.Stderr, "dry-run: target backend (%s) reachability check FAILED: %v\n", targetName, err)
	} else {
		_ = exists
		fmt.Fprintf(os.Stderr, "dry-run: target backend (%s) reachable\n", targetName)
	}

	return nil
}

// fillNullHashes fills NULL file_hash values by reading + hashing from source.
func fillNullHashes(
	ctx context.Context,
	pool *pgxpool.Pool,
	source StorageBackend,
	sourceName string,
	bucketFn func(string) string,
	batchSize int,
) error {
	var filled int64
	for {
		rows, err := pool.Query(ctx,
			`SELECT id, file_type, storage_path
			 FROM user_files
			 WHERE storage_backend = $1 AND file_hash IS NULL AND status != 'purging'
			 LIMIT $2`,
			sourceName, batchSize)
		if err != nil {
			return fmt.Errorf("fillNullHashes: query: %w", err)
		}

		type hashRow struct {
			id          string
			fileType    string
			storagePath string
		}
		var batch []hashRow
		for rows.Next() {
			var r hashRow
			if err := rows.Scan(&r.id, &r.fileType, &r.storagePath); err != nil {
				rows.Close()
				return fmt.Errorf("fillNullHashes: scan: %w", err)
			}
			batch = append(batch, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		if len(batch) == 0 {
			break
		}

		for _, rec := range batch {
			opts := migrBucketOpts(bucketFn, rec.fileType)
			rc, err := source.Read(ctx, rec.storagePath, opts)
			if err != nil {
				slog.Warn("fillNullHashes: read failed — skipping", "id", rec.id, "path", rec.storagePath, "error", err)
				continue
			}
			h := sha256.New()
			if _, err := io.Copy(h, rc); err != nil {
				rc.Close()
				slog.Warn("fillNullHashes: hash failed — skipping", "id", rec.id, "error", err)
				continue
			}
			rc.Close()

			hash := hex.EncodeToString(h.Sum(nil))
			if _, err := pool.Exec(ctx,
				`UPDATE user_files SET file_hash = $1 WHERE id = $2`,
				hash, rec.id); err != nil {
				slog.Warn("fillNullHashes: update failed", "id", rec.id, "error", err)
				continue
			}
			filled++
		}

		fmt.Fprintf(os.Stderr, "pre-pass: filled %d hashes so far...\r", filled)
	}

	if filled > 0 {
		fmt.Fprintf(os.Stderr, "\npre-pass: filled %d NULL file_hash entries\n", filled)
	}
	return nil
}

// fetchMigrateBatch fetches the next batch of files that need migration.
func fetchMigrateBatch(ctx context.Context, pool *pgxpool.Pool, sourceName string, batchSize int) ([]migrateRecord, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, user_id, file_type, storage_backend, storage_path, content_type,
		 file_name, size_bytes, COALESCE(file_hash, ''),
		 COALESCE(parent_type, ''), parent_id, has_sidecar
		 FROM user_files
		 WHERE storage_backend = $1 AND status != 'purging'
		 ORDER BY size_bytes ASC
		 LIMIT $2`,
		sourceName, batchSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var batch []migrateRecord
	for rows.Next() {
		var r migrateRecord
		var parentType string
		if err := rows.Scan(&r.ID, &r.UserID, &r.FileType, &r.StorageBackend, &r.StoragePath,
			&r.ContentType, &r.FileName, &r.SizeBytes, &r.FileHash,
			&parentType, &r.ParentID, &r.HasSidecar); err != nil {
			return nil, err
		}
		r.ParentType = parentType
		batch = append(batch, r)
	}
	return batch, rows.Err()
}

// processBatch copies a batch of files from source to target using workers goroutines.
func processBatch(
	ctx context.Context,
	pool *pgxpool.Pool,
	batch []migrateRecord,
	source, target StorageBackend,
	targetName, sourceName string,
	bucketFn func(string) string,
	basePath string,
	renderer *SchemaRenderer,
	workers int,
	totalMigrated, totalBytes, totalErrors *int64,
	grandTotal int64,
	start time.Time,
) error {
	ch := make(chan migrateRecord, len(batch))
	for _, rec := range batch {
		ch <- rec
	}
	close(ch)

	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for rec := range ch {
				if ctx.Err() != nil {
					return
				}
				if err := migrateFile(ctx, pool, rec, source, target, targetName,
					bucketFn, basePath, renderer); err != nil {
					slog.Error("migrate-storage: file failed",
						"id", rec.ID, "path", rec.StoragePath, "error", err)
					atomic.AddInt64(totalErrors, 1)
				} else {
					m := atomic.AddInt64(totalMigrated, 1)
					b := atomic.AddInt64(totalBytes, rec.SizeBytes)
					printMigrProgress(m, grandTotal, b, start)
				}
			}
		})
	}

	wg.Wait()
	return nil
}

// migrateFile copies one file from source to target and updates user_files.
func migrateFile(
	ctx context.Context,
	pool *pgxpool.Pool,
	rec migrateRecord,
	source, target StorageBackend,
	targetName string,
	bucketFn func(string) string,
	basePath string,
	renderer *SchemaRenderer,
) error {
	srcOpts := migrBucketOpts(bucketFn, rec.FileType)

	// Compute target path.
	targetPath, targetOpts, err := computeTargetPath(ctx, pool, rec, targetName, bucketFn, renderer)
	if err != nil {
		return fmt.Errorf("compute target path: %w", err)
	}

	// Check if already exists at target (idempotent).
	if exists, err := target.Exists(ctx, targetPath, targetOpts); err == nil && exists {
		return updateMigrRecord(ctx, pool, rec.ID, targetName, targetPath)
	}

	// Read from source.
	rc, err := source.Read(ctx, rec.StoragePath, srcOpts)
	if err != nil {
		return fmt.Errorf("read source %q: %w", rec.StoragePath, err)
	}
	defer rc.Close()

	// Hash-verifying reader.
	h := sha256.New()
	reader := io.TeeReader(rc, h)

	// Write to target.
	_, writtenHash, err := target.Write(ctx, targetPath, reader, rec.ContentType, false, targetOpts)
	if err != nil {
		return fmt.Errorf("write target %q: %w", targetPath, err)
	}

	// Verify hash.
	computedHash := hex.EncodeToString(h.Sum(nil))
	if rec.FileHash != "" && computedHash != rec.FileHash {
		_ = target.Delete(ctx, targetPath, targetOpts)
		return fmt.Errorf("hash mismatch: expected %s got %s", rec.FileHash, computedHash)
	}
	if writtenHash != "" && rec.FileHash != "" && writtenHash != rec.FileHash {
		_ = target.Delete(ctx, targetPath, targetOpts)
		return fmt.Errorf("target hash mismatch: expected %s got %s", rec.FileHash, writtenHash)
	}

	// Persist final hash (use known or computed).
	finalHash := rec.FileHash
	if finalHash == "" {
		finalHash = computedHash
	}

	if _, err := pool.Exec(ctx,
		`UPDATE user_files SET storage_backend = $1, storage_path = $2, file_hash = $3, updated_at = NOW()
		 WHERE id = $4`,
		targetName, targetPath, finalHash, rec.ID); err != nil {
		if isUniqueViolation(err) {
			slog.Info("migrate-storage: duplicate path on write — marking as migrated",
				"id", rec.ID, "target_path", targetPath)
			// Path collision: just flip the backend so the file exits the migration queue.
			_, fallbackErr := pool.Exec(ctx,
				`UPDATE user_files SET storage_backend = $1, updated_at = NOW() WHERE id = $2`,
				targetName, rec.ID)
			return fallbackErr
		}
		return fmt.Errorf("update DB record: %w", err)
	}

	// Copy companion sidecar if present (best-effort).
	if rec.HasSidecar {
		companionSrc := rec.StoragePath + ".meta"
		companionDst := targetPath + ".meta"
		srcRC, readErr := source.Read(ctx, companionSrc, srcOpts)
		if readErr == nil {
			_, _, writeErr := target.Write(ctx, companionDst, srcRC, "application/json", false, targetOpts)
			srcRC.Close()
			if writeErr != nil {
				slog.Warn("migrate-storage: companion copy failed", "src", companionSrc, "error", writeErr)
			}
		}
	}

	slog.Debug("migrate-storage: migrated file",
		"id", rec.ID, "to", targetName, "path", targetPath)
	return nil
}

// computeTargetPath computes the storage path on the target backend.
//
//   - Local→S3: hash-based key  {userID}/{dir}/{hash}{ext}
//   - S3→Local: schema-based    {userDir}/{dir}/{hash}{ext}  (fallback: ID-based)
func computeTargetPath(
	ctx context.Context,
	pool *pgxpool.Pool,
	rec migrateRecord,
	targetName string,
	bucketFn func(string) string,
	renderer *SchemaRenderer,
) (path string, opts *BackendOpts, err error) {
	ext := sanitizeExt(filepath.Ext(rec.FileName))

	switch targetName {
	case "s3":
		// Hash-based key for S3: {userID}/{dir}/{hash}{ext}
		dir := DirForType(rec.FileType)
		key := rec.FileHash
		if key == "" {
			key = rec.ID // fallback (should not happen after pre-pass)
		}
		path = fmt.Sprintf("%s/%s/%s%s", rec.UserID, dir, key, ext)
		if b := bucketFn(rec.FileType); b != "" {
			opts = &BackendOpts{Bucket: b}
		}

	case "local":
		// Schema-based path: look up user's library_dir_name.
		var dirName *string
		if qErr := pool.QueryRow(ctx,
			`SELECT library_dir_name FROM users WHERE id = $1`, rec.UserID).Scan(&dirName); qErr != nil {
			return "", nil, fmt.Errorf("getUserDir: %w", qErr)
		}
		userDir := rec.UserID
		if dirName != nil && *dirName != "" {
			userDir = *dirName
		}
		if strings.Contains(userDir, "..") || strings.Contains(userDir, "/") || strings.Contains(userDir, "\\") {
			return "", nil, fmt.Errorf("invalid library_dir_name: %q", userDir)
		}

		dir := DirForType(rec.FileType)
		key := rec.FileHash
		if key == "" {
			key = rec.ID
		}

		// Try schema-based path when we have enough tags.
		if renderer != nil && rec.ParentType != "" {
			schema := renderer.DefaultSchema(rec.FileType)
			if schema != "" {
				tags := extractTagsForFile(ctx, pool, rec.FileType, rec.ParentType, rec.ParentID)
				if len(tags) > 0 {
					rendered := renderer.RenderPath(rec.FileType, schema, tags, ext)
					path = filepath.Join(userDir, dir, rendered)
					return path, nil, nil
				}
			}
		}

		// Fallback: hash-based local path.
		path = filepath.Join(userDir, dir, key+ext)

	default:
		return "", nil, fmt.Errorf("unknown target backend: %q", targetName)
	}

	return path, opts, nil
}

// updateMigrRecord updates only the backend/path (file already present at target).
// Returns nil on unique_violation (23505) — the file was already migrated by another path.
func updateMigrRecord(ctx context.Context, pool *pgxpool.Pool, id, targetName, targetPath string) error {
	_, err := pool.Exec(ctx,
		`UPDATE user_files SET storage_backend = $1, storage_path = $2, updated_at = NOW()
		 WHERE id = $3`,
		targetName, targetPath, id)
	if isUniqueViolation(err) {
		slog.Info("migrate-storage: duplicate path — marking as already migrated",
			"id", id, "target_path", targetPath)
		// Path collision: another row already occupies this (backend, path).
		// Just flip the backend so this file is no longer picked up.
		_, err = pool.Exec(ctx,
			`UPDATE user_files SET storage_backend = $1, updated_at = NOW() WHERE id = $2`,
			targetName, id)
		return err
	}
	return err
}

// isUniqueViolation returns true if err is a PostgreSQL unique_violation (23505).
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return true
	}
	return false
}

// cleanupSourceFiles verifies all files have been migrated away from sourceName.
// Physical deletion of objects must be done via the storage backend's own tooling
// (e.g., `garage bucket purge` or `find /data/library -delete`) since the DB
// records have already been updated to the target backend.
func cleanupSourceFiles(ctx context.Context, pool *pgxpool.Pool, sourceName string) error {
	var count int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_files WHERE storage_backend = $1 AND status != 'purging'`,
		sourceName).Scan(&count); err != nil {
		return fmt.Errorf("cleanup: count remaining: %w", err)
	}
	if count > 0 {
		fmt.Fprintf(os.Stderr, "cleanup-source: WARNING: %d files still registered on %s backend — migration may be incomplete\n", count, sourceName)
	} else {
		fmt.Fprintf(os.Stderr, "cleanup-source: all files migrated from %s backend\n", sourceName)
		fmt.Fprintf(os.Stderr, "cleanup-source: physical objects can now be removed from %s using native tooling\n", sourceName)
	}
	return nil
}

// migrBucketOpts constructs BackendOpts with the correct S3 bucket for a file type.
func migrBucketOpts(bucketFn func(string) string, fileType string) *BackendOpts {
	if bucketFn == nil {
		return nil
	}
	b := bucketFn(fileType)
	if b == "" {
		return nil
	}
	return &BackendOpts{Bucket: b}
}

// printMigrProgress prints an ASCII progress bar to stderr.
// Example: [=====>    ] 312/1654 (18.9%) — 4.2 GB / 22.1 GB — ETA: 12m
func printMigrProgress(done, total, bytes int64, start time.Time) {
	if total <= 0 {
		fmt.Fprintf(os.Stderr, "\rmigrated: %d files, %s", done, formatMigrBytes(bytes))
		return
	}

	pct := float64(done) / float64(total) * 100
	elapsed := time.Since(start)
	var eta string
	if done > 0 && pct < 100 {
		remaining := float64(elapsed) * (float64(total) - float64(done)) / float64(done)
		eta = formatMigrDuration(time.Duration(remaining))
	} else {
		eta = "done"
	}

	const barWidth = 20
	filled := min(int(pct/100*barWidth), barWidth)
	bar := strings.Repeat("=", filled)
	if filled < barWidth {
		bar += ">"
		bar += strings.Repeat(" ", barWidth-filled-1)
	}

	fmt.Fprintf(os.Stderr, "\r[%s] %d/%d (%.1f%%) — %s — ETA: %s    ",
		bar, done, total, pct, formatMigrBytes(bytes), eta)
}

// formatMigrBytes formats byte counts for human display.
func formatMigrBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// formatMigrDuration formats a duration for human display.
func formatMigrDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}

// extractTagsForFile queries domain tables for schema rendering tags.
// Used by S3→Local migration to compute schema-based local paths.
//
// Returns nil when parentID is nil/empty — callers should then fall back to
// a flat path of the form {userDir}/{dir}/{hash}{ext}.
func extractTagsForFile(ctx context.Context, pool *pgxpool.Pool, fileType, parentType string, parentID *string) map[string]string {
	if parentID == nil || *parentID == "" {
		return nil
	}
	tags := make(map[string]string)

	switch parentType {
	case "activity":
		var startedAt time.Time
		var sportType, subType, title, city string
		var distanceM float64
		var elapsedTimeS int
		var format string
		err := pool.QueryRow(ctx,
			`SELECT started_at, sport_type,
			        COALESCE(sub_type, ''), COALESCE(title, ''),
			        COALESCE(city, ''), COALESCE(distance_m, 0),
			        COALESCE(elapsed_time_s, 0),
			        COALESCE(original_file_format, '')
			 FROM activities WHERE id = $1`, *parentID,
		).Scan(&startedAt, &sportType, &subType, &title, &city,
			&distanceM, &elapsedTimeS, &format)
		if err == nil {
			tags["year"] = startedAt.Format("2006")
			tags["month"] = startedAt.Format("01")
			tags["day"] = startedAt.Format("02")
			tags["time"] = startedAt.Format("1504")
			tags["sport"] = sportType
			if subType != "" {
				tags["subCategory"] = subType
			}
			if title != "" {
				tags["title"] = title
			}
			if city != "" {
				tags["city"] = city
			}
			if distanceM > 0 {
				tags["distance_km"] = strconv.FormatFloat(distanceM/1000.0, 'f', 1, 64)
			}
			if elapsedTimeS > 0 {
				h := elapsedTimeS / 3600
				m := (elapsedTimeS % 3600) / 60
				tags["duration_h"] = strconv.Itoa(h) + "-" + strconv.Itoa(m)
			}
			if format != "" {
				tags["format"] = format
			}
		}

	case "route":
		var title, sportType string
		err := pool.QueryRow(ctx,
			`SELECT COALESCE(title, ''), COALESCE(sport_type, '')
			 FROM routes WHERE id = $1`, *parentID,
		).Scan(&title, &sportType)
		if err == nil {
			if title != "" {
				tags["title"] = title
			}
			if sportType != "" {
				tags["sport"] = sportType
			}
		}

	case "gear":
		var name, category string
		err := pool.QueryRow(ctx,
			`SELECT COALESCE(name, ''), COALESCE(category, '')
			 FROM gear WHERE id = $1`, *parentID,
		).Scan(&name, &category)
		if err == nil {
			if name != "" {
				tags["title"] = name
			}
			if category != "" {
				tags["category"] = category
			}
		}

	case "gear_component":
		var name, category string
		err := pool.QueryRow(ctx,
			`SELECT COALESCE(name, ''), COALESCE(category, '')
			 FROM gear_components WHERE id = $1`, *parentID,
		).Scan(&name, &category)
		if err == nil {
			if name != "" {
				tags["title"] = name
			}
			if category != "" {
				tags["category"] = category
			}
		}

	case "tag":
		var name string
		err := pool.QueryRow(ctx,
			`SELECT COALESCE(name, '') FROM tags WHERE id = $1`, *parentID,
		).Scan(&name)
		if err == nil && name != "" {
			tags["title"] = name
		}

	case "marker":
		var name string
		err := pool.QueryRow(ctx,
			`SELECT COALESCE(name, '') FROM markers WHERE id = $1`, *parentID,
		).Scan(&name)
		if err == nil && name != "" {
			tags["title"] = name
		}

	case "photo":
		// Photos are media attached to an activity; pull tags from the activity.
		var activityID string
		err := pool.QueryRow(ctx,
			`SELECT COALESCE(activity_id::text, '') FROM activity_photos WHERE id = $1`, *parentID,
		).Scan(&activityID)
		if err == nil && activityID != "" {
			return extractTagsForFile(ctx, pool, fileType, "activity", &activityID)
		}
	}

	// Always include the current year/month as fallback so schema variables
	// never render as "unknown" for parent types we have partial data on.
	if _, ok := tags["year"]; !ok {
		now := time.Now()
		tags["year"] = now.Format("2006")
		tags["month"] = now.Format("01")
		tags["day"] = now.Format("02")
	}

	return tags
}
