package editor

import "time"

// EditSidecar is the JSON document stored in S3 for a track edit session.
type EditSidecar struct {
	Version    int       `json:"version"`
	ActivityID string    `json:"activity_id"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	Edits      []Edit    `json:"edits"`
}

// Edit represents a single edit operation on an activity track.
type Edit struct {
	Type       string   `json:"type"`                  // trim_start, trim_end, crop, hr_spike_smooth
	Timestamp  *string  `json:"timestamp,omitempty"`   // ISO 8601, for trim_start/trim_end
	FromUTC    *string  `json:"from_utc,omitempty"`    // crop start
	ToUTC      *string  `json:"to_utc,omitempty"`      // crop end
	Timestamps []string `json:"timestamps,omitempty"`  // hr_spike_smooth: affected point timestamps
	Method     string   `json:"method,omitempty"`      // interpolate, remove
}

// StreamPoint is the editor's simplified view of a track data point.
type StreamPoint struct {
	Time      time.Time
	HeartRate *int16
	Lat       *float64
	Lng       *float64
}
