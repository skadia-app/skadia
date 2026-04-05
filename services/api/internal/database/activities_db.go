package database

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/geo"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// devAppDeviceIndex is a sentinel device index for developer apps,
// placed after all ANT+/BLE sensors (which use 0-254).
const devAppDeviceIndex = 999

func (db *DB) CheckDuplicate(ctx context.Context, userID, fileHash string) (*model.Activity, error) {
	row := db.Pool.QueryRow(ctx,
		`SELECT id, sport_type, started_at FROM activities WHERE user_id = $1 AND file_hash = $2 LIMIT 1`,
		userID, fileHash,
	)
	var a model.Activity
	err := row.Scan(&a.ID, &a.SportType, &a.StartedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("check duplicate: %w", err)
	}
	return &a, nil
}

func (db *DB) CheckTimeOverlap(ctx context.Context, userID string, startedAt, endedAt time.Time) (*model.Activity, error) {
	row := db.Pool.QueryRow(ctx,
		`SELECT id, sport_type, started_at FROM activities
		 WHERE user_id = $1
		   AND started_at < $3
		   AND ended_at > $2
		 LIMIT 1`,
		userID, startedAt, endedAt,
	)
	var a model.Activity
	err := row.Scan(&a.ID, &a.SportType, &a.StartedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("check time overlap: %w", err)
	}
	return &a, nil
}

// rowQuerier abstracts pgxpool.Pool and pgx.Tx for InsertActivity.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Querier is implemented by both *pgxpool.Pool and pgx.Tx.
// Use it to pass either a pool or a transaction to functions that execute SQL.
type Querier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

// InsertActivityTx inserts an activity using an existing pgx.Tx.
// Use this when the insert must participate in a caller-managed transaction.
func (db *DB) InsertActivityTx(ctx context.Context, tx pgx.Tx, userID string, data *model.ActivityData) (*model.Activity, error) {
	return insertActivityOnQuerier(ctx, tx, userID, data)
}

func (db *DB) InsertActivity(ctx context.Context, userID string, data *model.ActivityData) (*model.Activity, error) {
	return insertActivityOnQuerier(ctx, db.Pool, userID, data)
}

func insertActivityOnQuerier(ctx context.Context, q rowQuerier, userID string, data *model.ActivityData) (*model.Activity, error) {
	var id string
	var createdAt, updatedAt time.Time
	var isElectric, isVirtual bool

	err := q.QueryRow(ctx, `
		INSERT INTO activities (
			user_id, file_hash, sport_type, sub_type,
			parsed_sport_type, parsed_sub_type,
			fit_sport_id, fit_sub_sport_id,
			started_at, ended_at, elapsed_time_s, moving_time_s,
			distance_m, elevation_gain_m, elevation_loss_m,
			max_elevation_m, min_elevation_m,
			avg_speed_ms, max_speed_ms,
			avg_heart_rate, max_heart_rate,
			avg_power_w, max_power_w, normalized_power_w,
			avg_cadence, avg_temperature_c,
			tss, trimp, is_indoor, is_electric, has_gps,
			is_virtual, exclude_from_heatmap, exclude_from_stats,
			start_location, end_location,
			original_filename, original_file_format,
			source, source_device,
			visibility,
			sport_conflict_pending,
			-- FIT SDK new fields below
			workout_id,
			total_cycles, total_work_j, total_fat_calories, metabolic_calories, total_moving_time_s,
			swim_stroke, pool_length_m, pool_length_unit,
			num_lengths, num_active_lengths, avg_stroke_count, avg_stroke_distance_m,
			stroke_count, zone_count,
			avg_fractional_cadence, max_fractional_cadence, total_fractional_cycles,
			avg_vertical_oscillation_mm, avg_stance_time_ms, avg_stance_time_pct,
			avg_stance_time_balance_pct, avg_step_length_mm, avg_vertical_ratio_pct,
			avg_grade_pct, avg_pos_grade_pct, avg_neg_grade_pct, max_pos_grade_pct, max_neg_grade_pct,
			avg_pos_vertical_speed_ms, avg_neg_vertical_speed_ms,
			max_pos_vertical_speed_ms, max_neg_vertical_speed_ms, avg_vam_ms,
			avg_altitude_m, total_fractional_ascent_m, total_fractional_descent_m,
			avg_left_torque_effectiveness_pct, avg_right_torque_effectiveness_pct,
			avg_left_pedal_smoothness_pct, avg_right_pedal_smoothness_pct,
			avg_combined_pedal_smoothness_pct,
			avg_left_pco_mm, avg_right_pco_mm,
			avg_left_power_phase_start_deg, avg_left_power_phase_end_deg,
			avg_left_power_phase_peak_start_deg, avg_left_power_phase_peak_end_deg,
			avg_right_power_phase_start_deg, avg_right_power_phase_end_deg,
			avg_right_power_phase_peak_start_deg, avg_right_power_phase_peak_end_deg,
			avg_power_position_w, max_power_position_w,
			avg_cadence_position, max_cadence_position,
			time_standing_s, stand_count,
			avg_lev_motor_power_w, max_lev_motor_power_w, lev_battery_consumption_pct,
			total_grit, total_flow, avg_grit, avg_flow,
			jump_count,
			max_ball_speed_ms, avg_ball_speed_ms, player_score, opponent_score, opponent_name,
			avg_total_hemoglobin_conc, min_total_hemoglobin_conc, max_total_hemoglobin_conc,
			avg_saturated_hemoglobin_pct, min_saturated_hemoglobin_pct, max_saturated_hemoglobin_pct,
			avg_depth_m, max_depth_m, surface_interval_s, dive_number,
			start_cns_pct, end_cns_pct, start_n2_pct, end_n2_pct, o2_toxicity_otu,
			max_respiration_rate, min_respiration_rate, sdrr_hrv_ms,
			min_core_temperature_c, max_core_temperature_c,
			avg_lap_time_s, best_lap_index, num_laps, first_lap_index,
			intensity_factor, left_right_balance, sport_profile_name, sport_index,
			nec_lat, nec_lng, swc_lat, swc_lng,
			is_manual_entry, workout_feel, workout_rpe,
			timezone,
			max_cadence, min_temperature_c, max_temperature_c,
			num_sessions,
			-- VO2max
			vo2max_ml_kg_min, vo2max_calibrated, vo2max_hr_source, vo2max_speed_source,
			-- GPS quality
			gps_accuracy_m
		) VALUES (
			-- Core ($1-$4, parsed reuses $3/$4)
			$1, $2, $3, $4,
			$3, $4,
			-- FIT Sport IDs ($5-$6)
			$5, $6,
			-- Time ($7-$10)
			$7, $8, $9, $10,
			-- Distance / Elevation ($11-$15)
			$11, $12, $13,
			$14, $15,
			-- Speed ($16-$17)
			$16, $17,
			-- Heart Rate ($18-$19)
			$18, $19,
			-- Power ($20-$22)
			$20, $21, $22,
			-- Cadence / Temperature ($23-$24)
			$23, $24,
			-- TSS / TRIMP / Flags ($25-$29)
			$25, $26, $27, $28, $29,
			-- Virtual / Exclude ($30-$32)
			$30, $31, $32,
			-- Start/End Location ($33-$36)
			CASE WHEN $33::float8 != 0 THEN ST_SetSRID(ST_MakePoint($34::float8, $33::float8), 4326) ELSE NULL END,
			CASE WHEN $35::float8 != 0 THEN ST_SetSRID(ST_MakePoint($36::float8, $35::float8), 4326) ELSE NULL END,
			-- File / Source ($37-$40)
			$37, $38,
			$39, $40,
			-- Visibility / Conflict ($41-$42)
			$41,
			$42,
			-- FIT SDK: Workout ($43)
			$43,
			-- FIT SDK: Cycles/Work/Calories ($44-$48)
			$44, $45, $46, $47, $48,
			-- FIT SDK: Swimming ($49-$57)
			$49, $50, $51,
			$52, $53, $54, $55,
			$56, $57,
			-- FIT SDK: Fractional Cadence ($58-$60)
			$58, $59, $60,
			-- FIT SDK: Running Dynamics ($61-$66)
			$61, $62, $63,
			$64, $65, $66,
			-- FIT SDK: Grade ($67-$71)
			$67, $68, $69, $70, $71,
			-- FIT SDK: Vertical Speed ($72-$76)
			$72, $73,
			$74, $75, $76,
			-- FIT SDK: Altitude / Fractional ($77-$79)
			$77, $78, $79,
			-- FIT SDK: Pedal Torque/Smoothness ($80-$84)
			$80, $81,
			$82, $83,
			$84,
			-- FIT SDK: PCO ($85-$86)
			$85, $86,
			-- FIT SDK: Power Phase Left ($87-$90)
			$87, $88,
			$89, $90,
			-- FIT SDK: Power Phase Right ($91-$94)
			$91, $92,
			$93, $94,
			-- FIT SDK: Power/Cadence Position ($95-$98)
			$95, $96,
			$97, $98,
			-- FIT SDK: Standing ($99-$100)
			$99, $100,
			-- FIT SDK: LEV/E-Bike ($101-$103)
			$101, $102, $103,
			-- FIT SDK: Grit/Flow ($104-$108)
			$104, $105, $106, $107,
			$108,
			-- FIT SDK: Ball Sports ($109-$113)
			$109, $110, $111, $112, $113,
			-- FIT SDK: Hemoglobin ($114-$119)
			$114, $115, $116,
			$117, $118, $119,
			-- FIT SDK: Diving ($120-$128)
			$120, $121, $122, $123,
			$124, $125, $126, $127, $128,
			-- FIT SDK: Respiration/HRV/Core ($129-$133)
			$129, $130, $131,
			$132, $133,
			-- FIT SDK: Lap Stats ($134-$137)
			$134, $135, $136, $137,
			-- FIT SDK: Intensity/Balance/Profile ($138-$141)
			$138, $139, $140, $141,
			-- Bounding Box ($142-$145)
			$142, $143, $144, $145,
			-- Manual / Workout ($146-$148)
			$146, $147, $148,
			-- Timezone ($149)
			$149,
			-- Max cadence / Min+Max temperature ($150-$152)
			$150, $151, $152,
			-- NumSessions ($153)
			$153,
			-- VO2max ($154-$157)
			$154, $155, $156, $157,
			-- GPS quality ($158)
			$158
		)
		RETURNING id, is_electric, is_virtual, created_at, updated_at`,
		userID, nilIfEmpty(data.FileHash), data.SportType, nilIfEmpty(data.SubType), // $1-$4
		nilIfZeroI16(data.FitSportID), nilIfZeroI16(data.FitSubSportID), // $5-$6
		data.StartedAt, nilIfZeroTime(data.EndedAt), nilIfZeroInt(data.ElapsedTimeS), nilIfZeroInt(data.MovingTimeS), // $7-$10
		nilIfZeroF64(data.DistanceM), nilIfZeroF64(data.ElevationGainM), nilIfZeroF64(data.ElevationLossM), // $11-$13
		nilIfZeroF64(data.MaxElevationM), nilIfZeroF64(data.MinElevationM), // $14-$15
		nilIfZeroF64(data.AvgSpeedMS), nilIfZeroF64(data.MaxSpeedMS), // $16-$17
		nilIfZeroF64(data.AvgHeartRate), nilIfZeroInt(data.MaxHeartRate), // $18-$19
		nilIfZeroF64(data.AvgPowerW), nilIfZeroInt(data.MaxPowerW), nilIfZeroF64(data.NormalizedPowerW), // $20-$22
		nilIfZeroF64(data.AvgCadence), nilIfZeroF64(data.AvgTemperatureC), // $23-$24
		nilIfZeroF64(data.TSS), nilIfZeroF64(data.TRIMP), data.IsIndoor, data.IsElectric, data.HasGPS, // $25-$29
		data.IsVirtual, data.IsVirtual, false, // $30-$32 (is_virtual, exclude_from_heatmap=is_virtual, exclude_from_stats=false)
		data.StartLat, data.StartLng, // $33-$34
		data.EndLat, data.EndLng, // $35-$36
		nilIfEmpty(data.OriginalName), nilIfEmpty(data.FileFormat), // $37-$38
		nilIfEmpty(data.Source), nilIfEmpty(data.SourceDevice), // $39-$40
		visibilityOrDefault(data.Visibility), // $41
		data.SportConflictPending,            // $42
		// FIT SDK new fields
		data.WorkoutID,                                                                                          // $43
		data.TotalCycles, data.TotalWorkJ, data.TotalFatCalories, data.MetabolicCalories, data.TotalMovingTimeS, // $44-$48
		data.SwimStroke, data.PoolLengthM, data.PoolLengthUnit, // $49-$51
		data.NumLengths, data.NumActiveLengths, data.AvgStrokeCount, data.AvgStrokeDistanceM, // $52-$55
		data.StrokeCount, data.ZoneCount, // $56-$57
		data.AvgFractionalCadence, data.MaxFractionalCadence, data.TotalFractionalCycles, // $58-$60
		data.AvgVerticalOscillationMm, data.AvgStanceTimeMs, data.AvgStanceTimePct, // $61-$63
		data.AvgStanceTimeBalancePct, data.AvgStepLengthMm, data.AvgVerticalRatioPct, // $64-$66
		data.AvgGradePct, data.AvgPosGradePct, data.AvgNegGradePct, data.MaxPosGradePct, data.MaxNegGradePct, // $67-$71
		data.AvgPosVerticalSpeedMs, data.AvgNegVerticalSpeedMs, // $72-$73
		data.MaxPosVerticalSpeedMs, data.MaxNegVerticalSpeedMs, data.AvgVamMs, // $74-$76
		data.AvgAltitudeM, data.TotalFractionalAscentM, data.TotalFractionalDescentM, // $77-$79
		data.AvgLeftTorqueEffectivenessPct, data.AvgRightTorqueEffectivenessPct, // $80-$81
		data.AvgLeftPedalSmoothnessPct, data.AvgRightPedalSmoothnessPct, // $82-$83
		data.AvgCombinedPedalSmoothnessPct,    // $84
		data.AvgLeftPcoMm, data.AvgRightPcoMm, // $85-$86
		data.AvgLeftPowerPhaseStartDeg, data.AvgLeftPowerPhaseEndDeg, // $87-$88
		data.AvgLeftPowerPhasePeakStartDeg, data.AvgLeftPowerPhasePeakEndDeg, // $89-$90
		data.AvgRightPowerPhaseStartDeg, data.AvgRightPowerPhaseEndDeg, // $91-$92
		data.AvgRightPowerPhasePeakStartDeg, data.AvgRightPowerPhasePeakEndDeg, // $93-$94
		data.AvgPowerPositionW, data.MaxPowerPositionW, // $95-$96
		data.AvgCadencePosition, data.MaxCadencePosition, // $97-$98
		data.TimeStandingS, data.StandCount, // $99-$100
		data.AvgLevMotorPowerW, data.MaxLevMotorPowerW, data.LevBatteryConsumptionPct, // $101-$103
		data.TotalGrit, data.TotalFlow, data.AvgGrit, data.AvgFlow, // $104-$107
		data.JumpCount,                                                                                    // $108
		data.MaxBallSpeedMs, data.AvgBallSpeedMs, data.PlayerScore, data.OpponentScore, data.OpponentName, // $109-$113
		data.AvgTotalHemoglobinConc, data.MinTotalHemoglobinConc, data.MaxTotalHemoglobinConc, // $114-$116
		data.AvgSaturatedHemoglobinPct, data.MinSaturatedHemoglobinPct, data.MaxSaturatedHemoglobinPct, // $117-$119
		data.AvgDepthM, data.MaxDepthM, data.SurfaceIntervalS, data.DiveNumber, // $120-$123
		data.StartCnsPct, data.EndCnsPct, data.StartN2Pct, data.EndN2Pct, data.O2ToxicityOtu, // $124-$128
		data.MaxRespirationRate, data.MinRespirationRate, data.SdrrHrvMs, // $129-$131
		data.MinCoreTemperatureC, data.MaxCoreTemperatureC, // $132-$133
		data.AvgLapTimeS, data.BestLapIndex, data.NumLaps, data.FirstLapIndex, // $134-$137
		data.IntensityFactor, data.LeftRightBalance, data.SportProfileName, data.SportIndex, // $138-$141
		data.NecLat, data.NecLng, data.SwcLat, data.SwcLng, // $142-$145
		data.IsManualEntry, nilIfZeroInt(data.WorkoutFeel), nilIfZeroInt(data.WorkoutRpe), // $146-$148
		nilIfEmpty(data.Timezone),                                                          // $149
		nilIfZeroInt(data.MaxCadence), data.MinTemperatureC, data.MaxTemperatureC, // $150-$152
		nilIfZeroInt(data.NumSessions),                                           // $153
		data.Vo2MaxMlKgMin, data.Vo2MaxCalibrated, data.Vo2MaxHrSource, data.Vo2MaxSpeedSource, // $154-$157
		nilIfZeroInt(data.GpsAccuracyM), // $158
	).Scan(&id, &isElectric, &isVirtual, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert activity: %w", err)
	}

	act := &model.Activity{
		ID:                   id,
		UserID:               userID,
		SportType:            data.SportType,
		SubType:              ptrString(data.SubType),
		Source:               ptrString(data.Source),
		SourceDevice:         ptrString(data.SourceDevice),
		StartedAt:            data.StartedAt,
		IsElectric:              isElectric,
		IsVirtual:            isVirtual,
		ExcludeFromHeatmap:   isVirtual,
		ExcludeFromStats:     false,
		IsIndoor:             data.IsIndoor,
		IsManualEntry:        data.IsManualEntry,
		HasGPS:               data.HasGPS,
		NeedsClassification:  data.SportType == "unknown",
		OriginalFilename:     ptrString(data.OriginalName),
		Visibility:           visibilityOrDefault(data.Visibility),
		EnrichmentStatus:     "pending",
		CreatedAt:            createdAt,
		UpdatedAt:            updatedAt,
		SportConflictPending: data.SportConflictPending,
	}
	if data.NumSessions > 0 {
		ns := data.NumSessions
		act.NumSessions = &ns
	}
	return act, nil
}

func (db *DB) InsertStreams(ctx context.Context, activityID string, streams []model.StreamPoint) error {
	if len(streams) == 0 {
		return nil
	}

	columns := []string{
		// Existing 15 columns (do not reorder)
		"time", "activity_id",
		"lat", "lng", "altitude_m", "speed_ms", "distance_m",
		"heart_rate", "power_w", "cadence", "temperature_c",
		"ground_contact_time_ms", "vertical_oscillation_mm", "stance_time_percent", "left_right_balance",
		// FIT SDK — Terrain & Movement (14)
		"respiration_rate", "stress_level",
		"grade_pct", "vertical_speed_ms", "gps_accuracy_m",
		"calories", "resistance", "total_cycles", "accumulated_power_w",
		"absolute_pressure_pa", "ascent_rate_ms", "time_from_course_s",
		"activity_type", "device_index",
		// FIT SDK — Running Dynamics extended (5)
		"step_length_mm", "vertical_ratio_pct", "stance_time_balance_pct",
		"cycle_length_m", "fractional_cadence",
		// FIT SDK — Pedal Analytics (7)
		"left_torque_effectiveness_pct", "right_torque_effectiveness_pct",
		"left_pedal_smoothness_pct", "right_pedal_smoothness_pct",
		"combined_pedal_smoothness_pct",
		"left_pco_mm", "right_pco_mm",
		// FIT SDK — Power Phase (8)
		"left_power_phase_start_deg", "left_power_phase_end_deg",
		"left_power_phase_peak_start_deg", "left_power_phase_peak_end_deg",
		"right_power_phase_start_deg", "right_power_phase_end_deg",
		"right_power_phase_peak_start_deg", "right_power_phase_peak_end_deg",
		// FIT SDK — Hemoglobin / Muscle Oxygen (6)
		"total_hemoglobin_conc", "total_hemoglobin_conc_min", "total_hemoglobin_conc_max",
		"saturated_hemoglobin_pct", "saturated_hemoglobin_pct_min", "saturated_hemoglobin_pct_max",
		// FIT SDK — E-Bike (5)
		"motor_power_w", "ebike_travel_range_km", "ebike_battery_level_pct",
		"ebike_assist_mode", "ebike_assist_level_pct",
		// FIT SDK — Health / Stress (4)
		"core_temperature_c", "enhanced_respiration_rate", "current_stress", "battery_soc_pct",
		// FIT SDK — MTB Metrics (2)
		"grit", "flow",
		// Garmin Stamina + GAP (3)
		"potential_stamina", "stamina", "gap_speed_ms",
		// FIT SDK — Racket / Ball Sports (3)
		"stroke_type", "zone", "ball_speed_ms",
		// FIT SDK — Diving (12)
		"depth_m", "next_stop_depth_m", "next_stop_time_s", "time_to_surface_s",
		"ndl_time_s", "cns_load_pct", "n2_load_pct", "air_time_remaining_s",
		"pressure_sac", "volume_sac", "rmv", "po2",
		// FIT SDK — GPS Metadata (1)
		"heading",
		// FIT SDK — Developer Fields (1)
		"developer_fields",
	}

	rows := make([][]any, len(streams))
	for i, s := range streams {
		rows[i] = []any{
			// Existing 15 columns (do not reorder)
			s.Time, activityID,
			s.Lat, s.Lng, s.AltitudeM, s.SpeedMS, s.DistanceM,
			s.HeartRate, s.PowerW, s.Cadence, s.TemperatureC,
			s.GroundContactTimeMs, s.VerticalOscillationMm, s.StanceTimePercent, s.LeftRightBalance,
			// FIT SDK — Terrain & Movement (14)
			s.RespirationRate, s.StressLevel,
			s.GradePct, s.VerticalSpeedMs, s.GpsAccuracyM,
			s.Calories, s.Resistance, s.TotalCycles, s.AccumulatedPowerW,
			s.AbsolutePressurePa, s.AscentRateMs, s.TimeFromCourseS,
			s.ActivityType, s.DeviceIndex,
			// FIT SDK — Running Dynamics extended (5)
			s.StepLengthMm, s.VerticalRatioPct, s.StanceTimeBalancePct,
			s.CycleLengthM, s.FractionalCadence,
			// FIT SDK — Pedal Analytics (7)
			s.LeftTorqueEffectivenessPct, s.RightTorqueEffectivenessPct,
			s.LeftPedalSmoothnessPct, s.RightPedalSmoothnessPct,
			s.CombinedPedalSmoothnessPct,
			s.LeftPcoMm, s.RightPcoMm,
			// FIT SDK — Power Phase (8)
			s.LeftPowerPhaseStartDeg, s.LeftPowerPhaseEndDeg,
			s.LeftPowerPhasePeakStartDeg, s.LeftPowerPhasePeakEndDeg,
			s.RightPowerPhaseStartDeg, s.RightPowerPhaseEndDeg,
			s.RightPowerPhasePeakStartDeg, s.RightPowerPhasePeakEndDeg,
			// FIT SDK — Hemoglobin / Muscle Oxygen (6)
			s.TotalHemoglobinConc, s.TotalHemoglobinConcMin, s.TotalHemoglobinConcMax,
			s.SaturatedHemoglobinPct, s.SaturatedHemoglobinPctMin, s.SaturatedHemoglobinPctMax,
			// FIT SDK — E-Bike (5)
			s.MotorPowerW, s.EbikeTravelRangeKm, s.EbikeBatteryLevelPct,
			s.EbikeAssistMode, s.EbikeAssistLevelPct,
			// FIT SDK — Health / Stress (4)
			s.CoreTemperatureC, s.EnhancedRespirationRate, s.CurrentStress, s.BatterySocPct,
			// FIT SDK — MTB Metrics (2)
			s.Grit, s.Flow,
			// Garmin Stamina + GAP (3)
			s.PotentialStamina, s.Stamina, s.GapSpeedMS,
			// FIT SDK — Racket / Ball Sports (3)
			s.StrokeType, s.Zone, s.BallSpeedMs,
			// FIT SDK — Diving (12)
			s.DepthM, s.NextStopDepthM, s.NextStopTimeS, s.TimeToSurfaceS,
			s.NdlTimeS, s.CnsLoadPct, s.N2LoadPct, s.AirTimeRemainingS,
			s.PressureSac, s.VolumeSac, s.Rmv, s.Po2,
			// FIT SDK — GPS Metadata (1)
			s.Heading,
			// FIT SDK — Developer Fields (1)
			s.DeveloperFields,
		}
	}

	_, err := db.Pool.CopyFrom(ctx,
		pgx.Identifier{"activity_streams"},
		columns,
		pgx.CopyFromRows(rows),
	)
	return err
}

// UpdateStreamSpeeds writes smoothed speed_ms values back to activity_streams.
// Uses a single UPDATE with unnest arrays for performance (~36k points in one query).
func (db *DB) UpdateStreamSpeeds(ctx context.Context, activityID string, streams []model.StreamPoint) error {
	times := make([]time.Time, 0, len(streams))
	speeds := make([]float64, 0, len(streams))
	for _, s := range streams {
		if s.SpeedMS == nil {
			continue
		}
		times = append(times, s.Time)
		speeds = append(speeds, *s.SpeedMS)
	}
	if len(times) == 0 {
		return nil
	}
	_, err := db.Pool.Exec(ctx, `
		UPDATE activity_streams AS a
		SET speed_ms = v.speed
		FROM unnest($1::timestamptz[], $2::float8[]) AS v(t, speed)
		WHERE a.activity_id = $3 AND a.time = v.t`,
		times, speeds, activityID,
	)
	if err != nil {
		return fmt.Errorf("update stream speeds: %w", err)
	}
	return nil
}

func (db *DB) InsertTrack(ctx context.Context, activityID string, trackPoints []model.TrackPoint) error {
	if len(trackPoints) < 2 {
		return nil
	}

	ls := geo.LineStringFromLatLng(trackPoints, func(p model.TrackPoint) (float64, float64) {
		return p.Lat, p.Lng
	})
	ewkb, err := geo.MarshalEWKB(ls, 4326)
	if err != nil {
		return fmt.Errorf("marshal track ewkb: %w", err)
	}

	_, err = db.Pool.Exec(ctx, `
		WITH g AS (SELECT ST_GeomFromEWKB($2) AS geom)
		INSERT INTO activity_tracks (activity_id, track_full, track_simplified, bbox, point_count)
		SELECT $1, g.geom, ST_Simplify(g.geom, 0.0001), ST_Envelope(g.geom), $3
		FROM g`,
		activityID, ewkb, len(trackPoints),
	)
	if err != nil {
		return fmt.Errorf("insert track: %w", err)
	}
	return nil
}

func (db *DB) InsertSensors(ctx context.Context, activityID string, sensors []model.SensorRecord) error {
	if len(sensors) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, s := range sensors {
		batch.Queue(`
			INSERT INTO activity_sensors (
				activity_id, device_index, serial_number, ant_device_number, source_type,
				antplus_device_type, all_device_types,
				manufacturer, manufacturer_id, garmin_product, garmin_product_id,
				product_name, software_version, hardware_version,
				battery_status, battery_voltage
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`,
			activityID, s.DeviceIndex, s.SerialNumber, s.AntDeviceNumber, s.SourceType,
			nilIfEmpty(s.AntplusDeviceType), s.AllDeviceTypes,
			nilIfEmpty(s.Manufacturer), nilIfZeroI16(s.ManufacturerID),
			nilIfEmpty(s.GarminProduct), nilIfZeroI32(s.GarminProductID),
			nilIfEmpty(s.ProductName), nilIfZeroF64(s.SoftwareVersion),
			nilIfZeroI16(s.HardwareVersion),
			nilIfEmpty(s.BatteryStatus), nilIfZeroF64(s.BatteryVoltage),
		)
	}

	br := db.Pool.SendBatch(ctx, batch)
	defer br.Close()

	for range sensors {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("insert sensor: %w", err)
		}
	}
	return nil
}

func (db *DB) UpdateActivityFileInfo(ctx context.Context, id, format, filename string) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE activities SET
			original_file_format = $2,
			original_filename = $3
		WHERE id = $1`,
		id, format, filename,
	)
	if err != nil {
		return fmt.Errorf("update file info: %w", err)
	}
	return nil
}

func (db *DB) GetActivity(ctx context.Context, id string) (*model.Activity, error) {
	var a model.Activity
	var hasTrack bool
	var sportConflictMeta []byte

	err := db.Pool.QueryRow(ctx, `
		SELECT
			a.id, a.user_id, a.file_hash, a.file_id, a.title, a.title_auto, a.description,
			a.sport_type, a.sub_type, a.fit_sport_id, a.fit_sub_sport_id, a.is_electric,
			a.is_virtual, a.exclude_from_heatmap, a.exclude_from_stats,
			a.started_at, a.ended_at, a.elapsed_time_s, a.moving_time_s,
			a.distance_m, a.elevation_gain_m, a.elevation_loss_m,
			a.max_elevation_m, a.min_elevation_m,
			a.avg_speed_ms, a.max_speed_ms,
			a.avg_heart_rate, a.max_heart_rate,
			a.avg_power_w, a.max_power_w, a.normalized_power_w,
			a.avg_cadence, a.avg_temperature_c,
			a.min_temperature_c, a.max_temperature_c, a.max_cadence,
			a.tss, a.trimp, a.is_indoor,
			a.source, a.source_device,
			a.tags, a.visibility, a.enrichment_status, a.original_filename, a.original_file_format,
			a.start_city, a.end_city, a.start_state, a.start_country, a.start_country_code,
			a.is_round_trip, a.intensity, a.time_of_day, a.sunrise_at, a.sunset_at, a.gear_id,
			a.created_at, a.updated_at,
			COALESCE(t.point_count, 0) > 0,
			a.location_lat, a.location_lng,
			a.lift_count, a.lift_distance_m, a.lift_ascent_m, a.lift_descent_m, a.lift_duration_s,
			a.exclude_lift_subtraction,
			COALESCE(a.elevation_source, 'device'),
			COALESCE(a.distance_source, 'device'),
			a.parent_id,
			a.fusion_sources,
			a.garmin_activity_id, a.strava_activity_id, a.komoot_tour_id,
		a.sport_conflict_pending,
		iq.metadata AS sport_conflict_meta,
		a.recording_device_id,
		COALESCE(rd.custom_name, rd.product_name) AS recording_device_name,
		ST_Y(a.start_location) AS start_lat,
		ST_X(a.start_location) AS start_lon,
		ST_Y(a.end_location) AS end_lat,
		ST_X(a.end_location) AS end_lon,
		-- FIT SDK extended fields
		a.workout_id,
		a.total_cycles, a.total_work_j, a.total_fat_calories, a.metabolic_calories, a.total_moving_time_s,
		a.swim_stroke, a.pool_length_m, a.pool_length_unit,
		a.num_lengths, a.num_active_lengths, a.avg_stroke_count, a.avg_stroke_distance_m,
		a.stroke_count, a.zone_count,
		a.avg_fractional_cadence, a.max_fractional_cadence, a.total_fractional_cycles,
		a.avg_vertical_oscillation_mm, a.avg_stance_time_ms, a.avg_stance_time_pct,
		a.avg_stance_time_balance_pct, a.avg_step_length_mm, a.avg_vertical_ratio_pct,
		a.avg_grade_pct, a.avg_pos_grade_pct, a.avg_neg_grade_pct, a.max_pos_grade_pct, a.max_neg_grade_pct,
		a.avg_pos_vertical_speed_ms, a.avg_neg_vertical_speed_ms,
		a.max_pos_vertical_speed_ms, a.max_neg_vertical_speed_ms, a.avg_vam_ms,
		a.avg_altitude_m, a.total_fractional_ascent_m, a.total_fractional_descent_m,
		a.avg_left_torque_effectiveness_pct, a.avg_right_torque_effectiveness_pct,
		a.avg_left_pedal_smoothness_pct, a.avg_right_pedal_smoothness_pct,
		a.avg_combined_pedal_smoothness_pct,
		a.avg_left_pco_mm, a.avg_right_pco_mm,
		a.avg_left_power_phase_start_deg, a.avg_left_power_phase_end_deg,
		a.avg_left_power_phase_peak_start_deg, a.avg_left_power_phase_peak_end_deg,
		a.avg_right_power_phase_start_deg, a.avg_right_power_phase_end_deg,
		a.avg_right_power_phase_peak_start_deg, a.avg_right_power_phase_peak_end_deg,
		a.avg_power_position_w, a.max_power_position_w,
		a.avg_cadence_position, a.max_cadence_position,
		a.time_standing_s, a.stand_count,
		a.avg_lev_motor_power_w, a.max_lev_motor_power_w, a.lev_battery_consumption_pct,
		a.total_grit, a.total_flow, a.avg_grit, a.avg_flow,
		a.jump_count,
		a.max_ball_speed_ms, a.avg_ball_speed_ms, a.player_score, a.opponent_score, a.opponent_name,
		a.avg_total_hemoglobin_conc, a.min_total_hemoglobin_conc, a.max_total_hemoglobin_conc,
		a.avg_saturated_hemoglobin_pct, a.min_saturated_hemoglobin_pct, a.max_saturated_hemoglobin_pct,
		a.avg_depth_m, a.max_depth_m, a.surface_interval_s, a.dive_number,
		a.start_cns_pct, a.end_cns_pct, a.start_n2_pct, a.end_n2_pct, a.o2_toxicity_otu,
		a.max_respiration_rate, a.min_respiration_rate, a.sdrr_hrv_ms,
		a.min_core_temperature_c, a.max_core_temperature_c,
		a.avg_lap_time_s, a.best_lap_index, a.num_laps, a.first_lap_index,
		a.intensity_factor, a.left_right_balance, a.sport_profile_name, a.sport_index,
		a.nec_lat, a.nec_lng, a.swc_lat, a.swc_lng,
		COALESCE(a.title_is_manual, false),
		a.is_manual_entry, a.workout_feel, a.workout_rpe,
		a.timezone,
		a.num_sessions,
		a.vo2max_ml_kg_min, a.vo2max_calibrated, a.vo2max_hr_source, a.vo2max_speed_source,
		a.gps_accuracy_m,
		a.is_group_secondary,
		rgm.group_id,
		a.running_form_duty_factor, a.running_form_vertical_ratio, a.running_form_gct_asymmetry_pct,
		a.running_form_fatigue_duty, a.running_form_fatigue_vr, a.running_form_tier
		FROM activities a
		LEFT JOIN activity_tracks t ON t.activity_id = a.id
		LEFT JOIN LATERAL (
			SELECT metadata FROM import_queue iq
			WHERE iq.queue_type = 'sport_conflict'
			  AND iq.status = 'pending'
			  AND (iq.metadata->>'activity_id') = a.id::text
			  AND iq.user_id = a.user_id
			LIMIT 1
		) iq ON true
		LEFT JOIN devices rd ON rd.id = a.recording_device_id
		LEFT JOIN recording_group_members rgm ON rgm.activity_id = a.id
		WHERE a.id = $1`, id,
	).Scan(
		&a.ID, &a.UserID, &a.FileHash, &a.FileID, &a.Title, &a.TitleAuto, &a.Description,
		&a.SportType, &a.SubType, &a.FitSportID, &a.FitSubSportID, &a.IsElectric,
		&a.IsVirtual, &a.ExcludeFromHeatmap, &a.ExcludeFromStats,
		&a.StartedAt, &a.EndedAt, &a.ElapsedTimeS, &a.MovingTimeS,
		&a.DistanceM, &a.ElevationGainM, &a.ElevationLossM,
		&a.MaxElevationM, &a.MinElevationM,
		&a.AvgSpeedMS, &a.MaxSpeedMS,
		&a.AvgHeartRate, &a.MaxHeartRate,
		&a.AvgPowerW, &a.MaxPowerW, &a.NormalizedPowerW,
		&a.AvgCadence, &a.AvgTemperatureC,
		&a.MinTemperatureC, &a.MaxTemperatureC, &a.MaxCadence,
		&a.TSS, &a.TRIMP, &a.IsIndoor,
		&a.Source, &a.SourceDevice,
		&a.Tags, &a.Visibility, &a.EnrichmentStatus, &a.OriginalFilename, &a.OriginalFileFormat,
		&a.StartCity, &a.EndCity, &a.StartState, &a.StartCountry, &a.StartCountryCode,
		&a.IsRoundTrip, &a.Intensity, &a.TimeOfDay, &a.SunriseAt, &a.SunsetAt, &a.GearID,
		&a.CreatedAt, &a.UpdatedAt,
		&hasTrack,
		&a.LocationLat, &a.LocationLng,
		&a.LiftCount, &a.LiftDistanceM, &a.LiftAscentM, &a.LiftDescentM, &a.LiftDurationS,
		&a.ExcludeLiftSubtraction,
		&a.ElevationSource,
		&a.DistanceSource,
		&a.ParentID,
		&a.FusionSources,
		&a.GarminActivityID, &a.StravaActivityID, &a.KomootTourID,
		&a.SportConflictPending,
		&sportConflictMeta,
		&a.RecordingDeviceID,
		&a.RecordingDeviceName,
		&a.StartLat, &a.StartLon, &a.EndLat, &a.EndLon,
		// FIT SDK extended fields
		&a.WorkoutID,
		&a.TotalCycles, &a.TotalWorkJ, &a.TotalFatCalories, &a.MetabolicCalories, &a.TotalMovingTimeS,
		&a.SwimStroke, &a.PoolLengthM, &a.PoolLengthUnit,
		&a.NumLengths, &a.NumActiveLengths, &a.AvgStrokeCount, &a.AvgStrokeDistanceM,
		&a.StrokeCount, &a.ZoneCount,
		&a.AvgFractionalCadence, &a.MaxFractionalCadence, &a.TotalFractionalCycles,
		&a.AvgVerticalOscillationMm, &a.AvgStanceTimeMs, &a.AvgStanceTimePct,
		&a.AvgStanceTimeBalancePct, &a.AvgStepLengthMm, &a.AvgVerticalRatioPct,
		&a.AvgGradePct, &a.AvgPosGradePct, &a.AvgNegGradePct, &a.MaxPosGradePct, &a.MaxNegGradePct,
		&a.AvgPosVerticalSpeedMs, &a.AvgNegVerticalSpeedMs,
		&a.MaxPosVerticalSpeedMs, &a.MaxNegVerticalSpeedMs, &a.AvgVamMs,
		&a.AvgAltitudeM, &a.TotalFractionalAscentM, &a.TotalFractionalDescentM,
		&a.AvgLeftTorqueEffectivenessPct, &a.AvgRightTorqueEffectivenessPct,
		&a.AvgLeftPedalSmoothnessPct, &a.AvgRightPedalSmoothnessPct,
		&a.AvgCombinedPedalSmoothnessPct,
		&a.AvgLeftPcoMm, &a.AvgRightPcoMm,
		&a.AvgLeftPowerPhaseStartDeg, &a.AvgLeftPowerPhaseEndDeg,
		&a.AvgLeftPowerPhasePeakStartDeg, &a.AvgLeftPowerPhasePeakEndDeg,
		&a.AvgRightPowerPhaseStartDeg, &a.AvgRightPowerPhaseEndDeg,
		&a.AvgRightPowerPhasePeakStartDeg, &a.AvgRightPowerPhasePeakEndDeg,
		&a.AvgPowerPositionW, &a.MaxPowerPositionW,
		&a.AvgCadencePosition, &a.MaxCadencePosition,
		&a.TimeStandingS, &a.StandCount,
		&a.AvgLevMotorPowerW, &a.MaxLevMotorPowerW, &a.LevBatteryConsumptionPct,
		&a.TotalGrit, &a.TotalFlow, &a.AvgGrit, &a.AvgFlow,
		&a.JumpCount,
		&a.MaxBallSpeedMs, &a.AvgBallSpeedMs, &a.PlayerScore, &a.OpponentScore, &a.OpponentName,
		&a.AvgTotalHemoglobinConc, &a.MinTotalHemoglobinConc, &a.MaxTotalHemoglobinConc,
		&a.AvgSaturatedHemoglobinPct, &a.MinSaturatedHemoglobinPct, &a.MaxSaturatedHemoglobinPct,
		&a.AvgDepthM, &a.MaxDepthM, &a.SurfaceIntervalS, &a.DiveNumber,
		&a.StartCnsPct, &a.EndCnsPct, &a.StartN2Pct, &a.EndN2Pct, &a.O2ToxicityOtu,
		&a.MaxRespirationRate, &a.MinRespirationRate, &a.SdrrHrvMs,
		&a.MinCoreTemperatureC, &a.MaxCoreTemperatureC,
		&a.AvgLapTimeS, &a.BestLapIndex, &a.NumLaps, &a.FirstLapIndex,
		&a.IntensityFactor, &a.LeftRightBalance, &a.SportProfileName, &a.SportIndex,
		&a.NecLat, &a.NecLng, &a.SwcLat, &a.SwcLng,
		&a.TitleIsManual,
		&a.IsManualEntry, &a.WorkoutFeel, &a.WorkoutRpe,
		&a.Timezone,
		&a.NumSessions,
		&a.Vo2MaxMlKgMin, &a.Vo2MaxCalibrated, &a.Vo2MaxHrSource, &a.Vo2MaxSpeedSource,
		&a.GpsAccuracyM,
		&a.IsGroupSecondary,
		&a.RecordingGroupID,
		&a.RunningFormDutyFactor, &a.RunningFormVerticalRatio, &a.RunningFormGCTAsymmetry,
		&a.RunningFormFatigueDuty, &a.RunningFormFatigueVR, &a.RunningFormTier,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get activity: %w", err)
	}
	a.HasGPS = hasTrack
	a.NeedsClassification = a.SportType == "unknown"
	if a.Tags == nil {
		a.Tags = []string{}
	}
	if sportConflictMeta != nil && a.SportConflictPending {
		var meta model.SportConflictMeta
		if json.Unmarshal(sportConflictMeta, &meta) == nil && meta.FileSport != "" {
			a.SportConflictMeta = &meta
		}
	}
	return &a, nil
}

// SetActivityTimezone sets the IANA timezone string on an activity.
// It validates the timezone against the IANA database before writing to prevent
// corrupt values from breaking all AT TIME ZONE queries for the user.
func (db *DB) SetActivityTimezone(ctx context.Context, activityID, timezone string) error {
	if timezone != "" {
		if _, err := time.LoadLocation(timezone); err != nil {
			return fmt.Errorf("invalid timezone %q: %w", timezone, err)
		}
	}
	_, err := db.Pool.Exec(ctx,
		`UPDATE activities SET timezone = $1 WHERE id = $2`,
		timezone, activityID)
	return err
}

// GetActivityWithWeather returns an activity by ID, including its weather readings.
func (db *DB) GetActivityWithWeather(ctx context.Context, id string) (*model.Activity, error) {
	a, err := db.GetActivity(ctx, id)
	if err != nil || a == nil {
		return a, err
	}
	readings, err := db.GetWeatherReadings(ctx, id)
	if err != nil {
		return a, nil // best-effort: return activity without weather on error
	}
	a.WeatherReadings = readings
	return a, nil
}

// allowedSortColumns maps allowed sort_by parameter values to SQL column names.
var allowedSortColumns = map[string]string{
	"started_at":       "started_at",
	"distance_m":       "distance_m",
	"elevation_gain_m": "elevation_gain_m",
	"elapsed_time_s":   "elapsed_time_s",
}

// buildFilterSQL constructs dynamic WHERE conditions from ActivityFilters.
// startIdx is the first $N placeholder index to use for filter args.
// Returns the SQL fragment (e.g. "AND sport_type = ANY($2) AND ...") and args slice.
func buildFilterSQL(filters model.ActivityFilters, startIdx int) (string, []any) {
	var conds []string
	var args []any
	idx := startIdx

	if len(filters.SportTypes) > 0 {
		idx++
		conds = append(conds, fmt.Sprintf("sport_type = ANY($%d)", idx))
		args = append(args, filters.SportTypes)
	}
	if len(filters.SubTypes) > 0 {
		idx++
		conds = append(conds, fmt.Sprintf("sub_type = ANY($%d)", idx))
		args = append(args, filters.SubTypes)
	}
	// NOTE: (started_at AT TIME ZONE COALESCE(timezone, 'UTC'))::date cannot use
	// the (user_id, started_at) index because it's a non-deterministic expression
	// over two columns. This is acceptable for per-user queries (filtered by user_id
	// first). If performance becomes an issue with 10k+ activities, consider a
	// stored generated local_date column.
	if filters.DateFrom != "" {
		idx++
		conds = append(conds, fmt.Sprintf("(started_at AT TIME ZONE COALESCE(timezone, 'UTC'))::date >= $%d::date", idx))
		args = append(args, filters.DateFrom)
	}
	if filters.DateTo != "" {
		idx++
		conds = append(conds, fmt.Sprintf("(started_at AT TIME ZONE COALESCE(timezone, 'UTC'))::date <= $%d::date", idx))
		args = append(args, filters.DateTo)
	}
	if filters.IsIndoor != nil {
		idx++
		conds = append(conds, fmt.Sprintf("is_indoor = $%d", idx))
		args = append(args, *filters.IsIndoor)
	}
	if filters.IsElectric != nil {
		idx++
		conds = append(conds, fmt.Sprintf("is_electric = $%d", idx))
		args = append(args, *filters.IsElectric)
	}
	if filters.IsVirtual != nil {
		idx++
		conds = append(conds, fmt.Sprintf("is_virtual = $%d", idx))
		args = append(args, *filters.IsVirtual)
	}
	if filters.HasGPS != nil {
		idx++
		conds = append(conds, fmt.Sprintf("has_gps = $%d", idx))
		args = append(args, *filters.HasGPS)
	}
	if filters.HasLocation != nil {
		if *filters.HasLocation {
			conds = append(conds, "location_lat IS NOT NULL AND location_lng IS NOT NULL")
		} else {
			conds = append(conds, "location_lat IS NULL OR location_lng IS NULL")
		}
	}
	if filters.GearID != "" {
		idx++
		conds = append(conds, fmt.Sprintf("gear_id = $%d", idx))
		args = append(args, filters.GearID)
	}
	if filters.Search != "" {
		idx++
		conds = append(conds, fmt.Sprintf("(title ILIKE '%%' || $%d || '%%' OR title_auto ILIKE '%%' || $%d || '%%')", idx, idx))
		args = append(args, filters.Search)
	}
	if len(filters.IncludeTags) > 0 {
		idx++
		conds = append(conds, fmt.Sprintf("id IN (SELECT activity_id FROM activity_tags WHERE tag_id = ANY($%d))", idx))
		args = append(args, filters.IncludeTags)
	}
	if len(filters.ExcludeTags) > 0 {
		idx++
		conds = append(conds, fmt.Sprintf("id NOT IN (SELECT activity_id FROM activity_tags WHERE tag_id = ANY($%d))", idx))
		args = append(args, filters.ExcludeTags)
	}
	if len(filters.IncludeGear) > 0 {
		idx++
		conds = append(conds, fmt.Sprintf("gear_id = ANY($%d)", idx))
		args = append(args, filters.IncludeGear)
	}
	if len(filters.ExcludeGear) > 0 {
		idx++
		conds = append(conds, fmt.Sprintf("(gear_id IS NULL OR gear_id != ALL($%d))", idx))
		args = append(args, filters.ExcludeGear)
	}
	if len(filters.IncludeDevices) > 0 {
		idx++
		conds = append(conds, fmt.Sprintf(
			"(recording_device_id = ANY($%d) OR id IN ("+
				"SELECT activity_id FROM activity_sensors WHERE device_id = ANY($%d) "+
				"UNION "+
				"SELECT activity_id FROM activity_developer_apps WHERE device_id = ANY($%d)))",
			idx, idx, idx))
		args = append(args, filters.IncludeDevices)
	}
	if len(filters.ExcludeDevices) > 0 {
		idx++
		conds = append(conds, fmt.Sprintf(
			"(recording_device_id IS NULL OR recording_device_id != ALL($%d)) "+
				"AND id NOT IN ("+
				"SELECT activity_id FROM activity_sensors WHERE device_id = ANY($%d) "+
				"UNION "+
				"SELECT activity_id FROM activity_developer_apps WHERE device_id = ANY($%d))",
			idx, idx, idx))
		args = append(args, filters.ExcludeDevices)
	}

	if len(conds) == 0 {
		return "", nil
	}
	return "AND " + strings.Join(conds, " AND "), args
}

func (db *DB) ListActivities(ctx context.Context, userID string, cursor string, limit int, sortBy string, sortDir string, filters model.ActivityFilters) (*model.ActivityList, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Validate sort parameters
	sqlCol, ok := allowedSortColumns[sortBy]
	if !ok {
		sortBy = "started_at"
		sqlCol = "started_at"
	}
	if sortDir != "asc" && sortDir != "desc" {
		sortDir = "desc"
	}

	// Determine comparison operator for cursor-based pagination.
	// DESC order: we want rows "less than" the cursor value → use <
	// ASC order: we want rows "greater than" the cursor value → use >
	cmpOp := "<"
	if sortDir == "asc" {
		cmpOp = ">"
	}

	// Parse cursor: base64(sortValue|id)
	// For started_at: sortValue is RFC3339Nano timestamp
	// For numeric columns: sortValue is the string representation of the number
	var cursorSortVal string
	var cursorID string
	hasCursor := false

	if cursor != "" {
		decoded, err := base64.URLEncoding.DecodeString(cursor)
		if err == nil {
			parts := strings.SplitN(string(decoded), "|", 2)
			if len(parts) == 2 {
				cursorSortVal = parts[0]
				cursorID = parts[1]
				hasCursor = true
			}
		}
	}

	var rows pgx.Rows
	var err error

	// Count with filters: $1 = userID, filter args start at $1
	countFilterSQL, countFilterArgs := buildFilterSQL(filters, 1)
	countArgs := []any{userID}
	countArgs = append(countArgs, countFilterArgs...)

	var total int
	if err := db.Pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT COUNT(*) FROM activities WHERE user_id = $1 AND parent_id IS NULL AND NOT is_group_secondary %s`, countFilterSQL), countArgs...,
	).Scan(&total); err != nil {
		return nil, fmt.Errorf("count activities: %w", err)
	}

	selectCols := `activities.id, sport_type, sub_type, title, started_at, distance_m,
		elapsed_time_s, elevation_gain_m, elevation_loss_m, is_indoor, is_electric, is_virtual, exclude_from_heatmap, exclude_from_stats, visibility,
		created_at, updated_at, sport_conflict_pending, timezone, rgm.group_id`

	// Build ORDER BY: primary sort column + tiebreaker id (same direction)
	orderClause := fmt.Sprintf("%s %s, id %s", sqlCol, strings.ToUpper(sortDir), strings.ToUpper(sortDir))

	// For nullable numeric columns, push NULLs to the end regardless of sort direction.
	// DESC → NULLS LAST (default for DESC, but be explicit)
	// ASC  → NULLS LAST (override default which is NULLS FIRST for ASC)
	if sqlCol != "started_at" {
		orderClause = fmt.Sprintf("%s %s NULLS LAST, id %s", sqlCol, strings.ToUpper(sortDir), strings.ToUpper(sortDir))
	}

	if hasCursor {
		if sortBy == "started_at" {
			// Parse cursor value as time
			cursorTime, parseErr := time.Parse(time.RFC3339Nano, cursorSortVal)
			if parseErr != nil {
				// Invalid cursor, fall through to no-cursor path
				hasCursor = false
			} else {
				// Base args: $1=userID, $2=cursorTime, $3=cursorID, then filters, then limit
				filterSQL, filterArgs := buildFilterSQL(filters, 3)
				allArgs := []any{userID, cursorTime, cursorID}
				allArgs = append(allArgs, filterArgs...)
				limitIdx := len(allArgs) + 1
				allArgs = append(allArgs, limit+1)

				query := fmt.Sprintf(`
					SELECT %s
					FROM activities
					LEFT JOIN recording_group_members rgm ON rgm.activity_id = activities.id
					WHERE user_id = $1
						AND parent_id IS NULL
						AND NOT is_group_secondary
						AND (%s, activities.id) %s ($2, $3)
						%s
					ORDER BY %s
					LIMIT $%d`, selectCols, sqlCol, cmpOp, filterSQL, orderClause, limitIdx)
				rows, err = db.Pool.Query(ctx, query, allArgs...)
			}
		} else if cursorSortVal == "" {
			// NULL cursor: the sort column was NULL for the last item.
			// Base args: $1=userID, $2=cursorID, then filters, then limit
			filterSQL, filterArgs := buildFilterSQL(filters, 2)
			allArgs := []any{userID, cursorID}
			allArgs = append(allArgs, filterArgs...)
			limitIdx := len(allArgs) + 1
			allArgs = append(allArgs, limit+1)

			query := fmt.Sprintf(`
				SELECT %s
				FROM activities
				LEFT JOIN recording_group_members rgm ON rgm.activity_id = activities.id
				WHERE user_id = $1
					AND parent_id IS NULL
					AND NOT is_group_secondary
					AND %s IS NULL
					AND activities.id %s $2
					%s
				ORDER BY %s
				LIMIT $%d`, selectCols, sqlCol, cmpOp, filterSQL, orderClause, limitIdx)
			rows, err = db.Pool.Query(ctx, query, allArgs...)
		} else {
			// Numeric column: parse as float64
			cursorNum, parseErr := strconv.ParseFloat(cursorSortVal, 64)
			if parseErr != nil {
				hasCursor = false
			} else {
				// Base args: $1=userID, $2=cursorNum, $3=cursorID, then filters, then limit
				filterSQL, filterArgs := buildFilterSQL(filters, 3)
				allArgs := []any{userID, cursorNum, cursorID}
				allArgs = append(allArgs, filterArgs...)
				limitIdx := len(allArgs) + 1
				allArgs = append(allArgs, limit+1)

				nullClause := fmt.Sprintf(`AND ((%s, activities.id) %s ($2, $3) OR %s IS NULL)`, sqlCol, cmpOp, sqlCol)
				query := fmt.Sprintf(`
					SELECT %s
					FROM activities
					LEFT JOIN recording_group_members rgm ON rgm.activity_id = activities.id
					WHERE user_id = $1
						AND parent_id IS NULL
						AND NOT is_group_secondary
						%s
						%s
					ORDER BY %s
					LIMIT $%d`, selectCols, nullClause, filterSQL, orderClause, limitIdx)
				rows, err = db.Pool.Query(ctx, query, allArgs...)
			}
		}
	}

	if !hasCursor {
		// Base args: $1=userID, then filters, then limit
		filterSQL, filterArgs := buildFilterSQL(filters, 1)
		allArgs := []any{userID}
		allArgs = append(allArgs, filterArgs...)
		limitIdx := len(allArgs) + 1
		allArgs = append(allArgs, limit+1)

		query := fmt.Sprintf(`
			SELECT %s
			FROM activities
			LEFT JOIN recording_group_members rgm ON rgm.activity_id = activities.id
			WHERE user_id = $1
			AND parent_id IS NULL
			AND NOT is_group_secondary
			%s
			ORDER BY %s
			LIMIT $%d`, selectCols, filterSQL, orderClause, limitIdx)
		rows, err = db.Pool.Query(ctx, query, allArgs...)
	}

	if err != nil {
		return nil, fmt.Errorf("list activities: %w", err)
	}
	defer rows.Close()

	var activities []model.Activity
	for rows.Next() {
		var a model.Activity
		if err := rows.Scan(
			&a.ID, &a.SportType, &a.SubType, &a.Title, &a.StartedAt, &a.DistanceM,
			&a.ElapsedTimeS, &a.ElevationGainM, &a.ElevationLossM, &a.IsIndoor, &a.IsElectric, &a.IsVirtual, &a.ExcludeFromHeatmap, &a.ExcludeFromStats, &a.Visibility,
			&a.CreatedAt, &a.UpdatedAt, &a.SportConflictPending, &a.Timezone, &a.RecordingGroupID,
		); err != nil {
			return nil, fmt.Errorf("scan activity: %w", err)
		}
		a.Tags = []string{}
		a.NeedsClassification = a.SportType == "unknown"
		activities = append(activities, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list activities: %w", err)
	}

	result := &model.ActivityList{
		Activities: activities,
		Total:      total,
	}

	if len(activities) > limit {
		result.Activities = activities[:limit]
		last := activities[limit-1]

		// Build cursor value based on sort column
		var sortVal string
		switch sortBy {
		case "started_at":
			sortVal = last.StartedAt.Format(time.RFC3339Nano)
		case "distance_m":
			if last.DistanceM != nil {
				sortVal = strconv.FormatFloat(*last.DistanceM, 'f', -1, 64)
			} else {
				sortVal = ""
			}
		case "elevation_gain_m":
			if last.ElevationGainM != nil {
				sortVal = strconv.FormatFloat(*last.ElevationGainM, 'f', -1, 64)
			} else {
				sortVal = ""
			}
		case "elapsed_time_s":
			if last.ElapsedTimeS != nil {
				sortVal = strconv.Itoa(*last.ElapsedTimeS)
			} else {
				sortVal = ""
			}
		}

		cursorStr := sortVal + "|" + last.ID
		result.NextCursor = base64.URLEncoding.EncodeToString([]byte(cursorStr))
	}

	if result.Activities == nil {
		result.Activities = []model.Activity{}
	}

	return result, nil
}

func (db *DB) UpdateActivity(ctx context.Context, id, userID string, update *model.ActivityUpdate) (*model.Activity, error) {
	b := newSetBuilder(3)

	if update.Title != nil {
		b.Set("title", *update.Title)
		b.SetExpr("title_is_manual = true")
	}
	if update.Description != nil {
		b.Set("description", *update.Description)
	}
	if update.SportType != nil || update.SubType != nil {
		// Load parsed values to compare — sport_type_is_manual should only be
		// true when the manual value differs from the parser-detected value.
		var parsedSportType, parsedSubType *string
		_ = db.Pool.QueryRow(ctx,
			`SELECT parsed_sport_type, parsed_sub_type FROM activities WHERE id = $1`, id,
		).Scan(&parsedSportType, &parsedSubType)

		if update.SportType != nil {
			b.Set("sport_type", *update.SportType)
		}
		if update.SubType != nil {
			b.Set("sub_type", *update.SubType)
		}

		// Determine if the effective values differ from parsed values.
		newST := derefOr(update.SportType, derefOr(parsedSportType, ""))
		newSub := derefOr(update.SubType, derefOr(parsedSubType, ""))
		parsedST := derefOr(parsedSportType, "")
		parsedSub := derefOr(parsedSubType, "")
		isManual := newST != parsedST || newSub != parsedSub
		b.SetExpr(fmt.Sprintf("sport_type_is_manual = %t", isManual))
	}
	if update.Tags != nil {
		b.Set("tags", update.Tags)
	}
	if update.Visibility != nil {
		b.Set("visibility", *update.Visibility)
	}
	if update.HideMap != nil {
		b.Set("hide_map", *update.HideMap)
	}
	if update.DataVisibility != nil {
		b.Set("data_visibility", update.DataVisibility)
	}
	if update.GearID != nil {
		if string(update.GearID) == "null" {
			b.SetExpr("gear_id = NULL")
		} else {
			var gearID string
			if err := json.Unmarshal(update.GearID, &gearID); err == nil && gearID != "" {
				b.Set("gear_id", gearID)
			}
		}
	}
	if update.LocationLat != nil {
		b.Set("location_lat", *update.LocationLat)
	}
	if update.LocationLng != nil {
		b.Set("location_lng", *update.LocationLng)
	}
	if update.ExcludeLiftSubtraction != nil {
		b.Set("exclude_lift_subtraction", *update.ExcludeLiftSubtraction)
	}
	if update.WorkoutFeel != nil {
		b.Set("workout_feel", *update.WorkoutFeel)
	}
	if update.WorkoutRpe != nil {
		b.Set("workout_rpe", *update.WorkoutRpe)
	}
	if update.ExcludeFromHeatmap != nil {
		if !*update.ExcludeFromHeatmap {
			var isVirtual bool
			_ = db.Pool.QueryRow(ctx, "SELECT is_virtual FROM activities WHERE id = $1 AND user_id = $2", id, userID).Scan(&isVirtual)
			if isVirtual {
				return nil, fmt.Errorf("cannot disable heatmap exclusion for virtual activities")
			}
		}
		b.Set("exclude_from_heatmap", *update.ExcludeFromHeatmap)
	}
	if update.ExcludeFromStats != nil {
		b.Set("exclude_from_stats", *update.ExcludeFromStats)
	}

	if !b.HasSets() {
		return db.GetActivity(ctx, id)
	}

	args := append([]any{id, userID}, b.Args()...)
	query := fmt.Sprintf("UPDATE activities SET %s WHERE id = $1 AND user_id = $2", b.Clause())
	_, err := db.Pool.Exec(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("update activity: %w", err)
	}

	return db.GetActivity(ctx, id)
}

func (db *DB) DeleteActivity(ctx context.Context, userID, id string) error {
	// Clean up any pending sport conflict queue entries for this activity
	_ = db.ResolveImportQueueByActivity(ctx, userID, id)
	result, err := db.Pool.Exec(ctx, `DELETE FROM activities WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete activity: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrActivityNotFound
	}
	return nil
}

// DeleteActivities deletes multiple activities by ID that belong to the given user.
// Returns the number of rows deleted. Activities not owned by userID are silently ignored.
func (db *DB) DeleteActivities(ctx context.Context, userID string, ids []string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	// userID is $1; activity IDs start at $2
	args := make([]any, 0, len(ids)+1)
	args = append(args, userID)
	placeholders := make([]string, len(ids))
	for i, id := range ids {
		args = append(args, id)
		placeholders[i] = fmt.Sprintf("$%d", i+2)
	}
	// Clean up sport conflict queue entries for the deleted activities
	cleanArgs := make([]any, 0, len(ids)+1)
	cleanArgs = append(cleanArgs, userID)
	cleanPlaceholders := make([]string, len(ids))
	for i, id := range ids {
		cleanArgs = append(cleanArgs, id)
		cleanPlaceholders[i] = fmt.Sprintf("metadata->>'activity_id' = $%d", i+2)
	}
	cleanQuery := fmt.Sprintf(
		`UPDATE import_queue SET status = 'resolved', resolved_at = NOW()
		 WHERE user_id = $1 AND queue_type = 'sport_conflict' AND status = 'pending'
		 AND (%s)`,
		strings.Join(cleanPlaceholders, " OR "))
	if _, cleanErr := db.Pool.Exec(ctx, cleanQuery, cleanArgs...); cleanErr != nil {
		slog.Warn("delete activities: failed to clean sport conflict queue", "error", cleanErr)
	}

	query := fmt.Sprintf(`DELETE FROM activities WHERE user_id = $1 AND id IN (%s)`,
		strings.Join(placeholders, ", "))
	result, err := db.Pool.Exec(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("delete activities: %w", err)
	}
	return result.RowsAffected(), nil
}

func (db *DB) GetStreams(ctx context.Context, activityID string) ([]model.StreamResponse, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT time, lat, lng, altitude_m, speed_ms, distance_m,
			heart_rate, power_w, cadence, temperature_c,
			ground_contact_time_ms, vertical_oscillation_mm, stance_time_percent, left_right_balance,
			-- FIT SDK — Terrain & Movement
			respiration_rate, stress_level,
			grade_pct, vertical_speed_ms, gps_accuracy_m,
			calories, resistance, total_cycles, accumulated_power_w,
			absolute_pressure_pa, ascent_rate_ms, time_from_course_s,
			activity_type, device_index,
			-- FIT SDK — Running Dynamics extended
			step_length_mm, vertical_ratio_pct, stance_time_balance_pct,
			cycle_length_m, fractional_cadence,
			-- FIT SDK — Pedal Analytics
			left_torque_effectiveness_pct, right_torque_effectiveness_pct,
			left_pedal_smoothness_pct, right_pedal_smoothness_pct,
			combined_pedal_smoothness_pct,
			left_pco_mm, right_pco_mm,
			-- FIT SDK — Power Phase
			left_power_phase_start_deg, left_power_phase_end_deg,
			left_power_phase_peak_start_deg, left_power_phase_peak_end_deg,
			right_power_phase_start_deg, right_power_phase_end_deg,
			right_power_phase_peak_start_deg, right_power_phase_peak_end_deg,
			-- FIT SDK — Hemoglobin / Muscle Oxygen
			total_hemoglobin_conc, total_hemoglobin_conc_min, total_hemoglobin_conc_max,
			saturated_hemoglobin_pct, saturated_hemoglobin_pct_min, saturated_hemoglobin_pct_max,
			-- FIT SDK — E-Bike
			motor_power_w, ebike_travel_range_km, ebike_battery_level_pct,
			ebike_assist_mode, ebike_assist_level_pct,
			-- FIT SDK — Health / Stress
			core_temperature_c, enhanced_respiration_rate, current_stress, battery_soc_pct,
			-- FIT SDK — MTB Metrics
			grit, flow,
			-- Garmin Stamina + GAP
			potential_stamina, stamina, gap_speed_ms,
			-- FIT SDK — Racket / Ball Sports
			stroke_type, zone, ball_speed_ms,
			-- FIT SDK — Diving
			depth_m, next_stop_depth_m, next_stop_time_s, time_to_surface_s,
			ndl_time_s, cns_load_pct, n2_load_pct, air_time_remaining_s,
			pressure_sac, volume_sac, rmv, po2,
			-- FIT SDK — GPS Metadata
			heading,
			-- FIT SDK — Developer Fields
			developer_fields
		FROM activity_streams
		WHERE activity_id = $1
		ORDER BY time ASC`,
		activityID,
	)
	if err != nil {
		return nil, fmt.Errorf("get streams: %w", err)
	}
	defer rows.Close()

	var streams []model.StreamResponse
	for rows.Next() {
		var s model.StreamResponse
		if err := rows.Scan(
			&s.Time, &s.Lat, &s.Lng, &s.AltitudeM, &s.SpeedMS, &s.DistanceM,
			&s.HeartRate, &s.PowerW, &s.Cadence, &s.TemperatureC,
			&s.GroundContactTimeMs, &s.VerticalOscillationMm, &s.StanceTimePercent, &s.LeftRightBalance,
			// FIT SDK — Terrain & Movement
			&s.RespirationRate, &s.StressLevel,
			&s.GradePct, &s.VerticalSpeedMs, &s.GpsAccuracyM,
			&s.Calories, &s.Resistance, &s.TotalCycles, &s.AccumulatedPowerW,
			&s.AbsolutePressurePa, &s.AscentRateMs, &s.TimeFromCourseS,
			&s.ActivityType, &s.DeviceIndex,
			// FIT SDK — Running Dynamics extended
			&s.StepLengthMm, &s.VerticalRatioPct, &s.StanceTimeBalancePct,
			&s.CycleLengthM, &s.FractionalCadence,
			// FIT SDK — Pedal Analytics
			&s.LeftTorqueEffectivenessPct, &s.RightTorqueEffectivenessPct,
			&s.LeftPedalSmoothnessPct, &s.RightPedalSmoothnessPct,
			&s.CombinedPedalSmoothnessPct,
			&s.LeftPcoMm, &s.RightPcoMm,
			// FIT SDK — Power Phase
			&s.LeftPowerPhaseStartDeg, &s.LeftPowerPhaseEndDeg,
			&s.LeftPowerPhasePeakStartDeg, &s.LeftPowerPhasePeakEndDeg,
			&s.RightPowerPhaseStartDeg, &s.RightPowerPhaseEndDeg,
			&s.RightPowerPhasePeakStartDeg, &s.RightPowerPhasePeakEndDeg,
			// FIT SDK — Hemoglobin / Muscle Oxygen
			&s.TotalHemoglobinConc, &s.TotalHemoglobinConcMin, &s.TotalHemoglobinConcMax,
			&s.SaturatedHemoglobinPct, &s.SaturatedHemoglobinPctMin, &s.SaturatedHemoglobinPctMax,
			// FIT SDK — E-Bike
			&s.MotorPowerW, &s.EbikeTravelRangeKm, &s.EbikeBatteryLevelPct,
			&s.EbikeAssistMode, &s.EbikeAssistLevelPct,
			// FIT SDK — Health / Stress
			&s.CoreTemperatureC, &s.EnhancedRespirationRate, &s.CurrentStress, &s.BatterySocPct,
			// FIT SDK — MTB Metrics
			&s.Grit, &s.Flow,
			// Garmin Stamina + GAP
			&s.PotentialStamina, &s.Stamina, &s.GapSpeedMS,
			// FIT SDK — Racket / Ball Sports
			&s.StrokeType, &s.Zone, &s.BallSpeedMs,
			// FIT SDK — Diving
			&s.DepthM, &s.NextStopDepthM, &s.NextStopTimeS, &s.TimeToSurfaceS,
			&s.NdlTimeS, &s.CnsLoadPct, &s.N2LoadPct, &s.AirTimeRemainingS,
			&s.PressureSac, &s.VolumeSac, &s.Rmv, &s.Po2,
			// FIT SDK — GPS Metadata
			&s.Heading,
			// FIT SDK — Developer Fields
			&s.DeveloperFields,
		); err != nil {
			return nil, fmt.Errorf("scan stream: %w", err)
		}
		streams = append(streams, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get streams: %w", err)
	}
	if streams == nil {
		streams = []model.StreamResponse{}
	}
	return streams, nil
}

// GetStreamPoints loads stream data for an activity as StreamPoint slices,
// which is the type required by activityutil.CalcMovingTime.
// All stored stream fields are returned (80+ columns).
func (db *DB) GetStreamPoints(ctx context.Context, activityID string) ([]model.StreamPoint, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT
			time, lat, lng, altitude_m, speed_ms, distance_m,
			heart_rate, power_w, cadence, temperature_c,
			ground_contact_time_ms, vertical_oscillation_mm, stance_time_percent, left_right_balance,
			respiration_rate, stress_level,
			grade_pct, vertical_speed_ms, gps_accuracy_m,
			calories, resistance, total_cycles, accumulated_power_w,
			absolute_pressure_pa, ascent_rate_ms, time_from_course_s,
			activity_type, device_index,
			step_length_mm, vertical_ratio_pct, stance_time_balance_pct,
			cycle_length_m, fractional_cadence,
			left_torque_effectiveness_pct, right_torque_effectiveness_pct,
			left_pedal_smoothness_pct, right_pedal_smoothness_pct,
			combined_pedal_smoothness_pct,
			left_pco_mm, right_pco_mm,
			left_power_phase_start_deg, left_power_phase_end_deg,
			left_power_phase_peak_start_deg, left_power_phase_peak_end_deg,
			right_power_phase_start_deg, right_power_phase_end_deg,
			right_power_phase_peak_start_deg, right_power_phase_peak_end_deg,
			total_hemoglobin_conc, total_hemoglobin_conc_min, total_hemoglobin_conc_max,
			saturated_hemoglobin_pct, saturated_hemoglobin_pct_min, saturated_hemoglobin_pct_max,
			motor_power_w, ebike_travel_range_km, ebike_battery_level_pct,
			ebike_assist_mode, ebike_assist_level_pct,
			core_temperature_c, enhanced_respiration_rate, current_stress, battery_soc_pct,
			grit, flow,
			potential_stamina, stamina, gap_speed_ms,
			stroke_type, zone, ball_speed_ms,
			depth_m, next_stop_depth_m, next_stop_time_s, time_to_surface_s,
			ndl_time_s, cns_load_pct, n2_load_pct, air_time_remaining_s,
			pressure_sac, volume_sac, rmv, po2,
			heading,
			developer_fields
		FROM activity_streams
		WHERE activity_id = $1
		ORDER BY time ASC`,
		activityID,
	)
	if err != nil {
		return nil, fmt.Errorf("get stream points: %w", err)
	}
	defer rows.Close()

	var streams []model.StreamPoint
	for rows.Next() {
		var s model.StreamPoint
		if err := rows.Scan(
			// Core (10)
			&s.Time, &s.Lat, &s.Lng, &s.AltitudeM, &s.SpeedMS, &s.DistanceM,
			&s.HeartRate, &s.PowerW, &s.Cadence, &s.TemperatureC,
			// Running dynamics original (4)
			&s.GroundContactTimeMs, &s.VerticalOscillationMm, &s.StanceTimePercent, &s.LeftRightBalance,
			// Terrain & Movement (14)
			&s.RespirationRate, &s.StressLevel,
			&s.GradePct, &s.VerticalSpeedMs, &s.GpsAccuracyM,
			&s.Calories, &s.Resistance, &s.TotalCycles, &s.AccumulatedPowerW,
			&s.AbsolutePressurePa, &s.AscentRateMs, &s.TimeFromCourseS,
			&s.ActivityType, &s.DeviceIndex,
			// Running Dynamics extended (5)
			&s.StepLengthMm, &s.VerticalRatioPct, &s.StanceTimeBalancePct,
			&s.CycleLengthM, &s.FractionalCadence,
			// Pedal Analytics (7)
			&s.LeftTorqueEffectivenessPct, &s.RightTorqueEffectivenessPct,
			&s.LeftPedalSmoothnessPct, &s.RightPedalSmoothnessPct,
			&s.CombinedPedalSmoothnessPct,
			&s.LeftPcoMm, &s.RightPcoMm,
			// Power Phase (8)
			&s.LeftPowerPhaseStartDeg, &s.LeftPowerPhaseEndDeg,
			&s.LeftPowerPhasePeakStartDeg, &s.LeftPowerPhasePeakEndDeg,
			&s.RightPowerPhaseStartDeg, &s.RightPowerPhaseEndDeg,
			&s.RightPowerPhasePeakStartDeg, &s.RightPowerPhasePeakEndDeg,
			// Hemoglobin / Muscle Oxygen (6)
			&s.TotalHemoglobinConc, &s.TotalHemoglobinConcMin, &s.TotalHemoglobinConcMax,
			&s.SaturatedHemoglobinPct, &s.SaturatedHemoglobinPctMin, &s.SaturatedHemoglobinPctMax,
			// E-Bike (5)
			&s.MotorPowerW, &s.EbikeTravelRangeKm, &s.EbikeBatteryLevelPct,
			&s.EbikeAssistMode, &s.EbikeAssistLevelPct,
			// Health / Stress (4)
			&s.CoreTemperatureC, &s.EnhancedRespirationRate, &s.CurrentStress, &s.BatterySocPct,
			// MTB Metrics (2)
			&s.Grit, &s.Flow,
			// Garmin Stamina + GAP (3)
			&s.PotentialStamina, &s.Stamina, &s.GapSpeedMS,
			// Racket / Ball Sports (3)
			&s.StrokeType, &s.Zone, &s.BallSpeedMs,
			// Diving (12)
			&s.DepthM, &s.NextStopDepthM, &s.NextStopTimeS, &s.TimeToSurfaceS,
			&s.NdlTimeS, &s.CnsLoadPct, &s.N2LoadPct, &s.AirTimeRemainingS,
			&s.PressureSac, &s.VolumeSac, &s.Rmv, &s.Po2,
			// GPS Metadata (1)
			&s.Heading,
			// Developer Fields (1)
			&s.DeveloperFields,
		); err != nil {
			return nil, fmt.Errorf("scan stream point: %w", err)
		}
		streams = append(streams, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get stream points: %w", err)
	}
	return streams, nil
}

// GetStreamPointsLightweight fetches only the 5 columns used by the best-effort
// compute functions: time, distance_m, altitude_m, power_w, gps_accuracy_m.
// This reduces network/memory overhead by ~16× compared to GetStreamPoints.
//
// NOTE(E-6): The Compute* functions in bestefforts/compute.go currently accept
// []model.StreamPoint. Using this getter in RecalcAll requires either:
//   a) Refactoring Compute* signatures to a generic/interface type, or
//   b) Adapting the service layer to work with StreamPointLight directly.
// Until that refactor, RecalcAll uses GetStreamPoints and this getter is
// available for the future migration.
func (db *DB) GetStreamPointsLightweight(ctx context.Context, activityID string) ([]model.StreamPointLight, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT time, distance_m, altitude_m, power_w, gps_accuracy_m
		 FROM activity_streams
		 WHERE activity_id = $1
		 ORDER BY time ASC`,
		activityID,
	)
	if err != nil {
		return nil, fmt.Errorf("get stream points lightweight: %w", err)
	}
	defer rows.Close()

	var points []model.StreamPointLight
	for rows.Next() {
		var p model.StreamPointLight
		if err := rows.Scan(&p.Time, &p.DistanceM, &p.AltitudeM, &p.PowerW, &p.GpsAccuracyM); err != nil {
			return nil, fmt.Errorf("scan stream point lightweight: %w", err)
		}
		points = append(points, p)
	}
	if points == nil {
		points = []model.StreamPointLight{}
	}
	return points, rows.Err()
}

// ListActivityIDsForUser returns all activity IDs belonging to the given user.
func (db *DB) ListActivityIDsForUser(ctx context.Context, userID string) ([]string, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT id FROM activities WHERE user_id = $1 ORDER BY started_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list activity ids: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan activity id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list activity ids: %w", err)
	}
	return ids, nil
}

// UpdateMovingTime sets moving_time_s for a single activity, scoped by user_id
// to prevent cross-user mutations.
func (db *DB) UpdateMovingTime(ctx context.Context, activityID, userID string, movingTimeS int) error {
	result, err := db.Pool.Exec(ctx,
		`UPDATE activities SET moving_time_s = $1 WHERE id = $2 AND user_id = $3`,
		movingTimeS, activityID, userID,
	)
	if err != nil {
		return fmt.Errorf("update moving time: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("activity not found or access denied")
	}
	return nil
}

// UpdateRunningForm persists computed running form metrics for an activity.
func (db *DB) UpdateRunningForm(ctx context.Context, activityID string, dutyFactor, verticalRatio, gctAsymmetry, fatigueDuty, fatigueVR *float64, tier *int) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE activities SET
			running_form_duty_factor = $2,
			running_form_vertical_ratio = $3,
			running_form_gct_asymmetry_pct = $4,
			running_form_fatigue_duty = $5,
			running_form_fatigue_vr = $6,
			running_form_tier = $7
		WHERE id = $1`,
		activityID, dutyFactor, verticalRatio, gctAsymmetry, fatigueDuty, fatigueVR, tier)
	return err
}

func (db *DB) GetSensors(ctx context.Context, activityID string) ([]model.SensorResponse, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT s.id, s.device_index, s.source_type, s.antplus_device_type, s.all_device_types,
			s.manufacturer, s.product_name, s.serial_number, s.ant_device_number,
			s.software_version, s.battery_status, s.device_id,
			COALESCE(d.custom_name, d.product_name) AS device_display_name
		FROM activity_sensors s
		LEFT JOIN devices d ON d.id = s.device_id
		WHERE s.activity_id = $1
		ORDER BY s.device_index`,
		activityID,
	)
	if err != nil {
		return nil, fmt.Errorf("get sensors: %w", err)
	}
	defer rows.Close()

	// Collect raw rows, then dedup by device_id (RC3: legacy activities may have
	// multiple rows for the same physical sensor with different device_types).
	type rawSensor struct {
		resp       model.SensorResponse
		deviceName *string
	}
	var raws []rawSensor

	for rows.Next() {
		var rs rawSensor
		var primaryType, manufacturer, productName, batteryStatus *string
		var allTypes []string
		var swVersion *float64

		if err := rows.Scan(
			&rs.resp.ID, &rs.resp.DeviceIndex, &rs.resp.SourceType, &primaryType, &allTypes,
			&manufacturer, &productName, &rs.resp.SerialNumber, &rs.resp.AntDeviceNumber,
			&swVersion, &batteryStatus, &rs.resp.DeviceID,
			&rs.deviceName,
		); err != nil {
			return nil, fmt.Errorf("scan sensor: %w", err)
		}

		if primaryType != nil {
			rs.resp.PrimaryType = *primaryType
			rs.resp.PrimaryTypeLabel = sensorTypeLabel(rs.resp.PrimaryType)
		}
		if manufacturer != nil {
			rs.resp.Manufacturer = *manufacturer
		}
		// Product name priority: devices table (resolved) > activity_sensors (raw FIT)
		if rs.deviceName != nil && *rs.deviceName != "" {
			rs.resp.ProductName = *rs.deviceName
		} else if productName != nil {
			rs.resp.ProductName = *productName
		}
		if batteryStatus != nil {
			rs.resp.BatteryStatus = *batteryStatus
		}
		rs.resp.AllTypes = allTypes
		if rs.resp.AllTypes == nil {
			rs.resp.AllTypes = []string{}
		}
		if swVersion != nil {
			rs.resp.SoftwareVersion = fmt.Sprintf("%.1f", *swVersion)
		}

		raws = append(raws, rs)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get sensors: %w", err)
	}

	// Dedup: merge rows with same device_id into one response (RC3).
	sensors := make([]model.SensorResponse, 0, len(raws))
	seen := map[string]int{} // device_id → index in sensors
	for _, rs := range raws {
		if rs.resp.DeviceID != nil && *rs.resp.DeviceID != "" {
			if idx, ok := seen[*rs.resp.DeviceID]; ok {
				// Merge: add device types from this row
				for _, dt := range rs.resp.AllTypes {
					if !slices.Contains(sensors[idx].AllTypes, dt) {
						sensors[idx].AllTypes = append(sensors[idx].AllTypes, dt)
					}
				}
				if sensors[idx].SerialNumber == nil && rs.resp.SerialNumber != nil {
					sensors[idx].SerialNumber = rs.resp.SerialNumber
				}
				if sensors[idx].AntDeviceNumber == nil && rs.resp.AntDeviceNumber != nil {
					sensors[idx].AntDeviceNumber = rs.resp.AntDeviceNumber
				}
				if sensors[idx].BatteryStatus == "" && rs.resp.BatteryStatus != "" {
					sensors[idx].BatteryStatus = rs.resp.BatteryStatus
				}
				continue
			}
			seen[*rs.resp.DeviceID] = len(sensors)
		}
		sensors = append(sensors, rs.resp)
	}

	// Developer Apps anhängen (Spec §4.1)
	devApps, err := db.GetDeveloperAppsForActivity(ctx, activityID)
	if err != nil {
		return nil, fmt.Errorf("get developer apps for sensors: %w", err)
	}

	for _, app := range devApps {
		// Dedup: wenn device_id bereits in seen Map (via ANT+/BLE), überspringen
		if app.DeviceID != nil && *app.DeviceID != "" {
			if _, exists := seen[*app.DeviceID]; exists {
				continue
			}
			seen[*app.DeviceID] = len(sensors)
		}

		resp := model.SensorResponse{
			ID:          app.ID,
			DeviceIndex: devAppDeviceIndex,
			SourceType:  "app",
			DeviceID:    app.DeviceID,
			AllTypes:    []string{},
		}
		if app.AppCategory != nil {
			resp.PrimaryType = *app.AppCategory
			resp.PrimaryTypeLabel = sensorTypeLabel(*app.AppCategory)
			resp.AllTypes = []string{*app.AppCategory}
		}
		if app.AppName != nil {
			resp.ProductName = *app.AppName
		}
		resp.SerialText = app.SerialNumber

		sensors = append(sensors, resp)
	}

	if sensors == nil {
		sensors = []model.SensorResponse{}
	}
	return sensors, nil
}

var sensorLabels = map[string]string{
	"heart_rate":                 "Herzfrequenzsensor",
	"bike_speed":                 "Geschwindigkeitssensor",
	"bike_cadence":               "Trittfrequenzsensor",
	"bike_speed_cadence":         "Geschwindigkeits-/Trittfrequenzsensor",
	"bike_power":                 "Leistungsmesser",
	"stride_speed_distance":      "Laufpod / Running Dynamics",
	"radar":                      "Rückwärtsradar",
	"bike_radar":                 "Rückwärtsradar",
	"shift":                      "Schaltung (Di2 / eTap)",
	"shifting":                   "Schaltung (Di2 / eTap)",
	"environment_sensor_legacy":  "Temperatursensor",
	"env_sensor":                 "Temperatursensor",
	"multi_sport_speed_distance": "Multi-Sport Sensor",
	"muscle_oxygen":              "Muskel-Sauerstoffsensor",
	"lights":                     "Fahrradbeleuchtung",
	"weight_scale":               "Waage",
	"fitness_equipment":          "Fitnessgerät",
	"app_running_power":          "Lauf-Leistungsmesser",
	"app_swim_goggle":            "Schwimmbrille",
	"app_bike_power":             "Rad-Leistungsmesser (App)",
	"app_wearable":               "Wearable-App",
	"app_biometric":              "Biometrie-Sensor",
}

func sensorTypeLabel(deviceType string) string {
	if label, ok := sensorLabels[deviceType]; ok {
		return label
	}
	return deviceType
}

// ReprocessInfo holds the data needed to reprocess an activity from its original file.
type ReprocessInfo struct {
	FileFormat        string
	SportTypeIsManual bool
	OriginalFilename  string
	FileID            string // user_files registry ID (ShadowFS)
	UserID            string // activity owner
}

// GetActivityReprocessInfo loads the fields needed to re-parse an activity from S3 or ShadowFS.
func (db *DB) GetActivityReprocessInfo(ctx context.Context, activityID string) (*ReprocessInfo, error) {
	var info ReprocessInfo
	var format, filename, fileID *string
	err := db.Pool.QueryRow(ctx,
		`SELECT original_file_format, COALESCE(sport_type_is_manual, false), original_filename,
		        COALESCE(file_id::text, ''), user_id
		 FROM activities WHERE id = $1`,
		activityID,
	).Scan(&format, &info.SportTypeIsManual, &filename, &fileID, &info.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get reprocess info: %w", err)
	}
	if format != nil {
		info.FileFormat = *format
	}
	if filename != nil {
		info.OriginalFilename = *filename
	}
	if fileID != nil {
		info.FileID = *fileID
	}
	return &info, nil
}

// CountStreamChannels returns the number of distinct non-NULL sensor channels
// present in the activity_streams for the given activity (0–6).
func (db *DB) CountStreamChannels(ctx context.Context, activityID string) (int, error) {
	var count int
	err := db.Pool.QueryRow(ctx, `
		SELECT
			(CASE WHEN COUNT(*) FILTER (WHERE heart_rate IS NOT NULL) > 0 THEN 1 ELSE 0 END) +
			(CASE WHEN COUNT(*) FILTER (WHERE cadence IS NOT NULL) > 0 THEN 1 ELSE 0 END) +
			(CASE WHEN COUNT(*) FILTER (WHERE power_w IS NOT NULL) > 0 THEN 1 ELSE 0 END) +
			(CASE WHEN COUNT(*) FILTER (WHERE temperature_c IS NOT NULL) > 0 THEN 1 ELSE 0 END) +
			(CASE WHEN COUNT(*) FILTER (WHERE altitude_m IS NOT NULL) > 0 THEN 1 ELSE 0 END) +
			(CASE WHEN COUNT(*) FILTER (WHERE speed_ms IS NOT NULL) > 0 THEN 1 ELSE 0 END)
		FROM activity_streams WHERE activity_id = $1`, activityID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count stream channels: %w", err)
	}
	return count, nil
}

// DeleteStreams removes all stream points for an activity.
func (db *DB) DeleteStreams(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM activity_streams WHERE activity_id = $1`, activityID)
	if err != nil {
		return fmt.Errorf("delete streams: %w", err)
	}
	return nil
}

// DeleteTrack removes the track geometry for an activity.
func (db *DB) DeleteTrack(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM activity_tracks WHERE activity_id = $1`, activityID)
	if err != nil {
		return fmt.Errorf("delete track: %w", err)
	}
	return nil
}

// DeleteSensors removes all sensor records for an activity.
func (db *DB) DeleteSensors(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM activity_sensors WHERE activity_id = $1`, activityID)
	if err != nil {
		return fmt.Errorf("delete sensors: %w", err)
	}
	return nil
}

// UpdateActivityMetrics updates all parser-derived fields on an activity.
// If skipSportType is true, sport_type and sub_type are NOT overwritten (manual override).
func (db *DB) UpdateActivityMetrics(ctx context.Context, activityID string, data *model.ActivityData, skipSportType bool) error {
	args := []any{activityID} // $1
	argIdx := 2

	idx := func() string {
		s := fmt.Sprintf("$%d", argIdx)
		argIdx++
		return s
	}

	var sets []string

	// Always update the parsed reference values so sport_type_is_manual
	// can be correctly determined on future PATCH requests.
	sets = append(sets, fmt.Sprintf("parsed_sport_type = %s", idx()))
	args = append(args, data.SportType)
	sets = append(sets, fmt.Sprintf("parsed_sub_type = %s", idx()))
	args = append(args, nilIfEmpty(data.SubType))

	if !skipSportType {
		sets = append(sets, fmt.Sprintf("sport_type = %s", idx()))
		args = append(args, data.SportType)
		sets = append(sets, fmt.Sprintf("sub_type = %s", idx()))
		args = append(args, nilIfEmpty(data.SubType))
		sets = append(sets, fmt.Sprintf("fit_sport_id = %s", idx()))
		args = append(args, nilIfZeroI16(data.FitSportID))
		sets = append(sets, fmt.Sprintf("fit_sub_sport_id = %s", idx()))
		args = append(args, nilIfZeroI16(data.FitSubSportID))
	}

	// Time fields
	sets = append(sets, fmt.Sprintf("started_at = %s", idx()))
	args = append(args, data.StartedAt)
	sets = append(sets, fmt.Sprintf("ended_at = %s", idx()))
	args = append(args, nilIfZeroTime(data.EndedAt))
	sets = append(sets, fmt.Sprintf("elapsed_time_s = %s", idx()))
	args = append(args, nilIfZeroInt(data.ElapsedTimeS))
	sets = append(sets, fmt.Sprintf("moving_time_s = %s", idx()))
	args = append(args, nilIfZeroInt(data.MovingTimeS))

	// Distance / elevation
	sets = append(sets, fmt.Sprintf("distance_m = %s", idx()))
	args = append(args, nilIfZeroF64(data.DistanceM))
	sets = append(sets, fmt.Sprintf("elevation_gain_m = %s", idx()))
	args = append(args, nilIfZeroF64(data.ElevationGainM))
	sets = append(sets, fmt.Sprintf("elevation_loss_m = %s", idx()))
	args = append(args, nilIfZeroF64(data.ElevationLossM))
	sets = append(sets, fmt.Sprintf("max_elevation_m = %s", idx()))
	args = append(args, nilIfZeroF64(data.MaxElevationM))
	sets = append(sets, fmt.Sprintf("min_elevation_m = %s", idx()))
	args = append(args, nilIfZeroF64(data.MinElevationM))

	// Speed
	sets = append(sets, fmt.Sprintf("avg_speed_ms = %s", idx()))
	args = append(args, nilIfZeroF64(data.AvgSpeedMS))
	sets = append(sets, fmt.Sprintf("max_speed_ms = %s", idx()))
	args = append(args, nilIfZeroF64(data.MaxSpeedMS))

	// Heart rate
	sets = append(sets, fmt.Sprintf("avg_heart_rate = %s", idx()))
	args = append(args, nilIfZeroF64(data.AvgHeartRate))
	sets = append(sets, fmt.Sprintf("max_heart_rate = %s", idx()))
	args = append(args, nilIfZeroInt(data.MaxHeartRate))

	// Power
	sets = append(sets, fmt.Sprintf("avg_power_w = %s", idx()))
	args = append(args, nilIfZeroF64(data.AvgPowerW))
	sets = append(sets, fmt.Sprintf("max_power_w = %s", idx()))
	args = append(args, nilIfZeroInt(data.MaxPowerW))
	sets = append(sets, fmt.Sprintf("normalized_power_w = %s", idx()))
	args = append(args, nilIfZeroF64(data.NormalizedPowerW))

	// Cadence / temperature
	sets = append(sets, fmt.Sprintf("avg_cadence = %s", idx()))
	args = append(args, nilIfZeroF64(data.AvgCadence))
	sets = append(sets, fmt.Sprintf("avg_temperature_c = %s", idx()))
	args = append(args, nilIfZeroF64(data.AvgTemperatureC))

	// Training load
	sets = append(sets, fmt.Sprintf("tss = %s", idx()))
	args = append(args, nilIfZeroF64(data.TSS))
	sets = append(sets, fmt.Sprintf("trimp = %s", idx()))
	args = append(args, nilIfZeroF64(data.TRIMP))
	sets = append(sets, fmt.Sprintf("is_indoor = %s", idx()))
	args = append(args, data.IsIndoor)
	sets = append(sets, fmt.Sprintf("is_electric = %s", idx()))
	args = append(args, data.IsElectric)
	sets = append(sets, fmt.Sprintf("is_virtual = %s", idx()))
	args = append(args, data.IsVirtual)
	sets = append(sets, fmt.Sprintf("has_gps = %s", idx()))
	args = append(args, data.HasGPS)

	// Location (start)
	latIdx := idx()
	lngIdx := idx()
	sets = append(sets, fmt.Sprintf(
		"start_location = CASE WHEN %s::float8 != 0 THEN ST_SetSRID(ST_MakePoint(%s::float8, %s::float8), 4326) ELSE NULL END",
		latIdx, lngIdx, latIdx))
	args = append(args, data.StartLat, data.StartLng)

	// Location (end)
	latIdx = idx()
	lngIdx = idx()
	sets = append(sets, fmt.Sprintf(
		"end_location = CASE WHEN %s::float8 != 0 THEN ST_SetSRID(ST_MakePoint(%s::float8, %s::float8), 4326) ELSE NULL END",
		latIdx, lngIdx, latIdx))
	args = append(args, data.EndLat, data.EndLng)

	// Source
	sets = append(sets, fmt.Sprintf("source = %s", idx()))
	args = append(args, nilIfEmpty(data.Source))
	sets = append(sets, fmt.Sprintf("source_device = %s", idx()))
	args = append(args, nilIfEmpty(data.SourceDevice))

	// FIT SDK — Workout
	sets = append(sets, fmt.Sprintf("workout_id = %s", idx()))
	args = append(args, data.WorkoutID)

	// FIT SDK — Movement / Cycles
	sets = append(sets, fmt.Sprintf("total_cycles = %s", idx()))
	args = append(args, data.TotalCycles)
	sets = append(sets, fmt.Sprintf("total_work_j = %s", idx()))
	args = append(args, data.TotalWorkJ)
	sets = append(sets, fmt.Sprintf("total_fat_calories = %s", idx()))
	args = append(args, data.TotalFatCalories)
	sets = append(sets, fmt.Sprintf("metabolic_calories = %s", idx()))
	args = append(args, data.MetabolicCalories)
	sets = append(sets, fmt.Sprintf("total_moving_time_s = %s", idx()))
	args = append(args, data.TotalMovingTimeS)

	// FIT SDK — Swimming
	sets = append(sets, fmt.Sprintf("swim_stroke = %s", idx()))
	args = append(args, data.SwimStroke)
	sets = append(sets, fmt.Sprintf("pool_length_m = %s", idx()))
	args = append(args, data.PoolLengthM)
	sets = append(sets, fmt.Sprintf("pool_length_unit = %s", idx()))
	args = append(args, data.PoolLengthUnit)
	sets = append(sets, fmt.Sprintf("num_lengths = %s", idx()))
	args = append(args, data.NumLengths)
	sets = append(sets, fmt.Sprintf("num_active_lengths = %s", idx()))
	args = append(args, data.NumActiveLengths)
	sets = append(sets, fmt.Sprintf("avg_stroke_count = %s", idx()))
	args = append(args, data.AvgStrokeCount)
	sets = append(sets, fmt.Sprintf("avg_stroke_distance_m = %s", idx()))
	args = append(args, data.AvgStrokeDistanceM)
	sets = append(sets, fmt.Sprintf("stroke_count = %s", idx()))
	args = append(args, data.StrokeCount)
	sets = append(sets, fmt.Sprintf("zone_count = %s", idx()))
	args = append(args, data.ZoneCount)

	// FIT SDK — Cadence Extended
	sets = append(sets, fmt.Sprintf("avg_fractional_cadence = %s", idx()))
	args = append(args, data.AvgFractionalCadence)
	sets = append(sets, fmt.Sprintf("max_fractional_cadence = %s", idx()))
	args = append(args, data.MaxFractionalCadence)
	sets = append(sets, fmt.Sprintf("total_fractional_cycles = %s", idx()))
	args = append(args, data.TotalFractionalCycles)

	// FIT SDK — Running Dynamics
	sets = append(sets, fmt.Sprintf("avg_vertical_oscillation_mm = %s", idx()))
	args = append(args, data.AvgVerticalOscillationMm)
	sets = append(sets, fmt.Sprintf("avg_stance_time_ms = %s", idx()))
	args = append(args, data.AvgStanceTimeMs)
	sets = append(sets, fmt.Sprintf("avg_stance_time_pct = %s", idx()))
	args = append(args, data.AvgStanceTimePct)
	sets = append(sets, fmt.Sprintf("avg_stance_time_balance_pct = %s", idx()))
	args = append(args, data.AvgStanceTimeBalancePct)
	sets = append(sets, fmt.Sprintf("avg_step_length_mm = %s", idx()))
	args = append(args, data.AvgStepLengthMm)
	sets = append(sets, fmt.Sprintf("avg_vertical_ratio_pct = %s", idx()))
	args = append(args, data.AvgVerticalRatioPct)

	// FIT SDK — Grade
	sets = append(sets, fmt.Sprintf("avg_grade_pct = %s", idx()))
	args = append(args, data.AvgGradePct)
	sets = append(sets, fmt.Sprintf("avg_pos_grade_pct = %s", idx()))
	args = append(args, data.AvgPosGradePct)
	sets = append(sets, fmt.Sprintf("avg_neg_grade_pct = %s", idx()))
	args = append(args, data.AvgNegGradePct)
	sets = append(sets, fmt.Sprintf("max_pos_grade_pct = %s", idx()))
	args = append(args, data.MaxPosGradePct)
	sets = append(sets, fmt.Sprintf("max_neg_grade_pct = %s", idx()))
	args = append(args, data.MaxNegGradePct)

	// FIT SDK — Vertical Speed
	sets = append(sets, fmt.Sprintf("avg_pos_vertical_speed_ms = %s", idx()))
	args = append(args, data.AvgPosVerticalSpeedMs)
	sets = append(sets, fmt.Sprintf("avg_neg_vertical_speed_ms = %s", idx()))
	args = append(args, data.AvgNegVerticalSpeedMs)
	sets = append(sets, fmt.Sprintf("max_pos_vertical_speed_ms = %s", idx()))
	args = append(args, data.MaxPosVerticalSpeedMs)
	sets = append(sets, fmt.Sprintf("max_neg_vertical_speed_ms = %s", idx()))
	args = append(args, data.MaxNegVerticalSpeedMs)
	sets = append(sets, fmt.Sprintf("avg_vam_ms = %s", idx()))
	args = append(args, data.AvgVamMs)

	// FIT SDK — Altitude Extended
	sets = append(sets, fmt.Sprintf("avg_altitude_m = %s", idx()))
	args = append(args, data.AvgAltitudeM)
	sets = append(sets, fmt.Sprintf("total_fractional_ascent_m = %s", idx()))
	args = append(args, data.TotalFractionalAscentM)
	sets = append(sets, fmt.Sprintf("total_fractional_descent_m = %s", idx()))
	args = append(args, data.TotalFractionalDescentM)

	// FIT SDK — Pedal Analytics
	sets = append(sets, fmt.Sprintf("avg_left_torque_effectiveness_pct = %s", idx()))
	args = append(args, data.AvgLeftTorqueEffectivenessPct)
	sets = append(sets, fmt.Sprintf("avg_right_torque_effectiveness_pct = %s", idx()))
	args = append(args, data.AvgRightTorqueEffectivenessPct)
	sets = append(sets, fmt.Sprintf("avg_left_pedal_smoothness_pct = %s", idx()))
	args = append(args, data.AvgLeftPedalSmoothnessPct)
	sets = append(sets, fmt.Sprintf("avg_right_pedal_smoothness_pct = %s", idx()))
	args = append(args, data.AvgRightPedalSmoothnessPct)
	sets = append(sets, fmt.Sprintf("avg_combined_pedal_smoothness_pct = %s", idx()))
	args = append(args, data.AvgCombinedPedalSmoothnessPct)
	sets = append(sets, fmt.Sprintf("avg_left_pco_mm = %s", idx()))
	args = append(args, data.AvgLeftPcoMm)
	sets = append(sets, fmt.Sprintf("avg_right_pco_mm = %s", idx()))
	args = append(args, data.AvgRightPcoMm)
	sets = append(sets, fmt.Sprintf("avg_left_power_phase_start_deg = %s", idx()))
	args = append(args, data.AvgLeftPowerPhaseStartDeg)
	sets = append(sets, fmt.Sprintf("avg_left_power_phase_end_deg = %s", idx()))
	args = append(args, data.AvgLeftPowerPhaseEndDeg)
	sets = append(sets, fmt.Sprintf("avg_left_power_phase_peak_start_deg = %s", idx()))
	args = append(args, data.AvgLeftPowerPhasePeakStartDeg)
	sets = append(sets, fmt.Sprintf("avg_left_power_phase_peak_end_deg = %s", idx()))
	args = append(args, data.AvgLeftPowerPhasePeakEndDeg)
	sets = append(sets, fmt.Sprintf("avg_right_power_phase_start_deg = %s", idx()))
	args = append(args, data.AvgRightPowerPhaseStartDeg)
	sets = append(sets, fmt.Sprintf("avg_right_power_phase_end_deg = %s", idx()))
	args = append(args, data.AvgRightPowerPhaseEndDeg)
	sets = append(sets, fmt.Sprintf("avg_right_power_phase_peak_start_deg = %s", idx()))
	args = append(args, data.AvgRightPowerPhasePeakStartDeg)
	sets = append(sets, fmt.Sprintf("avg_right_power_phase_peak_end_deg = %s", idx()))
	args = append(args, data.AvgRightPowerPhasePeakEndDeg)

	// FIT SDK — Power/Cadence by Position
	sets = append(sets, fmt.Sprintf("avg_power_position_w = %s", idx()))
	args = append(args, data.AvgPowerPositionW)
	sets = append(sets, fmt.Sprintf("max_power_position_w = %s", idx()))
	args = append(args, data.MaxPowerPositionW)
	sets = append(sets, fmt.Sprintf("avg_cadence_position = %s", idx()))
	args = append(args, data.AvgCadencePosition)
	sets = append(sets, fmt.Sprintf("max_cadence_position = %s", idx()))
	args = append(args, data.MaxCadencePosition)

	// FIT SDK — Standing/Sitting
	sets = append(sets, fmt.Sprintf("time_standing_s = %s", idx()))
	args = append(args, data.TimeStandingS)
	sets = append(sets, fmt.Sprintf("stand_count = %s", idx()))
	args = append(args, data.StandCount)

	// FIT SDK — E-Bike
	sets = append(sets, fmt.Sprintf("avg_lev_motor_power_w = %s", idx()))
	args = append(args, data.AvgLevMotorPowerW)
	sets = append(sets, fmt.Sprintf("max_lev_motor_power_w = %s", idx()))
	args = append(args, data.MaxLevMotorPowerW)
	sets = append(sets, fmt.Sprintf("lev_battery_consumption_pct = %s", idx()))
	args = append(args, data.LevBatteryConsumptionPct)

	// FIT SDK — MTB Grit/Flow
	sets = append(sets, fmt.Sprintf("total_grit = %s", idx()))
	args = append(args, data.TotalGrit)
	sets = append(sets, fmt.Sprintf("total_flow = %s", idx()))
	args = append(args, data.TotalFlow)
	sets = append(sets, fmt.Sprintf("avg_grit = %s", idx()))
	args = append(args, data.AvgGrit)
	sets = append(sets, fmt.Sprintf("avg_flow = %s", idx()))
	args = append(args, data.AvgFlow)

	// FIT SDK — Jumps
	sets = append(sets, fmt.Sprintf("jump_count = %s", idx()))
	args = append(args, data.JumpCount)

	// FIT SDK — Ball Sports
	sets = append(sets, fmt.Sprintf("max_ball_speed_ms = %s", idx()))
	args = append(args, data.MaxBallSpeedMs)
	sets = append(sets, fmt.Sprintf("avg_ball_speed_ms = %s", idx()))
	args = append(args, data.AvgBallSpeedMs)
	sets = append(sets, fmt.Sprintf("player_score = %s", idx()))
	args = append(args, data.PlayerScore)
	sets = append(sets, fmt.Sprintf("opponent_score = %s", idx()))
	args = append(args, data.OpponentScore)
	sets = append(sets, fmt.Sprintf("opponent_name = %s", idx()))
	args = append(args, data.OpponentName)

	// FIT SDK — Hemoglobin
	sets = append(sets, fmt.Sprintf("avg_total_hemoglobin_conc = %s", idx()))
	args = append(args, data.AvgTotalHemoglobinConc)
	sets = append(sets, fmt.Sprintf("min_total_hemoglobin_conc = %s", idx()))
	args = append(args, data.MinTotalHemoglobinConc)
	sets = append(sets, fmt.Sprintf("max_total_hemoglobin_conc = %s", idx()))
	args = append(args, data.MaxTotalHemoglobinConc)
	sets = append(sets, fmt.Sprintf("avg_saturated_hemoglobin_pct = %s", idx()))
	args = append(args, data.AvgSaturatedHemoglobinPct)
	sets = append(sets, fmt.Sprintf("min_saturated_hemoglobin_pct = %s", idx()))
	args = append(args, data.MinSaturatedHemoglobinPct)
	sets = append(sets, fmt.Sprintf("max_saturated_hemoglobin_pct = %s", idx()))
	args = append(args, data.MaxSaturatedHemoglobinPct)

	// FIT SDK — Diving
	sets = append(sets, fmt.Sprintf("avg_depth_m = %s", idx()))
	args = append(args, data.AvgDepthM)
	sets = append(sets, fmt.Sprintf("max_depth_m = %s", idx()))
	args = append(args, data.MaxDepthM)
	sets = append(sets, fmt.Sprintf("surface_interval_s = %s", idx()))
	args = append(args, data.SurfaceIntervalS)
	sets = append(sets, fmt.Sprintf("dive_number = %s", idx()))
	args = append(args, data.DiveNumber)
	sets = append(sets, fmt.Sprintf("start_cns_pct = %s", idx()))
	args = append(args, data.StartCnsPct)
	sets = append(sets, fmt.Sprintf("end_cns_pct = %s", idx()))
	args = append(args, data.EndCnsPct)
	sets = append(sets, fmt.Sprintf("start_n2_pct = %s", idx()))
	args = append(args, data.StartN2Pct)
	sets = append(sets, fmt.Sprintf("end_n2_pct = %s", idx()))
	args = append(args, data.EndN2Pct)
	sets = append(sets, fmt.Sprintf("o2_toxicity_otu = %s", idx()))
	args = append(args, data.O2ToxicityOtu)

	// FIT SDK — Respiration / HRV Extended
	sets = append(sets, fmt.Sprintf("max_respiration_rate = %s", idx()))
	args = append(args, data.MaxRespirationRate)
	sets = append(sets, fmt.Sprintf("min_respiration_rate = %s", idx()))
	args = append(args, data.MinRespirationRate)
	sets = append(sets, fmt.Sprintf("sdrr_hrv_ms = %s", idx()))
	args = append(args, data.SdrrHrvMs)

	// FIT SDK — Temperature Extended
	sets = append(sets, fmt.Sprintf("min_core_temperature_c = %s", idx()))
	args = append(args, data.MinCoreTemperatureC)
	sets = append(sets, fmt.Sprintf("max_core_temperature_c = %s", idx()))
	args = append(args, data.MaxCoreTemperatureC)

	// FIT SDK — Misc
	sets = append(sets, fmt.Sprintf("avg_lap_time_s = %s", idx()))
	args = append(args, data.AvgLapTimeS)
	sets = append(sets, fmt.Sprintf("best_lap_index = %s", idx()))
	args = append(args, data.BestLapIndex)
	sets = append(sets, fmt.Sprintf("num_laps = %s", idx()))
	args = append(args, data.NumLaps)
	sets = append(sets, fmt.Sprintf("first_lap_index = %s", idx()))
	args = append(args, data.FirstLapIndex)
	sets = append(sets, fmt.Sprintf("intensity_factor = %s", idx()))
	args = append(args, data.IntensityFactor)
	sets = append(sets, fmt.Sprintf("left_right_balance = %s", idx()))
	args = append(args, data.LeftRightBalance)
	sets = append(sets, fmt.Sprintf("sport_profile_name = %s", idx()))
	args = append(args, data.SportProfileName)
	sets = append(sets, fmt.Sprintf("sport_index = %s", idx()))
	args = append(args, data.SportIndex)
	sets = append(sets, fmt.Sprintf("nec_lat = %s", idx()))
	args = append(args, data.NecLat)
	sets = append(sets, fmt.Sprintf("nec_lng = %s", idx()))
	args = append(args, data.NecLng)
	sets = append(sets, fmt.Sprintf("swc_lat = %s", idx()))
	args = append(args, data.SwcLat)
	sets = append(sets, fmt.Sprintf("swc_lng = %s", idx()))
	args = append(args, data.SwcLng)

	// Manual entry / Workout feel & RPE
	sets = append(sets, fmt.Sprintf("is_manual_entry = %s", idx()))
	args = append(args, data.IsManualEntry)
	sets = append(sets, fmt.Sprintf("workout_feel = %s", idx()))
	args = append(args, nilIfZeroInt(data.WorkoutFeel))
	sets = append(sets, fmt.Sprintf("workout_rpe = %s", idx()))
	args = append(args, nilIfZeroInt(data.WorkoutRpe))

	// FIT SDK — GPS quality
	sets = append(sets, fmt.Sprintf("gps_accuracy_m = %s", idx()))
	args = append(args, nilIfZeroInt(data.GpsAccuracyM))

	// FIT SDK — New session fields (recompute support)
	sets = append(sets, fmt.Sprintf("max_cadence = %s", idx()))
	args = append(args, nilIfZeroInt(data.MaxCadence))
	sets = append(sets, fmt.Sprintf("min_temperature_c = %s", idx()))
	args = append(args, data.MinTemperatureC)
	sets = append(sets, fmt.Sprintf("max_temperature_c = %s", idx()))
	args = append(args, data.MaxTemperatureC)
	sets = append(sets, fmt.Sprintf("num_sessions = %s", idx()))
	args = append(args, nilIfZeroInt(data.NumSessions))
	sets = append(sets, fmt.Sprintf("vo2max_ml_kg_min = %s", idx()))
	args = append(args, data.Vo2MaxMlKgMin)
	sets = append(sets, fmt.Sprintf("vo2max_calibrated = %s", idx()))
	args = append(args, data.Vo2MaxCalibrated)
	sets = append(sets, fmt.Sprintf("vo2max_hr_source = %s", idx()))
	args = append(args, data.Vo2MaxHrSource)
	sets = append(sets, fmt.Sprintf("vo2max_speed_source = %s", idx()))
	args = append(args, data.Vo2MaxSpeedSource)

	// Always reset enrichment and timestamp
	sets = append(sets, "enrichment_status = 'pending'")

	query := fmt.Sprintf("UPDATE activities SET %s WHERE id = $1", strings.Join(sets, ", "))
	_, err := db.Pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update activity metrics: %w", err)
	}
	return nil
}

// ReprocessFilter defines optional filters for batch reprocessing.
type ReprocessFilter struct {
	SportTypes []string   `json:"sport_types,omitempty"`
	Format     string     `json:"format,omitempty"`
	Before     *time.Time `json:"before,omitempty"`
	After      *time.Time `json:"after,omitempty"`
	IDs        []string   `json:"ids,omitempty"`
	Scope      []string   `json:"scope,omitempty"` // streams, sensors, metrics, sport_type
}

// ListActivityIDsFiltered returns activity IDs matching the given filter criteria.
func (db *DB) ListActivityIDsFiltered(ctx context.Context, userID string, filter *ReprocessFilter) ([]string, error) {
	if filter == nil {
		return db.ListActivityIDsForUser(ctx, userID)
	}

	// If explicit IDs are given, use only those (scoped to user).
	if len(filter.IDs) > 0 {
		rows, err := db.Pool.Query(ctx,
			`SELECT id FROM activities WHERE user_id = $1 AND id = ANY($2) ORDER BY started_at DESC`,
			userID, filter.IDs,
		)
		if err != nil {
			return nil, fmt.Errorf("list filtered ids: %w", err)
		}
		defer rows.Close()
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return nil, fmt.Errorf("scan id: %w", err)
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("list filtered ids: %w", err)
		}
		return ids, nil
	}

	// Build dynamic WHERE clause.
	where := []string{"user_id = $1"}
	args := []any{userID}
	argIdx := 2

	if len(filter.SportTypes) > 0 {
		where = append(where, fmt.Sprintf("sport_type = ANY($%d)", argIdx))
		args = append(args, filter.SportTypes)
		argIdx++
	}
	if filter.Format != "" {
		where = append(where, fmt.Sprintf("original_file_format = $%d", argIdx))
		args = append(args, filter.Format)
		argIdx++
	}
	if filter.Before != nil {
		where = append(where, fmt.Sprintf("started_at < $%d", argIdx))
		args = append(args, *filter.Before)
		argIdx++
	}
	if filter.After != nil {
		where = append(where, fmt.Sprintf("started_at >= $%d", argIdx))
		args = append(args, *filter.After)
		argIdx++
	}

	query := fmt.Sprintf(
		`SELECT id FROM activities WHERE %s ORDER BY started_at DESC`,
		strings.Join(where, " AND "),
	)

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list filtered ids: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list filtered ids: %w", err)
	}
	return ids, nil
}

// FindActivitiesByTimeRange findet Aktivitäten in einem Zeitfenster mit passendem Sport.
// Wird für Tier-1 GPS-Identity-Matching im DuplicateDetector verwendet.
// Sport-Filter wird nur angewendet wenn sportType bekannt ist (nicht "unknown"/"generic"/leer),
// damit plattformübergreifende Duplikate erkannt werden (z.B. Garmin "cycling" vs Komoot "unknown").
func (db *DB) FindActivitiesByTimeRange(ctx context.Context, userID string, from, to time.Time, sportType string) ([]model.Activity, error) {
	knownSport := sportType != "" && sportType != "unknown" && sportType != "generic"

	var query string
	var args []any
	if knownSport {
		query = `SELECT id, sport_type, started_at, distance_m,
		                avg_heart_rate, max_heart_rate, avg_power_w, max_power_w,
		                avg_cadence, avg_temperature_c, elevation_gain_m
		         FROM activities
		         WHERE user_id = $1 AND started_at BETWEEN $2 AND $3 AND sport_type = $4
		         LIMIT 6`
		args = []any{userID, from, to, sportType}
	} else {
		query = `SELECT id, sport_type, started_at, distance_m,
		                avg_heart_rate, max_heart_rate, avg_power_w, max_power_w,
		                avg_cadence, avg_temperature_c, elevation_gain_m
		         FROM activities
		         WHERE user_id = $1 AND started_at BETWEEN $2 AND $3
		         LIMIT 6`
		args = []any{userID, from, to}
	}

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []model.Activity
	for rows.Next() {
		var a model.Activity
		if err := rows.Scan(
			&a.ID, &a.SportType, &a.StartedAt, &a.DistanceM,
			&a.AvgHeartRate, &a.MaxHeartRate, &a.AvgPowerW, &a.MaxPowerW,
			&a.AvgCadence, &a.AvgTemperatureC, &a.ElevationGainM,
		); err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// GetImportSettings lädt die Import-Einstellungen eines Nutzers.
// Bei Fehler oder fehlenden Einstellungen: sichere Defaults zurückgeben.
func (db *DB) GetImportSettings(ctx context.Context, userID string) (*model.ImportSettings, error) {
	var s model.ImportSettings
	err := db.Pool.QueryRow(ctx,
		`SELECT COALESCE(import_conflict_strategy, 'enrich_missing'),
		        COALESCE(gps_duplicate_threshold, 75),
		        COALESCE(fusion_time_window_min, 30)
		 FROM users WHERE id = $1`, userID,
	).Scan(&s.ConflictStrategy, &s.GPSDuplicateThreshold, &s.FusionTimeWindowMin)
	if err != nil {
		return &model.ImportSettings{ConflictStrategy: "enrich_missing", GPSDuplicateThreshold: 75, FusionTimeWindowMin: 30}, nil
	}
	return &s, nil
}

// UpdateImportSettings speichert die Import-Einstellungen eines Nutzers.
func (db *DB) UpdateImportSettings(ctx context.Context, userID string, s *model.ImportSettings) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE users SET import_conflict_strategy = $2, gps_duplicate_threshold = $3, fusion_time_window_min = $4 WHERE id = $1`,
		userID, s.ConflictStrategy, s.GPSDuplicateThreshold, s.FusionTimeWindowMin,
	)
	return err
}

// suppress unused import warning — pgxpool is used transitively via db.Pool
var _ = (*pgxpool.Pool)(nil)

// GetActivityStartEnd returns [startLat, startLon, endLat, endLon] for an activity.
func (db *DB) GetActivityStartEnd(ctx context.Context, activityID, userID string) ([4]float64, error) {
	var coords [4]float64
	err := db.Pool.QueryRow(ctx, `
		SELECT ST_Y(start_location), ST_X(start_location),
		       ST_Y(end_location), ST_X(end_location)
		FROM activities WHERE id = $1 AND user_id = $2
		AND start_location IS NOT NULL AND end_location IS NOT NULL`,
		activityID, userID).Scan(&coords[0], &coords[1], &coords[2], &coords[3])
	return coords, err
}
