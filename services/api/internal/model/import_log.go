package model

import (
	"encoding/json"
	"time"
)

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
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}
