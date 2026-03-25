package filestore

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ReconcileJob performs consistency checks between the filesystem and the
// user_files registry. It is designed to be run periodically (daily cron)
// or triggered manually via the admin API.
type ReconcileJob struct {
	pool         *pgxpool.Pool
	basePath     string
	renderer     *SchemaRenderer
	keepVersions int
}

// ReconcileResult summarises all findings from a reconciliation run.
type ReconcileResult struct {
	OrphanedFiles   int      `json:"orphaned_files"`
	DanglingRecords int      `json:"dangling_records"`
	SizeMismatches  int      `json:"size_mismatches"`
	ArchivedCleaned int      `json:"archived_cleaned"`
	TempCleaned     int      `json:"temp_cleaned"`
	DuplicatePaths  int      `json:"duplicate_paths"`
	PurgingCleaned  int      `json:"purging_cleaned"`
	RenameFixed     int      `json:"rename_fixed"`
	ArchiveWarnings []string `json:"archive_warnings,omitempty"` // users with >1GB archives
}

// NewReconcileJob creates a new reconciliation job.
func NewReconcileJob(pool *pgxpool.Pool, basePath string, renderer *SchemaRenderer, keepVersions int) *ReconcileJob {
	return &ReconcileJob{
		pool:         pool,
		basePath:     basePath,
		renderer:     renderer,
		keepVersions: keepVersions,
	}
}

// Run executes all 9 reconciliation checks and returns the combined result.
// The provided progressFn is called after each check with the current check
// index (1-9) so callers can update background_jobs progress.
func (j *ReconcileJob) Run(ctx context.Context, progressFn func(current int)) (ReconcileResult, error) {
	var result ReconcileResult

	type check struct {
		name string
		fn   func(ctx context.Context, r *ReconcileResult) error
	}

	checks := []check{
		{"orphaned_files", j.checkOrphanedFiles},
		{"dangling_records", j.checkDanglingRecords},
		{"size_mismatches", j.checkSizeMismatches},
		{"archived_cleanup", j.checkArchivedVersionCleanup},
		{"temp_cleanup", j.checkTempCleanup},
		{"duplicate_paths", j.checkDuplicatePaths},
		{"purging_cleanup", j.checkPurgingCleanup},
		{"rename_cleanup", j.checkRenameCleanup},
		{"archive_warnings", j.checkArchiveWarnings},
	}

	for i, c := range checks {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		slog.Info("filestore reconcile: running check", "check", c.name, "index", i+1)
		if err := c.fn(ctx, &result); err != nil {
			slog.Error("filestore reconcile: check failed", "check", c.name, "error", err)
			// Continue with remaining checks — partial results are better than none.
		}
		if progressFn != nil {
			progressFn(i + 1)
		}
	}

	slog.Info("filestore reconcile: complete",
		"orphaned_files", result.OrphanedFiles,
		"dangling_records", result.DanglingRecords,
		"size_mismatches", result.SizeMismatches,
		"archived_cleaned", result.ArchivedCleaned,
		"temp_cleaned", result.TempCleaned,
		"duplicate_paths", result.DuplicatePaths,
		"purging_cleaned", result.PurgingCleaned,
		"rename_fixed", result.RenameFixed,
		"archive_warnings", len(result.ArchiveWarnings),
	)
	return result, nil
}

// ---------- Check 1: Orphaned files ----------
// Files on disk with no registry entry. Only considers files with mtime >1h ago.
// Logs only — does NOT delete.
func (j *ReconcileJob) checkOrphanedFiles(ctx context.Context, r *ReconcileResult) error {
	// Build a set of all known storage_paths from the registry.
	rows, err := j.pool.Query(ctx,
		`SELECT storage_path FROM user_files WHERE storage_backend = 'local'`)
	if err != nil {
		return fmt.Errorf("query registry paths: %w", err)
	}
	defer rows.Close()

	knownPaths := make(map[string]struct{})
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return fmt.Errorf("scan storage_path: %w", err)
		}
		knownPaths[p] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate registry paths: %w", err)
	}

	cutoff := time.Now().Add(-1 * time.Hour)

	err = filepath.WalkDir(j.basePath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // skip unreadable entries
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			name := d.Name()
			// Skip special directories — these are not tracked in user_files.
			if name == "_temp" || name == "_cache" || name == "_archive" {
				return fs.SkipDir
			}
			return nil
		}

		// Skip sidecar/metadata files.
		if strings.HasSuffix(path, ".meta") || strings.HasSuffix(path, ".meta.json") {
			return nil
		}

		relPath, err := filepath.Rel(j.basePath, path)
		if err != nil {
			return nil
		}

		if _, ok := knownPaths[relPath]; ok {
			return nil // registered — not orphaned
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.ModTime().After(cutoff) {
			return nil // too recent — might be an in-flight write
		}

		slog.Warn("filestore reconcile: orphaned file (no registry entry)",
			"path", relPath, "size", info.Size(), "mtime", info.ModTime())
		r.OrphanedFiles++
		return nil
	})
	return err
}

// ---------- Check 2: Dangling records ----------
// Registry entries with no physical file. DELETE the record.
func (j *ReconcileJob) checkDanglingRecords(ctx context.Context, r *ReconcileResult) error {
	rows, err := j.pool.Query(ctx,
		`SELECT id, storage_path, user_id FROM user_files
		 WHERE storage_backend = 'local' AND status != 'purging'`)
	if err != nil {
		return fmt.Errorf("query records: %w", err)
	}
	defer rows.Close()

	type dangling struct {
		id, path, userID string
	}
	var toDelete []dangling

	for rows.Next() {
		var d dangling
		if err := rows.Scan(&d.id, &d.path, &d.userID); err != nil {
			return fmt.Errorf("scan record: %w", err)
		}
		fullPath := filepath.Join(j.basePath, d.path)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			toDelete = append(toDelete, d)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate records: %w", err)
	}

	for _, d := range toDelete {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		slog.Warn("filestore reconcile: dangling record (no physical file), deleting",
			"file_id", d.id, "path", d.path, "user_id", d.userID)
		if _, err := j.pool.Exec(ctx,
			`DELETE FROM user_files WHERE id = $1`, d.id); err != nil {
			slog.Error("filestore reconcile: failed to delete dangling record",
				"file_id", d.id, "error", err)
			continue
		}
		r.DanglingRecords++
	}
	return nil
}

// ---------- Check 3: Size mismatch ----------
// File size on disk differs from registry size_bytes. UPDATE the registry.
func (j *ReconcileJob) checkSizeMismatches(ctx context.Context, r *ReconcileResult) error {
	rows, err := j.pool.Query(ctx,
		`SELECT id, storage_path, size_bytes FROM user_files
		 WHERE storage_backend = 'local' AND status = 'active'`)
	if err != nil {
		return fmt.Errorf("query records: %w", err)
	}
	defer rows.Close()

	type mismatch struct {
		id       string
		path     string
		dbSize   int64
		diskSize int64
	}
	var fixes []mismatch

	for rows.Next() {
		var id, path string
		var dbSize int64
		if err := rows.Scan(&id, &path, &dbSize); err != nil {
			return fmt.Errorf("scan record: %w", err)
		}
		fullPath := filepath.Join(j.basePath, path)
		info, err := os.Stat(fullPath)
		if err != nil {
			continue // missing file handled by check 2
		}
		if info.Size() != dbSize {
			fixes = append(fixes, mismatch{id: id, path: path, dbSize: dbSize, diskSize: info.Size()})
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate records: %w", err)
	}

	for _, m := range fixes {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		slog.Warn("filestore reconcile: size mismatch, updating registry",
			"file_id", m.id, "path", m.path, "db_size", m.dbSize, "disk_size", m.diskSize)
		if _, err := j.pool.Exec(ctx,
			`UPDATE user_files SET size_bytes = $1, updated_at = NOW() WHERE id = $2`,
			m.diskSize, m.id); err != nil {
			slog.Error("filestore reconcile: failed to update size",
				"file_id", m.id, "error", err)
			continue
		}
		r.SizeMismatches++
	}
	return nil
}

// ---------- Check 4: Archived version cleanup ----------
// If keepVersions >= 0, delete oldest archived versions beyond the limit per
// (user_id, file_type, parent_id) group.
func (j *ReconcileJob) checkArchivedVersionCleanup(ctx context.Context, r *ReconcileResult) error {
	if j.keepVersions < 0 {
		return nil // -1 means keep all
	}

	// Find groups that exceed the version limit.
	rows, err := j.pool.Query(ctx, `
		SELECT user_id, file_type, parent_id, COUNT(*) as cnt
		FROM user_files
		WHERE status = 'archived' AND storage_backend = 'local' AND parent_id IS NOT NULL
		GROUP BY user_id, file_type, parent_id
		HAVING COUNT(*) > $1`, j.keepVersions)
	if err != nil {
		return fmt.Errorf("query archived groups: %w", err)
	}
	defer rows.Close()

	type group struct {
		userID, fileType string
		parentID         *string
		count            int
	}
	var groups []group

	for rows.Next() {
		var g group
		if err := rows.Scan(&g.userID, &g.fileType, &g.parentID, &g.count); err != nil {
			return fmt.Errorf("scan group: %w", err)
		}
		groups = append(groups, g)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate groups: %w", err)
	}

	for _, g := range groups {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Get the IDs to delete: oldest first, skip keepVersions newest.
		excess, err := j.pool.Query(ctx, `
			SELECT id, storage_path FROM user_files
			WHERE user_id = $1 AND file_type = $2 AND parent_id = $3 AND status = 'archived'
				AND storage_backend = 'local'
			ORDER BY created_at ASC
			LIMIT $4`,
			g.userID, g.fileType, g.parentID, g.count-j.keepVersions)
		if err != nil {
			slog.Error("filestore reconcile: failed to query excess archives",
				"user_id", g.userID, "error", err)
			continue
		}

		type entry struct {
			id, path string
		}
		var toRemove []entry
		for excess.Next() {
			var e entry
			if err := excess.Scan(&e.id, &e.path); err != nil {
				slog.Error("filestore reconcile: scan excess archive", "error", err)
				continue
			}
			toRemove = append(toRemove, e)
		}
		excess.Close()

		for _, e := range toRemove {
			fullPath := filepath.Join(j.basePath, e.path)
			if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
				slog.Error("filestore reconcile: failed to remove archived file",
					"file_id", e.id, "path", fullPath, "error", err)
				continue
			}
			if _, err := j.pool.Exec(ctx,
				`DELETE FROM user_files WHERE id = $1`, e.id); err != nil {
				slog.Error("filestore reconcile: failed to delete archive record",
					"file_id", e.id, "error", err)
				continue
			}
			cleanEmptyDirs(filepath.Dir(fullPath), j.basePath)
			r.ArchivedCleaned++
		}
	}
	return nil
}

// ---------- Check 5: Temp cleanup ----------
// Files in _temp/ directories older than 30 days with no import_queue entry.
func (j *ReconcileJob) checkTempCleanup(ctx context.Context, r *ReconcileResult) error {
	cutoff := time.Now().Add(-30 * 24 * time.Hour)

	err := filepath.WalkDir(j.basePath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// We only care about entries inside _temp/ directories.
		relPath, err := filepath.Rel(j.basePath, path)
		if err != nil {
			return nil
		}
		if !strings.Contains(relPath, "/_temp/") && !strings.HasPrefix(relPath, "_temp/") {
			return nil
		}
		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.ModTime().After(cutoff) {
			return nil // not old enough
		}

		// Check if this file is referenced by an active import_queue entry.
		fileName := filepath.Base(path)
		var exists bool
		if err := j.pool.QueryRow(ctx,
			`SELECT EXISTS(
				SELECT 1 FROM import_queue
				WHERE status IN ('pending', 'processing')
				  AND file_name = $1
			)`, fileName).Scan(&exists); err != nil {
			slog.Error("filestore reconcile: import_queue check failed",
				"file", fileName, "error", err)
			return nil
		}
		if exists {
			return nil // still referenced
		}

		slog.Info("filestore reconcile: removing stale temp file",
			"path", relPath, "mtime", info.ModTime())
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			slog.Error("filestore reconcile: failed to remove temp file",
				"path", path, "error", err)
			return nil
		}
		cleanEmptyDirs(filepath.Dir(path), j.basePath)
		r.TempCleaned++
		return nil
	})
	return err
}

// ---------- Check 6: Duplicate storage_path ----------
// Multiple registry entries with the same storage_path. Log warning.
func (j *ReconcileJob) checkDuplicatePaths(ctx context.Context, r *ReconcileResult) error {
	rows, err := j.pool.Query(ctx, `
		SELECT storage_path, COUNT(*) as cnt
		FROM user_files
		WHERE storage_backend = 'local'
		GROUP BY storage_path
		HAVING COUNT(*) > 1`)
	if err != nil {
		return fmt.Errorf("query duplicate paths: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var path string
		var cnt int
		if err := rows.Scan(&path, &cnt); err != nil {
			return fmt.Errorf("scan duplicate: %w", err)
		}
		slog.Warn("filestore reconcile: duplicate storage_path",
			"path", path, "count", cnt)
		r.DuplicatePaths++
	}
	return rows.Err()
}

// ---------- Check 7: Purging cleanup ----------
// Records with status='purging' where the physical file is already gone. DELETE record.
func (j *ReconcileJob) checkPurgingCleanup(ctx context.Context, r *ReconcileResult) error {
	rows, err := j.pool.Query(ctx,
		`SELECT id, storage_path FROM user_files
		 WHERE storage_backend = 'local' AND status = 'purging'`)
	if err != nil {
		return fmt.Errorf("query purging records: %w", err)
	}
	defer rows.Close()

	type entry struct {
		id, path string
	}
	var toDelete []entry

	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.id, &e.path); err != nil {
			return fmt.Errorf("scan purging record: %w", err)
		}
		fullPath := filepath.Join(j.basePath, e.path)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			toDelete = append(toDelete, e)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate purging records: %w", err)
	}

	for _, e := range toDelete {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		slog.Info("filestore reconcile: cleaning purging record (file already deleted)",
			"file_id", e.id, "path", e.path)
		if _, err := j.pool.Exec(ctx,
			`DELETE FROM user_files WHERE id = $1`, e.id); err != nil {
			slog.Error("filestore reconcile: failed to delete purging record",
				"file_id", e.id, "error", err)
			continue
		}
		r.PurgingCleaned++
	}
	return nil
}

// ---------- Check 8: Rename cleanup ----------
// Records with is_renaming=true: re-render the path from current schema,
// retry the rename, or finalize if already at target.
func (j *ReconcileJob) checkRenameCleanup(ctx context.Context, r *ReconcileResult) error {
	rows, err := j.pool.Query(ctx,
		`SELECT uf.id, uf.user_id, uf.storage_path, uf.file_type, uf.file_name,
		        COALESCE(u.library_dir_name, uf.user_id) AS user_dir
		 FROM user_files uf
		 JOIN users u ON u.id = uf.user_id
		 WHERE uf.is_renaming = true AND uf.storage_backend = 'local'`)
	if err != nil {
		return fmt.Errorf("query renaming records: %w", err)
	}
	defer rows.Close()

	type renaming struct {
		id, userID, storagePath, fileType, fileName, userDir string
	}
	var items []renaming

	for rows.Next() {
		var item renaming
		if err := rows.Scan(&item.id, &item.userID, &item.storagePath,
			&item.fileType, &item.fileName, &item.userDir); err != nil {
			return fmt.Errorf("scan renaming record: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate renaming records: %w", err)
	}

	for _, item := range items {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Query user's current schema for this file type.
		schema := j.renderer.DefaultSchema(item.fileType)
		var userSchema string
		err := j.pool.QueryRow(ctx,
			`SELECT schema_template FROM user_file_schemas WHERE user_id = $1 AND file_type = $2`,
			item.userID, item.fileType).Scan(&userSchema)
		if err == nil && userSchema != "" {
			schema = userSchema
		}

		ext := filepath.Ext(item.fileName)
		// We don't have the full tags here, so render with empty tags.
		// The rename handler should have stored the intended target in
		// storage_path already — if the file is already there, finalize.
		rendered := j.renderer.RenderPath(item.fileType, schema, nil, ext)
		targetRel := filepath.Join(item.userDir, DirForType(item.fileType), rendered)

		currentFull := filepath.Join(j.basePath, item.storagePath)
		targetFull := filepath.Join(j.basePath, targetRel)

		// Case 1: File is already at the target (or current == target) — just finalize.
		if item.storagePath == targetRel {
			if _, err := j.pool.Exec(ctx,
				`UPDATE user_files SET is_renaming = false, updated_at = NOW() WHERE id = $1`,
				item.id); err != nil {
				slog.Error("filestore reconcile: failed to finalize rename",
					"file_id", item.id, "error", err)
				continue
			}
			r.RenameFixed++
			continue
		}

		// Case 2: Source file exists — attempt the move.
		if _, err := os.Stat(currentFull); err == nil {
			targetRel = ResolveCollision(j.basePath, targetRel)
			targetFull = filepath.Join(j.basePath, targetRel)

			if err := os.MkdirAll(filepath.Dir(targetFull), 0o755); err != nil {
				slog.Error("filestore reconcile: mkdir for rename retry",
					"file_id", item.id, "target", targetFull, "error", err)
				continue
			}
			if err := os.Rename(currentFull, targetFull); err != nil {
				slog.Error("filestore reconcile: rename retry failed",
					"file_id", item.id, "from", currentFull, "to", targetFull, "error", err)
				continue
			}
			cleanEmptyDirs(filepath.Dir(currentFull), j.basePath)
			if _, err := j.pool.Exec(ctx,
				`UPDATE user_files SET storage_path = $1, is_renaming = false, updated_at = NOW()
				 WHERE id = $2`, targetRel, item.id); err != nil {
				slog.Error("filestore reconcile: failed to update renamed path",
					"file_id", item.id, "error", err)
				continue
			}
			slog.Info("filestore reconcile: rename retry succeeded",
				"file_id", item.id, "from", item.storagePath, "to", targetRel)
			r.RenameFixed++
			continue
		}

		// Case 3: Source file missing — check if target already exists
		// (rename happened but DB update failed).
		if _, err := os.Stat(targetFull); err == nil {
			if _, err := j.pool.Exec(ctx,
				`UPDATE user_files SET storage_path = $1, is_renaming = false, updated_at = NOW()
				 WHERE id = $2`, targetRel, item.id); err != nil {
				slog.Error("filestore reconcile: failed to finalize moved rename",
					"file_id", item.id, "error", err)
				continue
			}
			slog.Info("filestore reconcile: rename finalized (file already at target)",
				"file_id", item.id, "target", targetRel)
			r.RenameFixed++
			continue
		}

		// Both source and target missing — will be caught by check 2 (dangling records).
		slog.Warn("filestore reconcile: renaming record with no file at source or target",
			"file_id", item.id, "source", item.storagePath, "target", targetRel)
	}
	return nil
}

// ---------- Check 9: Archive size warning ----------
// Per-user SUM(size_bytes) WHERE status='archived'. Warn if >1GB.
func (j *ReconcileJob) checkArchiveWarnings(ctx context.Context, r *ReconcileResult) error {
	const warnThreshold int64 = 1024 * 1024 * 1024 // 1 GB

	rows, err := j.pool.Query(ctx, `
		SELECT uf.user_id, COALESCE(u.username, uf.user_id) AS username,
		       SUM(uf.size_bytes) AS total
		FROM user_files uf
		JOIN users u ON u.id = uf.user_id
		WHERE uf.status = 'archived'
		GROUP BY uf.user_id, u.username
		HAVING SUM(uf.size_bytes) > $1`, warnThreshold)
	if err != nil {
		return fmt.Errorf("query archive sizes: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var userID, username string
		var total int64
		if err := rows.Scan(&userID, &username, &total); err != nil {
			return fmt.Errorf("scan archive size: %w", err)
		}
		mb := total / (1024 * 1024)
		msg := fmt.Sprintf("user %s (%s): %d MB archived", username, userID, mb)
		slog.Warn("filestore reconcile: large archive", "user_id", userID,
			"username", username, "archive_mb", mb)
		r.ArchiveWarnings = append(r.ArchiveWarnings, msg)
	}
	return rows.Err()
}
