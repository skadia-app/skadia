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
	MinTemperatureC  float64
	MaxTemperatureC  float64

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

	Visibility string // "private", "public", "followers" — empty means default ("private")

	IsIndoor             bool
	IsEbike              bool
	SportConflictPending bool
	HasGPS               bool
	StartLat             float64
	StartLng             float64
	EndLat               float64
	EndLng               float64

	Streams     []StreamPoint
	RRIntervals []uint16 // R-R intervals in ms (from FIT HRV messages)
	Sensors     []SensorRecord
	TrackPoints []TrackPoint

	// RecordingDevice is the primary recording device extracted from the FIT FileId message.
	// Transient — not persisted directly in ActivityData but handed to the device registry.
	RecordingDevice *DeviceUpsert

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
	// FIT SDK — Terrain & Movement
	GradePct            *float64 `json:"grade_pct,omitempty"`
	VerticalSpeedMs     *float64 `json:"vertical_speed_ms,omitempty"`
	GpsAccuracyM        *int16   `json:"gps_accuracy_m,omitempty"`
	Calories            *int32   `json:"calories,omitempty"`
	Resistance          *int16   `json:"resistance,omitempty"`
	TotalCycles         *int32   `json:"total_cycles,omitempty"`
	AccumulatedPowerW   *int32   `json:"accumulated_power_w,omitempty"`
	AbsolutePressurePa  *int32   `json:"absolute_pressure_pa,omitempty"`
	AscentRateMs        *float64 `json:"ascent_rate_ms,omitempty"`
	TimeFromCourseS     *float64 `json:"time_from_course_s,omitempty"`
	ActivityType        *int16   `json:"activity_type,omitempty"`
	DeviceIndex         *int16   `json:"device_index,omitempty"`
	// FIT SDK — Running Dynamics (extended)
	StepLengthMm            *float64 `json:"step_length_mm,omitempty"`
	VerticalRatioPct        *float64 `json:"vertical_ratio_pct,omitempty"`
	StanceTimeBalancePct    *float64 `json:"stance_time_balance_pct,omitempty"`
	CycleLengthM            *float64 `json:"cycle_length_m,omitempty"`
	FractionalCadence       *float64 `json:"fractional_cadence,omitempty"`
	// FIT SDK — Pedal Analytics
	LeftTorqueEffectivenessPct  *float64 `json:"left_torque_effectiveness_pct,omitempty"`
	RightTorqueEffectivenessPct *float64 `json:"right_torque_effectiveness_pct,omitempty"`
	LeftPedalSmoothnessPct      *float64 `json:"left_pedal_smoothness_pct,omitempty"`
	RightPedalSmoothnessPct     *float64 `json:"right_pedal_smoothness_pct,omitempty"`
	CombinedPedalSmoothnessPct  *float64 `json:"combined_pedal_smoothness_pct,omitempty"`
	LeftPcoMm                   *int16   `json:"left_pco_mm,omitempty"`
	RightPcoMm                  *int16   `json:"right_pco_mm,omitempty"`
	// FIT SDK — Power Phase
	LeftPowerPhaseStartDeg     *float64 `json:"left_power_phase_start_deg,omitempty"`
	LeftPowerPhaseEndDeg       *float64 `json:"left_power_phase_end_deg,omitempty"`
	LeftPowerPhasePeakStartDeg *float64 `json:"left_power_phase_peak_start_deg,omitempty"`
	LeftPowerPhasePeakEndDeg   *float64 `json:"left_power_phase_peak_end_deg,omitempty"`
	RightPowerPhaseStartDeg     *float64 `json:"right_power_phase_start_deg,omitempty"`
	RightPowerPhaseEndDeg       *float64 `json:"right_power_phase_end_deg,omitempty"`
	RightPowerPhasePeakStartDeg *float64 `json:"right_power_phase_peak_start_deg,omitempty"`
	RightPowerPhasePeakEndDeg   *float64 `json:"right_power_phase_peak_end_deg,omitempty"`
	// FIT SDK — Hemoglobin / Muscle Oxygen
	TotalHemoglobinConc    *float64 `json:"total_hemoglobin_conc,omitempty"`
	TotalHemoglobinConcMin *float64 `json:"total_hemoglobin_conc_min,omitempty"`
	TotalHemoglobinConcMax *float64 `json:"total_hemoglobin_conc_max,omitempty"`
	SaturatedHemoglobinPct    *float64 `json:"saturated_hemoglobin_pct,omitempty"`
	SaturatedHemoglobinPctMin *float64 `json:"saturated_hemoglobin_pct_min,omitempty"`
	SaturatedHemoglobinPctMax *float64 `json:"saturated_hemoglobin_pct_max,omitempty"`
	// FIT SDK — E-Bike
	MotorPowerW          *int32 `json:"motor_power_w,omitempty"`
	EbikeTravelRangeKm   *int32 `json:"ebike_travel_range_km,omitempty"`
	EbikeBatteryLevelPct *int16 `json:"ebike_battery_level_pct,omitempty"`
	EbikeAssistMode      *int16 `json:"ebike_assist_mode,omitempty"`
	EbikeAssistLevelPct  *int16 `json:"ebike_assist_level_pct,omitempty"`
	// FIT SDK — Health / Stress
	CoreTemperatureC       *float64 `json:"core_temperature_c,omitempty"`
	EnhancedRespirationRate *float64 `json:"enhanced_respiration_rate,omitempty"`
	CurrentStress          *float64 `json:"current_stress,omitempty"`
	BatterySocPct          *float64 `json:"battery_soc_pct,omitempty"`
	// FIT SDK — MTB Metrics
	Grit *float64 `json:"grit,omitempty"`
	Flow *float64 `json:"flow,omitempty"`
	// FIT SDK — Racket / Ball Sports
	StrokeType  *int16   `json:"stroke_type,omitempty"`
	Zone        *int16   `json:"zone,omitempty"`
	BallSpeedMs *float64 `json:"ball_speed_ms,omitempty"`
	// FIT SDK — Diving
	DepthM             *float64 `json:"depth_m,omitempty"`
	NextStopDepthM     *float64 `json:"next_stop_depth_m,omitempty"`
	NextStopTimeS      *int32   `json:"next_stop_time_s,omitempty"`
	TimeToSurfaceS     *int32   `json:"time_to_surface_s,omitempty"`
	NdlTimeS           *int32   `json:"ndl_time_s,omitempty"`
	CnsLoadPct         *int16   `json:"cns_load_pct,omitempty"`
	N2LoadPct          *int32   `json:"n2_load_pct,omitempty"`
	AirTimeRemainingS  *int32   `json:"air_time_remaining_s,omitempty"`
	PressureSac        *float64 `json:"pressure_sac,omitempty"`
	VolumeSac          *float64 `json:"volume_sac,omitempty"`
	Rmv                *float64 `json:"rmv,omitempty"`
	Po2                *float64 `json:"po2,omitempty"`
	// FIT SDK — Developer Fields
	DeveloperFields map[string]any `json:"developer_fields,omitempty"`
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
	DeviceUpsert      *DeviceUpsert `json:"-"` // Transient, nicht serialisiert
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
	MinTemperatureC         *float64   `json:"min_temperature_c,omitempty"`
	MaxTemperatureC         *float64   `json:"max_temperature_c,omitempty"`
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
	WeatherReadings         []WeatherReading `json:"weather_readings,omitempty"`
	IsRoundTrip             bool       `json:"is_round_trip"`
	Intensity               *string    `json:"intensity,omitempty"`
	TimeOfDay               []string   `json:"time_of_day,omitempty"`
	SunriseAt               *time.Time `json:"sunrise_at,omitempty"`
	SunsetAt                *time.Time `json:"sunset_at,omitempty"`
	OriginalFilename        *string    `json:"original_filename,omitempty"`
	OriginalFileFormat      *string    `json:"original_file_format,omitempty"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
	// Manually assigned location from a location tag (independent of GPS start_lat/start_lng).
	LocationLat *float64 `json:"location_lat,omitempty"`
	LocationLng *float64 `json:"location_lng,omitempty"`
	// Lift statistics (aggregated from confirmed lift candidates).
	LiftCount              *int     `json:"lift_count,omitempty"`
	LiftDistanceM          *float64 `json:"lift_distance_m,omitempty"`
	LiftAscentM            *float64 `json:"lift_ascent_m,omitempty"`
	LiftDescentM           *float64 `json:"lift_descent_m,omitempty"`
	LiftDurationS          *int     `json:"lift_duration_s,omitempty"`
	ExcludeLiftSubtraction bool     `json:"exclude_lift_subtraction"`
	ElevationSource        string   `json:"elevation_source,omitempty" db:"elevation_source"`
	DistanceSource         string   `json:"distance_source,omitempty" db:"distance_source"`
	// Multisport parent/child
	ParentID   *string `json:"parent_id,omitempty"`
	ChildCount int     `json:"child_count,omitempty"` // Computed, not in DB
	// Fusion result: maps stream fields to source activity IDs.
	FusionSources json.RawMessage `json:"fusion_sources,omitempty"`
	// Platform IDs for deep-linking back to the source platform.
	GarminActivityID *int64 `json:"garmin_activity_id,omitempty"`
	StravaActivityID *int64 `json:"strava_activity_id,omitempty"`
	KomootTourID     *int64 `json:"komoot_tour_id,omitempty"`
	// Sport conflict resolution
	SportConflictPending bool               `json:"sport_conflict_pending"`
	SportConflictMeta    *SportConflictMeta `json:"sport_conflict,omitempty"`
	// Device registry link
	RecordingDeviceID   *string `json:"recording_device_id,omitempty"`
	RecordingDeviceName *string `json:"recording_device_name,omitempty"`
}

// SportConflictMeta holds conflict metadata embedded from import_queue.
type SportConflictMeta struct {
	FileSport      string `json:"file_sport"`
	FileSource     string `json:"file_source"`
	PlatformSport  string `json:"platform_sport"`
	PlatformSource string `json:"platform_source"`
}

// ActivityUpdate is the PATCH request body for updating an activity.
type ActivityUpdate struct {
	Title          *string         `json:"title,omitempty"`
	Description    *string         `json:"description,omitempty"`
	SportType      *string         `json:"sport_type,omitempty"`
	SubType        *string         `json:"sub_type,omitempty"`
	Tags           []string        `json:"tags,omitempty"`
	Visibility     *string         `json:"visibility,omitempty"`
	HideMap        *bool           `json:"hide_map,omitempty"`
	DataVisibility map[string]any  `json:"data_visibility,omitempty"`
	GearID         json.RawMessage `json:"gear_id,omitempty"`
	// Manually assigned location (independent of GPS start_lat/start_lng).
	LocationLat            *float64 `json:"location_lat,omitempty"`
	LocationLng            *float64 `json:"location_lng,omitempty"`
	ExcludeLiftSubtraction *bool    `json:"exclude_lift_subtraction,omitempty"`
}

// ActivityList is the paginated list response.
type ActivityList struct {
	Activities []Activity `json:"activities"`
	NextCursor string     `json:"next_cursor,omitempty"`
	Total      int        `json:"total"`
}

// ActivityFilters holds filter parameters for the ListActivities query.
type ActivityFilters struct {
	SportTypes  []string
	SubTypes    []string
	DateFrom    string
	DateTo      string
	IsIndoor    *bool
	IsEbike     *bool
	HasGPS      *bool
	GearID      string
	Search      string
	IncludeTags []string
	ExcludeTags []string
	IncludeGear    []string
	ExcludeGear    []string
	IncludeDevices []string
	ExcludeDevices []string
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
	// FIT SDK — Terrain & Movement
	GradePct            *float64 `json:"grade_pct,omitempty"`
	VerticalSpeedMs     *float64 `json:"vertical_speed_ms,omitempty"`
	GpsAccuracyM        *int16   `json:"gps_accuracy_m,omitempty"`
	Calories            *int32   `json:"calories,omitempty"`
	Resistance          *int16   `json:"resistance,omitempty"`
	TotalCycles         *int32   `json:"total_cycles,omitempty"`
	AccumulatedPowerW   *int32   `json:"accumulated_power_w,omitempty"`
	AbsolutePressurePa  *int32   `json:"absolute_pressure_pa,omitempty"`
	AscentRateMs        *float64 `json:"ascent_rate_ms,omitempty"`
	TimeFromCourseS     *float64 `json:"time_from_course_s,omitempty"`
	ActivityType        *int16   `json:"activity_type,omitempty"`
	DeviceIndex         *int16   `json:"device_index,omitempty"`
	// FIT SDK — Running Dynamics (extended)
	StepLengthMm            *float64 `json:"step_length_mm,omitempty"`
	VerticalRatioPct        *float64 `json:"vertical_ratio_pct,omitempty"`
	StanceTimeBalancePct    *float64 `json:"stance_time_balance_pct,omitempty"`
	CycleLengthM            *float64 `json:"cycle_length_m,omitempty"`
	FractionalCadence       *float64 `json:"fractional_cadence,omitempty"`
	// FIT SDK — Pedal Analytics
	LeftTorqueEffectivenessPct  *float64 `json:"left_torque_effectiveness_pct,omitempty"`
	RightTorqueEffectivenessPct *float64 `json:"right_torque_effectiveness_pct,omitempty"`
	LeftPedalSmoothnessPct      *float64 `json:"left_pedal_smoothness_pct,omitempty"`
	RightPedalSmoothnessPct     *float64 `json:"right_pedal_smoothness_pct,omitempty"`
	CombinedPedalSmoothnessPct  *float64 `json:"combined_pedal_smoothness_pct,omitempty"`
	LeftPcoMm                   *int16   `json:"left_pco_mm,omitempty"`
	RightPcoMm                  *int16   `json:"right_pco_mm,omitempty"`
	// FIT SDK — Power Phase
	LeftPowerPhaseStartDeg     *float64 `json:"left_power_phase_start_deg,omitempty"`
	LeftPowerPhaseEndDeg       *float64 `json:"left_power_phase_end_deg,omitempty"`
	LeftPowerPhasePeakStartDeg *float64 `json:"left_power_phase_peak_start_deg,omitempty"`
	LeftPowerPhasePeakEndDeg   *float64 `json:"left_power_phase_peak_end_deg,omitempty"`
	RightPowerPhaseStartDeg     *float64 `json:"right_power_phase_start_deg,omitempty"`
	RightPowerPhaseEndDeg       *float64 `json:"right_power_phase_end_deg,omitempty"`
	RightPowerPhasePeakStartDeg *float64 `json:"right_power_phase_peak_start_deg,omitempty"`
	RightPowerPhasePeakEndDeg   *float64 `json:"right_power_phase_peak_end_deg,omitempty"`
	// FIT SDK — Hemoglobin / Muscle Oxygen
	TotalHemoglobinConc    *float64 `json:"total_hemoglobin_conc,omitempty"`
	TotalHemoglobinConcMin *float64 `json:"total_hemoglobin_conc_min,omitempty"`
	TotalHemoglobinConcMax *float64 `json:"total_hemoglobin_conc_max,omitempty"`
	SaturatedHemoglobinPct    *float64 `json:"saturated_hemoglobin_pct,omitempty"`
	SaturatedHemoglobinPctMin *float64 `json:"saturated_hemoglobin_pct_min,omitempty"`
	SaturatedHemoglobinPctMax *float64 `json:"saturated_hemoglobin_pct_max,omitempty"`
	// FIT SDK — E-Bike
	MotorPowerW          *int32 `json:"motor_power_w,omitempty"`
	EbikeTravelRangeKm   *int32 `json:"ebike_travel_range_km,omitempty"`
	EbikeBatteryLevelPct *int16 `json:"ebike_battery_level_pct,omitempty"`
	EbikeAssistMode      *int16 `json:"ebike_assist_mode,omitempty"`
	EbikeAssistLevelPct  *int16 `json:"ebike_assist_level_pct,omitempty"`
	// FIT SDK — Health / Stress
	CoreTemperatureC       *float64 `json:"core_temperature_c,omitempty"`
	EnhancedRespirationRate *float64 `json:"enhanced_respiration_rate,omitempty"`
	CurrentStress          *float64 `json:"current_stress,omitempty"`
	BatterySocPct          *float64 `json:"battery_soc_pct,omitempty"`
	// FIT SDK — MTB Metrics
	Grit *float64 `json:"grit,omitempty"`
	Flow *float64 `json:"flow,omitempty"`
	// FIT SDK — Racket / Ball Sports
	StrokeType  *int16   `json:"stroke_type,omitempty"`
	Zone        *int16   `json:"zone,omitempty"`
	BallSpeedMs *float64 `json:"ball_speed_ms,omitempty"`
	// FIT SDK — Diving
	DepthM             *float64 `json:"depth_m,omitempty"`
	NextStopDepthM     *float64 `json:"next_stop_depth_m,omitempty"`
	NextStopTimeS      *int32   `json:"next_stop_time_s,omitempty"`
	TimeToSurfaceS     *int32   `json:"time_to_surface_s,omitempty"`
	NdlTimeS           *int32   `json:"ndl_time_s,omitempty"`
	CnsLoadPct         *int16   `json:"cns_load_pct,omitempty"`
	N2LoadPct          *int32   `json:"n2_load_pct,omitempty"`
	AirTimeRemainingS  *int32   `json:"air_time_remaining_s,omitempty"`
	PressureSac        *float64 `json:"pressure_sac,omitempty"`
	VolumeSac          *float64 `json:"volume_sac,omitempty"`
	Rmv                *float64 `json:"rmv,omitempty"`
	Po2                *float64 `json:"po2,omitempty"`
	// FIT SDK — Developer Fields
	DeveloperFields map[string]any `json:"developer_fields,omitempty"`
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
	DeviceID         *string  `json:"device_id,omitempty"`
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
	// ClimbingGrade is the manually entered difficulty rating (e.g. "6b", "V4", "7A+").
	// Not parseable from standard FIT data; must be entered by the user after import.
	ClimbingGrade string
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
	// ClimbingGrade is the manually set difficulty grade for a climbing/bouldering route.
	ClimbingGrade *string `json:"climbing_grade,omitempty"`
}

// SplitUpdate is the PATCH request body for updating a split.
type SplitUpdate struct {
	ClimbingGrade *string `json:"climbing_grade"`
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
	SportClass     *string `json:"sport_class,omitempty"`
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

// PrivacyZone represents a user-defined private zone (e.g. home, work).
type PrivacyZone struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	Lat       float64   `json:"lat"`
	Lng       float64   `json:"lng"`
	RadiusM   int       `json:"radius_m"`
	Color     string    `json:"color"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	// Polygon fields (zone_type = 'polygon')
	ZoneType string  `json:"zone_type"`         // 'circle' or 'polygon'
	Polygon  *string `json:"polygon,omitempty"` // GeoJSON Polygon string, set when zone_type='polygon'
}

// PrivacyZoneUpdate is the request body for updating a private zone.
type PrivacyZoneUpdate struct {
	Name     *string  `json:"name,omitempty"`
	Lat      *float64 `json:"lat,omitempty"`
	Lng      *float64 `json:"lng,omitempty"`
	RadiusM  *int     `json:"radius_m,omitempty"`
	Color    *string  `json:"color,omitempty"`
	IsActive *bool    `json:"is_active,omitempty"`
	// Polygon fields
	ZoneType *string `json:"zone_type,omitempty"` // 'circle' or 'polygon'
	Polygon  *string `json:"polygon,omitempty"`   // GeoJSON Polygon string
}

// GeoTag represents a geographic feature associated with an activity.
type GeoTag struct {
	ID           string    `json:"id"`
	ActivityID   string    `json:"activity_id"`
	Category     string    `json:"category"`
	Name         string    `json:"name"`
	OsmID        *int64    `json:"osm_id,omitempty"`
	Lat          *float64  `json:"lat,omitempty"`
	Lng          *float64  `json:"lng,omitempty"`
	Ele          *float64  `json:"ele,omitempty"`
	DistM        *float64  `json:"dist_m,omitempty"`
	DurationMin  *int      `json:"duration_min,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	MapType      *string   `json:"map_type,omitempty"       db:"map_type"`
	MapPointLat  *float64  `json:"map_point_lat,omitempty"  db:"map_point_lat"`
	MapPointLon  *float64  `json:"map_point_lon,omitempty"  db:"map_point_lon"`
	MapBboxWest  *float64  `json:"map_bbox_west,omitempty"  db:"map_bbox_west"`
	MapBboxSouth *float64  `json:"map_bbox_south,omitempty" db:"map_bbox_south"`
	MapBboxEast  *float64  `json:"map_bbox_east,omitempty"  db:"map_bbox_east"`
	MapBboxNorth *float64  `json:"map_bbox_north,omitempty" db:"map_bbox_north"`
	LengthM      *float64  `json:"length_m,omitempty"       db:"length_m"`
	EleM         *float64  `json:"ele_m,omitempty"          db:"ele_m"`
	// MapSection is a geometry column — read from DB as GeoJSON via ST_AsGeoJSON.
	MapSection *string `json:"map_section,omitempty" db:"-"`
	Source     *string `json:"source,omitempty"`
	AdminLevel *int16  `json:"admin_level,omitempty" db:"admin_level"`
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
	ConditionsLogic *string         `json:"conditions_logic,omitempty"`
	Description     *string         `json:"description,omitempty"`
	ActivityCount   int             `json:"activity_count"`
	CreatedAt       time.Time       `json:"created_at"`
	// Optional geo-coordinates — when set, the tag acts as a "location tag"
	// that appears as a pin on the map.
	Lat                *float64 `json:"lat,omitempty"`
	Lng                *float64 `json:"lng,omitempty"`
	CoverImageURL      *string  `json:"cover_image_url,omitempty"`
	CoverImageThumbURL *string  `json:"cover_image_thumb_url,omitempty"`
	Zone               *TagZone `json:"zone,omitempty"`
	HasAutomation      bool     `json:"has_automation"`
}

// TagZone represents an optional geographic zone attached to a location tag.
// zone_type is "circle" (center + radius_m) or "polygon" (polygon GeoJSON).
type TagZone struct {
	ID        string      `json:"id"`
	TagID     string      `json:"tag_id"`
	ZoneType  string      `json:"zone_type"`        // "circle" | "polygon"
	Center    *[2]float64 `json:"center,omitempty"` // [lng, lat]
	RadiusM   *float64    `json:"radius_m,omitempty"`
	Polygon   *string     `json:"polygon,omitempty"` // GeoJSON Polygon string
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// EnrichmentSettings holds user preferences for activity enrichment.
type EnrichmentSettings struct {
	UserID                  string  `json:"user_id"`
	EnableSummitDetection   bool    `json:"enable_summit_detection"`
	EnableWeatherEnrichment bool    `json:"enable_weather_enrichment"`
	EnableStopDetection     bool    `json:"enable_stop_detection"`
	EnableAscentDetection   bool    `json:"enable_ascent_detection"`
	EnableCommuteDetection  bool    `json:"enable_commute_detection"`
	EnableAutoNaming        bool    `json:"enable_auto_naming"`
	EnableLapDetection      bool    `json:"enable_lap_detection"`
	ClimbMinGradientPct     float64 `json:"climb_min_gradient_pct"`
	ClimbMinElevationM      float64 `json:"climb_min_elevation_m"`
	RoundTripThresholdM     float64 `json:"round_trip_threshold_m"`
	OCRInvoicePrompt        *string `json:"ocr_invoice_prompt,omitempty"`
	OCRGearPrompt           *string `json:"ocr_gear_prompt,omitempty"`

	// Detection module flags (Migration 000192).
	// Each flag controls whether the corresponding enrichment module runs.
	// Default: true (all modules enabled for new and existing users).
	// Note: module string literals ("summits", "lifts" etc.) in Go/NATS are stable identifiers.
	EnableForestDetection     bool `json:"enable_forest_detection"`
	EnableWaterDetection      bool `json:"enable_water_detection"`
	EnableSettlementDetection bool `json:"enable_settlement_detection"`
	EnableBorderDetection     bool `json:"enable_border_detection"`
	EnableLiftDetection       bool `json:"enable_lift_detection"`
	EnableSurfaceDetection    bool `json:"enable_surface_detection"`
	EnableIntervalDetection   bool `json:"enable_interval_detection"`
	EnableEmbeddings          bool `json:"enable_embeddings"`

	// Photo preview size: "2048" (default), "4k", or "original" (Migration 000120).
	PhotoPreviewSize string `json:"photo_preview_size"`

	// Terra incognita buffer radius in meters (default 1000, Migration 000121).
	TerraIncognitaRadiusM int `json:"terra_incognita_radius_m"`

	// Route duplicate detection thresholds (Migration 000143)
	RouteDuplicateHausdorffM   float64 `json:"route_duplicate_hausdorff_m"`
	RouteDuplicateLengthDiffKm float64 `json:"route_duplicate_length_diff_km"`

	// Number of frames to extract from videos for CLIP embedding (1–10, Migration 000146).
	VideoClipFrames int `json:"video_clip_frames"`

	// pHash Hamming-distance threshold for photo deduplication (1–10, Migration 000149).
	PhashThresholdBits int `json:"phash_threshold_bits"`
}

// EnabledModules returns the list of enrichment module names that are currently
// enabled. An empty slice means every module is enabled (backwards-compat
// default). This list is embedded directly in the NATS Stage2ReadyEvent so
// that the ML service can skip disabled modules without a DB round-trip.
// Note: module string literals ("summits", "lifts" etc.) are stable identifiers
// and do NOT change when DB column names change.
func (s *EnrichmentSettings) EnabledModules() []string {
	type flag struct {
		name    string
		enabled bool
	}
	flags := []flag{
		{"summits", s.EnableSummitDetection},
		{"lifts", s.EnableLiftDetection},
		{"settlements", s.EnableSettlementDetection},
		{"commute", s.EnableCommuteDetection},
		{"borders", s.EnableBorderDetection},
		{"water", s.EnableWaterDetection},
		{"forests", s.EnableForestDetection},
		{"surfaces", s.EnableSurfaceDetection},
		{"stops", s.EnableStopDetection},
		{"intervals", s.EnableIntervalDetection},
		{"ascents", s.EnableAscentDetection},
		{"embeddings", s.EnableEmbeddings},
	}

	// If all modules are enabled return nil so the NATS message stays small
	// and the ML service applies its default (run all).
	allEnabled := true
	for _, f := range flags {
		if !f.enabled {
			allEnabled = false
			break
		}
	}
	if allEnabled {
		return nil
	}

	var enabled []string
	for _, f := range flags {
		if f.enabled {
			enabled = append(enabled, f.name)
		}
	}
	return enabled
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
	WeightG          *int     `json:"weight_g,omitempty"`
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
	IsSystem        bool            `json:"is_system"`
	SystemType      *string         `json:"system_type"`
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
	PoiType     *string `json:"poi_type,omitempty"     db:"poi_type"`
	PoiName     *string `json:"poi_name,omitempty"     db:"poi_name"`
	PoiOsmID    *int64  `json:"poi_osm_id,omitempty"   db:"poi_osm_id"`
	PoiCategory *string `json:"poi_category,omitempty" db:"poi_category"`
	Confidence  *string `json:"confidence,omitempty"`
}

// ActivitySurface represents a detected surface type within an activity.
type ActivitySurface struct {
	ID          string  `json:"id"`
	Category    string  `json:"category"`     // "surface" or "highway" (Migration 000201)
	SurfaceType string  `json:"surface_type"`
	PathType    string  `json:"path_type"`
	LengthM     float64 `json:"length_m"`
	Percentage  float64 `json:"percentage"`
}

// EnrichmentModuleStatus represents the status of a single Stage-2 enrichment module.
type EnrichmentModuleStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "completed", "error", "no_data", "pending"
	Error  string `json:"error,omitempty"`
	Count  int    `json:"count"`
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

// WeatherReading is one weather snapshot for an activity.
// An activity has 0–2 readings stored in the activity_weather table.
type WeatherReading struct {
	ID              string    `json:"id"`
	ReadingType     string    `json:"reading_type"` // single|start|summit|midpoint
	Label           *string   `json:"label,omitempty"`
	RecordedAt      time.Time `json:"recorded_at"`
	Lat             *float64  `json:"lat,omitempty"`
	Lng             *float64  `json:"lng,omitempty"`
	ElevationM      *float64  `json:"elevation_m,omitempty"`
	TempC           *float64  `json:"temp_c,omitempty"`
	ApparentTempC   *float64  `json:"apparent_temp_c,omitempty"`
	PrecipitationMM *float64  `json:"precipitation_mm,omitempty"`
	WindSpeedMS     *float64  `json:"wind_speed_ms,omitempty"`
	WindGustsMS     *float64  `json:"wind_gusts_ms,omitempty"`
	Humidity        *int      `json:"humidity,omitempty"`
	UVIndex         *float64  `json:"uv_index,omitempty"`
	FreezingLevelM  *float64  `json:"freezing_level_m,omitempty"`
	Condition       *string   `json:"condition,omitempty"`
	Tags            []string  `json:"tags,omitempty"`
	Model           *string   `json:"model,omitempty"`
}

// WeatherPrefs holds the user's configurable weather behaviour settings.
type WeatherPrefs struct {
	ReadingMode        string   `json:"reading_mode"`
	MountainThresholdM int      `json:"mountain_threshold_m"`
	LongDurationS      int      `json:"long_duration_s"`
	DisplayFields      []string `json:"display_fields"`
	AutoTags           bool     `json:"auto_tags"`
}

// DefaultWeatherPrefs returns the application defaults (matches migration 000168 defaults).
func DefaultWeatherPrefs() WeatherPrefs {
	return WeatherPrefs{
		ReadingMode:        "auto",
		MountainThresholdM: 500,
		LongDurationS:      14400,
		AutoTags:           true,
		DisplayFields: []string{
			"temp_c", "apparent_temp_c", "precipitation_mm",
			"wind_speed_ms", "humidity", "condition",
		},
	}
}
