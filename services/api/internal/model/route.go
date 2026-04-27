package model

import (
	"encoding/json"
	"time"
)

type GeoJSONLineString struct {
	Type        string      `json:"type"`
	Coordinates [][]float64 `json:"coordinates"`
}

type RouteWaypoint struct {
	ID      string     `json:"id"`
	LatLng  [2]float64 `json:"latlng"`
	Type    string     `json:"type"` // start, via, end
	POIID   *int64     `json:"poi_id,omitempty"`
	POIName *string    `json:"poi_name,omitempty"`
	POIType *string    `json:"poi_type,omitempty"`
}

type RouteSegment struct {
	FromWaypoint  string             `json:"from_waypoint"`
	ToWaypoint    string             `json:"to_waypoint"`
	Mode          string             `json:"mode"` // routed, offroad
	ControlPoints [][2]float64       `json:"control_points,omitempty"`
	Smooth        bool               `json:"smooth"`
	DistanceM     float64            `json:"distance_m"`
	ElevationGain float64            `json:"elevation_gain"`
	ElevationLoss float64            `json:"elevation_loss"`
	Geometry      *GeoJSONLineString `json:"geometry,omitempty"`
}

type PlannedRoute struct {
	ID                 string          `json:"id"`
	UserID             string          `json:"user_id"`
	Name               string          `json:"name"`
	Description        *string         `json:"description,omitempty"`
	SportType          string          `json:"sport_type"`
	Difficulty         *string         `json:"difficulty,omitempty"`
	Profile            string          `json:"profile"`
	GeometryGeoJSON    string          `json:"geometry_geojson,omitempty"` // ST_AsGeoJSON
	Waypoints          []RouteWaypoint `json:"waypoints"`
	Segments           []RouteSegment  `json:"segments"`
	TotalDistanceM     *float64        `json:"total_distance_m,omitempty"`
	TotalElevationGain *float64        `json:"total_elevation_gain,omitempty"`
	TotalElevationLoss *float64        `json:"total_elevation_loss,omitempty"`
	EstimatedTimeMin   *int            `json:"estimated_time_min,omitempty"`
	TimeOverride       bool            `json:"time_override"`
	AutoTag            bool            `json:"auto_tag"`
	Photos             []RoutePhoto    `json:"photos,omitempty"`
	Tags               []Tag           `json:"tags,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
	// Import source tracking
	SourcePlatform *string `json:"source_platform,omitempty"` // "komoot", "garmin", "strava", nil=manual
	SourceID       *string `json:"source_id,omitempty"`       // Platform-specific ID
	SourceURL      *string `json:"source_url,omitempty"`      // Link back to source
	GeometryHash   *string `json:"geometry_hash,omitempty"`   // SHA256 of snapped geometry
	// Opaque JSONB columns — frontend produces and consumes; backend passes through.
	// ElevationProfile: persisted from planned_routes.elevation_profile (Migration 000325).
	// Enrichment: route enrichment metadata (Migration 000325).
	// Both are nil when the column is NULL.
	ElevationProfile json.RawMessage `json:"elevation_profile,omitempty"`
	Enrichment       json.RawMessage `json:"enrichment,omitempty"`
	// RoutingEngine: engine that produced the saved geometry ('badrouter'|'brouter').
	// NULL for legacy routes — frontend falls back to current state.routingEngine on load.
	// Migration 000332.
	RoutingEngine *string `json:"routing_engine,omitempty"` // 'badrouter' | 'brouter' | nil
	// RouteSchemaVersion distinguishes Pre-T0 (0) from T0 (1) elevation-profile layouts.
	// 0 = approach-exclusive profile (legacy). 1 = approach-inclusive, full profile incl. VE-Tail/Approach/Spline.
	// Default 0 (DB DEFAULT). Set to 1 by all user-triggered save paths. Migration 000334.
	RouteSchemaVersion int16 `json:"route_schema_version"`
}

// RouteImportData is the input struct for importing routes from external sources.
type RouteImportData struct {
	Name               string
	Description        *string
	SportType          string
	Profile            string
	GeometryGeoJSON    string
	WaypointsJSON      string          // "[]" if none
	SegmentsJSON       string          // "[]" if none
	TotalDistanceM     *float64
	TotalElevationGain *float64
	TotalElevationLoss *float64
	SourcePlatform     *string
	SourceID           *string
	SourceURL          *string
	// ElevationProfile is the merged route-level profile ([]routing.ElevationPoint
	// serialised as JSON). Nil means no profile — route_schema_version will be 0.
	ElevationProfile json.RawMessage
}

type RoutePhoto struct {
	ID           string    `json:"id"`
	RouteID      string    `json:"route_id"`
	UserID       string    `json:"user_id"`
	FileHash     string    `json:"file_hash"`
	OriginalName *string   `json:"original_name,omitempty"`
	Width        *int      `json:"width,omitempty"`
	Height       *int      `json:"height,omitempty"`
	SortOrder    int       `json:"sort_order"`
	CreatedAt    time.Time `json:"created_at"`
}
