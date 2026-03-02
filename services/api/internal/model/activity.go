package model

import (
	"encoding/json"
	"time"
)

// ActivityData is the parsed result of any activity file (FIT, GPX, TCX).
// All parsers produce this common struct.
type ActivityData struct {
	FileHash      string
	FileFormat    string // "fit", "gpx", "tcx"
	OriginalName  string
	Source        string // e.g. "Garmin", "Strava", "Komoot", "OSMTracker"
	SourceDevice  string // e.g. "Garmin Instinct 2X Solar", "Wahoo ELEMNT"
	SportType     string // lowercase from typedef.Sport.String()
	SubType       string // lowercase from typedef.SubSport.String()
	FitSportID    int16
	FitSubSportID int16
	StartedAt     time.Time
	EndedAt       time.Time
	ElapsedTimeS  int
	MovingTimeS   int

	DistanceM      float64
	ElevationGainM float64
	ElevationLossM float64
	MaxElevationM  float64
	MinElevationM  float64

	AvgSpeedMS       float64
	MaxSpeedMS       float64
	AvgHeartRate     float64
	MaxHeartRate     int
	AvgPowerW        float64
	MaxPowerW        int
	NormalizedPowerW float64
	AvgCadence       float64
	AvgTemperatureC  float64

	TSS   float64
	TRIMP float64

	IsIndoor bool
	HasGPS   bool
	StartLat float64
	StartLng float64
	EndLat   float64
	EndLng   float64

	Streams     []StreamPoint
	Sensors     []SensorRecord
	TrackPoints []TrackPoint
}

// StreamPoint is a single time-series data point.
// Pointer fields indicate optional/missing values.
type StreamPoint struct {
	Time         time.Time
	Lat          *float64
	Lng          *float64
	AltitudeM    *float64
	SpeedMS      *float64
	DistanceM    *float64
	HeartRate    *int16
	PowerW       *int16
	Cadence      *int16
	TemperatureC *int16
}

// TrackPoint is a GPS coordinate for PostGIS LineString construction.
type TrackPoint struct {
	Lat float64
	Lng float64
}

// SensorRecord holds extracted sensor info from FIT device_info messages.
type SensorRecord struct {
	DeviceIndex       int16
	SerialNumber      *int64
	SourceType        string // "antplus" or "bluetooth"
	AntplusDeviceType string
	AllDeviceTypes    []string
	Manufacturer      string
	ManufacturerID    int16
	GarminProduct     string
	GarminProductID   int32
	ProductName       string
	SoftwareVersion   float64
	HardwareVersion   int16
	BatteryStatus     string
	BatteryVoltage    float64
}

// Activity is the DB/API representation of an activity.
type Activity struct {
	ID               string     `json:"id"`
	UserID           string     `json:"user_id"`
	FileHash         *string    `json:"file_hash,omitempty"`
	Title            *string    `json:"title"`
	TitleAuto        *string    `json:"title_auto,omitempty"`
	Description      *string    `json:"description,omitempty"`
	SportType        string     `json:"sport_type"`
	SubType          *string    `json:"sub_type,omitempty"`
	FitSportID       *int16     `json:"fit_sport_id,omitempty"`
	FitSubSportID    *int16     `json:"fit_sub_sport_id,omitempty"`
	IsEbike          bool       `json:"is_ebike"`
	StartedAt        time.Time  `json:"started_at"`
	EndedAt          *time.Time `json:"ended_at,omitempty"`
	ElapsedTimeS     *int       `json:"elapsed_time_s,omitempty"`
	MovingTimeS      *int       `json:"moving_time_s,omitempty"`
	DistanceM        *float64   `json:"distance_m,omitempty"`
	ElevationGainM   *float64   `json:"elevation_gain_m,omitempty"`
	ElevationLossM   *float64   `json:"elevation_loss_m,omitempty"`
	MaxElevationM    *float64   `json:"max_elevation_m,omitempty"`
	MinElevationM    *float64   `json:"min_elevation_m,omitempty"`
	AvgSpeedMS       *float64   `json:"avg_speed_ms,omitempty"`
	MaxSpeedMS       *float64   `json:"max_speed_ms,omitempty"`
	AvgHeartRate     *float64   `json:"avg_heart_rate,omitempty"`
	MaxHeartRate     *int       `json:"max_heart_rate,omitempty"`
	AvgPowerW        *float64   `json:"avg_power_w,omitempty"`
	MaxPowerW        *int       `json:"max_power_w,omitempty"`
	NormalizedPowerW *float64   `json:"normalized_power_w,omitempty"`
	AvgCadence       *float64   `json:"avg_cadence,omitempty"`
	AvgTemperatureC  *float64   `json:"avg_temperature_c,omitempty"`
	TSS              *float64   `json:"tss,omitempty"`
	TRIMP            *float64   `json:"trimp,omitempty"`
	GearID           *string    `json:"gear_id,omitempty"`
	IsIndoor         bool       `json:"is_indoor"`
	Tags             []string   `json:"tags"`
	Visibility       string     `json:"visibility"`
	Source           *string    `json:"source,omitempty"`
	SourceDevice     *string    `json:"source_device,omitempty"`
	EnrichmentStatus string     `json:"enrichment_status"`
	NeedsClassification bool    `json:"needs_classification"`
	HasGPS             bool       `json:"has_gps"`
	StartCity          *string    `json:"start_city,omitempty"`
	EndCity            *string    `json:"end_city,omitempty"`
	StartState         *string    `json:"start_state,omitempty"`
	StartCountry       *string    `json:"start_country,omitempty"`
	StartCountryCode   *string    `json:"start_country_code,omitempty"`
	WeatherTempC       *float64   `json:"weather_temp_c,omitempty"`
	WeatherCondition   *string    `json:"weather_condition,omitempty"`
	WeatherHumidity    *int       `json:"weather_humidity,omitempty"`
	WeatherWindSpeedMS *float64   `json:"weather_wind_speed_ms,omitempty"`
	WeatherTags        []string   `json:"weather_tags,omitempty"`
	IsRoundTrip        bool       `json:"is_round_trip"`
	Intensity          *string    `json:"intensity,omitempty"`
	TimeOfDay          *string    `json:"time_of_day,omitempty"`
	OriginalFilename   *string    `json:"original_filename,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// ActivityUpdate is the PATCH request body for updating an activity.
type ActivityUpdate struct {
	Title          *string                `json:"title,omitempty"`
	Description    *string                `json:"description,omitempty"`
	SportType      *string                `json:"sport_type,omitempty"`
	SubType        *string                `json:"sub_type,omitempty"`
	Tags           []string               `json:"tags,omitempty"`
	Visibility     *string                `json:"visibility,omitempty"`
	HideMap        *bool                  `json:"hide_map,omitempty"`
	DataVisibility map[string]interface{} `json:"data_visibility,omitempty"`
}

// ActivityList is the paginated list response.
type ActivityList struct {
	Activities []Activity `json:"activities"`
	NextCursor string     `json:"next_cursor,omitempty"`
	Total      int        `json:"total"`
}

// StreamResponse is the API response for the streams endpoint.
type StreamResponse struct {
	Time         time.Time `json:"time"`
	Lat          *float64  `json:"lat,omitempty"`
	Lng          *float64  `json:"lng,omitempty"`
	AltitudeM    *float64  `json:"altitude_m,omitempty"`
	SpeedMS      *float64  `json:"speed_ms,omitempty"`
	DistanceM    *float64  `json:"distance_m,omitempty"`
	HeartRate    *int16    `json:"heart_rate,omitempty"`
	PowerW       *int16    `json:"power_w,omitempty"`
	Cadence      *int16    `json:"cadence,omitempty"`
	TemperatureC *int16    `json:"temperature_c,omitempty"`
}

// SensorResponse is the API response for the sensors endpoint.
type SensorResponse struct {
	ID               string   `json:"id"`
	DeviceIndex      int16    `json:"device_index"`
	SourceType       string   `json:"source_type"`
	PrimaryType      string   `json:"primary_type"`
	PrimaryTypeLabel string   `json:"primary_type_label"`
	AllTypes         []string `json:"all_types"`
	Manufacturer     string   `json:"manufacturer,omitempty"`
	ProductName      string   `json:"product_name,omitempty"`
	SerialNumber     *int64   `json:"serial_number,omitempty"`
	SoftwareVersion  string   `json:"software_version,omitempty"`
	BatteryStatus    string   `json:"battery_status,omitempty"`
}

// UploadResponse wraps the activity with optional warnings.
type UploadResponse struct {
	Activity *Activity `json:"activity"`
	Warnings []string  `json:"warnings,omitempty"`
}

// PMCDay represents a single day in the Performance Management Chart.
type PMCDay struct {
	UserID string  `json:"-"`
	Date   string  `json:"date"` // YYYY-MM-DD
	TSS    float64 `json:"tss"`
	CTL    float64 `json:"ctl"`
	ATL    float64 `json:"atl"`
	TSB    float64 `json:"tsb"`
}

// ProblemDetail is an RFC 7807 error response.
type ProblemDetail struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail"`
	Instance string `json:"instance,omitempty"`
}

// ActivityWithPath is a lightweight struct used for shadow FS rename operations.
type ActivityWithPath struct {
	ID                 string    `json:"id"`
	StartedAt          time.Time `json:"started_at"`
	SportType          string    `json:"sport_type"`
	Title              *string   `json:"title,omitempty"`
	DistanceM          *float64  `json:"distance_m,omitempty"`
	ElapsedTimeS       *int      `json:"elapsed_time_s,omitempty"`
	OriginalFileFormat *string   `json:"original_file_format,omitempty"`
	LocalFilePath      *string   `json:"local_file_path,omitempty"`
}

// HotfolderImport represents a record in the hotfolder_imports table.
type HotfolderImport struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	Filename   string    `json:"filename"`
	Status     string    `json:"status"`
	ErrorMsg   *string   `json:"error_msg,omitempty"`
	ActivityID *string   `json:"activity_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// PrivacyZone represents a user-defined privacy zone (e.g. home, work).
type PrivacyZone struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	Lat       float64   `json:"lat"`
	Lng       float64   `json:"lng"`
	RadiusM   int       `json:"radius_m"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

// PrivacyZoneUpdate is the request body for updating a privacy zone.
type PrivacyZoneUpdate struct {
	Name     *string  `json:"name,omitempty"`
	Lat      *float64 `json:"lat,omitempty"`
	Lng      *float64 `json:"lng,omitempty"`
	RadiusM  *int     `json:"radius_m,omitempty"`
	IsActive *bool    `json:"is_active,omitempty"`
}

// GeoTag represents a geographic feature associated with an activity.
type GeoTag struct {
	ID          string    `json:"id"`
	ActivityID  string    `json:"activity_id"`
	Category    string    `json:"category"`
	Name        string    `json:"name"`
	OsmID       *int64    `json:"osm_id,omitempty"`
	Lat         *float64  `json:"lat,omitempty"`
	Lng         *float64  `json:"lng,omitempty"`
	Ele         *float64  `json:"ele,omitempty"`
	DistM       *float64  `json:"dist_m,omitempty"`
	DurationMin *int      `json:"duration_min,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// EnrichmentSettings holds user preferences for activity enrichment.
type EnrichmentSettings struct {
	UserID                  string  `json:"user_id"`
	EnableSummitDetection   bool    `json:"enable_summit_detection"`
	EnableWeatherEnrichment bool    `json:"enable_weather_enrichment"`
	EnableStopDetection     bool    `json:"enable_stop_detection"`
	EnableClimbDetection    bool    `json:"enable_climb_detection"`
	EnableCommuteDetection  bool    `json:"enable_commute_detection"`
	EnableAutoNaming        bool    `json:"enable_auto_naming"`
	ClimbMinGradientPct     float64 `json:"climb_min_gradient_pct"`
	ClimbMinElevationM      float64 `json:"climb_min_elevation_m"`
	RoundTripThresholdM     float64 `json:"round_trip_threshold_m"`
}

// Gear represents a piece of equipment.
type Gear struct {
	ID               string          `json:"id"`
	UserID           string          `json:"user_id"`
	Name             string          `json:"name"`
	Type             string          `json:"type"`
	SubType          *string         `json:"sub_type,omitempty"`
	Brand            *string         `json:"brand,omitempty"`
	Model            *string         `json:"model,omitempty"`
	TotalDistanceM   float64         `json:"total_distance_m"`
	TotalTimeS       int64           `json:"total_time_s"`
	TotalActivities  int             `json:"total_activities"`
	PurchasedAt      *string         `json:"purchased_at,omitempty"`
	RetiredAt        *string         `json:"retired_at,omitempty"`
	IsRetired        bool            `json:"is_retired"`
	DefaultForSports []string        `json:"default_for_sports"`
	Notes            *string         `json:"notes,omitempty"`
	Components       []GearComponent `json:"components,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
}

// GearCreate is the request body for creating a gear item.
type GearCreate struct {
	Name             string   `json:"name"`
	Type             string   `json:"type"`
	SubType          *string  `json:"sub_type,omitempty"`
	Brand            *string  `json:"brand,omitempty"`
	Model            *string  `json:"model,omitempty"`
	PurchasedAt      *string  `json:"purchased_at,omitempty"`
	DefaultForSports []string `json:"default_for_sports,omitempty"`
	Notes            *string  `json:"notes,omitempty"`
}

// GearUpdate is the request body for updating a gear item.
type GearUpdate struct {
	Name             *string  `json:"name,omitempty"`
	Type             *string  `json:"type,omitempty"`
	SubType          *string  `json:"sub_type,omitempty"`
	Brand            *string  `json:"brand,omitempty"`
	Model            *string  `json:"model,omitempty"`
	PurchasedAt      *string  `json:"purchased_at,omitempty"`
	RetiredAt        *string  `json:"retired_at,omitempty"`
	IsRetired        *bool    `json:"is_retired,omitempty"`
	DefaultForSports []string `json:"default_for_sports,omitempty"`
	Notes            *string  `json:"notes,omitempty"`
}

// GearComponent represents a component attached to a gear item.
type GearComponent struct {
	ID                string  `json:"id"`
	GearID            string  `json:"gear_id"`
	ParentComponentID *string `json:"parent_component_id,omitempty"`
	Name              string  `json:"name"`
	Type              string  `json:"type"`
	Brand             *string `json:"brand,omitempty"`
	Model             *string `json:"model,omitempty"`
	TotalDistanceM    float64 `json:"total_distance_m"`
	InstalledAt       *string `json:"installed_at,omitempty"`
	RemovedAt         *string `json:"removed_at,omitempty"`
	IsRetired         bool    `json:"is_retired"`
	Notes             *string `json:"notes,omitempty"`
}

// GearComponentCreate is the request body for creating a gear component.
type GearComponentCreate struct {
	Name              string  `json:"name"`
	Type              string  `json:"type"`
	ParentComponentID *string `json:"parent_component_id,omitempty"`
	Brand             *string `json:"brand,omitempty"`
	Model             *string `json:"model,omitempty"`
	Notes             *string `json:"notes,omitempty"`
}

// GearServiceInterval represents a maintenance schedule for a component.
type GearServiceInterval struct {
	ID                   string   `json:"id"`
	ComponentID          string   `json:"component_id"`
	Name                 string   `json:"name"`
	IntervalType         string   `json:"interval_type"` // distance_m|time_s|elapsed_days
	IntervalValue        float64  `json:"interval_value"`
	IsRecurring          bool     `json:"is_recurring"`
	LastServiceAt        *string  `json:"last_service_at,omitempty"`
	LastServiceDistanceM *float64 `json:"last_service_distance_m,omitempty"`
	NextDueAt            *string  `json:"next_due_at,omitempty"`
}

// MaintenanceAlert represents a maintenance alert for a component.
type MaintenanceAlert struct {
	GearID         string  `json:"gear_id"`
	GearName       string  `json:"gear_name"`
	ComponentID    string  `json:"component_id"`
	ComponentName  string  `json:"component_name"`
	IntervalName   string  `json:"interval_name"`
	IntervalType   string  `json:"interval_type"`
	IsOverdue      bool    `json:"is_overdue"`
	CurrentValue   float64 `json:"current_value"`
	ThresholdValue float64 `json:"threshold_value"`
}

// AutomationRule represents a user-defined automation rule.
type AutomationRule struct {
	ID           string          `json:"id"`
	UserID       string          `json:"user_id"`
	Name         string          `json:"name"`
	IsActive     bool            `json:"is_active"`
	Priority     int             `json:"priority"`
	Conditions   json.RawMessage `json:"conditions"`
	Actions      json.RawMessage `json:"actions"`
	TimesApplied int             `json:"times_applied"`
	CreatedAt    time.Time       `json:"created_at"`
}

// AutomationRuleCreate is the request body for creating an automation rule.
type AutomationRuleCreate struct {
	Name       string          `json:"name"`
	Priority   int             `json:"priority"`
	Conditions json.RawMessage `json:"conditions"`
	Actions    json.RawMessage `json:"actions"`
}

// AutomationRuleUpdate is the request body for updating an automation rule.
type AutomationRuleUpdate struct {
	Name       *string          `json:"name,omitempty"`
	IsActive   *bool            `json:"is_active,omitempty"`
	Priority   *int             `json:"priority,omitempty"`
	Conditions *json.RawMessage `json:"conditions,omitempty"`
	Actions    *json.RawMessage `json:"actions,omitempty"`
}
