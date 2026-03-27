package shadowfs

import (
	"archive/zip"
	"context"
	"io"
	"time"
)

// ShadowFS is the unified storage abstraction for all user files.
type ShadowFS interface {
	Put(ctx context.Context, entry FileEntry, data io.Reader) (FileRecord, error)
	Get(ctx context.Context, userID, fileID string) (io.ReadCloser, error)
	Delete(ctx context.Context, userID, fileID string) error
	DeleteAllForUser(ctx context.Context, userID string) error
	List(ctx context.Context, userID string, fileType *string) ([]FileRecord, error)
	StreamToZip(ctx context.Context, userID string, zw *zip.Writer) error
	Resolve(ctx context.Context, userID, fileID string) (ResolvedFile, error)
	DiskUsage(ctx context.Context, userID string) (int64, error)
	Archive(ctx context.Context, userID, fileID, replacedByID string) error
	PutCompanion(ctx context.Context, userID, fileID, suffix string, data []byte) error
	GetCompanion(ctx context.Context, userID, fileID, suffix string) ([]byte, error)
	DeleteCompanions(ctx context.Context, userID, fileID string) error
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
func Ptr(s string) *string { return &s }

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
