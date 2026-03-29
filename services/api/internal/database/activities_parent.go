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
	var isEbike bool

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
			tss, trimp, is_indoor, is_ebike, has_gps,
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
			-- Start/End Location ($30-$33)
			CASE WHEN $30::float8 != 0 THEN ST_SetSRID(ST_MakePoint($31::float8, $30::float8), 4326) ELSE NULL END,
			CASE WHEN $32::float8 != 0 THEN ST_SetSRID(ST_MakePoint($33::float8, $32::float8), 4326) ELSE NULL END,
			-- File / Source ($34-$37)
			$34, $35,
			$36, $37,
			-- Parent ID ($38)
			$38,
			-- Visibility / Conflict ($39-$40)
			$39,
			$40,
			-- FIT SDK: Workout ($41)
			$41,
			-- FIT SDK: Cycles/Work/Calories ($42-$46)
			$42, $43, $44, $45, $46,
			-- FIT SDK: Swimming ($47-$55)
			$47, $48, $49,
			$50, $51, $52, $53,
			$54, $55,
			-- FIT SDK: Fractional Cadence ($56-$58)
			$56, $57, $58,
			-- FIT SDK: Running Dynamics ($59-$64)
			$59, $60, $61,
			$62, $63, $64,
			-- FIT SDK: Grade ($65-$69)
			$65, $66, $67, $68, $69,
			-- FIT SDK: Vertical Speed ($70-$74)
			$70, $71,
			$72, $73, $74,
			-- FIT SDK: Altitude / Fractional ($75-$77)
			$75, $76, $77,
			-- FIT SDK: Pedal Torque/Smoothness ($78-$82)
			$78, $79,
			$80, $81,
			$82,
			-- FIT SDK: PCO ($83-$84)
			$83, $84,
			-- FIT SDK: Power Phase Left ($85-$88)
			$85, $86,
			$87, $88,
			-- FIT SDK: Power Phase Right ($89-$92)
			$89, $90,
			$91, $92,
			-- FIT SDK: Power/Cadence Position ($93-$96)
			$93, $94,
			$95, $96,
			-- FIT SDK: Standing ($97-$98)
			$97, $98,
			-- FIT SDK: LEV/E-Bike ($99-$101)
			$99, $100, $101,
			-- FIT SDK: Grit/Flow ($102-$106)
			$102, $103, $104, $105,
			$106,
			-- FIT SDK: Ball Sports ($107-$111)
			$107, $108, $109, $110, $111,
			-- FIT SDK: Hemoglobin ($112-$117)
			$112, $113, $114,
			$115, $116, $117,
			-- FIT SDK: Diving ($118-$126)
			$118, $119, $120, $121,
			$122, $123, $124, $125, $126,
			-- FIT SDK: Respiration/HRV/Core ($127-$131)
			$127, $128, $129,
			$130, $131,
			-- FIT SDK: Lap Stats ($132-$135)
			$132, $133, $134, $135,
			-- FIT SDK: Intensity/Balance/Profile ($136-$139)
			$136, $137, $138, $139,
			-- Bounding Box ($140-$143)
			$140, $141, $142, $143
		)
		RETURNING id, is_ebike, created_at, updated_at`,
		userID, nilIfEmpty(data.FileHash), data.SportType, nilIfEmpty(data.SubType), // $1-$4
		nilIfZeroI16(data.FitSportID), nilIfZeroI16(data.FitSubSportID), // $5-$6
		data.StartedAt, nilIfZeroTime(data.EndedAt), nilIfZeroInt(data.ElapsedTimeS), nilIfZeroInt(data.MovingTimeS), // $7-$10
		nilIfZeroF64(data.DistanceM), nilIfZeroF64(data.ElevationGainM), nilIfZeroF64(data.ElevationLossM), // $11-$13
		nilIfZeroF64(data.MaxElevationM), nilIfZeroF64(data.MinElevationM), // $14-$15
		nilIfZeroF64(data.AvgSpeedMS), nilIfZeroF64(data.MaxSpeedMS), // $16-$17
		nilIfZeroF64(data.AvgHeartRate), nilIfZeroInt(data.MaxHeartRate), // $18-$19
		nilIfZeroF64(data.AvgPowerW), nilIfZeroInt(data.MaxPowerW), nilIfZeroF64(data.NormalizedPowerW), // $20-$22
		nilIfZeroF64(data.AvgCadence), nilIfZeroF64(data.AvgTemperatureC), // $23-$24
		nilIfZeroF64(data.TSS), nilIfZeroF64(data.TRIMP), data.IsIndoor, data.IsEbike, data.HasGPS, // $25-$29
		data.StartLat, data.StartLng, // $30-$31
		data.EndLat, data.EndLng, // $32-$33
		nilIfEmpty(data.OriginalName), nilIfEmpty(data.FileFormat), // $34-$35
		nilIfEmpty(data.Source), nilIfEmpty(data.SourceDevice), // $36-$37
		parentID,                             // $38
		visibilityOrDefault(data.Visibility), // $39
		data.SportConflictPending,            // $40
		// FIT SDK new fields
		data.WorkoutID,                                                                                          // $41
		data.TotalCycles, data.TotalWorkJ, data.TotalFatCalories, data.MetabolicCalories, data.TotalMovingTimeS, // $42-$46
		data.SwimStroke, data.PoolLengthM, data.PoolLengthUnit, // $47-$49
		data.NumLengths, data.NumActiveLengths, data.AvgStrokeCount, data.AvgStrokeDistanceM, // $50-$53
		data.StrokeCount, data.ZoneCount, // $54-$55
		data.AvgFractionalCadence, data.MaxFractionalCadence, data.TotalFractionalCycles, // $56-$58
		data.AvgVerticalOscillationMm, data.AvgStanceTimeMs, data.AvgStanceTimePct, // $59-$61
		data.AvgStanceTimeBalancePct, data.AvgStepLengthMm, data.AvgVerticalRatioPct, // $62-$64
		data.AvgGradePct, data.AvgPosGradePct, data.AvgNegGradePct, data.MaxPosGradePct, data.MaxNegGradePct, // $65-$69
		data.AvgPosVerticalSpeedMs, data.AvgNegVerticalSpeedMs, // $70-$71
		data.MaxPosVerticalSpeedMs, data.MaxNegVerticalSpeedMs, data.AvgVamMs, // $72-$74
		data.AvgAltitudeM, data.TotalFractionalAscentM, data.TotalFractionalDescentM, // $75-$77
		data.AvgLeftTorqueEffectivenessPct, data.AvgRightTorqueEffectivenessPct, // $78-$79
		data.AvgLeftPedalSmoothnessPct, data.AvgRightPedalSmoothnessPct, // $80-$81
		data.AvgCombinedPedalSmoothnessPct,    // $82
		data.AvgLeftPcoMm, data.AvgRightPcoMm, // $83-$84
		data.AvgLeftPowerPhaseStartDeg, data.AvgLeftPowerPhaseEndDeg, // $85-$86
		data.AvgLeftPowerPhasePeakStartDeg, data.AvgLeftPowerPhasePeakEndDeg, // $87-$88
		data.AvgRightPowerPhaseStartDeg, data.AvgRightPowerPhaseEndDeg, // $89-$90
		data.AvgRightPowerPhasePeakStartDeg, data.AvgRightPowerPhasePeakEndDeg, // $91-$92
		data.AvgPowerPositionW, data.MaxPowerPositionW, // $93-$94
		data.AvgCadencePosition, data.MaxCadencePosition, // $95-$96
		data.TimeStandingS, data.StandCount, // $97-$98
		data.AvgLevMotorPowerW, data.MaxLevMotorPowerW, data.LevBatteryConsumptionPct, // $99-$101
		data.TotalGrit, data.TotalFlow, data.AvgGrit, data.AvgFlow, // $102-$105
		data.JumpCount,                                                                                    // $106
		data.MaxBallSpeedMs, data.AvgBallSpeedMs, data.PlayerScore, data.OpponentScore, data.OpponentName, // $107-$111
		data.AvgTotalHemoglobinConc, data.MinTotalHemoglobinConc, data.MaxTotalHemoglobinConc, // $112-$114
		data.AvgSaturatedHemoglobinPct, data.MinSaturatedHemoglobinPct, data.MaxSaturatedHemoglobinPct, // $115-$117
		data.AvgDepthM, data.MaxDepthM, data.SurfaceIntervalS, data.DiveNumber, // $118-$121
		data.StartCnsPct, data.EndCnsPct, data.StartN2Pct, data.EndN2Pct, data.O2ToxicityOtu, // $122-$126
		data.MaxRespirationRate, data.MinRespirationRate, data.SdrrHrvMs, // $127-$129
		data.MinCoreTemperatureC, data.MaxCoreTemperatureC, // $130-$131
		data.AvgLapTimeS, data.BestLapIndex, data.NumLaps, data.FirstLapIndex, // $132-$135
		data.IntensityFactor, data.LeftRightBalance, data.SportProfileName, data.SportIndex, // $136-$139
		data.NecLat, data.NecLng, data.SwcLat, data.SwcLng, // $140-$143
	).Scan(&id, &isEbike, &createdAt, &updatedAt)
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
		IsEbike:              isEbike,
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
		       is_indoor, has_gps, enrichment_status, visibility,
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
			&a.IsIndoor, &a.HasGPS, &a.EnrichmentStatus, &a.Visibility,
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
