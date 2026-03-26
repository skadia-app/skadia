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

	// v3 — User-editable fields (not regenerable from FIT)
	Description   string   `json:"description,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	GearID        string   `json:"gear_id,omitempty"`
	GearName      string   `json:"gear_name,omitempty"`
	Visibility    string   `json:"visibility,omitempty"`
	WorkoutFeel   *int     `json:"workout_feel,omitempty"`
	WorkoutRpe    *int     `json:"workout_rpe,omitempty"`
	IsEbike       bool     `json:"is_ebike,omitempty"`
	IsVirtual     bool     `json:"is_virtual,omitempty"`
	IsManualEntry bool     `json:"is_manual_entry,omitempty"`
	Source        string   `json:"source,omitempty"`

	// v3.1 — Felder die für vollständigen Restore nötig sind
	UserID                 string   `json:"user_id"`
	ExcludeFromHeatmap     bool     `json:"exclude_from_heatmap,omitempty"`
	ExcludeFromStats       bool     `json:"exclude_from_stats,omitempty"`
	IsIndoor               bool     `json:"is_indoor,omitempty"`
	ExcludeLiftSubtraction bool     `json:"exclude_lift_subtraction,omitempty"`
	LocationLat            *float64 `json:"location_lat,omitempty"`
	LocationLng            *float64 `json:"location_lng,omitempty"`
	TitleIsManual          bool     `json:"title_is_manual,omitempty"`
}

// GenerateMeta creates a v3 MetaFile from ActivityData (FIT-parsed import data).
// User-editable fields are left empty — use GenerateMetaV3 to include them.
func GenerateMeta(activityID string, data *model.ActivityData) *MetaFile {
	return GenerateMetaV3(activityID, data, nil)
}

// GenerateMetaV3 creates a v3 MetaFile combining FIT data and user-editable fields.
//   - If data != nil: FIT fields come from data (import path).
//   - If data == nil && activity != nil: FIT fields come from the Activity model.
//   - User-editable fields always come from activity (when not nil).
func GenerateMetaV3(activityID string, data *model.ActivityData, activity *model.Activity) *MetaFile {
	m := &MetaFile{
		SchemaVersion: 3,
		SkadiaID:     activityID,
		CreatedAt: func() time.Time {
			if activity != nil {
				return activity.CreatedAt
			}
			return time.Now().UTC()
		}(),
	}

	// FIT fields: prefer ActivityData, fall back to Activity model.
	if data != nil {
		m.OriginalFilename = data.OriginalName
		m.FileHashSHA256 = data.FileHash
		m.FileFormat = data.FileFormat
		m.SportType = data.SportType
		m.SubType = data.SubType
		m.StartedAt = data.StartedAt
		m.DistanceM = data.DistanceM
		m.ElevationGainM = data.ElevationGainM
	} else if activity != nil {
		m.OriginalFilename = derefStr(activity.OriginalFilename)
		m.FileHashSHA256 = derefStr(activity.FileHash)
		m.FileFormat = derefStr(activity.OriginalFileFormat)
		m.SportType = activity.SportType
		m.SubType = derefStr(activity.SubType)
		m.StartedAt = activity.StartedAt
		m.DistanceM = derefFloat(activity.DistanceM)
		m.ElevationGainM = derefFloat(activity.ElevationGainM)
	}

	// User-editable fields from Activity model.
	if activity != nil {
		// Title only if manually set (auto-titles are regenerated on recompute).
		if activity.TitleIsManual {
			m.Title = derefStr(activity.Title)
		}
		m.Description = derefStr(activity.Description)
		m.Tags = activity.Tags
		m.GearID = derefStr(activity.GearID)
		m.Visibility = activity.Visibility
		m.WorkoutFeel = activity.WorkoutFeel
		m.WorkoutRpe = activity.WorkoutRpe
		m.IsEbike = activity.IsEbike
		m.IsVirtual = activity.IsVirtual
		m.Source = derefStr(activity.Source)
		m.UserID = activity.UserID
		m.ExcludeFromHeatmap = activity.ExcludeFromHeatmap
		m.ExcludeFromStats = activity.ExcludeFromStats
		m.IsIndoor = activity.IsIndoor
		m.ExcludeLiftSubtraction = activity.ExcludeLiftSubtraction
		m.LocationLat = activity.LocationLat
		m.LocationLng = activity.LocationLng
		m.TitleIsManual = activity.TitleIsManual
	}

	return m
}

func (m *MetaFile) Marshal() ([]byte, error) {
	return json.MarshalIndent(m, "", "  ")
}

func derefStr(p *string) string {
	if p != nil {
		return *p
	}
	return ""
}

func derefFloat(p *float64) float64 {
	if p != nil {
		return *p
	}
	return 0
}
