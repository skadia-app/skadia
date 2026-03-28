package model

import "time"

// LapRecord holds extracted lap data from FIT lap messages.
type LapRecord struct {
	ID                string // client-side UUID, generated before INSERT
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
	// FIT SDK — Cycles/Strokes
	TotalCycles           *int32   `json:"-"`
	TotalFatCalories      *int32   `json:"-"`
	AvgStrokeDistanceM    *float64 `json:"-"`
	FirstLengthIndex      *int32   `json:"-"`
	AvgFractionalCadence  *float64 `json:"-"`
	MaxFractionalCadence  *float64 `json:"-"`
	TotalFractionalCycles *float64 `json:"-"`
	StrokeCount           []int32  `json:"-"`
	ZoneCount             []int32  `json:"-"`
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
	// FIT SDK — Temperature Extended
	MaxTemperatureC     *int16   `json:"-"`
	MinTemperatureC     *int16   `json:"-"`
	AvgCoreTemperatureC *float64 `json:"-"`
	MinCoreTemperatureC *float64 `json:"-"`
	MaxCoreTemperatureC *float64 `json:"-"`
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
	// FIT SDK — L/R Balance
	LeftRightBalance *float64 `json:"-"`
	// FIT SDK — E-Bike
	AvgLevMotorPowerW        *int32   `json:"-"`
	MaxLevMotorPowerW        *int32   `json:"-"`
	LevBatteryConsumptionPct *float64 `json:"-"`
	// FIT SDK — Grit/Flow
	TotalGrit *float64 `json:"-"`
	TotalFlow *float64 `json:"-"`
	AvgGrit   *float64 `json:"-"`
	AvgFlow   *float64 `json:"-"`
	// FIT SDK — Jumps
	JumpCount *int32 `json:"-"`
	// FIT SDK — Scores
	PlayerScore   *int32 `json:"-"`
	OpponentScore *int32 `json:"-"`
	// FIT SDK — Hemoglobin (arrays for multi-sensor)
	AvgTotalHemoglobinConc    []float64 `json:"-"`
	MinTotalHemoglobinConc    []float64 `json:"-"`
	MaxTotalHemoglobinConc    []float64 `json:"-"`
	AvgSaturatedHemoglobinPct []float64 `json:"-"`
	MinSaturatedHemoglobinPct []float64 `json:"-"`
	MaxSaturatedHemoglobinPct []float64 `json:"-"`
	// FIT SDK — Diving
	AvgDepthM *float64 `json:"-"`
	MaxDepthM *float64 `json:"-"`
	// FIT SDK — Respiration
	AvgRespirationRate *float64 `json:"-"`
	MaxRespirationRate *float64 `json:"-"`
	// FIT SDK — Altitude Extended
	TotalFractionalAscentM  *float64 `json:"-"`
	TotalFractionalDescentM *float64 `json:"-"`
	// FIT SDK — Misc
	RepetitionNum *int32 `json:"-"`
	WktStepIndex  *int16 `json:"-"`
	EventGroup    *int16 `json:"-"`
	GpsAccuracyM  *int16 `json:"-"`
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
	// FIT SDK — Cycles/Strokes
	TotalCycles           *int32   `json:"total_cycles,omitempty"`
	TotalFatCalories      *int32   `json:"total_fat_calories,omitempty"`
	AvgStrokeDistanceM    *float64 `json:"avg_stroke_distance_m,omitempty"`
	FirstLengthIndex      *int32   `json:"first_length_index,omitempty"`
	AvgFractionalCadence  *float64 `json:"avg_fractional_cadence,omitempty"`
	MaxFractionalCadence  *float64 `json:"max_fractional_cadence,omitempty"`
	TotalFractionalCycles *float64 `json:"total_fractional_cycles,omitempty"`
	StrokeCount           []int32  `json:"stroke_count,omitempty"`
	ZoneCount             []int32  `json:"zone_count,omitempty"`
	// FIT SDK — Running Dynamics
	AvgVerticalOscillationMm *float64 `json:"avg_vertical_oscillation_mm,omitempty"`
	AvgStanceTimeMs          *float64 `json:"avg_stance_time_ms,omitempty"`
	AvgStanceTimePct         *float64 `json:"avg_stance_time_pct,omitempty"`
	AvgStanceTimeBalancePct  *float64 `json:"avg_stance_time_balance_pct,omitempty"`
	AvgStepLengthMm          *float64 `json:"avg_step_length_mm,omitempty"`
	AvgVerticalRatioPct      *float64 `json:"avg_vertical_ratio_pct,omitempty"`
	// FIT SDK — Grade
	AvgGradePct    *float64 `json:"avg_grade_pct,omitempty"`
	AvgPosGradePct *float64 `json:"avg_pos_grade_pct,omitempty"`
	AvgNegGradePct *float64 `json:"avg_neg_grade_pct,omitempty"`
	MaxPosGradePct *float64 `json:"max_pos_grade_pct,omitempty"`
	MaxNegGradePct *float64 `json:"max_neg_grade_pct,omitempty"`
	// FIT SDK — Vertical Speed
	AvgPosVerticalSpeedMs *float64 `json:"avg_pos_vertical_speed_ms,omitempty"`
	AvgNegVerticalSpeedMs *float64 `json:"avg_neg_vertical_speed_ms,omitempty"`
	MaxPosVerticalSpeedMs *float64 `json:"max_pos_vertical_speed_ms,omitempty"`
	MaxNegVerticalSpeedMs *float64 `json:"max_neg_vertical_speed_ms,omitempty"`
	AvgVamMs              *float64 `json:"avg_vam_ms,omitempty"`
	// FIT SDK — Temperature Extended
	MaxTemperatureC     *int16   `json:"max_temperature_c,omitempty"`
	MinTemperatureC     *int16   `json:"min_temperature_c,omitempty"`
	AvgCoreTemperatureC *float64 `json:"avg_core_temperature_c,omitempty"`
	MinCoreTemperatureC *float64 `json:"min_core_temperature_c,omitempty"`
	MaxCoreTemperatureC *float64 `json:"max_core_temperature_c,omitempty"`
	// FIT SDK — Pedal Analytics (15)
	AvgLeftTorqueEffectivenessPct  *float64 `json:"avg_left_torque_effectiveness_pct,omitempty"`
	AvgRightTorqueEffectivenessPct *float64 `json:"avg_right_torque_effectiveness_pct,omitempty"`
	AvgLeftPedalSmoothnessPct      *float64 `json:"avg_left_pedal_smoothness_pct,omitempty"`
	AvgRightPedalSmoothnessPct     *float64 `json:"avg_right_pedal_smoothness_pct,omitempty"`
	AvgCombinedPedalSmoothnessPct  *float64 `json:"avg_combined_pedal_smoothness_pct,omitempty"`
	AvgLeftPcoMm                   *int16   `json:"avg_left_pco_mm,omitempty"`
	AvgRightPcoMm                  *int16   `json:"avg_right_pco_mm,omitempty"`
	AvgLeftPowerPhaseStartDeg      *float64 `json:"avg_left_power_phase_start_deg,omitempty"`
	AvgLeftPowerPhaseEndDeg        *float64 `json:"avg_left_power_phase_end_deg,omitempty"`
	AvgLeftPowerPhasePeakStartDeg  *float64 `json:"avg_left_power_phase_peak_start_deg,omitempty"`
	AvgLeftPowerPhasePeakEndDeg    *float64 `json:"avg_left_power_phase_peak_end_deg,omitempty"`
	AvgRightPowerPhaseStartDeg     *float64 `json:"avg_right_power_phase_start_deg,omitempty"`
	AvgRightPowerPhaseEndDeg       *float64 `json:"avg_right_power_phase_end_deg,omitempty"`
	AvgRightPowerPhasePeakStartDeg *float64 `json:"avg_right_power_phase_peak_start_deg,omitempty"`
	AvgRightPowerPhasePeakEndDeg   *float64 `json:"avg_right_power_phase_peak_end_deg,omitempty"`
	// FIT SDK — Power/Cadence by Position
	AvgPowerPositionW  []int32 `json:"avg_power_position_w,omitempty"`
	MaxPowerPositionW  []int32 `json:"max_power_position_w,omitempty"`
	AvgCadencePosition []int16 `json:"avg_cadence_position,omitempty"`
	MaxCadencePosition []int16 `json:"max_cadence_position,omitempty"`
	// FIT SDK — Standing/Sitting
	TimeStandingS *float64 `json:"time_standing_s,omitempty"`
	StandCount    *int32   `json:"stand_count,omitempty"`
	// FIT SDK — L/R Balance
	LeftRightBalance *float64 `json:"left_right_balance,omitempty"`
	// FIT SDK — E-Bike
	AvgLevMotorPowerW        *int32   `json:"avg_lev_motor_power_w,omitempty"`
	MaxLevMotorPowerW        *int32   `json:"max_lev_motor_power_w,omitempty"`
	LevBatteryConsumptionPct *float64 `json:"lev_battery_consumption_pct,omitempty"`
	// FIT SDK — Grit/Flow
	TotalGrit *float64 `json:"total_grit,omitempty"`
	TotalFlow *float64 `json:"total_flow,omitempty"`
	AvgGrit   *float64 `json:"avg_grit,omitempty"`
	AvgFlow   *float64 `json:"avg_flow,omitempty"`
	// FIT SDK — Jumps
	JumpCount *int32 `json:"jump_count,omitempty"`
	// FIT SDK — Scores
	PlayerScore   *int32 `json:"player_score,omitempty"`
	OpponentScore *int32 `json:"opponent_score,omitempty"`
	// FIT SDK — Hemoglobin (arrays for multi-sensor)
	AvgTotalHemoglobinConc    []float64 `json:"avg_total_hemoglobin_conc,omitempty"`
	MinTotalHemoglobinConc    []float64 `json:"min_total_hemoglobin_conc,omitempty"`
	MaxTotalHemoglobinConc    []float64 `json:"max_total_hemoglobin_conc,omitempty"`
	AvgSaturatedHemoglobinPct []float64 `json:"avg_saturated_hemoglobin_pct,omitempty"`
	MinSaturatedHemoglobinPct []float64 `json:"min_saturated_hemoglobin_pct,omitempty"`
	MaxSaturatedHemoglobinPct []float64 `json:"max_saturated_hemoglobin_pct,omitempty"`
	// FIT SDK — Diving
	AvgDepthM *float64 `json:"avg_depth_m,omitempty"`
	MaxDepthM *float64 `json:"max_depth_m,omitempty"`
	// FIT SDK — Respiration
	AvgRespirationRate *float64 `json:"avg_respiration_rate,omitempty"`
	MaxRespirationRate *float64 `json:"max_respiration_rate,omitempty"`
	// FIT SDK — Altitude Extended
	TotalFractionalAscentM  *float64 `json:"total_fractional_ascent_m,omitempty"`
	TotalFractionalDescentM *float64 `json:"total_fractional_descent_m,omitempty"`
	// FIT SDK — Misc
	RepetitionNum *int32 `json:"repetition_num,omitempty"`
	WktStepIndex  *int16 `json:"wkt_step_index,omitempty"`
	EventGroup    *int16 `json:"event_group,omitempty"`
	GpsAccuracyM  *int16 `json:"gps_accuracy_m,omitempty"`
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
	// FIT SDK — Stroke Count per stroke type
	StrokeCount []int16 `json:"-"`
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

// EventRecord holds extracted event data from FIT event messages.
// Maps to activity_events table (Spec 4.1).
type EventRecord struct {
	Timestamp                     time.Time  `json:"timestamp"`
	Event                         int16      `json:"event"`
	EventType                     int16      `json:"event_type"`
	EventGroup                    *int16     `json:"event_group,omitempty"`
	Data                          *int64     `json:"data,omitempty"`
	FrontGearNum                  *int16     `json:"front_gear_num,omitempty"`
	FrontGear                     *int16     `json:"front_gear,omitempty"`
	RearGearNum                   *int16     `json:"rear_gear_num,omitempty"`
	RearGear                      *int16     `json:"rear_gear,omitempty"`
	RadarThreatLevelMax           *int16     `json:"radar_threat_level_max,omitempty"`
	RadarThreatCount              *int16     `json:"radar_threat_count,omitempty"`
	RadarThreatAvgApproachSpeedMs *float64   `json:"radar_threat_avg_approach_speed_ms,omitempty"`
	RadarThreatMaxApproachSpeedMs *float64   `json:"radar_threat_max_approach_speed_ms,omitempty"`
	Score                         *int32     `json:"score,omitempty"`
	OpponentScore                 *int32     `json:"opponent_score,omitempty"`
	DeviceIndex                   *int16     `json:"device_index,omitempty"`
	ActivityType                  *int16     `json:"activity_type,omitempty"`
	StartTimestamp                *time.Time `json:"start_timestamp,omitempty"`
}

// SetRecord holds extracted set data from FIT set messages.
// Maps to activity_sets table (Spec 4.2).
type SetRecord struct {
	MessageIndex       int16      `json:"message_index"`
	Timestamp          *time.Time `json:"timestamp,omitempty"`
	StartTime          *time.Time `json:"start_time,omitempty"`
	DurationS          *float64   `json:"duration_s,omitempty"`
	SetType            *int16     `json:"set_type,omitempty"`
	Repetitions        *int32     `json:"repetitions,omitempty"`
	WeightKg           *float64   `json:"weight_kg,omitempty"`
	WeightDisplayUnit  *int16     `json:"weight_display_unit,omitempty"`
	ExerciseCategories []int16    `json:"exercise_categories,omitempty"`
	ExerciseSubtypes   []int16    `json:"exercise_subtypes,omitempty"`
	WktStepIndex       *int16     `json:"wkt_step_index,omitempty"`
}

// JumpRecord holds extracted jump data from FIT jump messages.
// Maps to activity_jumps table (Spec 4.3).
type JumpRecord struct {
	Timestamp time.Time `json:"timestamp"`
	DistanceM *float64  `json:"distance_m,omitempty"`
	HeightM   *float64  `json:"height_m,omitempty"`
	HangTimeS *float64  `json:"hang_time_s,omitempty"`
	Score     *float64  `json:"score,omitempty"`
	Rotations *int16    `json:"rotations,omitempty"`
	SpeedMs   *float64  `json:"speed_ms,omitempty"`
	Lat       *float64  `json:"lat,omitempty"`
	Lng       *float64  `json:"lng,omitempty"`
}

// HrDataRecord holds extracted high-resolution HR data from FIT hr messages.
// Maps to activity_hr_data table (Spec 4.4).
type HrDataRecord struct {
	Timestamp           time.Time `json:"timestamp"`
	FractionalTimestamp *float64  `json:"fractional_timestamp,omitempty"`
	FilteredBpm         []int16   `json:"filtered_bpm,omitempty"`
	EventTimestamp      []float64 `json:"event_timestamp,omitempty"`
}

// SegmentLapRecord holds extracted segment lap data from FIT segment_lap messages.
// Maps to activity_segment_laps table (Spec 4.6, ~90 fields).
type SegmentLapRecord struct {
	ID           string // client-side UUID, generated before INSERT
	MessageIndex int16
	Timestamp    *time.Time `json:"-"`
	StartTime    *time.Time `json:"-"`
	Name         *string    `json:"-"`
	UUID         *string    `json:"-"`
	Status       *int16     `json:"-"`
	Manufacturer *int16     `json:"-"`
	Sport        *int16     `json:"-"`
	SubSport     *int16     `json:"-"`
	Event        *int16     `json:"-"`
	EventType    *int16     `json:"-"`
	EventGroup   *int16     `json:"-"`
	SportEvent   *int16     `json:"-"`
	// GPS
	StartLat *float64 `json:"-"`
	StartLng *float64 `json:"-"`
	EndLat   *float64 `json:"-"`
	EndLng   *float64 `json:"-"`
	NecLat   *float64 `json:"-"`
	NecLng   *float64 `json:"-"`
	SwcLat   *float64 `json:"-"`
	SwcLng   *float64 `json:"-"`
	// Time
	TotalElapsedTimeS *float64 `json:"-"`
	TotalTimerTimeS   *float64 `json:"-"`
	TotalMovingTimeS  *float64 `json:"-"`
	ActiveTimeS       *float64 `json:"-"`
	// Distance / Elevation
	TotalDistanceM          *float64 `json:"-"`
	TotalAscentM            *int32   `json:"-"`
	TotalDescentM           *int32   `json:"-"`
	TotalCycles             *int32   `json:"-"`
	TotalWorkJ              *int32   `json:"-"`
	AvgAltitudeM            *float64 `json:"-"`
	MaxAltitudeM            *float64 `json:"-"`
	MinAltitudeM            *float64 `json:"-"`
	TotalFractionalAscentM  *float64 `json:"-"`
	TotalFractionalDescentM *float64 `json:"-"`
	// Speed
	AvgSpeedMs *float64 `json:"-"`
	MaxSpeedMs *float64 `json:"-"`
	// Heart Rate
	AvgHeartRate *int16 `json:"-"`
	MaxHeartRate *int16 `json:"-"`
	MinHeartRate *int16 `json:"-"`
	// Cadence
	AvgCadence            *int16   `json:"-"`
	MaxCadence            *int16   `json:"-"`
	AvgFractionalCadence  *float64 `json:"-"`
	MaxFractionalCadence  *float64 `json:"-"`
	TotalFractionalCycles *float64 `json:"-"`
	// Power
	AvgPowerW        *int16   `json:"-"`
	MaxPowerW        *int16   `json:"-"`
	NormalizedPowerW *int16   `json:"-"`
	LeftRightBalance *float64 `json:"-"`
	// Grade
	AvgGradePct    *float64 `json:"-"`
	AvgPosGradePct *float64 `json:"-"`
	AvgNegGradePct *float64 `json:"-"`
	MaxPosGradePct *float64 `json:"-"`
	MaxNegGradePct *float64 `json:"-"`
	// Vertical Speed
	AvgPosVerticalSpeedMs *float64 `json:"-"`
	AvgNegVerticalSpeedMs *float64 `json:"-"`
	MaxPosVerticalSpeedMs *float64 `json:"-"`
	MaxNegVerticalSpeedMs *float64 `json:"-"`
	// Temperature
	AvgTemperatureC *int16 `json:"-"`
	MaxTemperatureC *int16 `json:"-"`
	GpsAccuracyM    *int16 `json:"-"`
	// Calories
	TotalCalories    *int32 `json:"-"`
	TotalFatCalories *int32 `json:"-"`
	// Pedal Analytics
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
	// Power/Cadence by Position
	AvgPowerPositionW  []int32 `json:"-"`
	MaxPowerPositionW  []int32 `json:"-"`
	AvgCadencePosition []int16 `json:"-"`
	MaxCadencePosition []int16 `json:"-"`
	// Standing / Sitting
	TimeStandingS *float64 `json:"-"`
	StandCount    *int32   `json:"-"`
	// Gear Shifts
	FrontGearShiftCount *int32 `json:"-"`
	RearGearShiftCount  *int32 `json:"-"`
	// Grit / Flow
	TotalGrit *float64 `json:"-"`
	TotalFlow *float64 `json:"-"`
	AvgGrit   *float64 `json:"-"`
	AvgFlow   *float64 `json:"-"`
	// Misc
	RepetitionNum *int32 `json:"-"`
	WktStepIndex  *int16 `json:"-"`
}

// WorkoutRecord holds structured workout data from FIT workout messages.
// Maps to workouts table (Spec 4.5).
type WorkoutRecord struct {
	ID             string   `json:"id,omitempty"`
	Name           *string  `json:"name,omitempty"`
	Description    *string  `json:"description,omitempty"`
	Sport          *int16   `json:"sport,omitempty"`
	SubSport       *int16   `json:"sub_sport,omitempty"`
	Capabilities   *int64   `json:"capabilities,omitempty"`
	NumValidSteps  *int16   `json:"num_valid_steps,omitempty"`
	PoolLengthM    *float64 `json:"pool_length_m,omitempty"`
	PoolLengthUnit *string  `json:"pool_length_unit,omitempty"`
	SourceHash     string   `json:"source_hash"`
}

// WorkoutStepRecord holds structured workout step data from FIT workout_step messages.
// Maps to workout_steps table (Spec 4.5).
type WorkoutStepRecord struct {
	MessageIndex              int16    `json:"message_index"`
	StepName                  *string  `json:"step_name,omitempty"`
	Notes                     *string  `json:"notes,omitempty"`
	DurationType              *int16   `json:"duration_type,omitempty"`
	DurationValue             *int32   `json:"duration_value,omitempty"`
	TargetType                *int16   `json:"target_type,omitempty"`
	TargetValue               *int32   `json:"target_value,omitempty"`
	CustomTargetLow           *int32   `json:"custom_target_low,omitempty"`
	CustomTargetHigh          *int32   `json:"custom_target_high,omitempty"`
	Intensity                 *int16   `json:"intensity,omitempty"`
	ExerciseCategory          *int16   `json:"exercise_category,omitempty"`
	ExerciseName              *int16   `json:"exercise_name,omitempty"`
	ExerciseWeightKg          *float64 `json:"exercise_weight_kg,omitempty"`
	WeightDisplayUnit         *int16   `json:"weight_display_unit,omitempty"`
	Equipment                 *int16   `json:"equipment,omitempty"`
	SecondaryTargetType       *int16   `json:"secondary_target_type,omitempty"`
	SecondaryTargetValue      *int32   `json:"secondary_target_value,omitempty"`
	SecondaryCustomTargetLow  *int32   `json:"secondary_custom_target_low,omitempty"`
	SecondaryCustomTargetHigh *int32   `json:"secondary_custom_target_high,omitempty"`
}

// DeveloperFieldDef holds Connect IQ developer field definitions.
// Maps to developer_field_definitions table (Spec 4.7).
type DeveloperFieldDef struct {
	ApplicationID         []byte  `json:"application_id,omitempty"`
	DeveloperDataIndex    int16   `json:"developer_data_index"`
	FieldDefinitionNumber int16   `json:"field_definition_number"`
	FieldName             string  `json:"field_name"`
	Units                 *string `json:"units,omitempty"`
	NativeMesgNum         *int16  `json:"native_mesg_num,omitempty"`
	NativeFieldNum        *int16  `json:"native_field_num,omitempty"`
	FitBaseType           *int16  `json:"fit_base_type,omitempty"`
	Scale                 *int16  `json:"scale,omitempty"`
	Offset                *int16  `json:"offset,omitempty"`
}
