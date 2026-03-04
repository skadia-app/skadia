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

	MinHeartRate            int
	TotalCalories           int
	AerobicTrainingEffect   float64
	AnaerobicTrainingEffect float64
	TrainingLoadPeak        float64
	AvgSpo2Pct              int
	AvgRespirationRate      float64
	AvgStressLevel          int
	WorkoutRpe              int
	WorkoutFeel             int
	SessionRmssdMs          int
	AvgCoreTemperatureC     float64
	ThresholdPowerW         int
	TimeInZoneData          []TimeInZoneData
	UserProfileImport       *UserProfileImport

	TSS   float64
	TRIMP float64

	Tags []string // existing tags on the activity (for enrichment merge)

	IsIndoor bool
	HasGPS   bool
	StartLat float64
	StartLng float64
	EndLat   float64
	EndLng   float64

	Streams     []StreamPoint
	RRIntervals []uint16 // R-R intervals in ms (from FIT HRV messages)
	Sensors     []SensorRecord
	TrackPoints []TrackPoint

	Laps           []LapRecord
	Splits         []SplitRecord
	SplitSummaries []SplitSummaryRecord
	Lengths        []LengthRecord
	ClimbProEvents []ClimbProEventRecord
}

// StreamPoint is a single time-series data point.
// Pointer fields indicate optional/missing values.
type StreamPoint struct {
	Time            time.Time
	Lat             *float64
	Lng             *float64
	AltitudeM       *float64
	SpeedMS         *float64
	DistanceM       *float64
	HeartRate       *int16
	PowerW          *int16
	Cadence         *int16
	TemperatureC    *int16
	RespirationRate *int16
	StressLevel     *int16
	// Running dynamics (from FIT record messages)
	GroundContactTimeMs   *int16   // stance_time, Scale: 10 → ms
	VerticalOscillationMm *int16   // vertical_oscillation, Scale: 10 → mm
	StanceTimePercent     *float32 // stance_time_percent, Scale: 100 → %
	LeftRightBalance      *int16   // left_right_balance masked value (0–100)
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
	AntDeviceNumber   *int32 // ANT+ device number (uint16 in FIT, nil if absent/invalid)
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
	ID                      string     `json:"id"`
	UserID                  string     `json:"user_id"`
	FileHash                *string    `json:"file_hash,omitempty"`
	Title                   *string    `json:"title"`
	TitleAuto               *string    `json:"title_auto,omitempty"`
	Description             *string    `json:"description,omitempty"`
	SportType               string     `json:"sport_type"`
	SubType                 *string    `json:"sub_type,omitempty"`
	FitSportID              *int16     `json:"fit_sport_id,omitempty"`
	FitSubSportID           *int16     `json:"fit_sub_sport_id,omitempty"`
	IsEbike                 bool       `json:"is_ebike"`
	StartedAt               time.Time  `json:"started_at"`
	EndedAt                 *time.Time `json:"ended_at,omitempty"`
	ElapsedTimeS            *int       `json:"elapsed_time_s,omitempty"`
	MovingTimeS             *int       `json:"moving_time_s,omitempty"`
	DistanceM               *float64   `json:"distance_m,omitempty"`
	ElevationGainM          *float64   `json:"elevation_gain_m,omitempty"`
	ElevationLossM          *float64   `json:"elevation_loss_m,omitempty"`
	MaxElevationM           *float64   `json:"max_elevation_m,omitempty"`
	MinElevationM           *float64   `json:"min_elevation_m,omitempty"`
	AvgSpeedMS              *float64   `json:"avg_speed_ms,omitempty"`
	MaxSpeedMS              *float64   `json:"max_speed_ms,omitempty"`
	AvgHeartRate            *float64   `json:"avg_heart_rate,omitempty"`
	MaxHeartRate            *int       `json:"max_heart_rate,omitempty"`
	AvgPowerW               *float64   `json:"avg_power_w,omitempty"`
	MaxPowerW               *int       `json:"max_power_w,omitempty"`
	NormalizedPowerW        *float64   `json:"normalized_power_w,omitempty"`
	AvgCadence              *float64   `json:"avg_cadence,omitempty"`
	AvgTemperatureC         *float64   `json:"avg_temperature_c,omitempty"`
	TSS                     *float64   `json:"tss,omitempty"`
	TRIMP                   *float64   `json:"trimp,omitempty"`
	MinHeartRate            *int       `json:"min_heart_rate,omitempty"`
	TotalCalories           *int       `json:"total_calories,omitempty"`
	AerobicTrainingEffect   *float64   `json:"aerobic_training_effect,omitempty"`
	AnaerobicTrainingEffect *float64   `json:"anaerobic_training_effect,omitempty"`
	TrainingLoadPeak        *float64   `json:"training_load_peak,omitempty"`
	AvgSpo2Pct              *int       `json:"avg_spo2_pct,omitempty"`
	AvgRespirationRate      *float64   `json:"avg_respiration_rate,omitempty"`
	AvgStressLevel          *int       `json:"avg_stress_level,omitempty"`
	WorkoutRpe              *int       `json:"workout_rpe,omitempty"`
	WorkoutFeel             *int       `json:"workout_feel,omitempty"`
	ThresholdPowerW         *int       `json:"threshold_power_w,omitempty"`
	VariabilityIndex        *float64   `json:"variability_index,omitempty"`
	EfficiencyFactor        *float64   `json:"efficiency_factor,omitempty"`
	AerobicDecoupling       *float64   `json:"aerobic_decoupling,omitempty"`
	GearID                  *string    `json:"gear_id,omitempty"`
	IsIndoor                bool       `json:"is_indoor"`
	Tags                    []string   `json:"tags"`
	Visibility              string     `json:"visibility"`
	Source                  *string    `json:"source,omitempty"`
	SourceDevice            *string    `json:"source_device,omitempty"`
	EnrichmentStatus        string     `json:"enrichment_status"`
	NeedsClassification     bool       `json:"needs_classification"`
	HasGPS                  bool       `json:"has_gps"`
	StartCity               *string    `json:"start_city,omitempty"`
	EndCity                 *string    `json:"end_city,omitempty"`
	StartState              *string    `json:"start_state,omitempty"`
	StartCountry            *string    `json:"start_country,omitempty"`
	StartCountryCode        *string    `json:"start_country_code,omitempty"`
	WeatherTempC            *float64   `json:"weather_temp_c,omitempty"`
	WeatherCondition        *string    `json:"weather_condition,omitempty"`
	WeatherHumidity         *int       `json:"weather_humidity,omitempty"`
	WeatherWindSpeedMS      *float64   `json:"weather_wind_speed_ms,omitempty"`
	WeatherTags             []string   `json:"weather_tags,omitempty"`
	IsRoundTrip             bool       `json:"is_round_trip"`
	Intensity               *string    `json:"intensity,omitempty"`
	TimeOfDay               []string   `json:"time_of_day,omitempty"`
	SunriseAt               *time.Time `json:"sunrise_at,omitempty"`
	SunsetAt                *time.Time `json:"sunset_at,omitempty"`
	OriginalFilename        *string    `json:"original_filename,omitempty"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
	// Manually assigned location from a location tag (independent of GPS start_lat/start_lng).
	LocationLat *float64 `json:"location_lat,omitempty"`
	LocationLng *float64 `json:"location_lng,omitempty"`
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
	GearID         json.RawMessage        `json:"gear_id,omitempty"`
	// Manually assigned location (independent of GPS start_lat/start_lng).
	LocationLat *float64 `json:"location_lat,omitempty"`
	LocationLng *float64 `json:"location_lng,omitempty"`
}

// ActivityList is the paginated list response.
type ActivityList struct {
	Activities []Activity `json:"activities"`
	NextCursor string     `json:"next_cursor,omitempty"`
	Total      int        `json:"total"`
}

// ActivityFilters holds filter parameters for the ListActivities query.
type ActivityFilters struct {
	SportTypes []string
	SubTypes   []string
	DateFrom   string
	DateTo     string
	IsIndoor   *bool
	HasGPS     *bool
	GearID     string
	Search     string
}

// StreamResponse is the API response for the streams endpoint.
type StreamResponse struct {
	Time            time.Time `json:"time"`
	Lat             *float64  `json:"lat,omitempty"`
	Lng             *float64  `json:"lng,omitempty"`
	AltitudeM       *float64  `json:"altitude_m,omitempty"`
	SpeedMS         *float64  `json:"speed_ms,omitempty"`
	DistanceM       *float64  `json:"distance_m,omitempty"`
	HeartRate       *int16    `json:"heart_rate,omitempty"`
	PowerW          *int16    `json:"power_w,omitempty"`
	Cadence         *int16    `json:"cadence,omitempty"`
	TemperatureC    *int16    `json:"temperature_c,omitempty"`
	RespirationRate *int16    `json:"respiration_rate,omitempty"`
	StressLevel     *int16    `json:"stress_level,omitempty"`
	// Running dynamics
	GroundContactTimeMs   *int16   `json:"ground_contact_time_ms,omitempty"`
	VerticalOscillationMm *int16   `json:"vertical_oscillation_mm,omitempty"`
	StanceTimePercent     *float32 `json:"stance_time_percent,omitempty"`
	LeftRightBalance      *int16   `json:"left_right_balance,omitempty"`
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
	AntDeviceNumber  *int32   `json:"ant_device_number,omitempty"`
	SoftwareVersion  string   `json:"software_version,omitempty"`
	BatteryStatus    string   `json:"battery_status,omitempty"`
}

// LapRecord holds extracted lap data from FIT lap messages.
type LapRecord struct {
	MessageIndex      int
	StartTime         time.Time
	EndTime           time.Time
	LapTrigger        string
	Sport             string
	SubSport          string
	TotalElapsedTimeS float64
	TotalTimerTimeS   float64
	TotalMovingTimeS  float64
	TotalDistanceM    float64
	TotalAscentM      int
	TotalDescentM     int
	TotalCalories     int
	AvgSpeedMS        float64
	MaxSpeedMS        float64
	AvgHeartRate      int
	MaxHeartRate      int
	MinHeartRate      int
	AvgCadence        int
	MaxCadence        int
	AvgPowerW         int
	MaxPowerW         int
	NormalizedPowerW  int
	AvgTemperatureC   int
	StartLat          float64
	StartLng          float64
	EndLat            float64
	EndLng            float64
	TotalWorkJ        int
	Intensity         string
	SwimStroke        string
	NumLengths        int
	NumActiveLengths  int
	AvgAltitudeM      float64
	MaxAltitudeM      float64
	MinAltitudeM      float64
}

// LapResponse is the API response for a single lap.
type LapResponse struct {
	ID                string     `json:"id"`
	MessageIndex      int        `json:"lap_index"`
	StartTime         *time.Time `json:"start_time,omitempty"`
	EndTime           *time.Time `json:"end_time,omitempty"`
	LapTrigger        string     `json:"lap_trigger,omitempty"`
	Sport             string     `json:"sport,omitempty"`
	SubSport          string     `json:"sub_sport,omitempty"`
	TotalElapsedTimeS *float64   `json:"elapsed_time_s,omitempty"`
	TotalTimerTimeS   *float64   `json:"timer_time_s,omitempty"`
	TotalMovingTimeS  *float64   `json:"moving_time_s,omitempty"`
	TotalDistanceM    *float64   `json:"distance_m,omitempty"`
	TotalAscentM      *int       `json:"total_ascent_m,omitempty"`
	TotalDescentM     *int       `json:"total_descent_m,omitempty"`
	TotalCalories     *int       `json:"total_calories,omitempty"`
	AvgSpeedMS        *float64   `json:"avg_speed_ms,omitempty"`
	MaxSpeedMS        *float64   `json:"max_speed_ms,omitempty"`
	AvgHeartRate      *int       `json:"avg_heart_rate,omitempty"`
	MaxHeartRate      *int       `json:"max_heart_rate,omitempty"`
	MinHeartRate      *int       `json:"min_heart_rate,omitempty"`
	AvgCadence        *int       `json:"avg_cadence,omitempty"`
	MaxCadence        *int       `json:"max_cadence,omitempty"`
	AvgPowerW         *int       `json:"avg_power_w,omitempty"`
	MaxPowerW         *int       `json:"max_power_w,omitempty"`
	NormalizedPowerW  *int       `json:"normalized_power_w,omitempty"`
	AvgTemperatureC   *int       `json:"avg_temperature_c,omitempty"`
	StartLat          *float64   `json:"start_lat,omitempty"`
	StartLng          *float64   `json:"start_lng,omitempty"`
	EndLat            *float64   `json:"end_lat,omitempty"`
	EndLng            *float64   `json:"end_lng,omitempty"`
	TotalWorkJ        *int       `json:"total_work_j,omitempty"`
	Intensity         string     `json:"intensity,omitempty"`
	SwimStroke        string     `json:"swim_stroke,omitempty"`
	NumLengths        *int       `json:"num_lengths,omitempty"`
	NumActiveLengths  *int       `json:"num_active_lengths,omitempty"`
	AvgAltitudeM      *float64   `json:"avg_altitude_m,omitempty"`
	MaxAltitudeM      *float64   `json:"max_altitude_m,omitempty"`
	MinAltitudeM      *float64   `json:"min_altitude_m,omitempty"`
}

// SplitRecord holds extracted split data from FIT split messages.
type SplitRecord struct {
	MessageIndex      int
	SplitType         string
	StartTime         time.Time
	EndTime           time.Time
	TotalElapsedTimeS float64
	TotalTimerTimeS   float64
	TotalMovingTimeS  float64
	TotalDistanceM    float64
	TotalAscentM      int
	TotalDescentM     int
	TotalCalories     int
	AvgSpeedMS        float64
	MaxSpeedMS        float64
	AvgVertSpeedMS    float64
	StartLat          float64
	StartLng          float64
	EndLat            float64
	EndLng            float64
	StartElevationM   float64
}

// SplitResponse is the API response for a single split.
type SplitResponse struct {
	ID                string     `json:"id"`
	MessageIndex      int        `json:"split_index"`
	SplitType         string     `json:"split_type,omitempty"`
	StartTime         *time.Time `json:"start_time,omitempty"`
	EndTime           *time.Time `json:"end_time,omitempty"`
	TotalElapsedTimeS *float64   `json:"elapsed_time_s,omitempty"`
	TotalTimerTimeS   *float64   `json:"timer_time_s,omitempty"`
	TotalMovingTimeS  *float64   `json:"moving_time_s,omitempty"`
	TotalDistanceM    *float64   `json:"distance_m,omitempty"`
	TotalAscentM      *int       `json:"total_ascent_m,omitempty"`
	TotalDescentM     *int       `json:"total_descent_m,omitempty"`
	TotalCalories     *int       `json:"total_calories,omitempty"`
	AvgSpeedMS        *float64   `json:"avg_speed_ms,omitempty"`
	MaxSpeedMS        *float64   `json:"max_speed_ms,omitempty"`
	AvgVertSpeedMS    *float64   `json:"avg_vert_speed_ms,omitempty"`
	StartLat          *float64   `json:"start_lat,omitempty"`
	StartLng          *float64   `json:"start_lng,omitempty"`
	EndLat            *float64   `json:"end_lat,omitempty"`
	EndLng            *float64   `json:"end_lng,omitempty"`
	StartElevationM   *float64   `json:"start_elevation_m,omitempty"`
}

// SplitSummaryRecord holds extracted split summary data from FIT split_summary messages.
type SplitSummaryRecord struct {
	MessageIndex     int
	SplitType        string
	NumSplits        int
	TotalTimerTimeS  float64
	TotalMovingTimeS float64
	TotalDistanceM   float64
	TotalAscentM     int
	TotalDescentM    int
	TotalCalories    int
	AvgSpeedMS       float64
	MaxSpeedMS       float64
	AvgVertSpeedMS   float64
	AvgHeartRate     int
	MaxHeartRate     int
}

// SplitSummaryResponse is the API response for a single split summary.
type SplitSummaryResponse struct {
	ID               string   `json:"id"`
	MessageIndex     int      `json:"summary_index"`
	SplitType        string   `json:"split_type,omitempty"`
	NumSplits        *int     `json:"num_splits,omitempty"`
	TotalTimerTimeS  *float64 `json:"timer_time_s,omitempty"`
	TotalMovingTimeS *float64 `json:"moving_time_s,omitempty"`
	TotalDistanceM   *float64 `json:"distance_m,omitempty"`
	TotalAscentM     *int     `json:"total_ascent_m,omitempty"`
	TotalDescentM    *int     `json:"total_descent_m,omitempty"`
	TotalCalories    *int     `json:"total_calories,omitempty"`
	AvgSpeedMS       *float64 `json:"avg_speed_ms,omitempty"`
	MaxSpeedMS       *float64 `json:"max_speed_ms,omitempty"`
	AvgVertSpeedMS   *float64 `json:"avg_vert_speed_ms,omitempty"`
	AvgHeartRate     *int     `json:"avg_hr,omitempty"`
	MaxHeartRate     *int     `json:"max_hr,omitempty"`
}

// LengthRecord holds extracted swim length data from FIT length messages.
type LengthRecord struct {
	MessageIndex       int
	StartTime          time.Time
	EndTime            time.Time
	TotalElapsedTimeS  float64
	TotalTimerTimeS    float64
	TotalStrokes       int
	TotalCalories      int
	AvgSpeedMS         float64
	AvgSwimmingCadence int
	SwimStroke         string
	LengthType         string
}

// LengthResponse is the API response for a single swim length.
type LengthResponse struct {
	ID                 string     `json:"id"`
	MessageIndex       int        `json:"length_index"`
	StartTime          *time.Time `json:"start_time,omitempty"`
	EndTime            *time.Time `json:"end_time,omitempty"`
	TotalElapsedTimeS  *float64   `json:"elapsed_time_s,omitempty"`
	TotalTimerTimeS    *float64   `json:"timer_time_s,omitempty"`
	TotalStrokes       *int       `json:"total_strokes,omitempty"`
	TotalCalories      *int       `json:"total_calories,omitempty"`
	AvgSpeedMS         *float64   `json:"avg_speed_ms,omitempty"`
	AvgSwimmingCadence *int       `json:"avg_swimming_cadence,omitempty"`
	SwimStroke         string     `json:"swim_stroke,omitempty"`
	LengthType         string     `json:"length_type,omitempty"`
}

// ClimbProEventRecord holds extracted ClimbPro event data from FIT climb_pro messages.
type ClimbProEventRecord struct {
	Timestamp     time.Time
	EventType     string
	ClimbNumber   int
	ClimbCategory int
	CurrentDistM  float64
	Lat           float64
	Lng           float64
}

// ClimbProEventResponse is the API response for a single ClimbPro event.
type ClimbProEventResponse struct {
	ID            string     `json:"id"`
	Timestamp     *time.Time `json:"timestamp,omitempty"`
	EventType     string     `json:"event_type,omitempty"`
	ClimbNumber   *int       `json:"climb_number,omitempty"`
	ClimbCategory *int       `json:"climb_category,omitempty"`
	CurrentDistM  *float64   `json:"current_dist_m,omitempty"`
	Lat           *float64   `json:"lat,omitempty"`
	Lng           *float64   `json:"lng,omitempty"`
}

// Climb represents a detected climb/ascent within an activity.
type Climb struct {
	ID             string  `json:"id"`
	ClimbNumber    int     `json:"climb_number"`
	StartDistanceM float64 `json:"start_distance_m"`
	EndDistanceM   float64 `json:"end_distance_m"`
	LengthM        float64 `json:"length_m"`
	ElevationGainM float64 `json:"elevation_gain_m"`
	AvgGradientPct float64 `json:"avg_gradient_pct"`
	MaxGradientPct float64 `json:"max_gradient_pct"`
	Category       string  `json:"category"`
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
	ACWR   float64 `json:"acwr"`
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
	SubType            *string   `json:"sub_type,omitempty"`
	Title              *string   `json:"title,omitempty"`
	City               *string   `json:"city,omitempty"`
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

// Tag represents a user-defined tag for categorizing activities.
type Tag struct {
	ID              string          `json:"id"`
	UserID          string          `json:"user_id"`
	Name            string          `json:"name"`
	Color           string          `json:"color"`
	TextColor       string          `json:"text_color"`
	Category        string          `json:"category"`
	IsSystem        bool            `json:"is_system"`
	IsAuto          bool            `json:"is_auto"`
	Conditions      json.RawMessage `json:"conditions,omitempty"`
	ConditionGroups json.RawMessage `json:"condition_groups,omitempty"`
	ConditionsLogic string          `json:"conditions_logic,omitempty"`
	Description     *string         `json:"description,omitempty"`
	ActivityCount   int             `json:"activity_count"`
	CreatedAt       time.Time       `json:"created_at"`
	// Optional geo-coordinates — when set, the tag acts as a "location tag"
	// that appears as a pin on the map.
	Lat *float64 `json:"lat,omitempty"`
	Lng *float64 `json:"lng,omitempty"`
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
	EnableLapDetection      bool    `json:"enable_lap_detection"`
	ClimbMinGradientPct     float64 `json:"climb_min_gradient_pct"`
	ClimbMinElevationM      float64 `json:"climb_min_elevation_m"`
	RoundTripThresholdM     float64 `json:"round_trip_threshold_m"`
}

// Gear represents a piece of equipment.
type Gear struct {
	ID                  string          `json:"id"`
	UserID              string          `json:"user_id"`
	Name                string          `json:"name"`
	Type                string          `json:"type"`
	SubType             *string         `json:"sub_type,omitempty"`
	Brand               *string         `json:"brand,omitempty"`
	Model               *string         `json:"model,omitempty"`
	TotalDistanceM      float64         `json:"total_distance_m"`
	TotalTimeS          int64           `json:"total_time_s"`
	TotalElevationGainM float64         `json:"total_elevation_gain_m"`
	TotalActivities     int             `json:"total_activities"`
	PurchasedAt         *string         `json:"purchased_at,omitempty"`
	RetiredAt           *string         `json:"retired_at,omitempty"`
	IsRetired           bool            `json:"is_retired"`
	Status              string          `json:"status"`
	DefaultForSports    []string        `json:"default_for_sports"`
	Notes               *string         `json:"notes,omitempty"`
	ReviewStatus        string          `json:"review_status,omitempty"`
	WeightG             *int            `json:"weight_g,omitempty"`
	SourceDocumentID    *string         `json:"source_document_id,omitempty"`
	Components          []GearComponent `json:"components,omitempty"`
	CreatedAt           time.Time       `json:"created_at"`
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
	ID                  string   `json:"id"`
	GearID              string   `json:"gear_id"`
	ParentComponentID   *string  `json:"parent_component_id,omitempty"`
	Name                string   `json:"name"`
	Type                string   `json:"type"`
	Brand               *string  `json:"brand,omitempty"`
	Model               *string  `json:"model,omitempty"`
	TotalDistanceM      float64  `json:"total_distance_m"`
	TotalElevationGainM float64  `json:"total_elevation_gain_m"`
	TotalTimeS          int64    `json:"total_time_s"`
	PurchasePrice       *float64 `json:"purchase_price,omitempty"`
	PurchaseDate        *string  `json:"purchase_date,omitempty"`
	WeightG             *int     `json:"weight_g,omitempty"`
	SerialNumber        *string  `json:"serial_number,omitempty"`
	InstalledAt         *string  `json:"installed_at,omitempty"`
	RemovedAt           *string  `json:"removed_at,omitempty"`
	IsRetired           bool     `json:"is_retired"`
	Status              string   `json:"status"`
	Notes               *string  `json:"notes,omitempty"`
}

// GearComponentCreate is the request body for creating a gear component.
type GearComponentCreate struct {
	Name              string   `json:"name"`
	Type              string   `json:"type"`
	ParentComponentID *string  `json:"parent_component_id,omitempty"`
	Brand             *string  `json:"brand,omitempty"`
	Model             *string  `json:"model,omitempty"`
	PurchasePrice     *float64 `json:"purchase_price,omitempty"`
	PurchaseDate      *string  `json:"purchase_date,omitempty"`
	WeightG           *int     `json:"weight_g,omitempty"`
	SerialNumber      *string  `json:"serial_number,omitempty"`
	Notes             *string  `json:"notes,omitempty"`
}

// ComponentAssignment represents a historical assignment of a component to a gear item.
type ComponentAssignment struct {
	ID             string  `json:"id"`
	ComponentID    string  `json:"component_id"`
	GearID         string  `json:"gear_id"`
	InstalledAt    string  `json:"installed_at"`
	RemovedAt      *string `json:"removed_at,omitempty"`
	DistanceM      float64 `json:"distance_m"`
	ElevationGainM float64 `json:"elevation_gain_m"`
	TimeS          int64   `json:"time_s"`
	Notes          *string `json:"notes,omitempty"`
}

// ChainRotationGroup represents a chain rotation group for a gear item.
type ChainRotationGroup struct {
	ID             string                `json:"id"`
	UserID         string                `json:"user_id"`
	GearID         string                `json:"gear_id"`
	Name           string                `json:"name"`
	SwapIntervalKM *float64              `json:"swap_interval_km,omitempty"`
	SwapIntervalHM *float64              `json:"swap_interval_hm,omitempty"`
	Members        []ChainRotationMember `json:"members"`
	CreatedAt      string                `json:"created_at"`
}

// ChainRotationMember represents a member within a chain rotation group.
type ChainRotationMember struct {
	ID                  string  `json:"id"`
	GroupID             string  `json:"group_id"`
	ComponentID         string  `json:"component_id"`
	ComponentName       string  `json:"component_name,omitempty"`
	Position            int     `json:"position"`
	IsActive            bool    `json:"is_active"`
	TotalDistanceM      float64 `json:"total_distance_m"`
	TotalElevationGainM float64 `json:"total_elevation_gain_m"`
	LastSwappedAt       *string `json:"last_swapped_at,omitempty"`
}

// GearServiceEvent represents a service event performed on a component.
type GearServiceEvent struct {
	ID          string   `json:"id"`
	ComponentID string   `json:"component_id"`
	IntervalID  *string  `json:"interval_id,omitempty"`
	PerformedAt string   `json:"performed_at"`
	DistanceAtM *float64 `json:"distance_at_m,omitempty"`
	Description *string  `json:"description,omitempty"`
	Cost        *float64 `json:"cost,omitempty"`
	Notes       *string  `json:"notes,omitempty"`
	CreatedAt   string   `json:"created_at"`
}

// ComponentTypePreset represents a predefined component type.
type ComponentTypePreset struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	Category             string   `json:"category"`
	GearType             *string  `json:"gear_type,omitempty"`
	DefaultIntervalType  *string  `json:"default_interval_type,omitempty"`
	DefaultIntervalValue *float64 `json:"default_interval_value,omitempty"`
	TrackElevation       bool     `json:"track_elevation"`
	SortOrder            int      `json:"sort_order"`
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
	ID              string          `json:"id"`
	UserID          string          `json:"user_id"`
	Name            string          `json:"name"`
	IsActive        bool            `json:"is_active"`
	Priority        int             `json:"priority"`
	Conditions      json.RawMessage `json:"conditions"`
	ConditionsLogic string          `json:"conditions_logic"`
	ConditionGroups json.RawMessage `json:"condition_groups"`
	Actions         json.RawMessage `json:"actions"`
	TimesApplied    int             `json:"times_applied"`
	CreatedAt       time.Time       `json:"created_at"`
}

// AutomationRuleCreate is the request body for creating an automation rule.
type AutomationRuleCreate struct {
	Name            string          `json:"name"`
	Priority        int             `json:"priority"`
	Conditions      json.RawMessage `json:"conditions"`
	ConditionsLogic string          `json:"conditions_logic"`
	ConditionGroups json.RawMessage `json:"condition_groups"`
	Actions         json.RawMessage `json:"actions"`
}

// AutomationRuleUpdate is the request body for updating an automation rule.
type AutomationRuleUpdate struct {
	Name            *string          `json:"name,omitempty"`
	IsActive        *bool            `json:"is_active,omitempty"`
	Priority        *int             `json:"priority,omitempty"`
	Conditions      *json.RawMessage `json:"conditions,omitempty"`
	ConditionsLogic *string          `json:"conditions_logic,omitempty"`
	ConditionGroups *json.RawMessage `json:"condition_groups,omitempty"`
	Actions         *json.RawMessage `json:"actions,omitempty"`
}

// BikeSetup represents a saved bike configuration/setup.
type BikeSetup struct {
	ID              string          `json:"id"`
	GearID          string          `json:"gear_id"`
	Name            string          `json:"name"`
	RiderWeightKg   *float64        `json:"rider_weight_kg,omitempty"`
	IntendedUse     *string         `json:"intended_use,omitempty"`
	Notes           *string         `json:"notes,omitempty"`
	SuspensionFront json.RawMessage `json:"suspension_front,omitempty"`
	SuspensionRear  json.RawMessage `json:"suspension_rear,omitempty"`
	Tires           json.RawMessage `json:"tires,omitempty"`
	ContactPoints   json.RawMessage `json:"contact_points,omitempty"`
	Brakes          json.RawMessage `json:"brakes,omitempty"`
	Drivetrain      json.RawMessage `json:"drivetrain,omitempty"`
	IsActive        bool            `json:"is_active"`
	CreatedAt       string          `json:"created_at"`
	UpdatedAt       string          `json:"updated_at"`
}

// BikeSetupCreate is the request body for creating a bike setup.
type BikeSetupCreate struct {
	Name            string          `json:"name"`
	RiderWeightKg   *float64        `json:"rider_weight_kg,omitempty"`
	IntendedUse     *string         `json:"intended_use,omitempty"`
	Notes           *string         `json:"notes,omitempty"`
	SuspensionFront json.RawMessage `json:"suspension_front,omitempty"`
	SuspensionRear  json.RawMessage `json:"suspension_rear,omitempty"`
	Tires           json.RawMessage `json:"tires,omitempty"`
	ContactPoints   json.RawMessage `json:"contact_points,omitempty"`
	Brakes          json.RawMessage `json:"brakes,omitempty"`
	Drivetrain      json.RawMessage `json:"drivetrain,omitempty"`
	IsActive        bool            `json:"is_active"`
}

// GearDocument represents a document/image attached to a gear item.
type GearDocument struct {
	ID               string    `json:"id"`
	UserID           string    `json:"user_id"`
	GearID           *string   `json:"gear_id"`
	ComponentID      *string   `json:"component_id"`
	ServiceEventID   *string   `json:"service_event_id"`
	DocType          string    `json:"doc_type"`
	Title            *string   `json:"title"`
	OriginalFilename string    `json:"original_filename"`
	ContentType      string    `json:"content_type"`
	FileSizeBytes    int64     `json:"file_size_bytes"`
	FileHash         *string   `json:"file_hash,omitempty"`
	S3Bucket         string    `json:"-"`
	S3Key            string    `json:"-"`
	ExtractedText    *string   `json:"extracted_text,omitempty"`
	ProcessingStatus string    `json:"processing_status"`
	CreatedAt        time.Time `json:"created_at"`
}

// GearReviewRequest is the request body for PATCH /gear/{id}/review.
type GearReviewRequest struct {
	ReviewStatus string `json:"review_status"` // confirmed | rejected
}

// ActivityStop represents a detected stop within an activity.
type ActivityStop struct {
	ID        string    `json:"id"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
	DurationS int       `json:"duration_s"`
	Lat       *float64  `json:"lat,omitempty"`
	Lng       *float64  `json:"lng,omitempty"`
	Name      *string   `json:"name,omitempty"`
}

// ActivitySurface represents a detected surface type within an activity.
type ActivitySurface struct {
	ID          string  `json:"id"`
	SurfaceType string  `json:"surface_type"`
	PathType    string  `json:"path_type"`
	LengthM     float64 `json:"length_m"`
	Percentage  float64 `json:"percentage"`
}

// BikeSetupUpdate is the request body for updating a bike setup.
type BikeSetupUpdate struct {
	Name            *string         `json:"name,omitempty"`
	RiderWeightKg   *float64        `json:"rider_weight_kg,omitempty"`
	IntendedUse     *string         `json:"intended_use,omitempty"`
	Notes           *string         `json:"notes,omitempty"`
	SuspensionFront json.RawMessage `json:"suspension_front,omitempty"`
	SuspensionRear  json.RawMessage `json:"suspension_rear,omitempty"`
	Tires           json.RawMessage `json:"tires,omitempty"`
	ContactPoints   json.RawMessage `json:"contact_points,omitempty"`
	Brakes          json.RawMessage `json:"brakes,omitempty"`
	Drivetrain      json.RawMessage `json:"drivetrain,omitempty"`
	IsActive        *bool           `json:"is_active,omitempty"`
}

// TagActivity is a lightweight activity summary for tag popups.
type TagActivity struct {
	ID        string    `json:"id"`
	Title     *string   `json:"title"`
	TitleAuto *string   `json:"title_auto,omitempty"`
	StartedAt time.Time `json:"started_at"`
	SportType string    `json:"sport_type"`
	DistanceM *float64  `json:"distance_m,omitempty"`
}

// TimeInZoneData represents time spent in a specific training zone.
type TimeInZoneData struct {
	ZoneType  string // "hr", "power", "speed", "cadence"
	ZoneIndex int
	ZoneHigh  float64 // zone upper boundary (bpm / watts / m/s / rpm)
	Seconds   int
}

// UserProfileImport holds data extracted from FIT user_profile + zones_target messages.
type UserProfileImport struct {
	Gender        string
	AgeYears      int
	HeightCm      int
	WeightKg      float32
	RestingHR     int
	MaxHRDetected int
	LTHR          int
	FTP           int
	MaxHR         int
}

// GearCostSummary represents the cost overview for a gear item.
type GearCostSummary struct {
	GearID      string   `json:"gear_id"`
	UserID      string   `json:"user_id"`
	Name        string   `json:"name"`
	TotalCost   float64  `json:"total_cost"`
	Currency    string   `json:"currency"`
	CostPerKm   *float64 `json:"cost_per_km,omitempty"`
	CostPerHour *float64 `json:"cost_per_hour,omitempty"`
}

// TimeInZone is the API response for a single zone.
type TimeInZone struct {
	ZoneType  string   `json:"zone_type"`
	ZoneIndex int      `json:"zone_index"`
	ZoneHigh  *float64 `json:"zone_high,omitempty"`
	Seconds   int      `json:"seconds"`
}

// ZoneDistributionResponse is the API response for zone distribution.
type ZoneDistributionResponse struct {
	HRZones     []TimeInZone `json:"hr_zones"`
	PowerZones  []TimeInZone `json:"power_zones"`
	HasFitZones bool         `json:"has_fit_zones"`
}

// PowerCurvePoint is a single point on the power duration curve.
type PowerCurvePoint struct {
	DurationS   int      `json:"duration_s"`
	PowerW      float64  `json:"power_w"`
	PowerWPerKg *float64 `json:"power_w_per_kg,omitempty"`
}

// PowerCurveResponse is the API response for the power duration curve.
type PowerCurveResponse struct {
	Points   []PowerCurvePoint `json:"points"`
	FtpW     *int              `json:"ftp_w,omitempty"`
	WeightKg *float32          `json:"weight_kg,omitempty"`
}
