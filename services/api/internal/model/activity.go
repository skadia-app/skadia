package model

import "time"

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
	IsIndoor         bool       `json:"is_indoor"`
	Tags             []string   `json:"tags"`
	Visibility       string     `json:"visibility"`
	Source           *string    `json:"source,omitempty"`
	SourceDevice     *string    `json:"source_device,omitempty"`
	EnrichmentStatus string     `json:"enrichment_status"`
	NeedsClassification bool    `json:"needs_classification"`
	HasGPS           bool       `json:"has_gps"`
	OriginalFilename *string    `json:"original_filename,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// ActivityUpdate is the PATCH request body for updating an activity.
type ActivityUpdate struct {
	Title       *string  `json:"title,omitempty"`
	Description *string  `json:"description,omitempty"`
	SportType   *string  `json:"sport_type,omitempty"`
	SubType     *string  `json:"sub_type,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Visibility  *string  `json:"visibility,omitempty"`
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

// ProblemDetail is an RFC 7807 error response.
type ProblemDetail struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail"`
	Instance string `json:"instance,omitempty"`
}
