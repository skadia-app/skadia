package importer

import "time"

// ImportMetadata carries platform-specific identifiers from the platform importer.
type ImportMetadata struct {
	GarminActivityID int64
	StravaActivityID int64
	KomootTourID     int64
	SourcePlatform   string           // "garmin" | "strava" | "komoot" | "webui" | "url" | "hotfolder"
	ParentID         string           // non-empty for multisport child activities
	ConflictStrategy ConflictStrategy // "enrich_missing" | "replace_all" | "skip" — empty = enrich_missing
}

// ActivityResult is returned by ProcessActivity.
type ActivityResult struct {
	ActivityID            string
	Status                string   // "created" | "enriched" | "replaced" | "skipped"
	PlatformIDsBackfilled []string // e.g. ["strava_activity_id"]
}

// PhotoMetadata carries information about a photo being imported.
type PhotoMetadata struct {
	OriginalFilename string
	ContentType      string
	TakenAt          *time.Time
	Lat              *float64
	Lng              *float64
	LocationSource   string // "exif" | "activity_track" | "manual"
	SourcePlatform   string
	IsVideo          bool
	DurationS        *float64
	VideoWidth       *int
	VideoHeight      *int
}

// PhotoResult is returned by ProcessPhoto.
type PhotoResult struct {
	PhotoID string
	Status  string // "created" | "replaced" | "duplicate_exact" | "duplicate_phash"
	PHash   int64  // 0 if computation failed (non-pointer, matches spec section 3.3)
}

// OrphanResult is returned by ProcessOrphanPhoto.
type OrphanResult struct {
	Status       string // "matched" | "queued"
	ActivityID   string // only if "matched"
	QueueEntryID string // only if "queued"
}
