package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// --- Laps ---

func (db *DB) InsertLaps(ctx context.Context, activityID string, laps []model.LapRecord) error {
	if len(laps) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, l := range laps {
		batch.Queue(`
			INSERT INTO activity_laps (
				id, activity_id, message_index, start_time, end_time,
				lap_trigger, sport, sub_sport,
				total_elapsed_time_s, total_timer_time_s, total_moving_time_s,
				total_distance_m, total_ascent_m, total_descent_m, total_calories,
				avg_speed_ms, max_speed_ms,
				avg_heart_rate, max_heart_rate, min_heart_rate,
				avg_cadence, max_cadence,
				avg_power_w, max_power_w, normalized_power_w,
				avg_temperature_c,
				start_lat, start_lng, end_lat, end_lng,
				total_work_j, intensity, swim_stroke,
				num_lengths, num_active_lengths,
				avg_altitude_m, max_altitude_m, min_altitude_m,
				-- Cycles/Strokes
				total_cycles, total_fat_calories, avg_stroke_distance_m,
				first_length_index, avg_fractional_cadence, max_fractional_cadence,
				total_fractional_cycles, stroke_count, zone_count,
				-- Running Dynamics
				avg_vertical_oscillation_mm, avg_stance_time_ms, avg_stance_time_pct,
				avg_stance_time_balance_pct, avg_step_length_mm, avg_vertical_ratio_pct,
				-- Grade
				avg_grade_pct, avg_pos_grade_pct, avg_neg_grade_pct,
				max_pos_grade_pct, max_neg_grade_pct,
				-- Vertical Speed
				avg_pos_vertical_speed_ms, avg_neg_vertical_speed_ms,
				max_pos_vertical_speed_ms, max_neg_vertical_speed_ms, avg_vam_ms,
				-- Temperature Extended
				max_temperature_c, min_temperature_c,
				avg_core_temperature_c, min_core_temperature_c, max_core_temperature_c,
				-- Pedal Analytics
				avg_left_torque_effectiveness_pct, avg_right_torque_effectiveness_pct,
				avg_left_pedal_smoothness_pct, avg_right_pedal_smoothness_pct,
				avg_combined_pedal_smoothness_pct,
				avg_left_pco_mm, avg_right_pco_mm,
				avg_left_power_phase_start_deg, avg_left_power_phase_end_deg,
				avg_left_power_phase_peak_start_deg, avg_left_power_phase_peak_end_deg,
				avg_right_power_phase_start_deg, avg_right_power_phase_end_deg,
				avg_right_power_phase_peak_start_deg, avg_right_power_phase_peak_end_deg,
				-- Power/Cadence by Position
				avg_power_position_w, max_power_position_w,
				avg_cadence_position, max_cadence_position,
				-- Standing/Sitting
				time_standing_s, stand_count,
				-- L/R Balance
				left_right_balance,
				-- E-Bike
				avg_lev_motor_power_w, max_lev_motor_power_w, lev_battery_consumption_pct,
				-- Grit/Flow
				total_grit, total_flow, avg_grit, avg_flow,
				-- Jumps
				jump_count,
				-- Scores
				player_score, opponent_score,
				-- Hemoglobin
				avg_total_hemoglobin_conc, min_total_hemoglobin_conc, max_total_hemoglobin_conc,
				avg_saturated_hemoglobin_pct, min_saturated_hemoglobin_pct, max_saturated_hemoglobin_pct,
				-- Diving
				avg_depth_m, max_depth_m,
				-- Respiration
				avg_respiration_rate, max_respiration_rate,
				-- Altitude Extended
				total_fractional_ascent_m, total_fractional_descent_m,
				-- Misc
				repetition_num, wkt_step_index, event_group, gps_accuracy_m
			) VALUES (
				$1, $2, $3, $4, $5,
				$6, $7, $8,
				$9, $10, $11,
				$12, $13, $14, $15,
				$16, $17,
				$18, $19, $20,
				$21, $22,
				$23, $24, $25,
				$26,
				$27, $28, $29, $30,
				$31, $32, $33,
				$34, $35,
				$36, $37, $38,
				$39, $40, $41,
				$42, $43, $44,
				$45, $46, $47,
				$48, $49, $50,
				$51, $52, $53,
				$54, $55, $56,
				$57, $58,
				$59, $60,
				$61, $62, $63,
				$64, $65,
				$66, $67,
				$68, $69, $70,
				$71, $72,
				$73, $74,
				$75, $76, $77,
				$78, $79,
				$80, $81,
				$82, $83, $84, $85, $86,
				$87, $88, $89, $90,
				$91, $92, $93, $94,
				$95,
				$96, $97,
				$98,
				$99, $100, $101,
				$102, $103, $104, $105,
				$106,
				$107, $108,
				$109, $110, $111,
				$112, $113, $114,
				$115, $116,
				$117, $118,
				$119, $120,
				$121, $122, $123, $124
			)`,
			l.ID, activityID, l.MessageIndex, nilIfZeroTime(l.StartTime), nilIfZeroTime(l.EndTime),
			nilIfEmpty(l.LapTrigger), nilIfEmpty(l.Sport), nilIfEmpty(l.SubSport),
			nilIfZeroF64(l.TotalElapsedTimeS), nilIfZeroF64(l.TotalTimerTimeS), nilIfZeroF64(l.TotalMovingTimeS),
			nilIfZeroF64(l.TotalDistanceM), nilIfZeroInt(l.TotalAscentM), nilIfZeroInt(l.TotalDescentM), nilIfZeroInt(l.TotalCalories),
			nilIfZeroF64(l.AvgSpeedMS), nilIfZeroF64(l.MaxSpeedMS),
			nilIfZeroInt(l.AvgHeartRate), nilIfZeroInt(l.MaxHeartRate), nilIfZeroInt(l.MinHeartRate),
			nilIfZeroInt(l.AvgCadence), nilIfZeroInt(l.MaxCadence),
			nilIfZeroInt(l.AvgPowerW), nilIfZeroInt(l.MaxPowerW), nilIfZeroInt(l.NormalizedPowerW),
			nilIfZeroInt(l.AvgTemperatureC),
			nilIfZeroF64(l.StartLat), nilIfZeroF64(l.StartLng), nilIfZeroF64(l.EndLat), nilIfZeroF64(l.EndLng),
			nilIfZeroInt(l.TotalWorkJ), nilIfEmpty(l.Intensity), nilIfEmpty(l.SwimStroke),
			nilIfZeroInt(l.NumLengths), nilIfZeroInt(l.NumActiveLengths),
			nilIfZeroF64(l.AvgAltitudeM), nilIfZeroF64(l.MaxAltitudeM), nilIfZeroF64(l.MinAltitudeM),
			// Cycles/Strokes
			l.TotalCycles, l.TotalFatCalories, l.AvgStrokeDistanceM,
			l.FirstLengthIndex, l.AvgFractionalCadence, l.MaxFractionalCadence,
			l.TotalFractionalCycles, l.StrokeCount, l.ZoneCount,
			// Running Dynamics
			l.AvgVerticalOscillationMm, l.AvgStanceTimeMs, l.AvgStanceTimePct,
			l.AvgStanceTimeBalancePct, l.AvgStepLengthMm, l.AvgVerticalRatioPct,
			// Grade
			l.AvgGradePct, l.AvgPosGradePct, l.AvgNegGradePct,
			l.MaxPosGradePct, l.MaxNegGradePct,
			// Vertical Speed
			l.AvgPosVerticalSpeedMs, l.AvgNegVerticalSpeedMs,
			l.MaxPosVerticalSpeedMs, l.MaxNegVerticalSpeedMs, l.AvgVamMs,
			// Temperature Extended
			l.MaxTemperatureC, l.MinTemperatureC,
			l.AvgCoreTemperatureC, l.MinCoreTemperatureC, l.MaxCoreTemperatureC,
			// Pedal Analytics
			l.AvgLeftTorqueEffectivenessPct, l.AvgRightTorqueEffectivenessPct,
			l.AvgLeftPedalSmoothnessPct, l.AvgRightPedalSmoothnessPct,
			l.AvgCombinedPedalSmoothnessPct,
			l.AvgLeftPcoMm, l.AvgRightPcoMm,
			l.AvgLeftPowerPhaseStartDeg, l.AvgLeftPowerPhaseEndDeg,
			l.AvgLeftPowerPhasePeakStartDeg, l.AvgLeftPowerPhasePeakEndDeg,
			l.AvgRightPowerPhaseStartDeg, l.AvgRightPowerPhaseEndDeg,
			l.AvgRightPowerPhasePeakStartDeg, l.AvgRightPowerPhasePeakEndDeg,
			// Power/Cadence by Position
			l.AvgPowerPositionW, l.MaxPowerPositionW,
			l.AvgCadencePosition, l.MaxCadencePosition,
			// Standing/Sitting
			l.TimeStandingS, l.StandCount,
			// L/R Balance
			l.LeftRightBalance,
			// E-Bike
			l.AvgLevMotorPowerW, l.MaxLevMotorPowerW, l.LevBatteryConsumptionPct,
			// Grit/Flow
			l.TotalGrit, l.TotalFlow, l.AvgGrit, l.AvgFlow,
			// Jumps
			l.JumpCount,
			// Scores
			l.PlayerScore, l.OpponentScore,
			// Hemoglobin
			l.AvgTotalHemoglobinConc, l.MinTotalHemoglobinConc, l.MaxTotalHemoglobinConc,
			l.AvgSaturatedHemoglobinPct, l.MinSaturatedHemoglobinPct, l.MaxSaturatedHemoglobinPct,
			// Diving
			l.AvgDepthM, l.MaxDepthM,
			// Respiration
			l.AvgRespirationRate, l.MaxRespirationRate,
			// Altitude Extended
			l.TotalFractionalAscentM, l.TotalFractionalDescentM,
			// Misc
			l.RepetitionNum, l.WktStepIndex, l.EventGroup, l.GpsAccuracyM,
		)
	}

	br := db.Pool.SendBatch(ctx, batch)
	defer br.Close()

	for range laps {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("insert lap: %w", err)
		}
	}
	return nil
}

func (db *DB) GetLaps(ctx context.Context, activityID string) ([]model.LapResponse, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, message_index, start_time, end_time,
			lap_trigger, sport, sub_sport,
			total_elapsed_time_s, total_timer_time_s, total_moving_time_s,
			total_distance_m, total_ascent_m, total_descent_m, total_calories,
			avg_speed_ms, max_speed_ms,
			avg_heart_rate, max_heart_rate, min_heart_rate,
			avg_cadence, max_cadence,
			avg_power_w, max_power_w, normalized_power_w,
			avg_temperature_c,
			start_lat, start_lng, end_lat, end_lng,
			total_work_j, intensity, swim_stroke,
			num_lengths, num_active_lengths,
			avg_altitude_m, max_altitude_m, min_altitude_m,
			-- Cycles/Strokes
			total_cycles, total_fat_calories, avg_stroke_distance_m,
			first_length_index, avg_fractional_cadence, max_fractional_cadence,
			total_fractional_cycles, stroke_count, zone_count,
			-- Running Dynamics
			avg_vertical_oscillation_mm, avg_stance_time_ms, avg_stance_time_pct,
			avg_stance_time_balance_pct, avg_step_length_mm, avg_vertical_ratio_pct,
			-- Grade
			avg_grade_pct, avg_pos_grade_pct, avg_neg_grade_pct,
			max_pos_grade_pct, max_neg_grade_pct,
			-- Vertical Speed
			avg_pos_vertical_speed_ms, avg_neg_vertical_speed_ms,
			max_pos_vertical_speed_ms, max_neg_vertical_speed_ms, avg_vam_ms,
			-- Temperature Extended
			max_temperature_c, min_temperature_c,
			avg_core_temperature_c, min_core_temperature_c, max_core_temperature_c,
			-- Pedal Analytics
			avg_left_torque_effectiveness_pct, avg_right_torque_effectiveness_pct,
			avg_left_pedal_smoothness_pct, avg_right_pedal_smoothness_pct,
			avg_combined_pedal_smoothness_pct,
			avg_left_pco_mm, avg_right_pco_mm,
			avg_left_power_phase_start_deg, avg_left_power_phase_end_deg,
			avg_left_power_phase_peak_start_deg, avg_left_power_phase_peak_end_deg,
			avg_right_power_phase_start_deg, avg_right_power_phase_end_deg,
			avg_right_power_phase_peak_start_deg, avg_right_power_phase_peak_end_deg,
			-- Power/Cadence by Position
			avg_power_position_w, max_power_position_w,
			avg_cadence_position, max_cadence_position,
			-- Standing/Sitting
			time_standing_s, stand_count,
			-- L/R Balance
			left_right_balance,
			-- E-Bike
			avg_lev_motor_power_w, max_lev_motor_power_w, lev_battery_consumption_pct,
			-- Grit/Flow
			total_grit, total_flow, avg_grit, avg_flow,
			-- Jumps
			jump_count,
			-- Scores
			player_score, opponent_score,
			-- Hemoglobin
			avg_total_hemoglobin_conc, min_total_hemoglobin_conc, max_total_hemoglobin_conc,
			avg_saturated_hemoglobin_pct, min_saturated_hemoglobin_pct, max_saturated_hemoglobin_pct,
			-- Diving
			avg_depth_m, max_depth_m,
			-- Respiration
			avg_respiration_rate, max_respiration_rate,
			-- Altitude Extended
			total_fractional_ascent_m, total_fractional_descent_m,
			-- Misc
			repetition_num, wkt_step_index, event_group, gps_accuracy_m
		FROM activity_laps
		WHERE activity_id = $1
		ORDER BY message_index`,
		activityID,
	)
	if err != nil {
		return nil, fmt.Errorf("get laps: %w", err)
	}
	defer rows.Close()

	var result []model.LapResponse
	for rows.Next() {
		var r model.LapResponse
		var lapTrigger, sport, subSport, intensity, swimStroke *string
		if err := rows.Scan(
			&r.ID, &r.MessageIndex, &r.StartTime, &r.EndTime,
			&lapTrigger, &sport, &subSport,
			&r.TotalElapsedTimeS, &r.TotalTimerTimeS, &r.TotalMovingTimeS,
			&r.TotalDistanceM, &r.TotalAscentM, &r.TotalDescentM, &r.TotalCalories,
			&r.AvgSpeedMS, &r.MaxSpeedMS,
			&r.AvgHeartRate, &r.MaxHeartRate, &r.MinHeartRate,
			&r.AvgCadence, &r.MaxCadence,
			&r.AvgPowerW, &r.MaxPowerW, &r.NormalizedPowerW,
			&r.AvgTemperatureC,
			&r.StartLat, &r.StartLng, &r.EndLat, &r.EndLng,
			&r.TotalWorkJ, &intensity, &swimStroke,
			&r.NumLengths, &r.NumActiveLengths,
			&r.AvgAltitudeM, &r.MaxAltitudeM, &r.MinAltitudeM,
			// Cycles/Strokes
			&r.TotalCycles, &r.TotalFatCalories, &r.AvgStrokeDistanceM,
			&r.FirstLengthIndex, &r.AvgFractionalCadence, &r.MaxFractionalCadence,
			&r.TotalFractionalCycles, &r.StrokeCount, &r.ZoneCount,
			// Running Dynamics
			&r.AvgVerticalOscillationMm, &r.AvgStanceTimeMs, &r.AvgStanceTimePct,
			&r.AvgStanceTimeBalancePct, &r.AvgStepLengthMm, &r.AvgVerticalRatioPct,
			// Grade
			&r.AvgGradePct, &r.AvgPosGradePct, &r.AvgNegGradePct,
			&r.MaxPosGradePct, &r.MaxNegGradePct,
			// Vertical Speed
			&r.AvgPosVerticalSpeedMs, &r.AvgNegVerticalSpeedMs,
			&r.MaxPosVerticalSpeedMs, &r.MaxNegVerticalSpeedMs, &r.AvgVamMs,
			// Temperature Extended
			&r.MaxTemperatureC, &r.MinTemperatureC,
			&r.AvgCoreTemperatureC, &r.MinCoreTemperatureC, &r.MaxCoreTemperatureC,
			// Pedal Analytics
			&r.AvgLeftTorqueEffectivenessPct, &r.AvgRightTorqueEffectivenessPct,
			&r.AvgLeftPedalSmoothnessPct, &r.AvgRightPedalSmoothnessPct,
			&r.AvgCombinedPedalSmoothnessPct,
			&r.AvgLeftPcoMm, &r.AvgRightPcoMm,
			&r.AvgLeftPowerPhaseStartDeg, &r.AvgLeftPowerPhaseEndDeg,
			&r.AvgLeftPowerPhasePeakStartDeg, &r.AvgLeftPowerPhasePeakEndDeg,
			&r.AvgRightPowerPhaseStartDeg, &r.AvgRightPowerPhaseEndDeg,
			&r.AvgRightPowerPhasePeakStartDeg, &r.AvgRightPowerPhasePeakEndDeg,
			// Power/Cadence by Position
			&r.AvgPowerPositionW, &r.MaxPowerPositionW,
			&r.AvgCadencePosition, &r.MaxCadencePosition,
			// Standing/Sitting
			&r.TimeStandingS, &r.StandCount,
			// L/R Balance
			&r.LeftRightBalance,
			// E-Bike
			&r.AvgLevMotorPowerW, &r.MaxLevMotorPowerW, &r.LevBatteryConsumptionPct,
			// Grit/Flow
			&r.TotalGrit, &r.TotalFlow, &r.AvgGrit, &r.AvgFlow,
			// Jumps
			&r.JumpCount,
			// Scores
			&r.PlayerScore, &r.OpponentScore,
			// Hemoglobin
			&r.AvgTotalHemoglobinConc, &r.MinTotalHemoglobinConc, &r.MaxTotalHemoglobinConc,
			&r.AvgSaturatedHemoglobinPct, &r.MinSaturatedHemoglobinPct, &r.MaxSaturatedHemoglobinPct,
			// Diving
			&r.AvgDepthM, &r.MaxDepthM,
			// Respiration
			&r.AvgRespirationRate, &r.MaxRespirationRate,
			// Altitude Extended
			&r.TotalFractionalAscentM, &r.TotalFractionalDescentM,
			// Misc
			&r.RepetitionNum, &r.WktStepIndex, &r.EventGroup, &r.GpsAccuracyM,
		); err != nil {
			return nil, fmt.Errorf("scan lap: %w", err)
		}
		if lapTrigger != nil {
			r.LapTrigger = *lapTrigger
		}
		if sport != nil {
			r.Sport = *sport
		}
		if subSport != nil {
			r.SubSport = *subSport
		}
		if intensity != nil {
			r.Intensity = *intensity
		}
		if swimStroke != nil {
			r.SwimStroke = *swimStroke
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get laps: %w", err)
	}
	if result == nil {
		result = []model.LapResponse{}
	}
	return result, nil
}

func (db *DB) DeleteLaps(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM activity_laps WHERE activity_id = $1`, activityID)
	if err != nil {
		return fmt.Errorf("delete laps: %w", err)
	}
	return nil
}

// --- Splits ---

func (db *DB) InsertSplits(ctx context.Context, activityID string, splits []model.SplitRecord) error {
	if len(splits) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, s := range splits {
		batch.Queue(`
			INSERT INTO activity_splits (
				activity_id, message_index, split_type,
				start_time, end_time,
				total_elapsed_time_s, total_timer_time_s, total_moving_time_s,
				total_distance_m, total_ascent_m, total_descent_m, total_calories,
				avg_speed_ms, max_speed_ms, avg_vert_speed_ms,
				start_lat, start_lng, end_lat, end_lng,
				start_elevation_m
			) VALUES (
				$1, $2, $3,
				$4, $5,
				$6, $7, $8,
				$9, $10, $11, $12,
				$13, $14, $15,
				$16, $17, $18, $19,
				$20
			)`,
			activityID, s.MessageIndex, nilIfEmpty(s.SplitType),
			nilIfZeroTime(s.StartTime), nilIfZeroTime(s.EndTime),
			nilIfZeroF64(s.TotalElapsedTimeS), nilIfZeroF64(s.TotalTimerTimeS), nilIfZeroF64(s.TotalMovingTimeS),
			nilIfZeroF64(s.TotalDistanceM), nilIfZeroInt(s.TotalAscentM), nilIfZeroInt(s.TotalDescentM), nilIfZeroInt(s.TotalCalories),
			nilIfZeroF64(s.AvgSpeedMS), nilIfZeroF64(s.MaxSpeedMS), nilIfZeroF64(s.AvgVertSpeedMS),
			nilIfZeroF64(s.StartLat), nilIfZeroF64(s.StartLng), nilIfZeroF64(s.EndLat), nilIfZeroF64(s.EndLng),
			nilIfZeroF64(s.StartElevationM),
		)
	}

	br := db.Pool.SendBatch(ctx, batch)
	defer br.Close()

	for range splits {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("insert split: %w", err)
		}
	}
	return nil
}

func (db *DB) GetSplits(ctx context.Context, activityID string) ([]model.SplitResponse, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, message_index, split_type,
			start_time, end_time,
			total_elapsed_time_s, total_timer_time_s, total_moving_time_s,
			total_distance_m, total_ascent_m, total_descent_m, total_calories,
			avg_speed_ms, max_speed_ms, avg_vert_speed_ms,
			start_lat, start_lng, end_lat, end_lng,
			start_elevation_m, climbing_grade
		FROM activity_splits
		WHERE activity_id = $1
		ORDER BY message_index`,
		activityID,
	)
	if err != nil {
		return nil, fmt.Errorf("get splits: %w", err)
	}
	defer rows.Close()

	var result []model.SplitResponse
	for rows.Next() {
		var r model.SplitResponse
		var splitType *string
		if err := rows.Scan(
			&r.ID, &r.MessageIndex, &splitType,
			&r.StartTime, &r.EndTime,
			&r.TotalElapsedTimeS, &r.TotalTimerTimeS, &r.TotalMovingTimeS,
			&r.TotalDistanceM, &r.TotalAscentM, &r.TotalDescentM, &r.TotalCalories,
			&r.AvgSpeedMS, &r.MaxSpeedMS, &r.AvgVertSpeedMS,
			&r.StartLat, &r.StartLng, &r.EndLat, &r.EndLng,
			&r.StartElevationM, &r.ClimbingGrade,
		); err != nil {
			return nil, fmt.Errorf("scan split: %w", err)
		}
		if splitType != nil {
			r.SplitType = *splitType
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get splits: %w", err)
	}
	if result == nil {
		result = []model.SplitResponse{}
	}
	return result, nil
}

// UpdateSplitClimbingGrade sets the climbing_grade for a specific split of an activity.
// Returns false if no matching split was found.
func (db *DB) UpdateSplitClimbingGrade(ctx context.Context, activityID, splitID string, grade *string) (bool, error) {
	tag, err := db.Pool.Exec(ctx, `
		UPDATE activity_splits
		SET climbing_grade = $1
		WHERE id = $2 AND activity_id = $3`,
		grade, splitID, activityID,
	)
	if err != nil {
		return false, fmt.Errorf("update split climbing grade: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (db *DB) DeleteSplits(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM activity_splits WHERE activity_id = $1`, activityID)
	if err != nil {
		return fmt.Errorf("delete splits: %w", err)
	}
	return nil
}

// --- Split Summaries ---

func (db *DB) InsertSplitSummaries(ctx context.Context, activityID string, summaries []model.SplitSummaryRecord) error {
	if len(summaries) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, s := range summaries {
		batch.Queue(`
			INSERT INTO activity_split_summaries (
				activity_id, message_index, split_type, num_splits,
				total_timer_time_s, total_moving_time_s,
				total_distance_m, total_ascent_m, total_descent_m, total_calories,
				avg_speed_ms, max_speed_ms, avg_vert_speed_ms,
				avg_heart_rate, max_heart_rate
			) VALUES (
				$1, $2, $3, $4,
				$5, $6,
				$7, $8, $9, $10,
				$11, $12, $13,
				$14, $15
			)`,
			activityID, s.MessageIndex, nilIfEmpty(s.SplitType), nilIfZeroInt(s.NumSplits),
			nilIfZeroF64(s.TotalTimerTimeS), nilIfZeroF64(s.TotalMovingTimeS),
			nilIfZeroF64(s.TotalDistanceM), nilIfZeroInt(s.TotalAscentM), nilIfZeroInt(s.TotalDescentM), nilIfZeroInt(s.TotalCalories),
			nilIfZeroF64(s.AvgSpeedMS), nilIfZeroF64(s.MaxSpeedMS), nilIfZeroF64(s.AvgVertSpeedMS),
			nilIfZeroInt(s.AvgHeartRate), nilIfZeroInt(s.MaxHeartRate),
		)
	}

	br := db.Pool.SendBatch(ctx, batch)
	defer br.Close()

	for range summaries {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("insert split summary: %w", err)
		}
	}
	return nil
}

func (db *DB) GetSplitSummaries(ctx context.Context, activityID string) ([]model.SplitSummaryResponse, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, message_index, split_type, num_splits,
			total_timer_time_s, total_moving_time_s,
			total_distance_m, total_ascent_m, total_descent_m, total_calories,
			avg_speed_ms, max_speed_ms, avg_vert_speed_ms,
			avg_heart_rate, max_heart_rate
		FROM activity_split_summaries
		WHERE activity_id = $1
		ORDER BY message_index`,
		activityID,
	)
	if err != nil {
		return nil, fmt.Errorf("get split summaries: %w", err)
	}
	defer rows.Close()

	var result []model.SplitSummaryResponse
	for rows.Next() {
		var r model.SplitSummaryResponse
		var splitType *string
		if err := rows.Scan(
			&r.ID, &r.MessageIndex, &splitType, &r.NumSplits,
			&r.TotalTimerTimeS, &r.TotalMovingTimeS,
			&r.TotalDistanceM, &r.TotalAscentM, &r.TotalDescentM, &r.TotalCalories,
			&r.AvgSpeedMS, &r.MaxSpeedMS, &r.AvgVertSpeedMS,
			&r.AvgHeartRate, &r.MaxHeartRate,
		); err != nil {
			return nil, fmt.Errorf("scan split summary: %w", err)
		}
		if splitType != nil {
			r.SplitType = *splitType
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get split summaries: %w", err)
	}
	if result == nil {
		result = []model.SplitSummaryResponse{}
	}
	return result, nil
}

func (db *DB) DeleteSplitSummaries(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM activity_split_summaries WHERE activity_id = $1`, activityID)
	if err != nil {
		return fmt.Errorf("delete split summaries: %w", err)
	}
	return nil
}

// --- Lengths ---

func (db *DB) InsertLengths(ctx context.Context, activityID string, lengths []model.LengthRecord) error {
	if len(lengths) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, l := range lengths {
		batch.Queue(`
			INSERT INTO activity_lengths (
				activity_id, message_index, start_time, end_time,
				total_elapsed_time_s, total_timer_time_s,
				total_strokes, total_calories,
				avg_speed_ms, avg_swimming_cadence,
				swim_stroke, length_type,
				stroke_count
			) VALUES (
				$1, $2, $3, $4,
				$5, $6,
				$7, $8,
				$9, $10,
				$11, $12,
				$13
			)`,
			activityID, l.MessageIndex, nilIfZeroTime(l.StartTime), nilIfZeroTime(l.EndTime),
			nilIfZeroF64(l.TotalElapsedTimeS), nilIfZeroF64(l.TotalTimerTimeS),
			nilIfZeroInt(l.TotalStrokes), nilIfZeroInt(l.TotalCalories),
			nilIfZeroF64(l.AvgSpeedMS), nilIfZeroInt(l.AvgSwimmingCadence),
			nilIfEmpty(l.SwimStroke), nilIfEmpty(l.LengthType),
			l.StrokeCount,
		)
	}

	br := db.Pool.SendBatch(ctx, batch)
	defer br.Close()

	for range lengths {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("insert length: %w", err)
		}
	}
	return nil
}

func (db *DB) GetLengths(ctx context.Context, activityID string) ([]model.LengthResponse, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, message_index, start_time, end_time,
			total_elapsed_time_s, total_timer_time_s,
			total_strokes, total_calories,
			avg_speed_ms, avg_swimming_cadence,
			swim_stroke, length_type
		FROM activity_lengths
		WHERE activity_id = $1
		ORDER BY message_index`,
		activityID,
	)
	if err != nil {
		return nil, fmt.Errorf("get lengths: %w", err)
	}
	defer rows.Close()

	var result []model.LengthResponse
	for rows.Next() {
		var r model.LengthResponse
		var swimStroke, lengthType *string
		if err := rows.Scan(
			&r.ID, &r.MessageIndex, &r.StartTime, &r.EndTime,
			&r.TotalElapsedTimeS, &r.TotalTimerTimeS,
			&r.TotalStrokes, &r.TotalCalories,
			&r.AvgSpeedMS, &r.AvgSwimmingCadence,
			&swimStroke, &lengthType,
		); err != nil {
			return nil, fmt.Errorf("scan length: %w", err)
		}
		if swimStroke != nil {
			r.SwimStroke = *swimStroke
		}
		if lengthType != nil {
			r.LengthType = *lengthType
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get lengths: %w", err)
	}
	if result == nil {
		result = []model.LengthResponse{}
	}
	return result, nil
}

func (db *DB) DeleteLengths(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM activity_lengths WHERE activity_id = $1`, activityID)
	if err != nil {
		return fmt.Errorf("delete lengths: %w", err)
	}
	return nil
}

// --- ClimbPro Events ---

func (db *DB) InsertClimbProEvents(ctx context.Context, activityID string, events []model.ClimbProEventRecord) error {
	if len(events) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, e := range events {
		batch.Queue(`
			INSERT INTO activity_climb_pro_events (
				activity_id, timestamp, event_type,
				climb_number, climb_category, current_dist_m,
				lat, lng
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			activityID, nilIfZeroTime(e.Timestamp), nilIfEmpty(e.EventType),
			nilIfZeroInt(e.ClimbNumber), nilIfZeroInt(e.ClimbCategory), nilIfZeroF64(e.CurrentDistM),
			nilIfZeroF64(e.Lat), nilIfZeroF64(e.Lng),
		)
	}

	br := db.Pool.SendBatch(ctx, batch)
	defer br.Close()

	for range events {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("insert climb pro event: %w", err)
		}
	}
	return nil
}

func (db *DB) GetClimbProEvents(ctx context.Context, activityID string) ([]model.ClimbProEventResponse, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, timestamp, event_type,
			climb_number, climb_category, current_dist_m,
			lat, lng
		FROM activity_climb_pro_events
		WHERE activity_id = $1
		ORDER BY timestamp`,
		activityID,
	)
	if err != nil {
		return nil, fmt.Errorf("get climb pro events: %w", err)
	}
	defer rows.Close()

	var result []model.ClimbProEventResponse
	for rows.Next() {
		var r model.ClimbProEventResponse
		var eventType *string
		if err := rows.Scan(
			&r.ID, &r.Timestamp, &eventType,
			&r.ClimbNumber, &r.ClimbCategory, &r.CurrentDistM,
			&r.Lat, &r.Lng,
		); err != nil {
			return nil, fmt.Errorf("scan climb pro event: %w", err)
		}
		if eventType != nil {
			r.EventType = *eventType
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get climb pro events: %w", err)
	}
	if result == nil {
		result = []model.ClimbProEventResponse{}
	}
	return result, nil
}

func (db *DB) DeleteClimbProEvents(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM activity_climb_pro_events WHERE activity_id = $1`, activityID)
	if err != nil {
		return fmt.Errorf("delete climb pro events: %w", err)
	}
	return nil
}
