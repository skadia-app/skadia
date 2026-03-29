package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func (db *DB) InsertActivitySegmentLaps(ctx context.Context, activityID string, laps []model.SegmentLapRecord) error {
	if len(laps) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, l := range laps {
		batch.Queue(`
			INSERT INTO activity_segment_laps (
				id, activity_id, message_index, timestamp, start_time,
				name, uuid, status, manufacturer, sport, sub_sport,
				event, event_type, event_group, sport_event,
				-- GPS
				start_lat, start_lng, end_lat, end_lng,
				nec_lat, nec_lng, swc_lat, swc_lng,
				-- Time
				total_elapsed_time_s, total_timer_time_s, total_moving_time_s, active_time_s,
				-- Distance / Elevation
				total_distance_m, total_ascent_m, total_descent_m,
				total_cycles, total_work_j,
				avg_altitude_m, max_altitude_m, min_altitude_m,
				total_fractional_ascent_m, total_fractional_descent_m,
				-- Speed
				avg_speed_ms, max_speed_ms,
				-- Heart Rate
				avg_heart_rate, max_heart_rate, min_heart_rate,
				-- Cadence
				avg_cadence, max_cadence,
				avg_fractional_cadence, max_fractional_cadence, total_fractional_cycles,
				-- Power
				avg_power_w, max_power_w, normalized_power_w, left_right_balance,
				-- Grade
				avg_grade_pct, avg_pos_grade_pct, avg_neg_grade_pct,
				max_pos_grade_pct, max_neg_grade_pct,
				-- Vertical Speed
				avg_pos_vertical_speed_ms, avg_neg_vertical_speed_ms,
				max_pos_vertical_speed_ms, max_neg_vertical_speed_ms,
				-- Temperature
				avg_temperature_c, max_temperature_c, gps_accuracy_m,
				-- Calories
				total_calories, total_fat_calories,
				-- Pedal Analytics
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
				-- Standing / Sitting
				time_standing_s, stand_count,
				-- Gear Shifts
				front_gear_shift_count, rear_gear_shift_count,
				-- Grit / Flow
				total_grit, total_flow, avg_grit, avg_flow,
				-- Misc
				repetition_num, wkt_step_index
			) VALUES (
				$1, $2, $3, $4, $5,
				$6, $7, $8, $9, $10, $11,
				$12, $13, $14, $15,
				$16, $17, $18, $19,
				$20, $21, $22, $23,
				$24, $25, $26, $27,
				$28, $29, $30,
				$31, $32,
				$33, $34, $35,
				$36, $37,
				$38, $39,
				$40, $41, $42,
				$43, $44,
				$45, $46, $47,
				$48, $49, $50, $51,
				$52, $53, $54,
				$55, $56,
				$57, $58,
				$59, $60,
				$61, $62, $63,
				$64, $65,
				$66, $67,
				$68, $69,
				$70, $71,
				$72, $73, $74, $75,
				$76, $77,
				$78, $79,
				$80, $81,
				$82, $83,
				$84, $85, $86, $87,
				$88, $89,
				$90, $91,
				$92, $93, $94
			)
			ON CONFLICT (activity_id, message_index) DO NOTHING`,
			l.ID, activityID, l.MessageIndex, l.Timestamp, l.StartTime,
			l.Name, l.UUID, l.Status, l.Manufacturer, l.Sport, l.SubSport,
			l.Event, l.EventType, l.EventGroup, l.SportEvent,
			// GPS
			l.StartLat, l.StartLng, l.EndLat, l.EndLng,
			l.NecLat, l.NecLng, l.SwcLat, l.SwcLng,
			// Time
			l.TotalElapsedTimeS, l.TotalTimerTimeS, l.TotalMovingTimeS, l.ActiveTimeS,
			// Distance / Elevation
			l.TotalDistanceM, l.TotalAscentM, l.TotalDescentM,
			l.TotalCycles, l.TotalWorkJ,
			l.AvgAltitudeM, l.MaxAltitudeM, l.MinAltitudeM,
			l.TotalFractionalAscentM, l.TotalFractionalDescentM,
			// Speed
			l.AvgSpeedMs, l.MaxSpeedMs,
			// Heart Rate
			l.AvgHeartRate, l.MaxHeartRate, l.MinHeartRate,
			// Cadence
			l.AvgCadence, l.MaxCadence,
			l.AvgFractionalCadence, l.MaxFractionalCadence, l.TotalFractionalCycles,
			// Power
			l.AvgPowerW, l.MaxPowerW, l.NormalizedPowerW, l.LeftRightBalance,
			// Grade
			l.AvgGradePct, l.AvgPosGradePct, l.AvgNegGradePct,
			l.MaxPosGradePct, l.MaxNegGradePct,
			// Vertical Speed
			l.AvgPosVerticalSpeedMs, l.AvgNegVerticalSpeedMs,
			l.MaxPosVerticalSpeedMs, l.MaxNegVerticalSpeedMs,
			// Temperature
			l.AvgTemperatureC, l.MaxTemperatureC, l.GpsAccuracyM,
			// Calories
			l.TotalCalories, l.TotalFatCalories,
			// Pedal Analytics
			l.AvgLeftTorqueEffectivenessPct, l.AvgRightTorqueEffectivenessPct,
			l.AvgLeftPedalSmoothnessPct, l.AvgRightPedalSmoothnessPct,
			l.AvgCombinedPedalSmoothnessPct,
			l.AvgLeftPcoMm, l.AvgRightPcoMm,
			l.AvgLeftPowerPhaseStartDeg, l.AvgLeftPowerPhaseEndDeg,
			l.AvgLeftPowerPhasePeakStartDeg, l.AvgLeftPowerPhasePeakEndDeg,
			l.AvgRightPowerPhaseStartDeg, l.AvgRightPowerPhaseEndDeg,
			l.AvgRightPowerPhasePeakStartDeg, l.AvgRightPowerPhasePeakEndDeg,
			l.AvgPowerPositionW, l.MaxPowerPositionW,
			l.AvgCadencePosition, l.MaxCadencePosition,
			// Standing / Sitting
			l.TimeStandingS, l.StandCount,
			// Gear Shifts
			l.FrontGearShiftCount, l.RearGearShiftCount,
			// Grit / Flow
			l.TotalGrit, l.TotalFlow, l.AvgGrit, l.AvgFlow,
			// Misc
			l.RepetitionNum, l.WktStepIndex,
		)
	}

	br := db.Pool.SendBatch(ctx, batch)
	defer br.Close()

	for range laps {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("insert activity segment lap: %w", err)
		}
	}
	return nil
}

func (db *DB) DeleteActivitySegmentLaps(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM activity_segment_laps WHERE activity_id = $1`, activityID)
	if err != nil {
		return fmt.Errorf("delete activity segment laps: %w", err)
	}
	return nil
}

func (db *DB) GetActivitySegmentLaps(ctx context.Context, activityID string, limit, offset int) ([]model.SegmentLapRecord, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, message_index, timestamp, start_time,
			name, uuid, status, manufacturer, sport, sub_sport,
			event, event_type, event_group, sport_event,
			start_lat, start_lng, end_lat, end_lng,
			nec_lat, nec_lng, swc_lat, swc_lng,
			total_elapsed_time_s, total_timer_time_s, total_moving_time_s, active_time_s,
			total_distance_m, total_ascent_m, total_descent_m,
			total_cycles, total_work_j,
			avg_altitude_m, max_altitude_m, min_altitude_m,
			total_fractional_ascent_m, total_fractional_descent_m,
			avg_speed_ms, max_speed_ms,
			avg_heart_rate, max_heart_rate, min_heart_rate,
			avg_cadence, max_cadence,
			avg_fractional_cadence, max_fractional_cadence, total_fractional_cycles,
			avg_power_w, max_power_w, normalized_power_w, left_right_balance,
			avg_grade_pct, avg_pos_grade_pct, avg_neg_grade_pct,
			max_pos_grade_pct, max_neg_grade_pct,
			avg_pos_vertical_speed_ms, avg_neg_vertical_speed_ms,
			max_pos_vertical_speed_ms, max_neg_vertical_speed_ms,
			avg_temperature_c, max_temperature_c, gps_accuracy_m,
			total_calories, total_fat_calories,
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
			front_gear_shift_count, rear_gear_shift_count,
			total_grit, total_flow, avg_grit, avg_flow,
			repetition_num, wkt_step_index
		FROM activity_segment_laps
		WHERE activity_id = $1
		ORDER BY message_index
		LIMIT $2 OFFSET $3`,
		activityID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("get activity segment laps: %w", err)
	}
	defer rows.Close()

	var result []model.SegmentLapRecord
	for rows.Next() {
		var r model.SegmentLapRecord
		if err := rows.Scan(
			&r.ID, &r.MessageIndex, &r.Timestamp, &r.StartTime,
			&r.Name, &r.UUID, &r.Status, &r.Manufacturer, &r.Sport, &r.SubSport,
			&r.Event, &r.EventType, &r.EventGroup, &r.SportEvent,
			&r.StartLat, &r.StartLng, &r.EndLat, &r.EndLng,
			&r.NecLat, &r.NecLng, &r.SwcLat, &r.SwcLng,
			&r.TotalElapsedTimeS, &r.TotalTimerTimeS, &r.TotalMovingTimeS, &r.ActiveTimeS,
			&r.TotalDistanceM, &r.TotalAscentM, &r.TotalDescentM,
			&r.TotalCycles, &r.TotalWorkJ,
			&r.AvgAltitudeM, &r.MaxAltitudeM, &r.MinAltitudeM,
			&r.TotalFractionalAscentM, &r.TotalFractionalDescentM,
			&r.AvgSpeedMs, &r.MaxSpeedMs,
			&r.AvgHeartRate, &r.MaxHeartRate, &r.MinHeartRate,
			&r.AvgCadence, &r.MaxCadence,
			&r.AvgFractionalCadence, &r.MaxFractionalCadence, &r.TotalFractionalCycles,
			&r.AvgPowerW, &r.MaxPowerW, &r.NormalizedPowerW, &r.LeftRightBalance,
			&r.AvgGradePct, &r.AvgPosGradePct, &r.AvgNegGradePct,
			&r.MaxPosGradePct, &r.MaxNegGradePct,
			&r.AvgPosVerticalSpeedMs, &r.AvgNegVerticalSpeedMs,
			&r.MaxPosVerticalSpeedMs, &r.MaxNegVerticalSpeedMs,
			&r.AvgTemperatureC, &r.MaxTemperatureC, &r.GpsAccuracyM,
			&r.TotalCalories, &r.TotalFatCalories,
			&r.AvgLeftTorqueEffectivenessPct, &r.AvgRightTorqueEffectivenessPct,
			&r.AvgLeftPedalSmoothnessPct, &r.AvgRightPedalSmoothnessPct,
			&r.AvgCombinedPedalSmoothnessPct,
			&r.AvgLeftPcoMm, &r.AvgRightPcoMm,
			&r.AvgLeftPowerPhaseStartDeg, &r.AvgLeftPowerPhaseEndDeg,
			&r.AvgLeftPowerPhasePeakStartDeg, &r.AvgLeftPowerPhasePeakEndDeg,
			&r.AvgRightPowerPhaseStartDeg, &r.AvgRightPowerPhaseEndDeg,
			&r.AvgRightPowerPhasePeakStartDeg, &r.AvgRightPowerPhasePeakEndDeg,
			&r.AvgPowerPositionW, &r.MaxPowerPositionW,
			&r.AvgCadencePosition, &r.MaxCadencePosition,
			&r.TimeStandingS, &r.StandCount,
			&r.FrontGearShiftCount, &r.RearGearShiftCount,
			&r.TotalGrit, &r.TotalFlow, &r.AvgGrit, &r.AvgFlow,
			&r.RepetitionNum, &r.WktStepIndex,
		); err != nil {
			return nil, fmt.Errorf("scan activity segment lap: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get activity segment laps: %w", err)
	}
	if result == nil {
		result = []model.SegmentLapRecord{}
	}
	return result, nil
}
