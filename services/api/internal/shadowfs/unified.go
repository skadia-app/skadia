package shadowfs

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"

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
