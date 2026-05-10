package model

import (
	"encoding/json"
	"time"
)

// Import type constants. Used as values for ImportLogEntry.ImportType.
// New types can be added here without a DB migration — there is no CHECK
// constraint on the import_type column.
const (
	ImportTypeActivity   = "activity"
	ImportTypeRoute      = "route"
	ImportTypeWorkout    = "workout"
	ImportTypeMonitoring = "monitoring"
	ImportTypePhoto      = "photo"
	ImportTypeVideo      = "video"
	ImportTypeDocument   = "document"
	ImportTypeGear       = "gear"
	ImportTypeSegment    = "segment"
	ImportTypeArchive    = "archive"
)

// validImportTypes is the source of truth for IsValidImportType.
// Frontend dropdown options must stay in sync with this set.
var validImportTypes = map[string]struct{}{
	ImportTypeActivity:   {},
	ImportTypeRoute:      {},
	ImportTypeWorkout:    {},
	ImportTypeMonitoring: {},
	ImportTypePhoto:      {},
	ImportTypeVideo:      {},
	ImportTypeDocument:   {},
	ImportTypeGear:       {},
	ImportTypeSegment:    {},
	ImportTypeArchive:    {},
}

// IsValidImportType reports whether v is one of the known import_type values.
// The empty string is NOT valid here (callers should pass a concrete type;
// "unknown" is a UI filter for IS NULL rows, not an import_type value).
func IsValidImportType(v string) bool {
	_, ok := validImportTypes[v]
	return ok
}

type ImportLogEntry struct {
	ID             string          `json:"id"`
	UserID         string          `json:"user_id"`
	SourcePlatform string          `json:"source_platform"`
	Filename       string          `json:"filename"`
	FileHash       *string         `json:"file_hash,omitempty"`
	FileSizeBytes  *int64          `json:"file_size_bytes,omitempty"`
	Status         string          `json:"status"`
	ActivityID     *string         `json:"activity_id,omitempty"`
	ErrorMsg       *string         `json:"error_msg,omitempty"`
	DurationMs     *int            `json:"duration_ms,omitempty"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
	BatchID        *string         `json:"batch_id,omitempty"`
	SkipReason     *string         `json:"skip_reason,omitempty"`
	ImportType     *string         `json:"import_type,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}
