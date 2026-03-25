package filestore

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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// maxExportBytes is the maximum total size for a ZIP export (10 GB).
const maxExportBytes int64 = 10 * 1024 * 1024 * 1024

// maxRaceRetries is the number of O_EXCL race-condition retries for Put.
const maxRaceRetries = 10

// LocalFileStore implements FileStore backed by the local filesystem.
type LocalFileStore struct {
	pool         *pgxpool.Pool
	basePath     string
	renderer     *SchemaRenderer
	keepVersions int
	maxUserBytes int64
}

// NewLocalFileStore creates a new local filesystem file store.
func NewLocalFileStore(pool *pgxpool.Pool, basePath string, renderer *SchemaRenderer, keepVersions int, maxUserBytes int64) (*LocalFileStore, error) {
	abs, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("filestore: resolve basePath: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("filestore: create basePath: %w", err)
	}
	return &LocalFileStore{
		pool:         pool,
		basePath:     abs,
		renderer:     renderer,
		keepVersions: keepVersions,
		maxUserBytes: maxUserBytes,
	}, nil
}

// Put stores a file on disk and registers it in user_files.
func (s *LocalFileStore) Put(ctx context.Context, entry FileEntry, data io.Reader) (FileRecord, error) {
	// Quota check.
	if s.maxUserBytes > 0 {
		used, err := diskUsage(ctx, s.pool, entry.UserID)
		if err != nil {
			return FileRecord{}, fmt.Errorf("filestore: quota check: %w", err)
		}
		if used+entry.SizeBytes > s.maxUserBytes {
			return FileRecord{}, fmt.Errorf("filestore: quota exceeded (used %d + %d > %d)", used, entry.SizeBytes, s.maxUserBytes)
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
			return FileRecord{}, fmt.Errorf("filestore: edit sidecar requires ParentID")
		}
		relPath = filepath.Join(userDir, DirForType("edit"), *entry.ParentID+".json")
	} else {
		// Query user schema, fall back to default.
		schema := s.renderer.DefaultSchema(entry.FileType)
		var userSchema string
		err := s.pool.QueryRow(ctx,
			`SELECT schema_template FROM user_file_schemas WHERE user_id = $1 AND file_type = $2`,
			entry.UserID, entry.FileType).Scan(&userSchema)
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
		return FileRecord{}, fmt.Errorf("filestore: mkdir: %w", err)
	}

	// Write file with O_EXCL race retry (except edit sidecars which overwrite).
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
				return FileRecord{}, fmt.Errorf("filestore: write: %w", err)
			}
			// Another process won the race — re-resolve collision.
			relPath = ResolveCollision(s.basePath, relPath)
			fullPath = filepath.Join(s.basePath, relPath)
			if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
				return FileRecord{}, fmt.Errorf("filestore: mkdir retry: %w", err)
			}
			if attempt == maxRaceRetries-1 {
				return FileRecord{}, fmt.Errorf("filestore: exhausted %d race retries", maxRaceRetries)
			}
		}
	}
	if err != nil {
		return FileRecord{}, fmt.Errorf("filestore: write: %w", err)
	}

	// Content-type detection.
	contentType := DetectContentType(entry.FileName, entry.ContentType, first512)

	// Override size and hash with what we actually wrote.
	if entry.SizeBytes == 0 {
		entry.SizeBytes = written
	}
	if entry.FileHash == "" {
		entry.FileHash = hashHex
	}

	// Register in DB.
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
		return FileRecord{}, fmt.Errorf("filestore: insert record: %w", err)
	}

	return rec, nil
}

// Get returns an io.ReadCloser for the file identified by fileID.
func (s *LocalFileStore) Get(ctx context.Context, userID, fileID string) (io.ReadCloser, error) {
	var storagePath, status string
	err := s.pool.QueryRow(ctx,
		`SELECT storage_path, status FROM user_files WHERE id = $1 AND user_id = $2`,
		fileID, userID).Scan(&storagePath, &status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("filestore: file not found")
		}
		return nil, fmt.Errorf("filestore: query: %w", err)
	}
	if status == "purging" {
		return nil, fmt.Errorf("filestore: file is being purged")
	}

	if err := s.validateLocalPath(storagePath); err != nil {
		return nil, err
	}

	fullPath := filepath.Join(s.basePath, storagePath)
	f, err := os.Open(fullPath)
	if err != nil {
		return nil, fmt.Errorf("filestore: open: %w", err)
	}
	return f, nil
}

// Delete removes a file from disk and the registry.
func (s *LocalFileStore) Delete(ctx context.Context, userID, fileID string) error {
	var storagePath, fileType string
	var hasSidecar bool
	err := s.pool.QueryRow(ctx,
		`SELECT storage_path, file_type, has_sidecar FROM user_files WHERE id = $1 AND user_id = $2`,
		fileID, userID).Scan(&storagePath, &fileType, &hasSidecar)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("filestore: file not found")
		}
		return fmt.Errorf("filestore: query: %w", err)
	}

	if err := s.validateLocalPath(storagePath); err != nil {
		return err
	}

	fullPath := filepath.Join(s.basePath, storagePath)

	// Remove the file.
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("filestore: remove: %w", err)
	}

	// Remove file-type-specific sidecars.
	s.removeSidecars(fullPath, fileType, hasSidecar)

	// Clean empty parent directories.
	cleanEmptyDirs(filepath.Dir(fullPath), s.basePath)

	// Delete registry entry.
	_, err = s.pool.Exec(ctx,
		`DELETE FROM user_files WHERE id = $1 AND user_id = $2`, fileID, userID)
	if err != nil {
		return fmt.Errorf("filestore: delete record: %w", err)
	}
	return nil
}

// DeleteAllForUser removes all files for a user (GDPR purge).
func (s *LocalFileStore) DeleteAllForUser(ctx context.Context, userID string) error {
	// Mark all files as purging.
	_, err := s.pool.Exec(ctx,
		`UPDATE user_files SET status = 'purging' WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("filestore: mark purging: %w", err)
	}

	// Query all files for deletion.
	rows, err := s.pool.Query(ctx,
		`SELECT storage_path, file_type, has_sidecar FROM user_files WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("filestore: query files: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var storagePath, fileType string
		var hasSidecar bool
		if err := rows.Scan(&storagePath, &fileType, &hasSidecar); err != nil {
			slog.Error("filestore: scan file for deletion", "err", err)
			continue
		}

		if err := s.validateLocalPath(storagePath); err != nil {
			slog.Warn("filestore: skip invalid path during purge", "path", storagePath, "err", err)
			continue
		}

		fullPath := filepath.Join(s.basePath, storagePath)
		if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
			slog.Warn("filestore: remove file during purge", "path", fullPath, "err", err)
		}
		s.removeSidecars(fullPath, fileType, hasSidecar)
	}
	if err := rows.Err(); err != nil {
		slog.Error("filestore: iterate files for deletion", "err", err)
	}

	// Safety-net: remove the user directory entirely.
	userDir, err := s.getUserDir(ctx, userID)
	if err == nil {
		userFullPath := filepath.Join(s.basePath, userDir)
		if err := s.validateLocalPath(userDir); err == nil {
			if err := os.RemoveAll(userFullPath); err != nil {
				slog.Warn("filestore: removeAll user dir", "path", userFullPath, "err", err)
			}
		}
	}

	// Clean _cache and _temp dirs.
	for _, sub := range []string{"_cache", "_temp"} {
		p := filepath.Join(s.basePath, userDir, sub)
		if err := os.RemoveAll(p); err != nil && !os.IsNotExist(err) {
			slog.Warn("filestore: remove cache/temp dir", "path", p, "err", err)
		}
	}

	// Clean journal entries.
	_, _ = s.pool.Exec(ctx,
		`DELETE FROM migration_filestore_journal WHERE user_id = $1`, userID)

	// Delete all registry entries.
	_, err = s.pool.Exec(ctx,
		`DELETE FROM user_files WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("filestore: delete records: %w", err)
	}
	return nil
}

// List returns all file records for a user, optionally filtered by file type.
func (s *LocalFileStore) List(ctx context.Context, userID string, fileType *string) ([]FileRecord, error) {
	return listRecords(ctx, s.pool, userID, fileType)
}

// DiskUsage returns total bytes used by a user.
func (s *LocalFileStore) DiskUsage(ctx context.Context, userID string) (int64, error) {
	return diskUsage(ctx, s.pool, userID)
}

// StreamToZip writes all user files into a zip archive (GDPR export).
func (s *LocalFileStore) StreamToZip(ctx context.Context, userID string, zw *zip.Writer) error {
	records, err := listRecords(ctx, s.pool, userID, nil)
	if err != nil {
		return fmt.Errorf("filestore: list records for zip: %w", err)
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
		if strings.Contains(zipPath, "..") {
			slog.Warn("filestore: skip path with .. in zip", "path", zipPath)
			continue
		}

		// Archived files go into _archive/ subdirectory.
		if rec.Status == "archived" {
			zipPath = filepath.Join("_archive", zipPath)
		}

		// Check export size limit.
		totalBytes += rec.SizeBytes
		if totalBytes > maxExportBytes {
			return fmt.Errorf("filestore: export exceeds %d byte limit", maxExportBytes)
		}

		fullPath := filepath.Join(s.basePath, rec.StoragePath)
		f, err := os.Open(fullPath)
		if err != nil {
			slog.Warn("filestore: skip missing file in zip", "path", fullPath, "err", err)
			continue
		}

		w, err := zw.Create(zipPath)
		if err != nil {
			f.Close()
			return fmt.Errorf("filestore: create zip entry: %w", err)
		}
		if _, err := io.Copy(w, f); err != nil {
			f.Close()
			return fmt.Errorf("filestore: write zip entry: %w", err)
		}
		f.Close()
	}
	return nil
}

// Resolve returns metadata for serving a file.
func (s *LocalFileStore) Resolve(ctx context.Context, userID, fileID string) (ResolvedFile, error) {
	var storagePath, contentType, fileName string
	var sizeBytes int64
	err := s.pool.QueryRow(ctx,
		`SELECT storage_path, content_type, file_name, size_bytes
		FROM user_files WHERE id = $1 AND user_id = $2`,
		fileID, userID).Scan(&storagePath, &contentType, &fileName, &sizeBytes)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ResolvedFile{}, fmt.Errorf("filestore: file not found")
		}
		return ResolvedFile{}, fmt.Errorf("filestore: query: %w", err)
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
func (s *LocalFileStore) Archive(ctx context.Context, userID, fileID, replacedByID string) error {
	if s.keepVersions == 0 {
		return s.Delete(ctx, userID, fileID)
	}

	var storagePath, fileType string
	err := s.pool.QueryRow(ctx,
		`SELECT storage_path, file_type FROM user_files WHERE id = $1 AND user_id = $2`,
		fileID, userID).Scan(&storagePath, &fileType)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("filestore: file not found")
		}
		return fmt.Errorf("filestore: query: %w", err)
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
		return fmt.Errorf("filestore: mkdir archive: %w", err)
	}

	// Move file.
	if err := os.Rename(fullPath, archiveFull); err != nil {
		return fmt.Errorf("filestore: rename to archive: %w", err)
	}

	// Clean empty source directories.
	cleanEmptyDirs(filepath.Dir(fullPath), s.basePath)

	// Update registry.
	_, err = s.pool.Exec(ctx,
		`UPDATE user_files SET storage_path = $1, status = 'archived', replaced_by = $2, updated_at = NOW()
		WHERE id = $3 AND user_id = $4`,
		archiveRel, replacedByID, fileID, userID)
	if err != nil {
		return fmt.Errorf("filestore: update archive record: %w", err)
	}
	return nil
}

// ---------- Helpers ----------

// getUserDir returns the relative user directory (library_dir_name or UUID fallback).
func (s *LocalFileStore) getUserDir(ctx context.Context, userID string) (string, error) {
	var dirName *string
	err := s.pool.QueryRow(ctx,
		`SELECT library_dir_name FROM users WHERE id = $1`, userID).Scan(&dirName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("filestore: user not found: %s", userID)
		}
		return "", fmt.Errorf("filestore: query user: %w", err)
	}

	name := userID
	if dirName != nil && *dirName != "" {
		name = *dirName
	}

	// Validate no path traversal in user dir name.
	if strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return "", fmt.Errorf("filestore: invalid library_dir_name: %q", name)
	}
	return name, nil
}

// validateLocalPath ensures the resolved path stays within basePath and is not a symlink escape.
func (s *LocalFileStore) validateLocalPath(relPath string) error {
	cleaned := filepath.Clean(relPath)
	if strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) {
		return fmt.Errorf("filestore: path traversal blocked: %q", relPath)
	}

	fullPath := filepath.Join(s.basePath, cleaned)
	if !strings.HasPrefix(fullPath, s.basePath+string(filepath.Separator)) && fullPath != s.basePath {
		return fmt.Errorf("filestore: path escapes basePath: %q", relPath)
	}

	// Check symlinks on existing paths.
	if _, err := os.Lstat(fullPath); err == nil {
		real, err := filepath.EvalSymlinks(fullPath)
		if err != nil {
			return fmt.Errorf("filestore: eval symlinks: %w", err)
		}
		if !strings.HasPrefix(real, s.basePath) {
			return fmt.Errorf("filestore: symlink escapes basePath: %q -> %q", fullPath, real)
		}
	} else {
		// File doesn't exist yet — check parent directory.
		parentDir := filepath.Dir(fullPath)
		if _, err := os.Lstat(parentDir); err == nil {
			real, err := filepath.EvalSymlinks(parentDir)
			if err != nil {
				return fmt.Errorf("filestore: eval parent symlinks: %w", err)
			}
			if !strings.HasPrefix(real, s.basePath) {
				return fmt.Errorf("filestore: parent symlink escapes basePath: %q -> %q", parentDir, real)
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
func (s *LocalFileStore) removeSidecars(fullPath, fileType string, hasSidecar bool) {
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
			slog.Warn("filestore: remove sidecar", "path", p, "err", err)
		}
	}
}

// safeRemove removes a file only if it matches the expected relative path.
func (s *LocalFileStore) safeRemove(fullPath, relPath string) {
	// Double-check: only remove if the path is within basePath.
	if err := s.validateLocalPath(relPath); err != nil {
		slog.Warn("filestore: skip cleanup of invalid path", "path", relPath, "err", err)
		return
	}
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		slog.Warn("filestore: cleanup failed", "path", fullPath, "err", err)
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
	} else if !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
		return 0, "", nil, readErr
	}

	hashHex = hex.EncodeToString(h.Sum(nil))
	return written, hashHex, first512, nil
}

// Ensure LocalFileStore implements FileStore at compile time.
var _ FileStore = (*LocalFileStore)(nil)

