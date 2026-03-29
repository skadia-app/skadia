package model

import "time"

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
	GradePct           *float64 `json:"grade_pct,omitempty"`
	VerticalSpeedMs    *float64 `json:"vertical_speed_ms,omitempty"`
	GpsAccuracyM       *int16   `json:"gps_accuracy_m,omitempty"`
	Calories           *int32   `json:"calories,omitempty"`
	Resistance         *int16   `json:"resistance,omitempty"`
	TotalCycles        *int32   `json:"total_cycles,omitempty"`
	AccumulatedPowerW  *int32   `json:"accumulated_power_w,omitempty"`
	AbsolutePressurePa *int32   `json:"absolute_pressure_pa,omitempty"`
	AscentRateMs       *float64 `json:"ascent_rate_ms,omitempty"`
	TimeFromCourseS    *float64 `json:"time_from_course_s,omitempty"`
	ActivityType       *int16   `json:"activity_type,omitempty"`
	DeviceIndex        *int16   `json:"device_index,omitempty"`
	// FIT SDK — Running Dynamics (extended)
	StepLengthMm         *float64 `json:"step_length_mm,omitempty"`
	VerticalRatioPct     *float64 `json:"vertical_ratio_pct,omitempty"`
	StanceTimeBalancePct *float64 `json:"stance_time_balance_pct,omitempty"`
	CycleLengthM         *float64 `json:"cycle_length_m,omitempty"`
	FractionalCadence    *float64 `json:"fractional_cadence,omitempty"`
	// FIT SDK — Pedal Analytics
	LeftTorqueEffectivenessPct  *float64 `json:"left_torque_effectiveness_pct,omitempty"`
	RightTorqueEffectivenessPct *float64 `json:"right_torque_effectiveness_pct,omitempty"`
	LeftPedalSmoothnessPct      *float64 `json:"left_pedal_smoothness_pct,omitempty"`
	RightPedalSmoothnessPct     *float64 `json:"right_pedal_smoothness_pct,omitempty"`
	CombinedPedalSmoothnessPct  *float64 `json:"combined_pedal_smoothness_pct,omitempty"`
	LeftPcoMm                   *int16   `json:"left_pco_mm,omitempty"`
	RightPcoMm                  *int16   `json:"right_pco_mm,omitempty"`
	// FIT SDK — Power Phase
	LeftPowerPhaseStartDeg      *float64 `json:"left_power_phase_start_deg,omitempty"`
	LeftPowerPhaseEndDeg        *float64 `json:"left_power_phase_end_deg,omitempty"`
	LeftPowerPhasePeakStartDeg  *float64 `json:"left_power_phase_peak_start_deg,omitempty"`
	LeftPowerPhasePeakEndDeg    *float64 `json:"left_power_phase_peak_end_deg,omitempty"`
	RightPowerPhaseStartDeg     *float64 `json:"right_power_phase_start_deg,omitempty"`
	RightPowerPhaseEndDeg       *float64 `json:"right_power_phase_end_deg,omitempty"`
	RightPowerPhasePeakStartDeg *float64 `json:"right_power_phase_peak_start_deg,omitempty"`
	RightPowerPhasePeakEndDeg   *float64 `json:"right_power_phase_peak_end_deg,omitempty"`
	// FIT SDK — Hemoglobin / Muscle Oxygen
	TotalHemoglobinConc       *float64 `json:"total_hemoglobin_conc,omitempty"`
	TotalHemoglobinConcMin    *float64 `json:"total_hemoglobin_conc_min,omitempty"`
	TotalHemoglobinConcMax    *float64 `json:"total_hemoglobin_conc_max,omitempty"`
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
	CoreTemperatureC        *float64 `json:"core_temperature_c,omitempty"`
	EnhancedRespirationRate *float64 `json:"enhanced_respiration_rate,omitempty"`
	CurrentStress           *float64 `json:"current_stress,omitempty"`
	BatterySocPct           *float64 `json:"battery_soc_pct,omitempty"`
	// FIT SDK — MTB Metrics
	Grit *float64 `json:"grit,omitempty"`
	Flow *float64 `json:"flow,omitempty"`
	// Garmin Stamina (undocumented record fields 137/138, 0-100%)
	PotentialStamina *int16 `json:"potential_stamina,omitempty"`
	Stamina          *int16 `json:"stamina,omitempty"`
	// Garmin GAP (undocumented record field 140, m/s)
	GapSpeedMS *float64 `json:"gap_speed_ms,omitempty"`
	// FIT SDK — Racket / Ball Sports
	StrokeType  *int16   `json:"stroke_type,omitempty"`
	Zone        *int16   `json:"zone,omitempty"`
	BallSpeedMs *float64 `json:"ball_speed_ms,omitempty"`
	// FIT SDK — Diving
	DepthM            *float64 `json:"depth_m,omitempty"`
	NextStopDepthM    *float64 `json:"next_stop_depth_m,omitempty"`
	NextStopTimeS     *int32   `json:"next_stop_time_s,omitempty"`
	TimeToSurfaceS    *int32   `json:"time_to_surface_s,omitempty"`
	NdlTimeS          *int32   `json:"ndl_time_s,omitempty"`
	CnsLoadPct        *int16   `json:"cns_load_pct,omitempty"`
	N2LoadPct         *int32   `json:"n2_load_pct,omitempty"`
	AirTimeRemainingS *int32   `json:"air_time_remaining_s,omitempty"`
	PressureSac       *float64 `json:"pressure_sac,omitempty"`
	VolumeSac         *float64 `json:"volume_sac,omitempty"`
	Rmv               *float64 `json:"rmv,omitempty"`
	Po2               *float64 `json:"po2,omitempty"`
	// FIT SDK — GPS Metadata
	Heading *float32 `json:"heading,omitempty"`
	// FIT SDK — Developer Fields
	DeveloperFields map[string]any `json:"developer_fields,omitempty"`
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
	GradePct           *float64 `json:"grade_pct,omitempty"`
	VerticalSpeedMs    *float64 `json:"vertical_speed_ms,omitempty"`
	GpsAccuracyM       *int16   `json:"gps_accuracy_m,omitempty"`
	Calories           *int32   `json:"calories,omitempty"`
	Resistance         *int16   `json:"resistance,omitempty"`
	TotalCycles        *int32   `json:"total_cycles,omitempty"`
	AccumulatedPowerW  *int32   `json:"accumulated_power_w,omitempty"`
	AbsolutePressurePa *int32   `json:"absolute_pressure_pa,omitempty"`
	AscentRateMs       *float64 `json:"ascent_rate_ms,omitempty"`
	TimeFromCourseS    *float64 `json:"time_from_course_s,omitempty"`
	ActivityType       *int16   `json:"activity_type,omitempty"`
	DeviceIndex        *int16   `json:"device_index,omitempty"`
	// FIT SDK — Running Dynamics (extended)
	StepLengthMm         *float64 `json:"step_length_mm,omitempty"`
	VerticalRatioPct     *float64 `json:"vertical_ratio_pct,omitempty"`
	StanceTimeBalancePct *float64 `json:"stance_time_balance_pct,omitempty"`
	CycleLengthM         *float64 `json:"cycle_length_m,omitempty"`
	FractionalCadence    *float64 `json:"fractional_cadence,omitempty"`
	// FIT SDK — Pedal Analytics
	LeftTorqueEffectivenessPct  *float64 `json:"left_torque_effectiveness_pct,omitempty"`
	RightTorqueEffectivenessPct *float64 `json:"right_torque_effectiveness_pct,omitempty"`
	LeftPedalSmoothnessPct      *float64 `json:"left_pedal_smoothness_pct,omitempty"`
	RightPedalSmoothnessPct     *float64 `json:"right_pedal_smoothness_pct,omitempty"`
	CombinedPedalSmoothnessPct  *float64 `json:"combined_pedal_smoothness_pct,omitempty"`
	LeftPcoMm                   *int16   `json:"left_pco_mm,omitempty"`
	RightPcoMm                  *int16   `json:"right_pco_mm,omitempty"`
	// FIT SDK — Power Phase
	LeftPowerPhaseStartDeg      *float64 `json:"left_power_phase_start_deg,omitempty"`
	LeftPowerPhaseEndDeg        *float64 `json:"left_power_phase_end_deg,omitempty"`
	LeftPowerPhasePeakStartDeg  *float64 `json:"left_power_phase_peak_start_deg,omitempty"`
	LeftPowerPhasePeakEndDeg    *float64 `json:"left_power_phase_peak_end_deg,omitempty"`
	RightPowerPhaseStartDeg     *float64 `json:"right_power_phase_start_deg,omitempty"`
	RightPowerPhaseEndDeg       *float64 `json:"right_power_phase_end_deg,omitempty"`
	RightPowerPhasePeakStartDeg *float64 `json:"right_power_phase_peak_start_deg,omitempty"`
	RightPowerPhasePeakEndDeg   *float64 `json:"right_power_phase_peak_end_deg,omitempty"`
	// FIT SDK — Hemoglobin / Muscle Oxygen
	TotalHemoglobinConc       *float64 `json:"total_hemoglobin_conc,omitempty"`
	TotalHemoglobinConcMin    *float64 `json:"total_hemoglobin_conc_min,omitempty"`
	TotalHemoglobinConcMax    *float64 `json:"total_hemoglobin_conc_max,omitempty"`
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
	CoreTemperatureC        *float64 `json:"core_temperature_c,omitempty"`
	EnhancedRespirationRate *float64 `json:"enhanced_respiration_rate,omitempty"`
	CurrentStress           *float64 `json:"current_stress,omitempty"`
	BatterySocPct           *float64 `json:"battery_soc_pct,omitempty"`
	// FIT SDK — MTB Metrics
	Grit *float64 `json:"grit,omitempty"`
	Flow *float64 `json:"flow,omitempty"`
	// Garmin Stamina (undocumented record fields 137/138, 0-100%)
	PotentialStamina *int16 `json:"potential_stamina,omitempty"`
	Stamina          *int16 `json:"stamina,omitempty"`
	// Garmin GAP (undocumented record field 140, m/s)
	GapSpeedMS *float64 `json:"gap_speed_ms,omitempty"`
	// FIT SDK — Racket / Ball Sports
	StrokeType  *int16   `json:"stroke_type,omitempty"`
	Zone        *int16   `json:"zone,omitempty"`
	BallSpeedMs *float64 `json:"ball_speed_ms,omitempty"`
	// FIT SDK — Diving
	DepthM            *float64 `json:"depth_m,omitempty"`
	NextStopDepthM    *float64 `json:"next_stop_depth_m,omitempty"`
	NextStopTimeS     *int32   `json:"next_stop_time_s,omitempty"`
	TimeToSurfaceS    *int32   `json:"time_to_surface_s,omitempty"`
	NdlTimeS          *int32   `json:"ndl_time_s,omitempty"`
	CnsLoadPct        *int16   `json:"cns_load_pct,omitempty"`
	N2LoadPct         *int32   `json:"n2_load_pct,omitempty"`
	AirTimeRemainingS *int32   `json:"air_time_remaining_s,omitempty"`
	PressureSac       *float64 `json:"pressure_sac,omitempty"`
	VolumeSac         *float64 `json:"volume_sac,omitempty"`
	Rmv               *float64 `json:"rmv,omitempty"`
	Po2               *float64 `json:"po2,omitempty"`
	// FIT SDK — GPS Metadata
	Heading *float32 `json:"heading,omitempty"`
	// FIT SDK — Developer Fields
	DeveloperFields map[string]any `json:"developer_fields,omitempty"`
}
