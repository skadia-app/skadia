package sidecar

import (
	"encoding/json"
	"time"
)

// ManualActivityFile is the S3-stored source file for manual activities and
// Strava CSV-only imports. SchemaVersion 3 distinguishes it from v2 .meta.
type ManualActivityFile struct {
	SchemaVersion  int               `json:"schema_version"`
	SkadiaID      string            `json:"skadia_id,omitempty"`
	FileFormat     string            `json:"file_format"`
	SportType      string            `json:"sport_type"`
	SubType        string            `json:"sub_type,omitempty"`
	Title          string            `json:"title,omitempty"`
	Description    string            `json:"description,omitempty"`
	StartedAt      time.Time         `json:"started_at"`
	ElapsedTimeS   float64           `json:"elapsed_time_s,omitempty"`
	MovingTimeS    float64           `json:"moving_time_s,omitempty"`
	DistanceM      float64           `json:"distance_m,omitempty"`
	ElevationGainM float64           `json:"elevation_gain_m,omitempty"`
	ElevationLossM float64           `json:"elevation_loss_m,omitempty"`
	Tags           []string          `json:"tags,omitempty"`
	GearID         string            `json:"gear_id,omitempty"`
	IsEbike        bool              `json:"is_ebike,omitempty"`
	Source         string            `json:"source,omitempty"`
	SourceFields   map[string]string `json:"source_fields,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
}

func (m *ManualActivityFile) Marshal() ([]byte, error) {
	return json.MarshalIndent(m, "", "  ")
}

func ParseManualActivityFile(data []byte) (*ManualActivityFile, error) {
	var m ManualActivityFile
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}
