package database

import (
	"context"
	"fmt"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// InsertMultisportParent inserts a bare parent activity for a multisport session.
// The parent holds no metrics itself; metrics are aggregated from children via
// AggregateParentMetrics after all children are inserted.
// file_hash is stored so re-imports of the same file can detect the existing parent.
func (db *DB) InsertMultisportParent(ctx context.Context, act *model.Activity) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, sport_type, title, started_at, file_hash, visibility, enrichment_status)
		VALUES ($1, $2, $3, $4, $5, $6, 'private', 'complete')`,
		act.ID, act.UserID, act.SportType, act.Title, act.StartedAt, act.FileHash)
	return err
}

// FindMultisportParentByHash checks if a multisport parent with the given file_hash exists.
// Returns the parent's activity ID if found, empty string otherwise.
func (db *DB) FindMultisportParentByHash(ctx context.Context, userID, fileHash string) (string, error) {
	var id string
	err := db.Pool.QueryRow(ctx, `
		SELECT id FROM activities
		WHERE user_id = $1 AND file_hash = $2 AND sport_type = 'multisport' AND deleted_at IS NULL
		LIMIT 1`, userID, fileHash).Scan(&id)
	if err != nil {
		return "", nil // not found is not an error
	}
	return id, nil
}

// InsertActivityWithParent inserts an activity and links it to a multisport parent.
// Mirrors InsertActivity but adds parent_id to the INSERT.
func (db *DB) InsertActivityWithParent(ctx context.Context, userID string, data *model.ActivityData, parentID string) (*model.Activity, error) {
	var id string
	var createdAt, updatedAt time.Time
	var isElectric bool

	// Re-use the same column set as InsertActivity, adding parent_id.
	err := db.Pool.QueryRow(ctx, `
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
			parent_id,
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
			nec_lat, nec_lng, swc_lat, swc_lng
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
			-- Virtual / stats flags ($30-$32)
			$30, $31, $32,
			-- Start/End Location ($33-$36)
			CASE WHEN $33::float8 != 0 THEN ST_SetSRID(ST_MakePoint($34::float8, $33::float8), 4326) ELSE NULL END,
			CASE WHEN $35::float8 != 0 THEN ST_SetSRID(ST_MakePoint($36::float8, $35::float8), 4326) ELSE NULL END,
			-- File / Source ($37-$40)
			$37, $38,
			$39, $40,
			-- Parent ID ($41)
			$41,
			-- Visibility / Conflict ($42-$43)
			$42,
			$43,
			-- FIT SDK: Workout ($44)
			$44,
			-- FIT SDK: Cycles/Work/Calories ($45-$49)
			$45, $46, $47, $48, $49,
			-- FIT SDK: Swimming ($50-$58)
			$50, $51, $52,
			$53, $54, $55, $56,
			$57, $58,
			-- FIT SDK: Fractional Cadence ($59-$61)
			$59, $60, $61,
			-- FIT SDK: Running Dynamics ($62-$67)
			$62, $63, $64,
			$65, $66, $67,
			-- FIT SDK: Grade ($68-$72)
			$68, $69, $70, $71, $72,
			-- FIT SDK: Vertical Speed ($73-$77)
			$73, $74,
			$75, $76, $77,
			-- FIT SDK: Altitude / Fractional ($78-$80)
			$78, $79, $80,
			-- FIT SDK: Pedal Torque/Smoothness ($81-$85)
			$81, $82,
			$83, $84,
			$85,
			-- FIT SDK: PCO ($86-$87)
			$86, $87,
			-- FIT SDK: Power Phase Left ($88-$91)
			$88, $89,
			$90, $91,
			-- FIT SDK: Power Phase Right ($92-$95)
			$92, $93,
			$94, $95,
			-- FIT SDK: Power/Cadence Position ($96-$99)
			$96, $97,
			$98, $99,
			-- FIT SDK: Standing ($100-$101)
			$100, $101,
			-- FIT SDK: LEV/E-Bike ($102-$104)
			$102, $103, $104,
			-- FIT SDK: Grit/Flow ($105-$109)
			$105, $106, $107, $108,
			$109,
			-- FIT SDK: Ball Sports ($110-$114)
			$110, $111, $112, $113, $114,
			-- FIT SDK: Hemoglobin ($115-$120)
			$115, $116, $117,
			$118, $119, $120,
			-- FIT SDK: Diving ($121-$129)
			$121, $122, $123, $124,
			$125, $126, $127, $128, $129,
			-- FIT SDK: Respiration/HRV/Core ($130-$134)
			$130, $131, $132,
			$133, $134,
			-- FIT SDK: Lap Stats ($135-$138)
			$135, $136, $137, $138,
			-- FIT SDK: Intensity/Balance/Profile ($139-$142)
			$139, $140, $141, $142,
			-- Bounding Box ($143-$146)
			$143, $144, $145, $146
		)
		RETURNING id, is_electric, created_at, updated_at`,
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
		data.IsVirtual, data.IsVirtual, false, // $30-$32
		data.StartLat, data.StartLng, // $33-$34
		data.EndLat, data.EndLng, // $35-$36
		nilIfEmpty(data.OriginalName), nilIfEmpty(data.FileFormat), // $37-$38
		nilIfEmpty(data.Source), nilIfEmpty(data.SourceDevice), // $39-$40
		parentID,                             // $41
		visibilityOrDefault(data.Visibility), // $42
		data.SportConflictPending,            // $43
		// FIT SDK new fields
		data.WorkoutID,                                                                                          // $44
		data.TotalCycles, data.TotalWorkJ, data.TotalFatCalories, data.MetabolicCalories, data.TotalMovingTimeS, // $45-$49
		data.SwimStroke, data.PoolLengthM, data.PoolLengthUnit, // $50-$52
		data.NumLengths, data.NumActiveLengths, data.AvgStrokeCount, data.AvgStrokeDistanceM, // $53-$56
		data.StrokeCount, data.ZoneCount, // $57-$58
		data.AvgFractionalCadence, data.MaxFractionalCadence, data.TotalFractionalCycles, // $59-$61
		data.AvgVerticalOscillationMm, data.AvgStanceTimeMs, data.AvgStanceTimePct, // $62-$64
		data.AvgStanceTimeBalancePct, data.AvgStepLengthMm, data.AvgVerticalRatioPct, // $65-$67
		data.AvgGradePct, data.AvgPosGradePct, data.AvgNegGradePct, data.MaxPosGradePct, data.MaxNegGradePct, // $68-$72
		data.AvgPosVerticalSpeedMs, data.AvgNegVerticalSpeedMs, // $73-$74
		data.MaxPosVerticalSpeedMs, data.MaxNegVerticalSpeedMs, data.AvgVamMs, // $75-$77
		data.AvgAltitudeM, data.TotalFractionalAscentM, data.TotalFractionalDescentM, // $78-$80
		data.AvgLeftTorqueEffectivenessPct, data.AvgRightTorqueEffectivenessPct, // $81-$82
		data.AvgLeftPedalSmoothnessPct, data.AvgRightPedalSmoothnessPct, // $83-$84
		data.AvgCombinedPedalSmoothnessPct,    // $85
		data.AvgLeftPcoMm, data.AvgRightPcoMm, // $86-$87
		data.AvgLeftPowerPhaseStartDeg, data.AvgLeftPowerPhaseEndDeg, // $88-$89
		data.AvgLeftPowerPhasePeakStartDeg, data.AvgLeftPowerPhasePeakEndDeg, // $90-$91
		data.AvgRightPowerPhaseStartDeg, data.AvgRightPowerPhaseEndDeg, // $92-$93
		data.AvgRightPowerPhasePeakStartDeg, data.AvgRightPowerPhasePeakEndDeg, // $94-$95
		data.AvgPowerPositionW, data.MaxPowerPositionW, // $96-$97
		data.AvgCadencePosition, data.MaxCadencePosition, // $98-$99
		data.TimeStandingS, data.StandCount, // $100-$101
		data.AvgLevMotorPowerW, data.MaxLevMotorPowerW, data.LevBatteryConsumptionPct, // $102-$104
		data.TotalGrit, data.TotalFlow, data.AvgGrit, data.AvgFlow, // $105-$108
		data.JumpCount,                                                                                    // $109
		data.MaxBallSpeedMs, data.AvgBallSpeedMs, data.PlayerScore, data.OpponentScore, data.OpponentName, // $110-$114
		data.AvgTotalHemoglobinConc, data.MinTotalHemoglobinConc, data.MaxTotalHemoglobinConc, // $115-$117
		data.AvgSaturatedHemoglobinPct, data.MinSaturatedHemoglobinPct, data.MaxSaturatedHemoglobinPct, // $118-$120
		data.AvgDepthM, data.MaxDepthM, data.SurfaceIntervalS, data.DiveNumber, // $121-$124
		data.StartCnsPct, data.EndCnsPct, data.StartN2Pct, data.EndN2Pct, data.O2ToxicityOtu, // $125-$129
		data.MaxRespirationRate, data.MinRespirationRate, data.SdrrHrvMs, // $130-$132
		data.MinCoreTemperatureC, data.MaxCoreTemperatureC, // $133-$134
		data.AvgLapTimeS, data.BestLapIndex, data.NumLaps, data.FirstLapIndex, // $135-$138
		data.IntensityFactor, data.LeftRightBalance, data.SportProfileName, data.SportIndex, // $139-$142
		data.NecLat, data.NecLng, data.SwcLat, data.SwcLng, // $143-$146
	).Scan(&id, &isElectric, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert activity with parent: %w", err)
	}

	act := &model.Activity{
		ID:                   id,
		UserID:               userID,
		SportType:            data.SportType,
		SubType:              ptrString(data.SubType),
		Source:               ptrString(data.Source),
		SourceDevice:         ptrString(data.SourceDevice),
		StartedAt:            data.StartedAt,
		IsElectric:           isElectric,
		IsVirtual:            data.IsVirtual,
		ExcludeFromHeatmap:   data.IsVirtual,
		IsIndoor:             data.IsIndoor,
		HasGPS:               data.HasGPS,
		NeedsClassification:  data.SportType == "unknown",
		OriginalFilename:     ptrString(data.OriginalName),
		Visibility:           visibilityOrDefault(data.Visibility),
		EnrichmentStatus:     "pending",
		ParentID:             &parentID,
		SportConflictPending: data.SportConflictPending,
	}
	return act, nil
}

// GetChildActivities returns all child activities for a given parent ID.
func (db *DB) GetChildActivities(ctx context.Context, parentID string) ([]model.Activity, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, user_id, sport_type, sub_type, title, title_auto, started_at,
		       elapsed_time_s, moving_time_s, distance_m, elevation_gain_m,
		       elevation_loss_m, avg_heart_rate, avg_power_w, parent_id,
		       is_indoor, is_electric, is_virtual, exclude_from_heatmap, has_gps, enrichment_status, visibility,
		       COALESCE(tags, '{}'),
		       file_hash, file_id, original_filename, original_file_format
		FROM activities
		WHERE parent_id = $1
		ORDER BY started_at`, parentID)
	if err != nil {
		return nil, fmt.Errorf("get child activities: %w", err)
	}
	defer rows.Close()

	var activities []model.Activity
	for rows.Next() {
		var a model.Activity
		if err := rows.Scan(
			&a.ID, &a.UserID, &a.SportType, &a.SubType, &a.Title, &a.TitleAuto, &a.StartedAt,
			&a.ElapsedTimeS, &a.MovingTimeS, &a.DistanceM, &a.ElevationGainM,
			&a.ElevationLossM, &a.AvgHeartRate, &a.AvgPowerW, &a.ParentID,
			&a.IsIndoor, &a.IsElectric, &a.IsVirtual, &a.ExcludeFromHeatmap, &a.HasGPS, &a.EnrichmentStatus, &a.Visibility,
			&a.Tags,
			&a.FileHash, &a.FileID, &a.OriginalFilename, &a.OriginalFileFormat,
		); err != nil {
			return nil, fmt.Errorf("scan child activity: %w", err)
		}
		if a.Tags == nil {
			a.Tags = []string{}
		}
		activities = append(activities, a)
	}
	return activities, rows.Err()
}

// GetActivitiesByIDs loads multiple activities and verifies ownership via user_id.
func (db *DB) GetActivitiesByIDs(ctx context.Context, userID string, ids []string) ([]model.Activity, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT id, user_id, sport_type, title, started_at,
		        elapsed_time_s, moving_time_s, distance_m, elevation_gain_m, parent_id
		 FROM activities
		 WHERE id = ANY($1::uuid[]) AND user_id = $2`,
		ids, userID)
	if err != nil {
		return nil, fmt.Errorf("get activities by ids: %w", err)
	}
	defer rows.Close()
	var result []model.Activity
	for rows.Next() {
		var a model.Activity
		if err := rows.Scan(
			&a.ID, &a.UserID, &a.SportType, &a.Title, &a.StartedAt,
			&a.ElapsedTimeS, &a.MovingTimeS, &a.DistanceM, &a.ElevationGainM, &a.ParentID,
		); err != nil {
			return nil, fmt.Errorf("scan activity: %w", err)
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

// SetActivitiesParent sets parent_id on multiple activities (with ownership check).
func (db *DB) SetActivitiesParent(ctx context.Context, userID string, activityIDs []string, parentID string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE activities SET parent_id = $1
		 WHERE id = ANY($2::uuid[]) AND user_id = $3`,
		parentID, activityIDs, userID)
	return err
}

// AggregateParentMetrics updates a parent activity's metrics by summing its children's values.
func (db *DB) AggregateParentMetrics(ctx context.Context, parentID string) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE activities SET
			elapsed_time_s  = (SELECT SUM(elapsed_time_s)   FROM activities WHERE parent_id = $1 AND deleted_at IS NULL),
			moving_time_s   = (SELECT SUM(moving_time_s)    FROM activities WHERE parent_id = $1 AND deleted_at IS NULL),
			distance_m      = (SELECT SUM(distance_m)       FROM activities WHERE parent_id = $1 AND deleted_at IS NULL),
			elevation_gain_m = (SELECT SUM(elevation_gain_m) FROM activities WHERE parent_id = $1 AND deleted_at IS NULL),
			elevation_loss_m = (SELECT SUM(elevation_loss_m) FROM activities WHERE parent_id = $1 AND deleted_at IS NULL),
			total_calories  = (SELECT SUM(total_calories)   FROM activities WHERE parent_id = $1 AND deleted_at IS NULL)
		WHERE id = $1`, parentID)
	return err
}
