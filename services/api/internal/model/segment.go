package model

import (
	"encoding/json"
	"time"
)

type Segment struct {
	ID                string          `json:"id" db:"id"`
	CreatedBy         string          `json:"created_by" db:"created_by"`
	Name              string          `json:"name" db:"name"`
	Description       *string         `json:"description,omitempty" db:"description"`
	SportGroup        string          `json:"sport_group" db:"sport_group"`
	AllowedSportTypes []string        `json:"allowed_sport_types,omitempty" db:"allowed_sport_types"`
	AllowedSubTypes   []string        `json:"allowed_sub_types,omitempty" db:"allowed_sub_types"`
	SegmentType       string          `json:"segment_type" db:"segment_type"`
	SegmentTypeMeta   json.RawMessage `json:"segment_type_meta" db:"segment_type_meta"`
	SourceActivityID  *string         `json:"source_activity_id,omitempty" db:"source_activity_id"`
	OsmLiftID         *int64          `json:"osm_lift_id,omitempty" db:"osm_lift_id"`
	DistanceM         *float64        `json:"distance_m,omitempty" db:"distance_m"`
	ElevationGainM    *float64        `json:"elevation_gain_m,omitempty" db:"elevation_gain_m"`
	EffortCount       int             `json:"effort_count" db:"effort_count"`
	AthleteCount      int             `json:"athlete_count" db:"athlete_count"`
	Visibility        string          `json:"visibility" db:"visibility"`
	RouteHash         *string         `json:"route_hash,omitempty" db:"route_hash"`
	CreatedAt         time.Time       `json:"created_at" db:"created_at"`
	// Joined field: latest version track as GeoJSON
	TrackGeoJSON *string `json:"track_geojson,omitempty" db:"track_geojson"`
}

type SegmentCreate struct {
	Name             string          `json:"name"`
	SportGroup       string          `json:"sport_group"`
	SegmentType      string          `json:"segment_type"`
	SegmentTypeMeta  json.RawMessage `json:"segment_type_meta,omitempty"`
	GeoJSONTrack     string          `json:"geojson_track"`
	SourceActivityID *string         `json:"source_activity_id,omitempty"`
	Description      *string         `json:"description,omitempty"`
}

type SegmentUpdate struct {
	Name            *string          `json:"name,omitempty"`
	Description     *string          `json:"description,omitempty"`
	SegmentTypeMeta *json.RawMessage `json:"segment_type_meta,omitempty"`
}

type SegmentVersion struct {
	ID            string    `json:"id" db:"id"`
	SegmentID     string    `json:"segment_id" db:"segment_id"`
	VersionNumber int       `json:"version_number" db:"version_number"`
	TrackGeoJSON  string    `json:"track_geojson" db:"track_geojson"`
	ValidFrom     string    `json:"valid_from" db:"valid_from"`
	ChangeNote    *string   `json:"change_note,omitempty" db:"change_note"`
	CreatedBy     *string   `json:"created_by,omitempty" db:"created_by"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

type SegmentEffort struct {
	ID               string    `json:"id"`
	SegmentID        string    `json:"segment_id"`
	SegmentVersionID string    `json:"segment_version_id"`
	ActivityID       string    `json:"activity_id"`
	UserID           string    `json:"user_id"`
	StartedAt        time.Time `json:"started_at"`
	ElapsedTimeS     int       `json:"elapsed_time_s"`
	IsReverse        bool      `json:"is_reverse"`
	SportType        string    `json:"sport_type"`
	SubType          string    `json:"sub_type"`
	GearID           *string   `json:"gear_id,omitempty"`
	AvgPowerW        *float64  `json:"avg_power_w,omitempty"`
	AvgHeartRate     *float64  `json:"avg_heart_rate,omitempty"`
	AvgSpeedMs       *float64  `json:"avg_speed_ms,omitempty"`
	RankOverall      *int      `json:"rank_overall,omitempty"`
	IsPersonalBest   bool      `json:"is_personal_best"`
	IsSegmentRecord  bool      `json:"is_segment_record"`
	InLeaderboard    bool      `json:"in_leaderboard"`
	CreatedAt        time.Time `json:"created_at"`
	// Joined fields
	SegmentName *string `json:"segment_name,omitempty"`
	DisplayName *string `json:"display_name,omitempty"`
	GearName    *string `json:"gear_name,omitempty"`
}

// DetectedLap represents an automatically detected lap/round within an activity.
type DetectedLap struct {
	ID            string    `json:"id"`
	ActivityID    string    `json:"activity_id"`
	LapNumber     int       `json:"lap_number"`
	StartTime     time.Time `json:"start_time"`
	EndTime       time.Time `json:"end_time"`
	ElapsedTimeS  int       `json:"elapsed_time_s"`
	DistanceM     *float64  `json:"distance_m,omitempty"`
	AvgSpeedMs    *float64  `json:"avg_speed_ms,omitempty"`
	AvgHeartRate  *float64  `json:"avg_heart_rate,omitempty"`
	AvgPowerW     *float64  `json:"avg_power_w,omitempty"`
	AvgCadence    *float64  `json:"avg_cadence,omitempty"`
	TotalAscentM  *float64  `json:"total_ascent_m,omitempty"`
	TotalDescentM *float64  `json:"total_descent_m,omitempty"`
	StartLat      *float64  `json:"start_lat,omitempty"`
	StartLng      *float64  `json:"start_lng,omitempty"`
}
