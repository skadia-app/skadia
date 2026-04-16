package shadowfs

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"time"
)

// ErrNotFound is returned by GetCompanionAt (and similar path-based readers)
// when the requested object does not exist in the underlying storage.
var ErrNotFound = errors.New("shadowfs: object not found")

// ShadowFS is the unified storage abstraction for all user files.
type ShadowFS interface {
	Put(ctx context.Context, entry FileEntry, data io.Reader) (FileRecord, error)
	Get(ctx context.Context, userID, fileID string) (io.ReadCloser, error)
	Delete(ctx context.Context, userID, fileID string) error
	DeleteAllForUser(ctx context.Context, userID string) error
	List(ctx context.Context, userID string, fileType *string) ([]FileRecord, error)
	// StreamToZip writes all exportable user files into zw. The return value
	// `failedPaths` lists ZIP entry paths whose content could NOT be copied
	// (e.g. corrupt S3 blobs, transient network errors): the entry exists in
	// the ZIP but is empty/truncated. Callers SHOULD surface this list in the
	// export manifest so the user knows the export is incomplete.
	StreamToZip(ctx context.Context, userID string, zw *zip.Writer) (failedPaths []string, err error)
	Resolve(ctx context.Context, userID, fileID string) (ResolvedFile, error)
	DiskUsage(ctx context.Context, userID string) (int64, error)
	Archive(ctx context.Context, userID, fileID, replacedByID string) error
	PutCompanion(ctx context.Context, userID, fileID, suffix string, data []byte) error
	GetCompanion(ctx context.Context, userID, fileID, suffix string) ([]byte, error)
	DeleteCompanions(ctx context.Context, userID, fileID string) error

	// PutCompanionAt writes raw bytes to an arbitrary ShadowFS path without
	// requiring a user_files row. Used for fusion + manual-entry sidecars that
	// do not correspond to a source file.
	// path must be a relative key such as "{user_id}/activity-{activity_id}.meta".
	PutCompanionAt(ctx context.Context, path string, data []byte) error

	// GetCompanionAt reads raw bytes from an arbitrary ShadowFS path.
	// Returns (nil, ErrNotFound) when the object does not exist.
	GetCompanionAt(ctx context.Context, path string) ([]byte, error)

	// DeleteCompanionAt removes an arbitrary-path sidecar.
	// Errors are silently swallowed (idempotent cleanup).
	DeleteCompanionAt(ctx context.Context, path string) error
}

// FileEntry is the input for Put().
type FileEntry struct {
	UserID      string
	FileType    string // "activity", "media", "document", "health", "route", "edit", "chat"
	ContentType string // MIME type
	FileName    string // Original filename (display)
	SizeBytes   int64
	FileHash    string            // SHA256
	ParentType  *string           // "activity", "gear", "gear_component", "route", "tag", "marker"
	ParentID    *string           // ID of owner
	HasSidecar  bool              // true if .meta sidecar will follow
	Tags        map[string]string // Metadata for schema rendering
}

// FileRecord is a registry entry returned from Put()/List().
type FileRecord struct {
	ID             string
	UserID         string
	FileType       string
	StorageBackend string // "local" or "s3"
	StoragePath    string // Relative path (local) or S3 key
	ContentType    string
	FileName       string
	SizeBytes      int64
	FileHash       *string
	ParentType     *string
	ParentID       *string
	HasSidecar     bool
	IsRenaming     bool
	Status         string  // "active", "archived", "purging"
	ReplacedBy     *string // file_id of replacement (when archived)
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ResolvedFile contains access metadata for serving a file.
type ResolvedFile struct {
	Type        string    // "local" or "s3"
	Path        string    // Filesystem path (local only)
	URL         string    // Pre-signed URL (S3 only)
	ExpiresAt   time.Time // URL expiry (S3 only)
	ContentType string
	FileName    string
	SizeBytes   int64
}

// Ptr returns a pointer to the given string value.
// Exported for use by handlers constructing FileEntry values.
//
//go:fix inline
func Ptr(s string) *string { return new(s) }

// fileTypeDirName maps file types to their directory names in the library.
var fileTypeDirName = map[string]string{
	"activity": "activities",
	"media":    "media",
	"document": "documents",
	"health":   "health",
	"route":    "routes",
	"report":   "reports",
	"edit":     "activities/_edits",
	"chat":     "chat",
	"thumb":    "media/_thumbs",
	"preview":  "media/_previews",
}

// DirForType returns the directory name for a file type.
// Returns "other" for unknown file types so that unrecognised types land in a
// predictable, non-traversable directory rather than inheriting the raw fileType
// string which could contain characters unsafe in path construction.
func DirForType(fileType string) string {
	if d, ok := fileTypeDirName[fileType]; ok {
		return d
	}
	return "other"
}
