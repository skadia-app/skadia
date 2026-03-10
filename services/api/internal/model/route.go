package model

import "time"

type RouteWaypoint struct {
	ID      string     `json:"id"`
	LatLng  [2]float64 `json:"latlng"`
	Type    string     `json:"type"` // start, via, end
	POIID   *int64     `json:"poi_id,omitempty"`
	POIName *string    `json:"poi_name,omitempty"`
	POIType *string    `json:"poi_type,omitempty"`
}

type RouteSegment struct {
	FromWaypoint  string       `json:"from_waypoint"`
	ToWaypoint    string       `json:"to_waypoint"`
	Mode          string       `json:"mode"` // routed, offroad
	ControlPoints [][2]float64 `json:"control_points,omitempty"`
	Smooth        bool         `json:"smooth"`
	DistanceM     float64      `json:"distance_m"`
	ElevationGain float64      `json:"elevation_gain"`
	ElevationLoss float64      `json:"elevation_loss"`
}

type PlannedRoute struct {
	ID                 string         `json:"id"`
	UserID             string         `json:"user_id"`
	Name               string         `json:"name"`
	Description        *string        `json:"description,omitempty"`
	SportType          string         `json:"sport_type"`
	Difficulty         *string        `json:"difficulty,omitempty"`
	Profile            string         `json:"profile"`
	GeometryGeoJSON    string         `json:"geometry_geojson,omitempty"` // ST_AsGeoJSON
	Waypoints          []RouteWaypoint `json:"waypoints"`
	Segments           []RouteSegment  `json:"segments"`
	TotalDistanceM     *float64       `json:"total_distance_m,omitempty"`
	TotalElevationGain *float64       `json:"total_elevation_gain,omitempty"`
	TotalElevationLoss *float64       `json:"total_elevation_loss,omitempty"`
	EstimatedTimeMin   *int           `json:"estimated_time_min,omitempty"`
	TimeOverride       bool           `json:"time_override"`
	AutoTag            bool           `json:"auto_tag"`
	Photos             []RoutePhoto   `json:"photos,omitempty"`
	Tags               []Tag          `json:"tags,omitempty"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

type RoutePhoto struct {
	ID           string    `json:"id"`
	RouteID      string    `json:"route_id"`
	UserID       string    `json:"user_id"`
	FileHash     string    `json:"file_hash"`
	S3Key        string    `json:"s3_key"`
	S3KeyThumb   string    `json:"s3_key_thumb"`
	S3KeyPreview *string   `json:"s3_key_preview,omitempty"`
	OriginalName *string   `json:"original_name,omitempty"`
	Width        *int      `json:"width,omitempty"`
	Height       *int      `json:"height,omitempty"`
	SortOrder    int       `json:"sort_order"`
	CreatedAt    time.Time `json:"created_at"`
}
