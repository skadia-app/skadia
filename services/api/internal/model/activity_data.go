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
	LapTimeInZones          map[string][]TimeInZoneData `json:"-"` // key = lap UUID
	SegmentLapTimeInZones   map[string][]TimeInZoneData `json:"-"` // key = segment lap UUID
	UserProfileImport       *UserProfileImport

	TSS   float64
	TRIMP float64

	Tags []string // existing tags on the activity (for enrichment merge)

	Visibility string // "private", "public", "followers" — empty means default ("private")

	IsIndoor             bool
	IsEbike              bool
	IsVirtual            bool
	IsManualEntry        bool
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

	// FIT SDK — New record collections (write-path only, not serialized directly)
	Events             []EventRecord       `json:"-"`
	Sets               []SetRecord         `json:"-"`
	Jumps              []JumpRecord        `json:"-"`
	HrData             []HrDataRecord      `json:"-"`
	SegmentLaps        []SegmentLapRecord  `json:"-"`
	Workout            *WorkoutRecord      `json:"-"`
	WorkoutSteps       []WorkoutStepRecord `json:"-"`
	DeveloperFieldDefs []DeveloperFieldDef `json:"-"`
	WorkoutID          *string             `json:"workout_id,omitempty"`

	// FIT SDK — Movement / Cycles
	TotalCycles       *int32   `json:"-"`
	TotalWorkJ        *int32   `json:"-"`
	TotalFatCalories  *int32   `json:"-"`
	MetabolicCalories *int32   `json:"-"`
	TotalMovingTimeS  *float64 `json:"-"`

	// FIT SDK — Swimming
	SwimStroke         *string  `json:"-"`
	PoolLengthM        *float64 `json:"-"`
	PoolLengthUnit     *string  `json:"-"`
	NumLengths         *int32   `json:"-"`
	NumActiveLengths   *int32   `json:"-"`
	AvgStrokeCount     *float64 `json:"-"`
	AvgStrokeDistanceM *float64 `json:"-"`
	StrokeCount        []int32  `json:"-"`
	ZoneCount          []int32  `json:"-"`

	// FIT SDK — Cadence Extended
	AvgFractionalCadence  *float64 `json:"-"`
	MaxFractionalCadence  *float64 `json:"-"`
	TotalFractionalCycles *float64 `json:"-"`

	// FIT SDK — Running Dynamics
	AvgVerticalOscillationMm *float64 `json:"-"`
	AvgStanceTimeMs          *float64 `json:"-"`
	AvgStanceTimePct         *float64 `json:"-"`
	AvgStanceTimeBalancePct  *float64 `json:"-"`
	AvgStepLengthMm          *float64 `json:"-"`
	AvgVerticalRatioPct      *float64 `json:"-"`

	// FIT SDK — Grade
	AvgGradePct    *float64 `json:"-"`
	AvgPosGradePct *float64 `json:"-"`
	AvgNegGradePct *float64 `json:"-"`
	MaxPosGradePct *float64 `json:"-"`
	MaxNegGradePct *float64 `json:"-"`

	// FIT SDK — Vertical Speed
	AvgPosVerticalSpeedMs *float64 `json:"-"`
	AvgNegVerticalSpeedMs *float64 `json:"-"`
	MaxPosVerticalSpeedMs *float64 `json:"-"`
	MaxNegVerticalSpeedMs *float64 `json:"-"`
	AvgVamMs              *float64 `json:"-"`

	// FIT SDK — Altitude Extended
	AvgAltitudeM            *float64 `json:"-"`
	TotalFractionalAscentM  *float64 `json:"-"`
	TotalFractionalDescentM *float64 `json:"-"`

	// FIT SDK — Pedal Analytics (15)
	AvgLeftTorqueEffectivenessPct  *float64 `json:"-"`
	AvgRightTorqueEffectivenessPct *float64 `json:"-"`
	AvgLeftPedalSmoothnessPct      *float64 `json:"-"`
	AvgRightPedalSmoothnessPct     *float64 `json:"-"`
	AvgCombinedPedalSmoothnessPct  *float64 `json:"-"`
	AvgLeftPcoMm                   *int16   `json:"-"`
	AvgRightPcoMm                  *int16   `json:"-"`
	AvgLeftPowerPhaseStartDeg      *float64 `json:"-"`
	AvgLeftPowerPhaseEndDeg        *float64 `json:"-"`
	AvgLeftPowerPhasePeakStartDeg  *float64 `json:"-"`
	AvgLeftPowerPhasePeakEndDeg    *float64 `json:"-"`
	AvgRightPowerPhaseStartDeg     *float64 `json:"-"`
	AvgRightPowerPhaseEndDeg       *float64 `json:"-"`
	AvgRightPowerPhasePeakStartDeg *float64 `json:"-"`
	AvgRightPowerPhasePeakEndDeg   *float64 `json:"-"`

	// FIT SDK — Power/Cadence by Position
	AvgPowerPositionW  []int32 `json:"-"`
	MaxPowerPositionW  []int32 `json:"-"`
	AvgCadencePosition []int16 `json:"-"`
	MaxCadencePosition []int16 `json:"-"`

	// FIT SDK — Standing/Sitting
	TimeStandingS *float64 `json:"-"`
	StandCount    *int32   `json:"-"`

	// FIT SDK — E-Bike
	AvgLevMotorPowerW        *int32   `json:"-"`
	MaxLevMotorPowerW        *int32   `json:"-"`
	LevBatteryConsumptionPct *float64 `json:"-"`

	// FIT SDK — MTB Grit/Flow
	TotalGrit *float64 `json:"-"`
	TotalFlow *float64 `json:"-"`
	AvgGrit   *float64 `json:"-"`
	AvgFlow   *float64 `json:"-"`

	// FIT SDK — Jumps
	JumpCount *int32 `json:"-"`

	// FIT SDK — Ball Sports
	MaxBallSpeedMs *float64 `json:"-"`
	AvgBallSpeedMs *float64 `json:"-"`
	PlayerScore    *int32   `json:"-"`
	OpponentScore  *int32   `json:"-"`
	OpponentName   *string  `json:"-"`

	// FIT SDK — Hemoglobin (arrays for multi-sensor)
	AvgTotalHemoglobinConc    []float64 `json:"-"`
	MinTotalHemoglobinConc    []float64 `json:"-"`
	MaxTotalHemoglobinConc    []float64 `json:"-"`
	AvgSaturatedHemoglobinPct []float64 `json:"-"`
	MinSaturatedHemoglobinPct []float64 `json:"-"`
	MaxSaturatedHemoglobinPct []float64 `json:"-"`

	// FIT SDK — Diving
	AvgDepthM        *float64 `json:"-"`
	MaxDepthM        *float64 `json:"-"`
	SurfaceIntervalS *int32   `json:"-"`
	DiveNumber       *int32   `json:"-"`
	StartCnsPct      *int16   `json:"-"`
	EndCnsPct        *int16   `json:"-"`
	StartN2Pct       *int32   `json:"-"`
	EndN2Pct         *int32   `json:"-"`
	O2ToxicityOtu    *int32   `json:"-"`

	// FIT SDK — Respiration / HRV Extended
	MaxRespirationRate *float64 `json:"-"`
	MinRespirationRate *float64 `json:"-"`
	SdrrHrvMs          *int16   `json:"-"`

	// FIT SDK — Temperature Extended
	MinCoreTemperatureC *float64 `json:"-"`
	MaxCoreTemperatureC *float64 `json:"-"`

	// FIT SDK — Misc
	AvgLapTimeS      *float64 `json:"-"`
	BestLapIndex     *int16   `json:"-"`
	NumLaps          *int32   `json:"-"`
	FirstLapIndex    *int32   `json:"-"`
	IntensityFactor  *float64 `json:"-"`
	LeftRightBalance *float64 `json:"-"`
	SportProfileName *string  `json:"-"`
	SportIndex       *int16   `json:"-"`
	NecLat           *float64 `json:"-"`
	NecLng           *float64 `json:"-"`
	SwcLat           *float64 `json:"-"`
	SwcLng           *float64 `json:"-"`
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
