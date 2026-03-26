package shadowfs

import (
	"bytes"
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
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UnifiedShadowFS implements ShadowFS with per-file backend dispatch.
// Replaces LocalShadowFS + S3ShadowFS.
type UnifiedShadowFS struct {
	pool             *pgxpool.Pool
	local            StorageBackend // nil when no local path configured
	s3               StorageBackend // nil when no S3 credentials configured
	target           atomic.Value   // stores string: "local" or "s3"
	basePath         string         // LIBRARY_BASE_PATH (for getUserDir, schema rendering)
	renderer         *SchemaRenderer
	keepVersions     int
	maxUserBytes     int64
	s3BucketDefault  string
	s3BucketActivity string
	s3BucketMedia    string
}

// NewUnifiedShadowFS creates a UnifiedShadowFS.
func NewUnifiedShadowFS(
	pool *pgxpool.Pool,
	local, s3 StorageBackend,
	targetBackend string,
	basePath string,
	renderer *SchemaRenderer,
	keepVersions int,
	maxUserBytes int64,
	s3BucketDefault, s3BucketActivity, s3BucketMedia string,
) *UnifiedShadowFS {
	u := &UnifiedShadowFS{
		pool:             pool,
		local:            local,
		s3:               s3,
		basePath:         basePath,
		renderer:         renderer,
		keepVersions:     keepVersions,
		maxUserBytes:     maxUserBytes,
		s3BucketDefault:  s3BucketDefault,
		s3BucketActivity: s3BucketActivity,
		s3BucketMedia:    s3BucketMedia,
	}
	u.target.Store(targetBackend)
	return u
}

// backendFor returns the StorageBackend for the given storage_backend value.
func (u *UnifiedShadowFS) backendFor(storageBackend string) (StorageBackend, error) {
	switch storageBackend {
	case "local":
		if u.local == nil {
			return nil, fmt.Errorf("local backend not configured but file has storage_backend=local")
		}
		return u.local, nil
	case "s3":
		if u.s3 == nil {
			return nil, fmt.Errorf("s3 backend not configured but file has storage_backend=s3")
		}
		return u.s3, nil
	default:
		return nil, fmt.Errorf("unknown storage_backend: %q", storageBackend)
	}
}

// targetBackend returns the current write target.
func (u *UnifiedShadowFS) targetBackend() (StorageBackend, string, error) {
	t := u.target.Load().(string)
	be, err := u.backendFor(t)
	return be, t, err
}

// bucketFor returns the S3 bucket for a given file type (Spec §2.2).
func (u *UnifiedShadowFS) bucketFor(fileType string) string {
	switch fileType {
	case "activity", "health":
		if u.s3BucketActivity != "" {
			return u.s3BucketActivity
		}
	case "media", "document", "thumb", "preview":
		if u.s3BucketMedia != "" {
			return u.s3BucketMedia
		}
	}
	return u.s3BucketDefault
}

// BucketFor returns the S3 bucket for a given file type. Exported for Reconciler + admin_storage.
func (u *UnifiedShadowFS) BucketFor(fileType string) string {
	return u.bucketFor(fileType)
}

// bucketOpts returns BackendOpts with the correct bucket for a file type.
func (u *UnifiedShadowFS) bucketOpts(fileType string) *BackendOpts {
	b := u.bucketFor(fileType)
	if b == "" {
		return nil
	}
	return &BackendOpts{Bucket: b}
}

// getUserDir returns the library directory name for a user.
func (u *UnifiedShadowFS) getUserDir(ctx context.Context, userID string) (string, error) {
	var dirName *string
	err := u.pool.QueryRow(ctx,
		`SELECT library_dir_name FROM users WHERE id = $1`, userID).Scan(&dirName)
	if err != nil {
		return "", fmt.Errorf("shadowfs: query user: %w", err)
	}
	name := userID
	if dirName != nil && *dirName != "" {
		name = *dirName
	}
	if strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return "", fmt.Errorf("shadowfs: invalid library_dir_name: %q", name)
	}
	return name, nil
}

// getUserDirOrFallback returns the user's library directory name, falling back to the
// raw userID if the user row is gone (e.g. CASCADE delete already ran). Used by
// DeleteAllForUser for the safety-net local RemoveAll.
func (u *UnifiedShadowFS) getUserDirOrFallback(ctx context.Context, userID string) string {
	dir, err := u.getUserDir(ctx, userID)
	if err == nil {
		return dir
	}
	slog.Warn("shadowfs: getUserDir failed, falling back to userID", "userID", userID, "err", err)
	return userID // User row gone (CASCADE) — fall back to UUID
}

// allBuckets returns all distinct configured S3 buckets.
func (u *UnifiedShadowFS) allBuckets() []string {
	seen := map[string]bool{}
	var buckets []string
	for _, ft := range []string{"activity", "media", "document"} {
		b := u.bucketFor(ft)
		if b != "" && !seen[b] {
			seen[b] = true
			buckets = append(buckets, b)
		}
	}
	return buckets
}

// peekReader wraps an io.Reader to capture the first N bytes for content-type
// detection while allowing the full stream to be read exactly once.
type peekReader struct {
	peeked []byte
	r      io.Reader // io.MultiReader(peeked, rest)
}

func newPeekReader(r io.Reader, n int) (*peekReader, error) {
	buf := make([]byte, n)
	nr, err := io.ReadAtLeast(r, buf, 1)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, err
	}
	peeked := buf[:nr]
	return &peekReader{
		peeked: peeked,
		r:      io.MultiReader(bytes.NewReader(peeked), r),
	}, nil
}

func (p *peekReader) Read(b []byte) (int, error) { return p.r.Read(b) }

// Put stores a file and registers it in user_files.
func (u *UnifiedShadowFS) Put(ctx context.Context, entry FileEntry, data io.Reader) (FileRecord, error) {
	// 1. Validate file_type.
	dir, ok := fileTypeDirName[entry.FileType]
	if !ok {
		return FileRecord{}, fmt.Errorf("shadowfs: unknown file_type: %q", entry.FileType)
	}

	// 2. Validate ParentID as UUID (if set).
	if entry.ParentID != nil && *entry.ParentID != "" {
		if _, err := uuid.Parse(*entry.ParentID); err != nil {
			return FileRecord{}, fmt.Errorf("shadowfs: invalid parent_id: %w", err)
		}
	}

	// 3. Quota pre-check (excludes thumb/preview — they don't count toward quota).
	if entry.FileType != "thumb" && entry.FileType != "preview" && u.maxUserBytes > 0 {
		used, err := diskUsage(ctx, u.pool, entry.UserID)
		if err != nil {
			return FileRecord{}, fmt.Errorf("shadowfs: quota check: %w", err)
		}
		if used+entry.SizeBytes > u.maxUserBytes {
			return FileRecord{}, fmt.Errorf("shadowfs: quota exceeded (used %d + %d > %d)", used, entry.SizeBytes, u.maxUserBytes)
		}
	}

	ext := sanitizeExt(filepath.Ext(entry.FileName))

	// 4. Get target backend.
	be, targetName, err := u.targetBackend()
	if err != nil {
		return FileRecord{}, err
	}
	opts := u.bucketOpts(entry.FileType)

	// Peek first 512 bytes for content-type detection (both paths need it).
	pr, err := newPeekReader(data, 512)
	if err != nil {
		return FileRecord{}, fmt.Errorf("shadowfs: peek: %w", err)
	}
	contentType := DetectContentType(entry.FileName, entry.ContentType, pr.peeked)

	// 5. Path computation + write (differs by target).
	var storagePath, fileHash string
	var written int64

	switch targetName {
	case "local":
		userDir, err := u.getUserDir(ctx, entry.UserID)
		if err != nil {
			return FileRecord{}, err
		}

		if entry.FileType == "edit" {
			if entry.ParentID == nil {
				return FileRecord{}, fmt.Errorf("shadowfs: edit sidecar requires ParentID")
			}
			storagePath = filepath.Join(userDir, DirForType("edit"), *entry.ParentID+".json")
		} else {
			schema := u.renderer.DefaultSchema(entry.FileType)
			var userSchema string
			schemaErr := u.pool.QueryRow(ctx,
				`SELECT schema_template FROM user_file_schemas WHERE user_id = $1 AND file_type = $2`,
				entry.UserID, entry.FileType).Scan(&userSchema)
			if schemaErr != nil && !errors.Is(schemaErr, pgx.ErrNoRows) {
				return FileRecord{}, fmt.Errorf("shadowfs: query schema: %w", schemaErr)
			}
			if schemaErr == nil && userSchema != "" {
				schema = userSchema
			}
			rendered := u.renderer.RenderPath(entry.FileType, schema, entry.Tags, ext)
			storagePath = filepath.Join(userDir, DirForType(entry.FileType), rendered)
		}

		// Security check.
		if err := ValidateRelPath(u.basePath, storagePath); err != nil {
			return FileRecord{}, err
		}

		// Collision resolution (skip for edit sidecars — they upsert).
		if entry.FileType != "edit" {
			storagePath = ResolveCollision(u.basePath, storagePath)
		}

		// Write with O_EXCL race retry (except edit sidecars which overwrite).
		exclusive := entry.FileType != "edit"
		for attempt := range 10 {
			w, h, writeErr := be.Write(ctx, storagePath, pr, contentType, exclusive, opts)
			if writeErr == nil {
				written = w
				fileHash = h
				break
			}
			if !errors.Is(writeErr, os.ErrExist) {
				return FileRecord{}, fmt.Errorf("shadowfs: write: %w", writeErr)
			}
			// Another process won the race — re-resolve collision.
			storagePath = ResolveCollision(u.basePath, storagePath)
			if attempt == 9 {
				return FileRecord{}, fmt.Errorf("shadowfs: exhausted 10 race retries")
			}
		}

	case "s3":
		// Buffer all data — compute hash for content-addressed path.
		allData, readErr := io.ReadAll(pr)
		if readErr != nil {
			return FileRecord{}, fmt.Errorf("shadowfs: read all: %w", readErr)
		}
		h := sha256.Sum256(allData)
		fileHash = hex.EncodeToString(h[:])
		storagePath = entry.UserID + "/" + dir + "/" + fileHash + ext

		w, _, writeErr := be.Write(ctx, storagePath, bytes.NewReader(allData), contentType, false, opts)
		if writeErr != nil {
			return FileRecord{}, fmt.Errorf("shadowfs: s3 write: %w", writeErr)
		}
		written = w
	}

	// 6. Database INSERT/UPSERT.
	var rec FileRecord
	sizeBytes := entry.SizeBytes
	if sizeBytes == 0 {
		sizeBytes = written
	}

	if entry.FileType == "edit" {
		// Edit sidecar: upsert by (storage_backend, storage_path).
		err = u.pool.QueryRow(ctx,
			`INSERT INTO user_files (user_id, file_type, file_name, content_type, storage_backend,
			    storage_path, size_bytes, file_hash, parent_type, parent_id, has_sidecar, status)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'active')
			 ON CONFLICT (storage_backend, storage_path) DO UPDATE SET
			    size_bytes = EXCLUDED.size_bytes, file_hash = EXCLUDED.file_hash,
			    content_type = EXCLUDED.content_type, updated_at = NOW()
			 RETURNING id, user_id, file_type, file_name, content_type, storage_backend,
			    storage_path, size_bytes, file_hash, parent_type, parent_id, has_sidecar,
			    is_renaming, status, replaced_by, created_at, updated_at`,
			entry.UserID, entry.FileType, entry.FileName, contentType, targetName,
			storagePath, sizeBytes, fileHash, entry.ParentType, entry.ParentID, entry.HasSidecar,
		).Scan(&rec.ID, &rec.UserID, &rec.FileType, &rec.FileName, &rec.ContentType,
			&rec.StorageBackend, &rec.StoragePath, &rec.SizeBytes, &rec.FileHash,
			&rec.ParentType, &rec.ParentID, &rec.HasSidecar, &rec.IsRenaming,
			&rec.Status, &rec.ReplacedBy, &rec.CreatedAt, &rec.UpdatedAt)
	} else if targetName == "s3" {
		// S3: content-addressed — upsert by (storage_backend, storage_path).
		err = u.pool.QueryRow(ctx,
			`INSERT INTO user_files (user_id, file_type, file_name, content_type, storage_backend,
			    storage_path, size_bytes, file_hash, parent_type, parent_id, has_sidecar, status)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'active')
			 ON CONFLICT (storage_backend, storage_path)
			 DO UPDATE SET file_name = EXCLUDED.file_name, content_type = EXCLUDED.content_type,
			    size_bytes = EXCLUDED.size_bytes, file_hash = EXCLUDED.file_hash, updated_at = NOW()
			 RETURNING id, user_id, file_type, file_name, content_type, storage_backend,
			    storage_path, size_bytes, file_hash, parent_type, parent_id, has_sidecar,
			    is_renaming, status, replaced_by, created_at, updated_at`,
			entry.UserID, entry.FileType, entry.FileName, contentType, targetName,
			storagePath, sizeBytes, fileHash, entry.ParentType, entry.ParentID, entry.HasSidecar,
		).Scan(&rec.ID, &rec.UserID, &rec.FileType, &rec.FileName, &rec.ContentType,
			&rec.StorageBackend, &rec.StoragePath, &rec.SizeBytes, &rec.FileHash,
			&rec.ParentType, &rec.ParentID, &rec.HasSidecar, &rec.IsRenaming,
			&rec.Status, &rec.ReplacedBy, &rec.CreatedAt, &rec.UpdatedAt)
	} else {
		// Local: plain insert (collision already resolved via O_EXCL).
		err = u.pool.QueryRow(ctx,
			`INSERT INTO user_files (user_id, file_type, file_name, content_type, storage_backend,
			    storage_path, size_bytes, file_hash, parent_type, parent_id, has_sidecar, status)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'active')
			 RETURNING id, user_id, file_type, file_name, content_type, storage_backend,
			    storage_path, size_bytes, file_hash, parent_type, parent_id, has_sidecar,
			    is_renaming, status, replaced_by, created_at, updated_at`,
			entry.UserID, entry.FileType, entry.FileName, contentType, targetName,
			storagePath, sizeBytes, fileHash, entry.ParentType, entry.ParentID, entry.HasSidecar,
		).Scan(&rec.ID, &rec.UserID, &rec.FileType, &rec.FileName, &rec.ContentType,
			&rec.StorageBackend, &rec.StoragePath, &rec.SizeBytes, &rec.FileHash,
			&rec.ParentType, &rec.ParentID, &rec.HasSidecar, &rec.IsRenaming,
			&rec.Status, &rec.ReplacedBy, &rec.CreatedAt, &rec.UpdatedAt)
	}
	if err != nil {
		// Cleanup written file on DB failure.
		if delErr := be.Delete(ctx, storagePath, opts); delErr != nil {
			slog.Error("shadowfs: cleanup after DB error failed",
				"path", storagePath, "dbErr", err, "delErr", delErr)
		}
		return FileRecord{}, fmt.Errorf("shadowfs: insert user_files: %w", err)
	}

	// 7. Post-write quota check.
	if entry.FileType != "thumb" && entry.FileType != "preview" && u.maxUserBytes > 0 && written > 0 {
		used, qErr := diskUsage(ctx, u.pool, entry.UserID)
		if qErr != nil {
			slog.Warn("shadowfs: post-write quota check failed", "error", qErr)
		} else if used > u.maxUserBytes {
			// Roll back: delete file + DB row.
			if delErr := be.Delete(ctx, storagePath, opts); delErr != nil {
				slog.Error("shadowfs: quota rollback delete failed", "path", storagePath, "err", delErr)
			}
			_, _ = u.pool.Exec(ctx, `DELETE FROM user_files WHERE id = $1`, rec.ID)
			return FileRecord{}, fmt.Errorf("shadowfs: storage quota exceeded (post-write check)")
		}
	}

	return rec, nil
}
