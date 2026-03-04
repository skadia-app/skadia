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
				activity_id, message_index, start_time, end_time,
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
				avg_altitude_m, max_altitude_m, min_altitude_m
			) VALUES (
				$1, $2, $3, $4,
				$5, $6, $7,
				$8, $9, $10,
				$11, $12, $13, $14,
				$15, $16,
				$17, $18, $19,
				$20, $21,
				$22, $23, $24,
				$25,
				$26, $27, $28, $29,
				$30, $31, $32,
				$33, $34,
				$35, $36, $37
			)`,
			activityID, l.MessageIndex, nilIfZeroTime(l.StartTime), nilIfZeroTime(l.EndTime),
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
			avg_altitude_m, max_altitude_m, min_altitude_m
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
				swim_stroke, length_type
			) VALUES (
				$1, $2, $3, $4,
				$5, $6,
				$7, $8,
				$9, $10,
				$11, $12
			)`,
			activityID, l.MessageIndex, nilIfZeroTime(l.StartTime), nilIfZeroTime(l.EndTime),
			nilIfZeroF64(l.TotalElapsedTimeS), nilIfZeroF64(l.TotalTimerTimeS),
			nilIfZeroInt(l.TotalStrokes), nilIfZeroInt(l.TotalCalories),
			nilIfZeroF64(l.AvgSpeedMS), nilIfZeroInt(l.AvgSwimmingCadence),
			nilIfEmpty(l.SwimStroke), nilIfEmpty(l.LengthType),
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
