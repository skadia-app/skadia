package gdprartifacts

import (
	"context"
	"io"
	"time"
)

type ObjectRole string
type ObjectState string
type ArtifactFormat string

const (
	RoleSingleZIP   ObjectRole = "single_zip"
	RoleManifest    ObjectRole = "manifest"
	RoleMetadataZIP ObjectRole = "metadata_zip"
	RoleFilesZIP    ObjectRole = "files_zip"

	ObjectPresent ObjectState = "present"
	ObjectMissing ObjectState = "missing"
	ObjectUnknown ObjectState = "unknown"

	ArtifactFormatSingleZIP ArtifactFormat = "single_zip"
	ArtifactFormatMultipart ArtifactFormat = "multipart"
)

type ArtifactPrefix struct {
	Kind   string
	UserID string
	JobID  string
}

type ObjectSpec struct {
	Kind        string
	UserID      string
	JobID       string
	Role        ObjectRole
	Name        string
	ContentType string
	ExpiresAt   time.Time
}

type ObjectRef struct {
	Backend     string     `json:"backend"`
	Kind        string     `json:"kind,omitempty"`
	UserID      string     `json:"user_id,omitempty"`
	JobID       string     `json:"job_id,omitempty"`
	Role        ObjectRole `json:"role"`
	Name        string     `json:"name"`
	Bucket      string     `json:"bucket,omitempty"`
	Key         string     `json:"key,omitempty"`
	Path        string     `json:"path,omitempty"`
	SHA256      string     `json:"sha256"`
	SizeBytes   int64      `json:"size_bytes"`
	ContentType string     `json:"content_type"`
	CreatedAt   time.Time  `json:"created_at,omitempty"`
	ExpiresAt   time.Time  `json:"expires_at,omitempty"`
}

type RangeSpec struct {
	Start *int64
	End   *int64
}

type ObjectStat struct {
	State     ObjectState
	SizeBytes int64
	SHA256    string
	Err       error
}

type ObjectReader interface {
	io.ReadCloser
	Size() int64
	ContentType() string
}

type SizedReaderAt interface {
	io.ReaderAt
	io.Closer
	Size() int64
}

type ObjectWriter interface {
	io.Writer
	Commit(context.Context) (ObjectRef, error)
	Abort(context.Context) error
}

type Store interface {
	CreateWriter(context.Context, ObjectSpec) (ObjectWriter, error)
	Open(context.Context, ObjectRef, *RangeSpec) (ObjectReader, error)
	OpenReaderAt(context.Context, ObjectRef) (SizedReaderAt, error)
	Stat(context.Context, ObjectRef) (ObjectStat, error)
	Delete(context.Context, ObjectRef) error
	DeletePrefix(context.Context, ArtifactPrefix) error
}

type LifecycleEnsurer interface {
	EnsureLifecycle(context.Context) error
}

type IncompleteMultipartAborter interface {
	AbortIncompleteUploads(context.Context, ArtifactPrefix) error
}

type IncompleteMultipartCounter interface {
	CountIncompleteUploads(context.Context, ArtifactPrefix) (int, error)
}

type CapacityRequest struct {
	Kind          string
	UserID        string
	ExpectedBytes int64
}

type CapacityResult struct {
	AvailableBytes int64
	Allowed        bool
	Reason         string
}

type CapacityChecker interface {
	CheckAvailable(context.Context, CapacityRequest) (CapacityResult, error)
}
