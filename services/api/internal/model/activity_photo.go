package model

import "time"

type ActivityPhoto struct {
	ID               string     `json:"id"`
	UserID           string     `json:"user_id"`
	ActivityID       *string    `json:"activity_id"`
	S3Bucket         string     `json:"-"`
	S3Key            string     `json:"-"`
	S3KeyThumb       string     `json:"-"`
	OriginalFilename string     `json:"original_filename"`
	ContentType      string     `json:"content_type"`
	FileSizeBytes    int64      `json:"file_size_bytes"`
	FileHash         *string    `json:"-"`
	TakenAt          *time.Time `json:"taken_at"`
	Lat              *float64   `json:"lat"`
	Lng              *float64   `json:"lng"`
	LocationSource   *string    `json:"location_source"`
	CreatedAt        time.Time  `json:"created_at"`
	// Computed URLs (nicht in DB)
	ThumbURL string `json:"thumb_url,omitempty"`
	FullURL  string `json:"full_url,omitempty"`
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
