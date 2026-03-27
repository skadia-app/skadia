package model

import "time"

type ActivityPhoto struct {
	ID               string     `json:"id"`
	UserID           string     `json:"user_id"`
	ActivityID       *string    `json:"activity_id"`
	FileID           *string    `json:"-"` // user_files.id (ShadowFS registry)
	S3Bucket         string     `json:"-"`
	S3Key            string     `json:"-"`
	S3KeyThumb       string     `json:"-"`
	S3KeyPreview     string     `json:"-"` // large preview (WQHD/4K); empty = use original
	OriginalFilename string     `json:"original_filename"`
	ContentType      string     `json:"content_type"`
	FileSizeBytes    int64      `json:"file_size_bytes"`
	FileHash         *string    `json:"-"`
	TakenAt          *time.Time `json:"taken_at"`
	Lat              *float64   `json:"lat"`
	Lng              *float64   `json:"lng"`
	LocationSource   *string    `json:"location_source"`
	CreatedAt        time.Time  `json:"created_at"`
	// Video-Metadaten (nur befüllt wenn is_video=true)
	IsVideo     bool     `json:"is_video"`
	DurationS   *float64 `json:"duration_s,omitempty"` // Video-Länge in Sekunden
	VideoWidth  *int     `json:"video_width,omitempty"`
	VideoHeight *int     `json:"video_height,omitempty"`
	// Image metadata
	PHash  *int64 `json:"phash,omitempty" db:"phash"`
	Width  *int   `json:"width,omitempty" db:"width"`
	Height *int   `json:"height,omitempty" db:"height"`
	// Computed URLs (nicht in DB)
	ThumbURL    string `json:"thumb_url,omitempty"`
	PreviewURL  string `json:"preview_url,omitempty"` // large preview (WQHD/4K) or same as full_url
	FullURL     string `json:"full_url,omitempty"`
	ThumbFileID string `json:"thumb_file_id,omitempty"` // user_files.id of the thumbnail (ShadowFS)
}

// MapPhotoEntry ist ein kompakter Eintrag für den Map-Overlay
type MapPhotoEntry struct {
	ID            string     `json:"id"`
	ActivityID    *string    `json:"activity_id"`
	ActivityTitle *string    `json:"activity_title"`
	Lat           float64    `json:"lat"`
	Lng           float64    `json:"lng"`
	TakenAt       *time.Time `json:"taken_at"`
	ThumbURL      string     `json:"thumb_url"`
}

// PhotoFeedActivitySummary ist die kompakte Aktivitätszusammenfassung im Photo-Feed.
type PhotoFeedActivitySummary struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	SportType      string   `json:"sport_type"`
	StartedAt      *string  `json:"started_at,omitempty"` // RFC3339
	DistanceM      *float64 `json:"distance_m,omitempty"`
	MovingTimeS    *int     `json:"moving_time_s,omitempty"`
	ElevationGainM *float64 `json:"elevation_gain_m,omitempty"`
	TrackGeoJSON   *string  `json:"track_geojson,omitempty"`
	Lat            *float64 `json:"lat,omitempty"`
	Lng            *float64 `json:"lng,omitempty"`
}

// PhotoFeedGroup ist eine Gruppe im paginierten Feed.
type PhotoFeedGroup struct {
	GroupKey  string                    `json:"group_key"`
	GroupType string                    `json:"group_type"` // "activity" | "day"
	Activity  *PhotoFeedActivitySummary `json:"activity,omitempty"`
	Photos    []ActivityPhoto           `json:"photos"`
}

// PhotoFeedPage ist die paginierte Antwort für GET /photos.
type PhotoFeedPage struct {
	Groups   []PhotoFeedGroup `json:"groups"`
	NextPage *int             `json:"next_page,omitempty"`
	Total    int              `json:"total"`
}

// PhotoSearchResult ist ein einzelnes Suchergebnis.
type PhotoSearchResult struct {
	Photo      ActivityPhoto             `json:"photo"`
	Activity   *PhotoFeedActivitySummary `json:"activity,omitempty"`
	Similarity float64                   `json:"similarity"`
	MatchType  string                    `json:"match_type"` // "clip" | "title"
}
