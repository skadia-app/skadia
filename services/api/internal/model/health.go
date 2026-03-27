package model

import "time"

// HealthDailySummary aggregates daily activity and wellness metrics.
type HealthDailySummary struct {
	UserID              string    `json:"user_id"`
	CalendarDate        string    `json:"calendar_date"` // YYYY-MM-DD
	TotalSteps          *int      `json:"total_steps"`
	StepGoal            *int      `json:"step_goal"`
	TotalDistanceM      *float64  `json:"total_distance_m"`
	ActiveSeconds       *int      `json:"active_seconds"`
	HighlyActiveSeconds *int      `json:"highly_active_seconds"`
	RestingHeartRate    *int      `json:"resting_heart_rate"`
	MinHeartRate        *int      `json:"min_heart_rate"`
	MaxHeartRate        *int      `json:"max_heart_rate"`
	TotalCalories       *float64  `json:"total_calories"`
	ActiveCalories      *float64  `json:"active_calories"`
	BmrCalories         *float64  `json:"bmr_calories"`
	StressAvg           *float64  `json:"stress_avg"`
	BodyBatteryMin      *int      `json:"body_battery_min"`
	BodyBatteryMax      *int      `json:"body_battery_max"`
	HydrationML         *float64  `json:"hydration_ml"`
	HydrationGoalML     *float64  `json:"hydration_goal_ml"`
	SourceType          string    `json:"source_type"`
	ExtraData           *string   `json:"extra_data,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
}

// HealthSleep holds sleep stage breakdown and score for a night.
type HealthSleep struct {
	UserID          string    `json:"user_id"`
	CalendarDate    string    `json:"calendar_date"` // YYYY-MM-DD
	SleepStartUTC   time.Time `json:"sleep_start_utc"`
	SleepEndUTC     time.Time `json:"sleep_end_utc"`
	DeepSeconds     *int      `json:"deep_seconds"`
	LightSeconds    *int      `json:"light_seconds"`
	RemSeconds      *int      `json:"rem_seconds"`
	AwakeSeconds    *int      `json:"awake_seconds"`
	ScoreOverall    *int      `json:"score_overall"`
	ScoreQuality    *int      `json:"score_quality"`
	ScoreDuration   *int      `json:"score_duration"`
	ScoreDeep       *int      `json:"score_deep"`
	ScoreRem        *int      `json:"score_rem"`
	RespirationAvg  *float64  `json:"respiration_avg"`
	AwakeCount      *int      `json:"awake_count"`
	RestlessMoments *int      `json:"restless_moments"`
	AvgSleepStress  *float64  `json:"avg_sleep_stress"`
	SourceType      string    `json:"source_type"`
	ExtraData       *string   `json:"extra_data,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// HealthHRVSnapshot is a single health snapshot measurement (Garmin Health Snapshot feature).
// Contains HRV, heart rate, SpO2, respiration, and stress from a 2-minute measurement.
type HealthHRVSnapshot struct {
	UserID          string    `json:"user_id"`
	MeasuredAtUTC   time.Time `json:"measured_at_utc"`
	RmssdMS         *float64  `json:"rmssd_ms"`
	SdrrMS          *float64  `json:"sdrr_ms"`
	AvgHeartRateBPM *int16    `json:"avg_heart_rate_bpm"`
	SpO2Pct         *int16    `json:"spo2_pct"`
	RespirationBRPM *int16    `json:"respiration_brpm"`
	StressLevel     *int16    `json:"stress_level"`
	Title           *string   `json:"title"`
	TimeOfDay       *string   `json:"time_of_day"`
	SourceType      string    `json:"source_type"`
	ExtraData       *string   `json:"extra_data,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// HealthWeightEntry is a single body composition measurement.
type HealthWeightEntry struct {
	UserID        string    `json:"user_id"`
	MeasuredAtUTC time.Time `json:"measured_at_utc"`
	WeightG       int       `json:"weight_g"`
	BMI           *float64  `json:"bmi"`
	BodyFatPct    *float64  `json:"body_fat_pct"`
	MuscleMassG   *int      `json:"muscle_mass_g"`
	BoneMassG     *int      `json:"bone_mass_g"`
	SourceType    string    `json:"source_type"`
	ExtraData     *string   `json:"extra_data,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// HealthWeightCreate is used when manually creating a weight entry.
type HealthWeightCreate struct {
	MeasuredAtUTC string   `json:"measured_at_utc"` // RFC3339 string
	WeightG       int      `json:"weight_g"`
	BMI           *float64 `json:"bmi"`
	BodyFatPct    *float64 `json:"body_fat_pct"`
	MuscleMassG   *int     `json:"muscle_mass_g"`
	BoneMassG     *int     `json:"bone_mass_g"`
	SourceType    string   `json:"source_type"`
}

// HealthReadinessScore holds daily readiness and recovery factors.
type HealthReadinessScore struct {
	UserID                 string    `json:"user_id"`
	CalendarDate           string    `json:"calendar_date"` // YYYY-MM-DD
	Score                  *int      `json:"score"`
	Level                  *string   `json:"level"`
	SleepScore             *int      `json:"sleep_score"`
	HRVWeeklyAvgMS         *float64  `json:"hrv_weekly_avg_ms"`
	RecoveryTimeMin        *int      `json:"recovery_time_min"`
	AcwrPct                *float64  `json:"acwr_pct"`
	HRVFactorPct           *float64  `json:"hrv_factor_pct"`
	SleepHistoryFactorPct  *float64  `json:"sleep_history_factor_pct"`
	StressHistoryFactorPct *float64  `json:"stress_history_factor_pct"`
	RecoveryTimeFactorPct  *float64  `json:"recovery_time_factor_pct"`
	SourceType             string    `json:"source_type"`
	ExtraData              *string   `json:"extra_data,omitempty"`
	CreatedAt              time.Time `json:"created_at"`
}

// HealthTrainingLoad holds daily training load (ATL/CTL/ACWR).
type HealthTrainingLoad struct {
	UserID       string    `json:"user_id"`
	CalendarDate string    `json:"calendar_date"` // YYYY-MM-DD
	ATLAcute     *float64  `json:"atl_acute"`
	CTLChronic   *float64  `json:"ctl_chronic"`
	AcwrPct      *float64  `json:"acwr_pct"`
	AcwrStatus   *string   `json:"acwr_status"`
	SourceType   string    `json:"source_type"`
	ExtraData    *string   `json:"extra_data,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// HealthStressSnapshot is a point-in-time stress measurement.
type HealthStressSnapshot struct {
	UserID        string    `json:"user_id"`
	MeasuredAtUTC time.Time `json:"measured_at_utc"`
	StressLevel   *float64  `json:"stress_level"`
	SourceType    string    `json:"source_type"`
	ExtraData     *string   `json:"extra_data,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// HealthSpO2Snapshot is a point-in-time blood oxygen measurement.
type HealthSpO2Snapshot struct {
	UserID        string    `json:"user_id"`
	MeasuredAtUTC time.Time `json:"measured_at_utc"`
	SpO2Pct       *float64  `json:"spo2_pct"`
	SourceType    string    `json:"source_type"`
	ExtraData     *string   `json:"extra_data,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// HealthBloodPressure is a single blood pressure measurement.
type HealthBloodPressure struct {
	UserID        string    `json:"user_id"`
	MeasuredAtUTC time.Time `json:"measured_at_utc"`
	Systolic      *int      `json:"systolic"`
	Diastolic     *int      `json:"diastolic"`
	Pulse         *int      `json:"pulse"`
	SourceType    string    `json:"source_type"`
	ExtraData     *string   `json:"extra_data,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// HealthFitnessAge holds a computed or device-reported fitness age.
type HealthFitnessAge struct {
	UserID       string    `json:"user_id"`
	CalendarDate string    `json:"calendar_date"` // YYYY-MM-DD
	FitnessAge   *float64  `json:"fitness_age"`
	SourceType   string    `json:"source_type"`
	ExtraData    *string   `json:"extra_data,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// HealthHRVNightValue is a 5-minute RMSSD measurement during sleep.
// Sourced from Garmin monitoring FIT files (message hrv_value / 281).
type HealthHRVNightValue struct {
	UserID     string    `json:"user_id"`
	TS         time.Time `json:"ts"`
	RmssdMS    float64   `json:"rmssd_ms"`
	SourceType string    `json:"source_type"`
}

// MonitoringSample represents one row in health_monitoring_samples.
type MonitoringSample struct {
	UserID            string     `json:"-"`
	TS                time.Time  `json:"ts"`
	HeartRate         *int16     `json:"heart_rate,omitempty"`
	ActivityType      *int16     `json:"activity_type,omitempty"`
	Intensity         *int16     `json:"intensity,omitempty"`
	TemperatureC      *float64   `json:"temperature_c,omitempty"`
	TemperatureMinC   *float64   `json:"temperature_min_c,omitempty"`
	TemperatureMaxC   *float64   `json:"temperature_max_c,omitempty"`
	Stress            *int16     `json:"stress,omitempty"`
	BodyBattery       *int16     `json:"body_battery,omitempty"`
	BodyBatteryDelta  *int16     `json:"body_battery_delta,omitempty"`
	RespirationRate   *float64   `json:"respiration_rate,omitempty"`
	SleepLevel        *int16     `json:"sleep_level,omitempty"`
	SpO2Pct           *int16     `json:"spo2_pct,omitempty"`
	SpO2Confidence    *int16     `json:"spo2_confidence,omitempty"`
	WristTemperatureC *float64   `json:"wrist_temperature_c,omitempty"`
}

// TrainingZone represents a zone configuration for a sport.
type TrainingZone struct {
	ID                       string    `json:"id"`
	UserID                   string    `json:"user_id"`
	Sport                    int16     `json:"sport"`
	SubSport                 int16     `json:"sub_sport"`
	ZoneType                 string    `json:"zone_type"`
	MaxHeartRate             *int16    `json:"max_heart_rate,omitempty"`
	ThresholdHeartRate       *int16    `json:"threshold_heart_rate,omitempty"`
	FunctionalThresholdPower *int32    `json:"functional_threshold_power,omitempty"`
	HrCalcType               *int16    `json:"hr_calc_type,omitempty"`
	PwrCalcType              *int16    `json:"pwr_calc_type,omitempty"`
	ZoneNames                []string  `json:"zone_names"`
	ZoneHighs                []float64 `json:"zone_highs"`
	SourceType               string    `json:"source_type"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

// HealthOverview aggregates the most recent value of every health metric.
type HealthOverview struct {
	LatestDailySummary  *HealthDailySummary   `json:"latest_daily_summary"`
	LatestSleep         *HealthSleep          `json:"latest_sleep"`
	LatestHRV           *HealthHRVSnapshot    `json:"latest_hrv"`
	LatestWeight        *HealthWeightEntry    `json:"latest_weight"`
	LatestReadiness     *HealthReadinessScore `json:"latest_readiness"`
	LatestTrainingLoad  *HealthTrainingLoad   `json:"latest_training_load"`
	LatestStress        *HealthStressSnapshot `json:"latest_stress"`
	LatestSpO2          *HealthSpO2Snapshot   `json:"latest_spo2"`
	LatestBloodPressure *HealthBloodPressure  `json:"latest_blood_pressure"`
	LatestFitnessAge    *HealthFitnessAge     `json:"latest_fitness_age"`
}
