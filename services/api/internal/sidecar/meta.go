package sidecar

import (
	"encoding/json"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

type MetaFile struct {
	SchemaVersion    int       `json:"schema_version"`
	SkadiaID        string    `json:"skadia_id"`
	OriginalFilename string    `json:"original_filename"`
	FileHashSHA256   string    `json:"file_hash_sha256"`
	FileFormat       string    `json:"file_format"`
	SportType        string    `json:"sport_type"`
	SubType          string    `json:"sub_type"`
	Title            string    `json:"title,omitempty"`
	StartedAt        time.Time `json:"started_at"`
	DistanceM        float64   `json:"distance_m,omitempty"`
	ElevationGainM   float64   `json:"elevation_gain_m,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

func GenerateMeta(activityID string, data *model.ActivityData) *MetaFile {
	return &MetaFile{
		SchemaVersion:    2,
		SkadiaID:        activityID,
		OriginalFilename: data.OriginalName,
		FileHashSHA256:   data.FileHash,
		FileFormat:       data.FileFormat,
		SportType:        data.SportType,
		SubType:          data.SubType,
		StartedAt:        data.StartedAt,
		DistanceM:        data.DistanceM,
		ElevationGainM:   data.ElevationGainM,
		CreatedAt:        time.Now().UTC(),
	}
}

func (m *MetaFile) Marshal() ([]byte, error) {
	return json.MarshalIndent(m, "", "  ")
}
