package shadowfs

import (
	"context"
	"io"
)

// BackendOpts carries optional per-call parameters.
// LocalBackend ignores Bucket. S3Backend uses it for multi-bucket dispatch.
type BackendOpts struct {
	Bucket string // S3 bucket override (leer = Default-Bucket)
}

// StorageBackend handles physical file I/O for a single storage type.
// UnifiedShadowFS dispatches to the correct backend based on user_files.storage_backend.
type StorageBackend interface {
	// Read opens a file for reading.
	Read(ctx context.Context, path string, opts *BackendOpts) (io.ReadCloser, error)

	// Write stores data, returns bytes written + SHA256 hex hash.
	// When exclusive is true, LocalBackend uses O_EXCL (fail if file exists).
	// S3Backend ignores the exclusive flag (hash-based keys are unique).
	// S3Backend buffers entire stream in memory (up to 500 MB) — known limitation.
	// NOTE: Content-type detection (first 512 bytes) is the CALLER's responsibility.
	Write(ctx context.Context, path string, data io.Reader, contentType string, exclusive bool, opts *BackendOpts) (written int64, hash string, err error)

	// Delete removes a single file.
	Delete(ctx context.Context, path string, opts *BackendOpts) error

	// Exists checks if a file exists.
	Exists(ctx context.Context, path string, opts *BackendOpts) (bool, error)

	// DeletePrefix removes all objects under a prefix (GDPR purge, legacy cleanup).
	DeletePrefix(ctx context.Context, prefix string, opts *BackendOpts) error

	// ListFunc iterates all object paths under a prefix, calling fn for each.
	// Streaming — no unbounded []string. Stops on first fn error.
	ListFunc(ctx context.Context, prefix string, opts *BackendOpts, fn func(path string) error) error
}
