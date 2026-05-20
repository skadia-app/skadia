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
	FromWaypoint  string       `json:"from_waypoint"`
	ToWaypoint    string       `json:"to_waypoint"`
	Mode          string       `json:"mode"` // routed, offroad
	ControlPoints [][2]float64 `json:"control_points,omitempty"`
	Smooth        bool         `json:"smooth"`
	DistanceM     float64      `json:"distance_m"`
	// TotalDistanceM is the approach-inclusive distance (DistanceM plus the
	// click-to-projection straight-line approach legs). Absent for legacy
	// segments and BRouter/offroad-only segments. T0 (Migration 000334).
	TotalDistanceM     *float64           `json:"total_distance_m,omitempty"`
	ElevationGain      float64            `json:"elevation_gain"`
	ElevationLoss      float64            `json:"elevation_loss"`
	ElevationEstimated *bool              `json:"elevation_estimated,omitempty"`
	Geometry           *GeoJSONLineString `json:"geometry,omitempty"`
	// ApproachFrom / ApproachTo are the off-network polylines linking a
	// click-point to the nearest road projection. Frontend-produced and
	// frontend-consumed; backend stores them opaquely so they survive a
	// save/reload roundtrip and the offroad-approach pieces of a saved route
	// are not silently discarded.
	ApproachFrom [][]float64 `json:"approach_from,omitempty"`
	ApproachTo   [][]float64 `json:"approach_to,omitempty"`
	// SubSegments carry per-edge highway/surface/scale metadata (BadRouter).
	// Opaque pass-through — backend persists but does not interpret.
	SubSegments json.RawMessage `json:"sub_segments,omitempty"`
	// ElevationProfile is the per-segment T0 profile (RouteElevationPoint[]).
	// Opaque pass-through.
	ElevationProfile json.RawMessage `json:"elevation_profile,omitempty"`
}

type PlannedRoute struct {
	ID                 string          `json:"id"`
	UserID             string          `json:"user_id"`
	Name               string          `json:"name"`
	Description        *string         `json:"description,omitempty"`
	SportType          string          `json:"sport_type"`
	SubType            *string         `json:"sub_type,omitempty"`
	IsElectric         bool            `json:"is_electric"`
	IsIndoor           bool            `json:"is_indoor"`
	IsVirtual          bool            `json:"is_virtual"`
	IsCommute          bool            `json:"is_commute"`
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
	// Version is the optimistic-concurrency token (Migration 000345). Source of
	// the HTTP strong ETag emitted on GET; PATCH callers send the last observed
	// value via If-Match. Bumped by every successful UPDATE. Default 1 in DB.
	Version int64 `json:"version"`
}

type ObjectAccessScope struct {
	ID          string              `json:"id"`
	OwnerKind   string              `json:"owner_kind"`
	OwnerUserID *string             `json:"owner_user_id,omitempty"`
	OwnerRef    *string             `json:"owner_ref,omitempty"`
	ScopeKind   string              `json:"scope_kind"`
	Label       *string             `json:"label,omitempty"`
	CreatedBy   *string             `json:"created_by,omitempty"`
	CreatedAt   time.Time           `json:"created_at"`
	RevokedAt   *time.Time          `json:"revoked_at,omitempty"`
	Metadata    json.RawMessage     `json:"metadata,omitempty"`
	Grants      []ObjectAccessGrant `json:"grants,omitempty"`
}

type ObjectAccessGrant struct {
	ID                    string          `json:"id"`
	AccessScopeID         string          `json:"access_scope_id"`
	PrincipalKind         string          `json:"principal_kind"`
	PrincipalUserID       *string         `json:"principal_user_id,omitempty"`
	PrincipalRef          *string         `json:"principal_ref,omitempty"`
	PrincipalSelector     json.RawMessage `json:"principal_selector,omitempty"`
	PrincipalSelectorHash string          `json:"principal_selector_hash,omitempty"`
	LinkTokenHash         *string         `json:"-"`
	Permission            string          `json:"permission"`
	CreatedAt             time.Time       `json:"created_at"`
	RevokedAt             *time.Time      `json:"revoked_at,omitempty"`
}

type ObjectAccessScopeInput struct {
	OwnerUserID string
	ScopeKind   string
	Label       string
	CreatedBy   string
	Grants      []ObjectAccessGrantInput
}

type ObjectAccessGrantInput struct {
	PrincipalKind     string
	PrincipalUserID   *string
	PrincipalRef      *string
	PrincipalSelector map[string]any
	LinkTokenHash     *string
	Permission        string
}

type ObjectAccessPrincipalRef struct {
	PrincipalKind         string
	PrincipalRef          string
	PrincipalSelectorHash string
}

type PlannedRouteSnapshot struct {
	ID                    string  `json:"id"`
	RouteID               *string `json:"route_id,omitempty"`
	UserID                *string `json:"user_id,omitempty"`
	SourceRouteVersion    int64   `json:"source_route_version"`
	SnapshotSchemaKey     string  `json:"snapshot_schema_key"`
	SnapshotSchemaVersion int     `json:"snapshot_schema_version"`
	RouteName             string  `json:"route_name"`
	// RouteDescription is route metadata copied into the immutable route-state
	// snapshot. Long-form guide content belongs to optional publication-domain
	// rows anchored by RouteCoursePublication.ID, not to this hot-path snapshot.
	RouteDescription   *string         `json:"route_description,omitempty"`
	SportType          string          `json:"sport_type"`
	SubType            *string         `json:"sub_type,omitempty"`
	IsElectric         bool            `json:"is_electric"`
	IsIndoor           bool            `json:"is_indoor"`
	IsVirtual          bool            `json:"is_virtual"`
	IsCommute          bool            `json:"is_commute"`
	Profile            string          `json:"profile"`
	RoutingEngine      *string         `json:"routing_engine,omitempty"`
	RouteSchemaVersion int16           `json:"route_schema_version"`
	GeometryGeoJSON    string          `json:"geometry_geojson,omitempty"`
	Waypoints          json.RawMessage `json:"waypoints"`
	Segments           json.RawMessage `json:"segments"`
	ElevationProfile   json.RawMessage `json:"elevation_profile,omitempty"`
	Enrichment         json.RawMessage `json:"enrichment,omitempty"`
	TotalDistanceM     *float64        `json:"total_distance_m,omitempty"`
	TotalElevationGain *float64        `json:"total_elevation_gain,omitempty"`
	TotalElevationLoss *float64        `json:"total_elevation_loss,omitempty"`
	EstimatedTimeMin   *int            `json:"estimated_time_min,omitempty"`
	GeometryHash       string          `json:"geometry_hash"`
	SnapshotHash       string          `json:"snapshot_hash"`
	CreatedAt          time.Time       `json:"created_at"`
}

type CreatePlannedRouteSnapshotInput struct {
	RouteID            string
	UserID             string
	SourceRouteVersion int64
	OperationID        string
	Reason             string
}

type RouteCoursePublication struct {
	// ID is also the future public guide-page anchor. Optional guide content
	// and tour reports stay in separate domain tables outside course hot paths.
	ID      string  `json:"id"`
	RouteID *string `json:"route_id,omitempty"`
	// RouteSnapshotID is the immutable route-state anchor used by matching,
	// profile projection, export, and deletion. It is not an attachment bucket.
	RouteSnapshotID string `json:"route_snapshot_id"`
	// CourseSegmentID/CourseSegmentVersionID anchor the rankable course object.
	CourseSegmentID              string          `json:"course_segment_id"`
	CourseSegmentVersionID       string          `json:"course_segment_version_id"`
	PublicationState             string          `json:"publication_state"`
	PublishPolicy                string          `json:"publish_policy"`
	EquivalenceDecision          string          `json:"equivalence_decision"`
	VisibilitySnapshot           string          `json:"visibility_snapshot"`
	AccessScopeIDSnapshot        *string         `json:"access_scope_id_snapshot,omitempty"`
	TargetPublicationID          *string         `json:"target_publication_id,omitempty"`
	TargetCourseSegmentID        *string         `json:"target_course_segment_id,omitempty"`
	TargetCourseSegmentVersionID *string         `json:"target_course_segment_version_id,omitempty"`
	CompatibilityEvidence        json.RawMessage `json:"compatibility_evidence,omitempty"`
	ActorUserID                  *string         `json:"actor_user_id,omitempty"`
	OperationID                  string          `json:"operation_id"`
	OperationInputHash           string          `json:"operation_input_hash"`
	ObservedRouteVersion         int64           `json:"observed_route_version"`
	CreatedAt                    time.Time       `json:"created_at"`
	RetiredAt                    *time.Time      `json:"retired_at,omitempty"`
	Metadata                     json.RawMessage `json:"metadata,omitempty"`
}

type PublishRouteSnapshotInput struct {
	RouteSnapshotID              string
	ActorUserID                  string
	SourceRouteVersion           int64
	PublishPolicy                string
	EquivalenceDecision          string
	Visibility                   string
	AccessScopeID                *string
	CompatibilityPreset          string
	TargetPublicationID          *string
	TargetCourseSegmentID        *string
	TargetCourseSegmentVersionID *string
	CompatibilityEvidence        map[string]any
	OperationID                  string
}

type RouteRelationship struct {
	ID                 string          `json:"id"`
	SourceRouteID      string          `json:"source_route_id"`
	TargetRouteID      string          `json:"target_route_id"`
	RelationType       string          `json:"relation_type"`
	ActorUserID        *string         `json:"actor_user_id,omitempty"`
	SourceSnapshotID   *string         `json:"source_snapshot_id,omitempty"`
	TargetSnapshotID   *string         `json:"target_snapshot_id,omitempty"`
	OperationID        string          `json:"operation_id"`
	OperationInputHash string          `json:"operation_input_hash"`
	CreatedAt          time.Time       `json:"created_at"`
	Metadata           json.RawMessage `json:"metadata,omitempty"`
}

type RouteCourseRelationship struct {
	ID                           string          `json:"id"`
	SourcePublicationID          *string         `json:"source_publication_id,omitempty"`
	TargetPublicationID          *string         `json:"target_publication_id,omitempty"`
	SourceCourseSegmentID        string          `json:"source_course_segment_id"`
	SourceCourseSegmentVersionID *string         `json:"source_course_segment_version_id,omitempty"`
	TargetCourseSegmentID        string          `json:"target_course_segment_id"`
	TargetCourseSegmentVersionID *string         `json:"target_course_segment_version_id,omitempty"`
	RelationType                 string          `json:"relation_type"`
	ActorUserID                  *string         `json:"actor_user_id,omitempty"`
	OperationID                  string          `json:"operation_id"`
	OperationInputHash           string          `json:"operation_input_hash"`
	CreatedAt                    time.Time       `json:"created_at"`
	Metadata                     json.RawMessage `json:"metadata,omitempty"`
}

type RouteProfileSnapshot struct {
	ID                   string          `json:"id"`
	RouteSnapshotID      string          `json:"route_snapshot_id"`
	ProfileSchemaKey     string          `json:"profile_schema_key"`
	ProfileSchemaVersion int             `json:"profile_schema_version"`
	SourceKind           string          `json:"source_kind"`
	SourceVersion        string          `json:"source_version"`
	SourceHash           string          `json:"source_hash"`
	Confidence           string          `json:"confidence"`
	ComputedAt           time.Time       `json:"computed_at"`
	DistanceM            *float64        `json:"distance_m,omitempty"`
	ElevationGainM       *float64        `json:"elevation_gain_m,omitempty"`
	ElevationLossM       *float64        `json:"elevation_loss_m,omitempty"`
	ForestPct            *float64        `json:"forest_pct,omitempty"`
	ProtectedPct         *float64        `json:"protected_pct,omitempty"`
	SurfaceDistribution  json.RawMessage `json:"surface_distribution,omitempty"`
	WayDistribution      json.RawMessage `json:"way_distribution,omitempty"`
	DifficultyScales     json.RawMessage `json:"difficulty_scales,omitempty"`
	ContextCounts        json.RawMessage `json:"context_counts,omitempty"`
	Ranges               json.RawMessage `json:"ranges,omitempty"`
}

type RouteCourseProfileProjection struct {
	SubjectKind                string    `json:"subject_kind"`
	SubjectID                  string    `json:"subject_id"`
	SubjectVersionID           *string   `json:"subject_version_id,omitempty"`
	SubjectVersionKey          string    `json:"subject_version_key"`
	PublicationID              *string   `json:"publication_id,omitempty"`
	RouteSnapshotID            *string   `json:"route_snapshot_id,omitempty"`
	ProfileSnapshotID          string    `json:"profile_snapshot_id"`
	Visibility                 string    `json:"visibility"`
	AccessScopeID              *string   `json:"access_scope_id,omitempty"`
	SportType                  string    `json:"sport_type"`
	SubType                    *string   `json:"sub_type,omitempty"`
	IsElectric                 bool      `json:"is_electric"`
	IsIndoor                   bool      `json:"is_indoor"`
	IsVirtual                  bool      `json:"is_virtual"`
	IsCommute                  bool      `json:"is_commute"`
	DistanceM                  *float64  `json:"distance_m,omitempty"`
	ElevationGainM             *float64  `json:"elevation_gain_m,omitempty"`
	ElevationLossM             *float64  `json:"elevation_loss_m,omitempty"`
	SACMaxRank                 *int      `json:"sac_max_rank,omitempty"`
	SACDominantRank            *int      `json:"sac_dominant_rank,omitempty"`
	SACWeightedMeanRank        *float64  `json:"sac_weighted_mean_rank,omitempty"`
	MTBMaxRank                 *int      `json:"mtb_max_rank,omitempty"`
	MTBDominantRank            *int      `json:"mtb_dominant_rank,omitempty"`
	MTBWeightedMeanRank        *float64  `json:"mtb_weighted_mean_rank,omitempty"`
	ViaFerrataMaxRank          *int      `json:"via_ferrata_max_rank,omitempty"`
	ViaFerrataDominantRank     *int      `json:"via_ferrata_dominant_rank,omitempty"`
	ViaFerrataWeightedMeanRank *float64  `json:"via_ferrata_weighted_mean_rank,omitempty"`
	ForestPct                  *float64  `json:"forest_pct,omitempty"`
	ProtectedPct               *float64  `json:"protected_pct,omitempty"`
	PrimarySurface             *string   `json:"primary_surface,omitempty"`
	PrimaryWayType             *string   `json:"primary_way_type,omitempty"`
	SourceHash                 string    `json:"source_hash"`
	GeneratedAt                time.Time `json:"generated_at"`
}

type PublishedRouteCourse struct {
	PublicationID              string    `json:"publication_id"`
	RouteID                    *string   `json:"route_id,omitempty"`
	RouteSnapshotID            string    `json:"route_snapshot_id"`
	CourseSegmentID            string    `json:"course_segment_id"`
	CourseSegmentVersionID     string    `json:"course_segment_version_id"`
	Name                       string    `json:"name"`
	Visibility                 string    `json:"visibility"`
	AccessScopeID              *string   `json:"access_scope_id,omitempty"`
	SportType                  string    `json:"sport_type"`
	SubType                    *string   `json:"sub_type,omitempty"`
	IsElectric                 bool      `json:"is_electric"`
	IsIndoor                   bool      `json:"is_indoor"`
	IsVirtual                  bool      `json:"is_virtual"`
	IsCommute                  bool      `json:"is_commute"`
	DistanceM                  *float64  `json:"distance_m,omitempty"`
	ElevationGainM             *float64  `json:"elevation_gain_m,omitempty"`
	ElevationLossM             *float64  `json:"elevation_loss_m,omitempty"`
	SACMaxRank                 *int      `json:"sac_max_rank,omitempty"`
	MTBMaxRank                 *int      `json:"mtb_max_rank,omitempty"`
	ViaFerrataMaxRank          *int      `json:"via_ferrata_max_rank,omitempty"`
	PrimarySurface             *string   `json:"primary_surface,omitempty"`
	PrimaryWayType             *string   `json:"primary_way_type,omitempty"`
	HasVariants                bool      `json:"has_variants"`
	PublishedAt                time.Time `json:"published_at"`
	CurrentSegmentVersionID    *string   `json:"current_segment_version_id,omitempty"`
	CurrentSegmentVersionValid bool      `json:"current_segment_version_valid"`
}

type RoutePublicationEvent struct {
	ID                 string          `json:"id"`
	ActorUserID        *string         `json:"actor_user_id,omitempty"`
	Operation          string          `json:"operation"`
	OperationID        string          `json:"operation_id"`
	OperationInputHash string          `json:"operation_input_hash"`
	SubjectKind        string          `json:"subject_kind"`
	SubjectID          string          `json:"subject_id"`
	BeforeSnapshotHash *string         `json:"before_snapshot_hash,omitempty"`
	AfterSnapshotHash  string          `json:"after_snapshot_hash"`
	CreatedAt          time.Time       `json:"created_at"`
	Metadata           json.RawMessage `json:"metadata,omitempty"`
}

// RouteImportData is the input struct for importing routes from external sources.
type RouteImportData struct {
	Name               string
	Description        *string
	SportType          string
	SubType            *string
	IsElectric         bool
	IsIndoor           bool
	IsVirtual          bool
	IsCommute          bool
	Profile            string
	GeometryGeoJSON    string
	WaypointsJSON      string // "[]" if none
	SegmentsJSON       string // "[]" if none
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
	FileID       string    `json:"-"`
	FileHash     string    `json:"file_hash"`
	OriginalName *string   `json:"original_name,omitempty"`
	Width        *int      `json:"width,omitempty"`
	Height       *int      `json:"height,omitempty"`
	SortOrder    int       `json:"sort_order"`
	CreatedAt    time.Time `json:"created_at"`
	ThumbURL     string    `json:"thumb_url,omitempty"`
	PreviewURL   string    `json:"preview_url,omitempty"`
	FullURL      string    `json:"full_url,omitempty"`
}
