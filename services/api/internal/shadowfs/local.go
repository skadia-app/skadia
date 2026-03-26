package shadowfs

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// maxExportBytes is the maximum total size for a ZIP export (10 GB).
const maxExportBytes int64 = 10 * 1024 * 1024 * 1024

// maxRaceRetries is the number of O_EXCL race-condition retries for Put.
const maxRaceRetries = 10

// LocalShadowFS implements ShadowFS backed by the local filesystem.
type LocalShadowFS struct {
	pool         *pgxpool.Pool
	basePath     string
	renderer     *SchemaRenderer
	keepVersions int
	maxUserBytes int64
}

// NewLocalShadowFS creates a new local filesystem file store.
func NewLocalShadowFS(pool *pgxpool.Pool, basePath string, renderer *SchemaRenderer, keepVersions int, maxUserBytes int64) (*LocalShadowFS, error) {
	abs, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("shadowfs: resolve basePath: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("shadowfs: create basePath: %w", err)
	}
	return &LocalShadowFS{
		pool:         pool,
		basePath:     abs,
		renderer:     renderer,
		keepVersions: keepVersions,
		maxUserBytes: maxUserBytes,
	}, nil
}

// Put stores a file on disk and registers it in user_files.
func (s *LocalShadowFS) Put(ctx context.Context, entry FileEntry, data io.Reader) (FileRecord, error) {
	// Validate file type.
	if _, ok := fileTypeDirName[entry.FileType]; !ok {
		return FileRecord{}, fmt.Errorf("unknown file type %q", entry.FileType)
	}

	// Validate ParentID as UUID if set (parity with S3ShadowFS).
	if entry.ParentID != nil && *entry.ParentID != "" {
		if _, err := uuid.Parse(*entry.ParentID); err != nil {
			return FileRecord{}, fmt.Errorf("invalid parent_id: %w", err)
		}
	}

	// Quota check: soft limit — rejects if already over quota, but does NOT
	// serialize concurrent uploads (advisory lock would kill bulk import perf
	// with 10,000s of monitoring FITs). Concurrent uploads may briefly overshoot
	// by sum of in-flight files. Post-write check (below) + Reconciliation job
	// catch overages. This is acceptable for a safety-valve quota.
	// NOTE: When entry.SizeBytes is 0 (streaming uploads where size is unknown
	// upfront), this pre-check passes unconditionally. The post-write check
	// after writeFile() uses the actual written size to enforce the quota.
	if s.maxUserBytes > 0 {
		used, err := diskUsage(ctx, s.pool, entry.UserID)
		if err != nil {
			return FileRecord{}, fmt.Errorf("shadowfs: quota check: %w", err)
		}
		if used+entry.SizeBytes > s.maxUserBytes {
			return FileRecord{}, fmt.Errorf("shadowfs: quota exceeded (used %d + %d > %d)", used, entry.SizeBytes, s.maxUserBytes)
		}
	}

	userDir, err := s.getUserDir(ctx, entry.UserID)
	if err != nil {
		return FileRecord{}, err
	}

	ext := filepath.Ext(entry.FileName)

	// Build the relative storage path (under basePath).
	var relPath string
	if entry.FileType == "edit" {
		// Edit sidecars use a fixed ID-based path.
		if entry.ParentID == nil {
			return FileRecord{}, fmt.Errorf("shadowfs: edit sidecar requires ParentID")
		}
		relPath = filepath.Join(userDir, DirForType("edit"), *entry.ParentID+".json")
	} else {
		// Query user schema, fall back to default.
		// TODO: Consider caching user schemas per-session for bulk imports (currently 1 query per Put)
		schema := s.renderer.DefaultSchema(entry.FileType)
		var userSchema string
		err := s.pool.QueryRow(ctx,
			`SELECT schema_template FROM user_file_schemas WHERE user_id = $1 AND file_type = $2`,
			entry.UserID, entry.FileType).Scan(&userSchema)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return FileRecord{}, fmt.Errorf("shadowfs: query schema: %w", err)
		}
		if err == nil && userSchema != "" {
			schema = userSchema
		}

		rendered := s.renderer.RenderPath(entry.FileType, schema, entry.Tags, ext)
		relPath = filepath.Join(userDir, DirForType(entry.FileType), rendered)
	}

	// Security check before writing.
	if err := s.validateLocalPath(relPath); err != nil {
		return FileRecord{}, err
	}

	fullPath := filepath.Join(s.basePath, relPath)

	// Collision resolution (skip for edit sidecars — they upsert).
	if entry.FileType != "edit" {
		relPath = ResolveCollision(s.basePath, relPath)
		fullPath = filepath.Join(s.basePath, relPath)
	}

	// Create parent directories.
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return FileRecord{}, fmt.Errorf("shadowfs: mkdir: %w", err)
	}

	// Write file with O_EXCL race retry (except edit sidecars which overwrite).
	// Safe: O_EXCL fails atomically at open before any data is read. Do not change writeFile to read data before opening.
	// NOTE: The io.Reader is consumed by the first writeFile call. On O_EXCL collision the reader is already
	// exhausted, so we call writeFile with the same data variable — this is intentional: writeFile opens
	// the file only after O_EXCL succeeds, so no bytes are consumed on failure. This is safe but fragile:
	// if writeFile is ever changed to read from data before opening the file, the retry loop would silently
	// produce empty files. The test for this contract is: writeFile must not read from data before os.OpenFile.
	var written int64
	var hashHex string
	var first512 []byte

	if entry.FileType == "edit" {
		written, hashHex, first512, err = writeFile(fullPath, data, false)
	} else {
		for attempt := range maxRaceRetries {
			written, hashHex, first512, err = writeFile(fullPath, data, true)
			if err == nil {
				break
			}
			if !errors.Is(err, os.ErrExist) {
				return FileRecord{}, fmt.Errorf("shadowfs: write: %w", err)
			}
			// Another process won the race — re-resolve collision.
			relPath = ResolveCollision(s.basePath, relPath)
			fullPath = filepath.Join(s.basePath, relPath)
			if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
				return FileRecord{}, fmt.Errorf("shadowfs: mkdir retry: %w", err)
			}
			if attempt == maxRaceRetries-1 {
				return FileRecord{}, fmt.Errorf("shadowfs: exhausted %d race retries", maxRaceRetries)
			}
		}
	}
	if err != nil {
		return FileRecord{}, fmt.Errorf("shadowfs: write: %w", err)
	}

	// Content-type detection.
	contentType := DetectContentType(entry.FileName, entry.ContentType, first512)

	// Register in DB — always use computed values (written/hashHex), not entry fields.
	var rec FileRecord
	rec.StorageBackend = "local"
	rec.StoragePath = relPath
	rec.ContentType = contentType
	rec.FileName = entry.FileName
	rec.SizeBytes = written
	rec.FileHash = hashHex
	rec.UserID = entry.UserID
	rec.FileType = entry.FileType
	rec.ParentType = entry.ParentType
	rec.ParentID = entry.ParentID
	rec.HasSidecar = entry.HasSidecar
	rec.Status = "active"

	if entry.FileType == "edit" {
		// Upsert for edit sidecars.
		err = s.pool.QueryRow(ctx,
			`INSERT INTO user_files (user_id, file_type, storage_backend, storage_path, content_type,
				file_name, size_bytes, file_hash, parent_type, parent_id, has_sidecar, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'active')
			ON CONFLICT (storage_backend, storage_path) DO UPDATE SET
				size_bytes = EXCLUDED.size_bytes,
				file_hash = EXCLUDED.file_hash,
				content_type = EXCLUDED.content_type,
				updated_at = NOW()
			RETURNING id, created_at, updated_at`,
			entry.UserID, entry.FileType, "local", relPath, contentType,
			entry.FileName, written, hashHex, entry.ParentType, entry.ParentID, entry.HasSidecar,
		).Scan(&rec.ID, &rec.CreatedAt, &rec.UpdatedAt)
	} else {
		err = s.pool.QueryRow(ctx,
			`INSERT INTO user_files (user_id, file_type, storage_backend, storage_path, content_type,
				file_name, size_bytes, file_hash, parent_type, parent_id, has_sidecar, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'active')
			RETURNING id, created_at, updated_at`,
			entry.UserID, entry.FileType, "local", relPath, contentType,
			entry.FileName, written, hashHex, entry.ParentType, entry.ParentID, entry.HasSidecar,
		).Scan(&rec.ID, &rec.CreatedAt, &rec.UpdatedAt)
	}
	if err != nil {
		// DB insert failed — clean up the file if we own it.
		s.safeRemove(fullPath, relPath)
		return FileRecord{}, fmt.Errorf("shadowfs: insert record: %w", err)
	}

	// Post-write quota check: catches cases where SizeBytes was 0 on entry.
	// RACE NOTE: Between diskUsage() and the file deletion/DB rollback below, another concurrent
	// Get() or quota check could read a stale usage value. This is an accepted trade-off: the
	// advisory lock in DeleteAllForUser is the only hard serialisation point. The post-write
	// rollback here is best-effort; the periodic ReconcileJob catches any remaining overage.
	if s.maxUserBytes > 0 && written > 0 {
		usage, qErr := diskUsage(ctx, s.pool, entry.UserID)
		if qErr != nil {
			slog.Warn("shadowfs: post-write quota check failed", "error", qErr)
		}
		if usage > s.maxUserBytes {
			s.safeRemove(fullPath, relPath)
			_, _ = s.pool.Exec(ctx, `DELETE FROM user_files WHERE id = $1`, rec.ID)
			return FileRecord{}, fmt.Errorf("shadowfs: storage quota exceeded (post-write check)")
		}
	}

	return rec, nil
}

// Get returns an io.ReadCloser for the file identified by fileID.
func (s *LocalShadowFS) Get(ctx context.Context, userID, fileID string) (io.ReadCloser, error) {
	var storagePath, status string
	err := s.pool.QueryRow(ctx,
		`SELECT storage_path, status FROM user_files WHERE id = $1 AND user_id = $2`,
		fileID, userID).Scan(&storagePath, &status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("shadowfs: file not found")
		}
		return nil, fmt.Errorf("shadowfs: query: %w", err)
	}
	if status == "purging" {
		return nil, fmt.Errorf("shadowfs: file is being purged")
	}

	if err := s.validateLocalPath(storagePath); err != nil {
		return nil, err
	}

	fullPath := filepath.Join(s.basePath, storagePath)
	f, err := os.Open(fullPath)
	if err != nil {
		return nil, fmt.Errorf("shadowfs: open: %w", err)
	}
	return f, nil
}

// Delete removes a file from disk and the registry.
func (s *LocalShadowFS) Delete(ctx context.Context, userID, fileID string) error {
	var storagePath, fileType string
	var hasSidecar bool
	err := s.pool.QueryRow(ctx,
		`SELECT storage_path, file_type, has_sidecar FROM user_files WHERE id = $1 AND user_id = $2`,
		fileID, userID).Scan(&storagePath, &fileType, &hasSidecar)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("shadowfs: file not found")
		}
		return fmt.Errorf("shadowfs: query: %w", err)
	}

	if err := s.validateLocalPath(storagePath); err != nil {
		return err
	}

	fullPath := filepath.Join(s.basePath, storagePath)

	// Delete registry entry FIRST. If DB delete fails, file stays (correct).
	// If os.Remove fails after DB delete, reconciler catches the orphan.
	_, err = s.pool.Exec(ctx,
		`DELETE FROM user_files WHERE id = $1 AND user_id = $2`, fileID, userID)
	if err != nil {
		return fmt.Errorf("shadowfs: delete record: %w", err)
	}

	// Remove the file.
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		slog.Warn("shadowfs: remove file after DB delete (reconciler will clean up)", "path", fullPath, "error", err)
	}

	// Remove file-type-specific sidecars.
	s.removeSidecars(fullPath, fileType, hasSidecar)

	// Clean empty parent directories.
	cleanEmptyDirs(filepath.Dir(fullPath), s.basePath)

	return nil
}

// DeleteAllForUser removes all files for a user (GDPR purge).
// Acquires a per-user advisory lock to prevent races with concurrent uploads.
func (s *LocalShadowFS) DeleteAllForUser(ctx context.Context, userID string) error {
	// Advisory lock prevents concurrent uploads from creating files while we purge.
	// Must use a pinned connection — advisory locks are session-level and would
	// silently release if the pooled connection is returned between calls.
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("shadowfs: acquire conn: %w", err)
	}
	defer conn.Release()

	// L56: Use Go-side FNV-1a 64-bit instead of PostgreSQL's hashtext (32-bit, collision-prone).
	lockKey := advisoryLockKey("shadowfs:" + userID)
	if _, err := conn.Exec(ctx,
		`SELECT pg_advisory_lock($1)`, lockKey); err != nil {
		return fmt.Errorf("shadowfs: advisory lock: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, lockKey)
	}()

	// Mark all files as purging.
	_, err = conn.Exec(ctx,
		`UPDATE user_files SET status = 'purging' WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("shadowfs: mark purging: %w", err)
	}

	// Query all files for deletion.
	rows, err := conn.Query(ctx,
		`SELECT storage_path, file_type, has_sidecar FROM user_files WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("shadowfs: query files: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var storagePath, fileType string
		var hasSidecar bool
		if err := rows.Scan(&storagePath, &fileType, &hasSidecar); err != nil {
			slog.Error("shadowfs: scan file for deletion", "err", err)
			continue
		}

		if err := s.validateLocalPath(storagePath); err != nil {
			slog.Warn("shadowfs: skip invalid path during purge", "path", storagePath, "err", err)
			continue
		}

		fullPath := filepath.Join(s.basePath, storagePath)
		if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
			slog.Warn("shadowfs: remove file during purge", "path", fullPath, "err", err)
		}
		s.removeSidecars(fullPath, fileType, hasSidecar)
	}
	if err := rows.Err(); err != nil {
		slog.Error("shadowfs: iterate files for deletion", "err", err)
	}

	// Safety-net: remove the user directory entirely (includes _cache/_temp subdirs).
	userDir := s.getUserDirOrFallback(ctx, userID)
	if userDir != "" {
		userFullPath := filepath.Join(s.basePath, userDir)
		if err := s.validateLocalPath(userDir); err == nil {
			if err := os.RemoveAll(userFullPath); err != nil {
				slog.Warn("shadowfs: removeAll user dir", "path", userFullPath, "err", err)
			}
		}
	}

	// Delete all registry entries.
	_, err = conn.Exec(ctx,
		`DELETE FROM user_files WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("shadowfs: delete records: %w", err)
	}
	return nil
}

// List returns all file records for a user, optionally filtered by file type.
func (s *LocalShadowFS) List(ctx context.Context, userID string, fileType *string) ([]FileRecord, error) {
	return listRecords(ctx, s.pool, userID, fileType)
}

// DiskUsage returns total bytes used by a user.
func (s *LocalShadowFS) DiskUsage(ctx context.Context, userID string) (int64, error) {
	return diskUsage(ctx, s.pool, userID)
}

// StreamToZip writes all user files into a zip archive (GDPR export).
func (s *LocalShadowFS) StreamToZip(ctx context.Context, userID string, zw *zip.Writer) error {
	records, err := listRecords(ctx, s.pool, userID, nil)
	if err != nil {
		return fmt.Errorf("shadowfs: list records for zip: %w", err)
	}

	userDir, err := s.getUserDir(ctx, userID)
	if err != nil {
		return err
	}

	var totalBytes int64
	for _, rec := range records {
		// Skip _cache paths.
		if strings.Contains(rec.StoragePath, "/_cache/") {
			continue
		}

		// Strip user-dir prefix for zip paths.
		zipPath := strings.TrimPrefix(rec.StoragePath, userDir+"/")
		if zipPath == rec.StoragePath {
			// Fallback: use as-is if prefix doesn't match.
			zipPath = rec.StoragePath
		}

		// ZIP Slip prevention.
		zipPath = filepath.Clean(zipPath)
		if strings.Contains(zipPath, "..") || strings.HasPrefix(zipPath, "/") {
			slog.Warn("shadowfs: skip path with .. in zip", "path", zipPath)
			continue
		}

		// Archived files go into _archive/ subdirectory.
		if rec.Status == "archived" {
			zipPath = filepath.Join("_archive", zipPath)
		}

		// Check export size limit.
		totalBytes += rec.SizeBytes
		if totalBytes > maxExportBytes {
			return fmt.Errorf("shadowfs: export exceeds %d byte limit", maxExportBytes)
		}

		fullPath := filepath.Join(s.basePath, rec.StoragePath)
		if err := func() error {
			f, err := os.Open(fullPath)
			if err != nil {
				slog.Warn("shadowfs: skip missing file in zip", "path", fullPath, "err", err)
				return nil
			}
			defer f.Close()

			w, err := zw.Create(zipPath)
			if err != nil {
				return fmt.Errorf("shadowfs: create zip entry: %w", err)
			}
			if _, err := io.Copy(w, f); err != nil {
				return fmt.Errorf("shadowfs: write zip entry: %w", err)
			}
			return nil
		}(); err != nil {
			return err
		}
	}
	return nil
}

// Resolve returns metadata for serving a file.
func (s *LocalShadowFS) Resolve(ctx context.Context, userID, fileID string) (ResolvedFile, error) {
	var storagePath, contentType, fileName string
	var sizeBytes int64
	err := s.pool.QueryRow(ctx,
		`SELECT storage_path, content_type, file_name, size_bytes
		FROM user_files WHERE id = $1 AND user_id = $2 AND status != 'purging'`,
		fileID, userID).Scan(&storagePath, &contentType, &fileName, &sizeBytes)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ResolvedFile{}, fmt.Errorf("shadowfs: file not found")
		}
		return ResolvedFile{}, fmt.Errorf("shadowfs: query: %w", err)
	}

	if err := s.validateLocalPath(storagePath); err != nil {
		return ResolvedFile{}, err
	}

	fullPath := filepath.Join(s.basePath, storagePath)
	return ResolvedFile{
		Type:        "local",
		Path:        fullPath,
		ContentType: contentType,
		FileName:    fileName,
		SizeBytes:   sizeBytes,
	}, nil
}

// Archive moves a file to the _archive/ directory with a version suffix.
func (s *LocalShadowFS) Archive(ctx context.Context, userID, fileID, replacedByID string) error {
	if s.keepVersions == 0 {
		return s.Delete(ctx, userID, fileID)
	}

	var storagePath, fileType string
	err := s.pool.QueryRow(ctx,
		`SELECT storage_path, file_type FROM user_files WHERE id = $1 AND user_id = $2`,
		fileID, userID).Scan(&storagePath, &fileType)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("shadowfs: file not found")
		}
		return fmt.Errorf("shadowfs: query: %w", err)
	}

	if err := s.validateLocalPath(storagePath); err != nil {
		return err
	}

	fullPath := filepath.Join(s.basePath, storagePath)

	// Build archive path: insert _archive/ after the user dir, add _v1 suffix.
	dir := filepath.Dir(storagePath)
	base := filepath.Base(storagePath)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)

	archiveRel := filepath.Join(dir, "_archive", stem+"_v1"+ext)
	archiveRel = ResolveCollision(s.basePath, archiveRel)
	archiveFull := filepath.Join(s.basePath, archiveRel)

	if err := s.validateLocalPath(archiveRel); err != nil {
		return err
	}

	// Create archive directory.
	if err := os.MkdirAll(filepath.Dir(archiveFull), 0o755); err != nil {
		return fmt.Errorf("shadowfs: mkdir archive: %w", err)
	}

	// Set is_renaming=true BEFORE os.Rename so reconciler can recover crashed archives.
	_, err = s.pool.Exec(ctx,
		`UPDATE user_files SET is_renaming = true, updated_at = NOW() WHERE id = $1 AND user_id = $2`,
		fileID, userID)
	if err != nil {
		return fmt.Errorf("shadowfs: set is_renaming: %w", err)
	}

	// Move file.
	if err := os.Rename(fullPath, archiveFull); err != nil {
		// Roll back is_renaming flag on rename failure.
		_, _ = s.pool.Exec(ctx,
			`UPDATE user_files SET is_renaming = false, updated_at = NOW() WHERE id = $1 AND user_id = $2`,
			fileID, userID)
		return fmt.Errorf("shadowfs: rename to archive: %w", err)
	}

	// Clean empty source directories.
	cleanEmptyDirs(filepath.Dir(fullPath), s.basePath)

	// Update registry and clear is_renaming.
	_, err = s.pool.Exec(ctx,
		`UPDATE user_files SET storage_path = $1, status = 'archived', replaced_by = $2,
			is_renaming = false, updated_at = NOW()
		WHERE id = $3 AND user_id = $4`,
		archiveRel, replacedByID, fileID, userID)
	if err != nil {
		return fmt.Errorf("shadowfs: update archive record: %w", err)
	}
	return nil
}

// ---------- Helpers ----------

// getUserDir returns the relative user directory (library_dir_name or UUID fallback).
func (s *LocalShadowFS) getUserDir(ctx context.Context, userID string) (string, error) {
	var dirName *string
	err := s.pool.QueryRow(ctx,
		`SELECT library_dir_name FROM users WHERE id = $1`, userID).Scan(&dirName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("shadowfs: user not found: %s", userID)
		}
		return "", fmt.Errorf("shadowfs: query user: %w", err)
	}

	name := userID
	if dirName != nil && *dirName != "" {
		name = *dirName
	}

	// Validate no path traversal in user dir name.
	if strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return "", fmt.Errorf("shadowfs: invalid library_dir_name: %q", name)
	}
	return name, nil
}

// getUserDirOrFallback returns the user directory, falling back to the userID itself
// when the user row has already been deleted (e.g., during GDPR purge).
func (s *LocalShadowFS) getUserDirOrFallback(ctx context.Context, userID string) string {
	dir, err := s.getUserDir(ctx, userID)
	if err == nil {
		return dir
	}
	// User row gone (CASCADE already fired) — fall back to UUID-based directory.
	return userID
}

// validateLocalPath ensures the resolved path stays within basePath and is not a symlink escape.
// TOCTOU note: a symlink can be swapped between the Lstat/EvalSymlinks check and the subsequent
// os.Open/os.Create call. In practice this race is mitigated by running the API inside a
// container with a bind-mounted data volume — no external process can write to basePath.
func (s *LocalShadowFS) validateLocalPath(relPath string) error {
	return ValidateRelPath(s.basePath, relPath)
}

// ValidateRelPath checks that relPath stays within basePath and contains no
// symlink escapes. It is the package-level equivalent of validateLocalPath and
// is used by handler code that operates outside of a LocalShadowFS instance.
func ValidateRelPath(basePath, relPath string) error {
	cleaned := filepath.Clean(relPath)
	if strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) {
		return fmt.Errorf("shadowfs: path traversal blocked: %q", relPath)
	}
	fullPath := filepath.Join(basePath, cleaned)
	if !strings.HasPrefix(fullPath, basePath+string(filepath.Separator)) && fullPath != basePath {
		return fmt.Errorf("shadowfs: path escapes basePath: %q", relPath)
	}
	// Check symlinks on existing paths.
	if _, err := os.Lstat(fullPath); err == nil {
		real, err := filepath.EvalSymlinks(fullPath)
		if err != nil {
			return fmt.Errorf("shadowfs: eval symlinks: %w", err)
		}
		if !strings.HasPrefix(real, basePath) {
			return fmt.Errorf("shadowfs: symlink escapes basePath: %q -> %q", fullPath, real)
		}
	} else {
		parentDir := filepath.Dir(fullPath)
		if _, err := os.Lstat(parentDir); err == nil {
			real, err := filepath.EvalSymlinks(parentDir)
			if err != nil {
				return fmt.Errorf("shadowfs: eval parent symlinks: %w", err)
			}
			if !strings.HasPrefix(real, basePath) {
				return fmt.Errorf("shadowfs: parent symlink escapes basePath: %q -> %q", parentDir, real)
			}
		}
	}
	return nil
}

// cleanEmptyDirs removes empty directories walking up to stopAt.
func cleanEmptyDirs(dir, stopAt string) {
	for dir != stopAt && strings.HasPrefix(dir, stopAt) {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

// removeSidecars removes file-type-specific sidecar files.
func (s *LocalShadowFS) removeSidecars(fullPath, fileType string, hasSidecar bool) {
	if !hasSidecar {
		return
	}
	var sidecarPaths []string
	switch fileType {
	case "media":
		ext := filepath.Ext(fullPath)
		stem := strings.TrimSuffix(fullPath, ext)
		sidecarPaths = append(sidecarPaths, stem+".meta.json")
	case "activity":
		sidecarPaths = append(sidecarPaths, fullPath+".meta")
	}
	for _, p := range sidecarPaths {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			slog.Warn("shadowfs: remove sidecar", "path", p, "err", err)
		}
	}
}

// safeRemove removes a file only if it matches the expected relative path.
func (s *LocalShadowFS) safeRemove(fullPath, relPath string) {
	// Double-check: only remove if the path is within basePath.
	if err := s.validateLocalPath(relPath); err != nil {
		slog.Warn("shadowfs: skip cleanup of invalid path", "path", relPath, "err", err)
		return
	}
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		slog.Warn("shadowfs: cleanup failed", "path", fullPath, "err", err)
	}
}

// writeFile writes data to path, computing SHA256 and capturing the first 512 bytes.
// If exclusive is true, uses O_EXCL to prevent overwriting.
func writeFile(path string, data io.Reader, exclusive bool) (written int64, hashHex string, first512 []byte, err error) {
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if exclusive {
		flags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	}

	f, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return 0, "", nil, err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()

	h := sha256.New()

	// Read first 512 bytes for content-type detection.
	head := make([]byte, 512)
	n, readErr := io.ReadFull(data, head)
	if n > 0 {
		first512 = head[:n]
		if _, err := h.Write(first512); err != nil {
			return 0, "", nil, err
		}
		if _, err := f.Write(first512); err != nil {
			return 0, "", nil, err
		}
		written = int64(n)
	}

	// Write the rest.
	if readErr == nil || errors.Is(readErr, io.ErrUnexpectedEOF) {
		// There may be more data.
		mw := io.MultiWriter(f, h)
		rest, copyErr := io.Copy(mw, data)
		if copyErr != nil {
			return 0, "", nil, copyErr
		}
		written += rest
	} else if !errors.Is(readErr, io.EOF) {
		return 0, "", nil, readErr
	}

	hashHex = hex.EncodeToString(h.Sum(nil))
	return written, hashHex, first512, nil
}

// Ensure LocalShadowFS implements ShadowFS at compile time.
var _ ShadowFS = (*LocalShadowFS)(nil)

