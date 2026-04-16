package sidecar

import (
	"encoding/json"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// FusionMemberSnapshot captures per-field-key fusion lineage at merge-time.
// Used by the sidecar writer to persist a self-contained record; consumed
// by GDPR export/import to reconstruct memberships across restores.
type FusionMemberSnapshot struct {
	SourceID    string `json:"source_id"`
	SourceTitle string `json:"source_title,omitempty"`
	FieldKey    string `json:"field_key"`
}

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
	IsElectric    bool `json:"is_electric,omitempty"`
	IsElectricOld bool `json:"is_ebike,omitempty"` // legacy compat for old exports
	IsVirtual     bool `json:"is_virtual,omitempty"`
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

	// v2 fields — present only for fusion activities (omitempty keeps regular
	// metadata files free of fusion noise).
	Fusion        bool                   `json:"fusion,omitempty"`
	FusionRequest json.RawMessage        `json:"fusion_request,omitempty"` // copy of activity_edits.edit_data
	FusionMembers []FusionMemberSnapshot `json:"fusion_members,omitempty"`
}

// GenerateMeta creates a MetaFile from ActivityData (FIT-parsed import data).
// User-editable fields are left empty — use GenerateMetaFull to include them.
func GenerateMeta(activityID string, data *model.ActivityData) *MetaFile {
	return GenerateMetaFull(activityID, data, nil)
}

// GenerateMetaFull creates a MetaFile combining FIT data and user-editable fields.
//   - If data != nil: FIT fields come from data (import path).
//   - If data == nil && activity != nil: FIT fields come from the Activity model.
//   - User-editable fields always come from activity (when not nil).
func GenerateMetaFull(activityID string, data *model.ActivityData, activity *model.Activity) *MetaFile {
	m := &MetaFile{
		SchemaVersion: 2,
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
		m.IsElectric = activity.IsElectric
		m.IsVirtual = activity.IsVirtual
		m.IsManualEntry = activity.IsManualEntry
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

// GenerateMetaFusion creates a MetaFile for a fusion or manual-entry activity.
// No file_id since these activities have no source-file.
// memberships may be nil/empty for manual-entry; the Fusion flag is derived.
// editData is the raw JSONB from activity_edits.edit_data ({"fusion_merge",...});
// may be nil for manual-entry.
func GenerateMetaFusion(activityID string, activity *model.Activity, memberships []FusionMemberSnapshot, editData json.RawMessage) *MetaFile {
	m := GenerateMetaFull(activityID, nil, activity)
	m.Fusion = len(memberships) > 0
	m.FusionRequest = editData
	m.FusionMembers = memberships
	return m
}

// ReadMeta unmarshals MetaFile bytes with feature-detection on the fusion
// fields. If a legacy file has fusion_members populated but fusion=false
// (malformed), the flag is corrected on read. SchemaVersion is used only
// informationally — all branches are driven by field presence, not version.
func ReadMeta(data []byte) (*MetaFile, error) {
	var m MetaFile
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if len(m.FusionMembers) > 0 && !m.Fusion {
		m.Fusion = true
	}
	return &m, nil
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
