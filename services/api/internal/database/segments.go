package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func (db *DB) ListSegments(ctx context.Context, userID string, sportGroup, segmentType, name, bbox string) ([]model.Segment, error) {
	query := `
		SELECT s.id, s.created_by, s.name, s.description, s.sport_group,
		       s.allowed_sport_types, s.allowed_sub_types,
		       s.segment_type, s.segment_type_meta,
		       s.source_activity_id, s.osm_lift_id,
		       s.distance_m, s.elevation_gain_m,
		       s.effort_count, s.athlete_count,
		       s.visibility, s.route_hash, s.created_at,
		       ST_AsGeoJSON(sv.track) AS track_geojson
		FROM segments s
		LEFT JOIN LATERAL (
			SELECT track FROM segment_versions
			WHERE segment_id = s.id
			ORDER BY version_number DESC
			LIMIT 1
		) sv ON true
		WHERE (s.visibility = 'public' OR s.created_by = $1)`

	args := []any{userID}
	argIdx := 2

	if sportGroup != "" {
		query += fmt.Sprintf(" AND s.sport_group = $%d", argIdx)
		args = append(args, sportGroup)
		argIdx++
	}
	if segmentType != "" {
		query += fmt.Sprintf(" AND s.segment_type = $%d", argIdx)
		args = append(args, segmentType)
		argIdx++
	}
	if name != "" {
		query += fmt.Sprintf(" AND s.name ILIKE $%d", argIdx)
		args = append(args, "%"+name+"%")
		argIdx++
	}
	if bbox != "" {
		parts := strings.Split(bbox, ",")
		if len(parts) == 4 {
			query += fmt.Sprintf(
				` AND sv.track IS NOT NULL AND ST_Intersects(sv.track, ST_MakeEnvelope($%d, $%d, $%d, $%d, 4326))`,
				argIdx, argIdx+1, argIdx+2, argIdx+3,
			)
			args = append(args, parts[0], parts[1], parts[2], parts[3])
			argIdx += 4
		}
	}

	query += " ORDER BY s.created_at DESC LIMIT 200"

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list segments: %w", err)
	}
	defer rows.Close()

	var segments []model.Segment
	for rows.Next() {
		var s model.Segment
		err := rows.Scan(
			&s.ID, &s.CreatedBy, &s.Name, &s.Description, &s.SportGroup,
			&s.AllowedSportTypes, &s.AllowedSubTypes,
			&s.SegmentType, &s.SegmentTypeMeta,
			&s.SourceActivityID, &s.OsmLiftID,
			&s.DistanceM, &s.ElevationGainM,
			&s.EffortCount, &s.AthleteCount,
			&s.Visibility, &s.RouteHash, &s.CreatedAt,
			&s.TrackGeoJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("scan segment: %w", err)
		}
		segments = append(segments, s)
	}
	if segments == nil {
		segments = []model.Segment{}
	}
	return segments, nil
}

func (db *DB) CreateSegment(ctx context.Context, userID string, create *model.SegmentCreate) (*model.Segment, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	segmentTypeMeta := create.SegmentTypeMeta
	if segmentTypeMeta == nil {
		segmentTypeMeta = json.RawMessage("{}")
	}

	// Insert segment
	var s model.Segment
	err = tx.QueryRow(ctx, `
		INSERT INTO segments (created_by, name, description, sport_group, segment_type, segment_type_meta, source_activity_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_by, name, description, sport_group,
		          allowed_sport_types, allowed_sub_types,
		          segment_type, segment_type_meta,
		          source_activity_id, osm_lift_id,
		          distance_m, elevation_gain_m,
		          effort_count, athlete_count,
		          visibility, route_hash, created_at`,
		userID, create.Name, create.Description, create.SportGroup,
		create.SegmentType, segmentTypeMeta, create.SourceActivityID,
	).Scan(
		&s.ID, &s.CreatedBy, &s.Name, &s.Description, &s.SportGroup,
		&s.AllowedSportTypes, &s.AllowedSubTypes,
		&s.SegmentType, &s.SegmentTypeMeta,
		&s.SourceActivityID, &s.OsmLiftID,
		&s.DistanceM, &s.ElevationGainM,
		&s.EffortCount, &s.AthleteCount,
		&s.Visibility, &s.RouteHash, &s.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert segment: %w", err)
	}

	// Insert first version with track geometry
	var trackGeoJSON string
	err = tx.QueryRow(ctx, `
		INSERT INTO segment_versions (segment_id, version_number, track, start_point, end_point, valid_from, created_by)
		VALUES (
			$1, 1,
			ST_SetSRID(ST_GeomFromGeoJSON($2), 4326),
			ST_StartPoint(ST_SetSRID(ST_GeomFromGeoJSON($2), 4326)),
			ST_EndPoint(ST_SetSRID(ST_GeomFromGeoJSON($2), 4326)),
			CURRENT_DATE, $3
		)
		RETURNING ST_AsGeoJSON(track)`,
		s.ID, create.GeoJSONTrack, userID,
	).Scan(&trackGeoJSON)
	if err != nil {
		return nil, fmt.Errorf("insert segment version: %w", err)
	}
	s.TrackGeoJSON = &trackGeoJSON

	// Update distance_m via PostGIS
	_, err = tx.Exec(ctx, `
		UPDATE segments SET distance_m = (
			SELECT ST_Length(ST_Transform(track, 3857))
			FROM segment_versions WHERE segment_id = $1 AND version_number = 1
		) WHERE id = $1`, s.ID)
	if err != nil {
		return nil, fmt.Errorf("update distance: %w", err)
	}

	// Re-fetch distance
	tx.QueryRow(ctx, `SELECT distance_m FROM segments WHERE id = $1`, s.ID).Scan(&s.DistanceM)

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &s, nil
}

func (db *DB) GetSegment(ctx context.Context, segmentID string) (*model.Segment, error) {
	var s model.Segment
	err := db.Pool.QueryRow(ctx, `
		SELECT s.id, s.created_by, s.name, s.description, s.sport_group,
		       s.allowed_sport_types, s.allowed_sub_types,
		       s.segment_type, s.segment_type_meta,
		       s.source_activity_id, s.osm_lift_id,
		       s.distance_m, s.elevation_gain_m,
		       s.effort_count, s.athlete_count,
		       s.visibility, s.route_hash, s.created_at,
		       ST_AsGeoJSON(sv.track) AS track_geojson
		FROM segments s
		LEFT JOIN LATERAL (
			SELECT track FROM segment_versions
			WHERE segment_id = s.id
			ORDER BY version_number DESC
			LIMIT 1
		) sv ON true
		WHERE s.id = $1`, segmentID,
	).Scan(
		&s.ID, &s.CreatedBy, &s.Name, &s.Description, &s.SportGroup,
		&s.AllowedSportTypes, &s.AllowedSubTypes,
		&s.SegmentType, &s.SegmentTypeMeta,
		&s.SourceActivityID, &s.OsmLiftID,
		&s.DistanceM, &s.ElevationGainM,
		&s.EffortCount, &s.AthleteCount,
		&s.Visibility, &s.RouteHash, &s.CreatedAt,
		&s.TrackGeoJSON,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get segment: %w", err)
	}
	return &s, nil
}

func (db *DB) UpdateSegment(ctx context.Context, segmentID string, update *model.SegmentUpdate) (*model.Segment, error) {
	setClauses := []string{}
	args := []any{}
	argIdx := 1

	if update.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *update.Name)
		argIdx++
	}
	if update.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *update.Description)
		argIdx++
	}
	if update.SegmentTypeMeta != nil {
		setClauses = append(setClauses, fmt.Sprintf("segment_type_meta = $%d", argIdx))
		args = append(args, *update.SegmentTypeMeta)
		argIdx++
	}

	if len(setClauses) == 0 {
		return db.GetSegment(ctx, segmentID)
	}

	query := fmt.Sprintf("UPDATE segments SET %s WHERE id = $%d",
		strings.Join(setClauses, ", "), argIdx)
	args = append(args, segmentID)

	tag, err := db.Pool.Exec(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("update segment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, nil
	}

	return db.GetSegment(ctx, segmentID)
}

func (db *DB) DeleteSegment(ctx context.Context, userID, segmentID string) error {
	tag, err := db.Pool.Exec(ctx,
		`DELETE FROM segments WHERE id = $1 AND created_by = $2`,
		segmentID, userID)
	if err != nil {
		return fmt.Errorf("delete segment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("segment not found or not owned by user")
	}
	return nil
}

func (db *DB) GetSegmentLeaderboard(ctx context.Context, segmentID, sportType string, isReverse *bool, gearID string) ([]model.SegmentEffort, error) {
	query := `
		SELECT se.id, se.segment_id, se.segment_version_id, se.activity_id,
		       se.user_id, se.started_at, se.elapsed_time_s,
		       se.is_reverse, se.sport_type, se.sub_type, se.gear_id,
		       se.avg_power_w, se.avg_heart_rate, se.avg_speed_ms,
		       se.rank_overall, se.is_personal_best, se.is_segment_record,
		       se.in_leaderboard, se.created_at,
		       s.name AS segment_name,
		       u.display_name,
		       g.name AS gear_name
		FROM segment_efforts se
		JOIN segments s ON s.id = se.segment_id
		JOIN users u ON u.id = se.user_id
		LEFT JOIN gear g ON g.id = se.gear_id
		WHERE se.segment_id = $1 AND se.in_leaderboard = true`

	args := []any{segmentID}
	argIdx := 2

	if sportType != "" {
		query += fmt.Sprintf(" AND se.sport_type = $%d", argIdx)
		args = append(args, sportType)
		argIdx++
	}
	if isReverse != nil {
		query += fmt.Sprintf(" AND se.is_reverse = $%d", argIdx)
		args = append(args, *isReverse)
		argIdx++
	}
	if gearID != "" {
		query += fmt.Sprintf(" AND se.gear_id = $%d", argIdx)
		args = append(args, gearID)
		argIdx++
	}

	query += " ORDER BY se.elapsed_time_s ASC LIMIT 100"

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get leaderboard: %w", err)
	}
	defer rows.Close()

	var efforts []model.SegmentEffort
	for rows.Next() {
		var e model.SegmentEffort
		err := rows.Scan(
			&e.ID, &e.SegmentID, &e.SegmentVersionID, &e.ActivityID,
			&e.UserID, &e.StartedAt, &e.ElapsedTimeS,
			&e.IsReverse, &e.SportType, &e.SubType, &e.GearID,
			&e.AvgPowerW, &e.AvgHeartRate, &e.AvgSpeedMs,
			&e.RankOverall, &e.IsPersonalBest, &e.IsSegmentRecord,
			&e.InLeaderboard, &e.CreatedAt,
			&e.SegmentName, &e.DisplayName, &e.GearName,
		)
		if err != nil {
			return nil, fmt.Errorf("scan effort: %w", err)
		}
		efforts = append(efforts, e)
	}
	if efforts == nil {
		efforts = []model.SegmentEffort{}
	}
	return efforts, nil
}

func (db *DB) GetActivitySegmentEfforts(ctx context.Context, activityID string) ([]model.SegmentEffort, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT se.id, se.segment_id, se.segment_version_id, se.activity_id,
		       se.user_id, se.started_at, se.elapsed_time_s,
		       se.is_reverse, se.sport_type, se.sub_type, se.gear_id,
		       se.avg_power_w, se.avg_heart_rate, se.avg_speed_ms,
		       se.rank_overall, se.is_personal_best, se.is_segment_record,
		       se.in_leaderboard, se.created_at,
		       s.name AS segment_name,
		       NULL::text AS display_name,
		       g.name AS gear_name
		FROM segment_efforts se
		JOIN segments s ON s.id = se.segment_id
		LEFT JOIN gear g ON g.id = se.gear_id
		WHERE se.activity_id = $1
		ORDER BY se.started_at ASC`, activityID)
	if err != nil {
		return nil, fmt.Errorf("get activity segment efforts: %w", err)
	}
	defer rows.Close()

	var efforts []model.SegmentEffort
	for rows.Next() {
		var e model.SegmentEffort
		err := rows.Scan(
			&e.ID, &e.SegmentID, &e.SegmentVersionID, &e.ActivityID,
			&e.UserID, &e.StartedAt, &e.ElapsedTimeS,
			&e.IsReverse, &e.SportType, &e.SubType, &e.GearID,
			&e.AvgPowerW, &e.AvgHeartRate, &e.AvgSpeedMs,
			&e.RankOverall, &e.IsPersonalBest, &e.IsSegmentRecord,
			&e.InLeaderboard, &e.CreatedAt,
			&e.SegmentName, &e.DisplayName, &e.GearName,
		)
		if err != nil {
			return nil, fmt.Errorf("scan effort: %w", err)
		}
		efforts = append(efforts, e)
	}
	if efforts == nil {
		efforts = []model.SegmentEffort{}
	}
	return efforts, nil
}

func (db *DB) InsertSegmentEffort(ctx context.Context, effort *model.SegmentEffort) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO segment_efforts (
			segment_id, segment_version_id, activity_id, user_id,
			started_at, elapsed_time_s, is_reverse, sport_type, sub_type,
			gear_id, avg_power_w, avg_heart_rate, avg_speed_ms
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		effort.SegmentID, effort.SegmentVersionID, effort.ActivityID, effort.UserID,
		effort.StartedAt, effort.ElapsedTimeS, effort.IsReverse, effort.SportType, effort.SubType,
		effort.GearID, effort.AvgPowerW, effort.AvgHeartRate, effort.AvgSpeedMs,
	)
	if err != nil {
		return fmt.Errorf("insert segment effort: %w", err)
	}
	return nil
}

func (db *DB) UpdateSegmentEffortRanks(ctx context.Context, segmentID string) error {
	// Re-rank all efforts for this segment, grouped by (sport_type, is_reverse)
	_, err := db.Pool.Exec(ctx, `
		WITH ranked AS (
			SELECT id, started_at,
			       ROW_NUMBER() OVER (
			           PARTITION BY sport_type, is_reverse
			           ORDER BY elapsed_time_s ASC
			       ) AS rk,
			       ROW_NUMBER() OVER (
			           PARTITION BY sport_type, is_reverse, user_id
			           ORDER BY elapsed_time_s ASC
			       ) AS user_rk,
			       CASE WHEN ROW_NUMBER() OVER (
			           PARTITION BY sport_type, is_reverse
			           ORDER BY elapsed_time_s ASC
			       ) = 1 THEN true ELSE false END AS is_record
			FROM segment_efforts
			WHERE segment_id = $1 AND in_leaderboard = true
		)
		UPDATE segment_efforts se
		SET rank_overall = r.rk,
		    is_personal_best = (r.user_rk = 1),
		    is_segment_record = r.is_record
		FROM ranked r
		WHERE se.id = r.id AND se.started_at = r.started_at`, segmentID)
	if err != nil {
		return fmt.Errorf("update effort ranks: %w", err)
	}

	// Update effort_count and athlete_count on the segment
	_, err = db.Pool.Exec(ctx, `
		UPDATE segments SET
			effort_count = (SELECT COUNT(*) FROM segment_efforts WHERE segment_id = $1),
			athlete_count = (SELECT COUNT(DISTINCT user_id) FROM segment_efforts WHERE segment_id = $1)
		WHERE id = $1`, segmentID)
	if err != nil {
		return fmt.Errorf("update segment counts: %w", err)
	}

	return nil
}

// MatchableSegment holds the info needed for segment matching.
type MatchableSegment struct {
	SegmentID  string
	VersionID  string
	SportGroup string
}

// GetMatchableSegments returns segments that match the given sport type and are visible to the user.
func (db *DB) GetMatchableSegments(ctx context.Context, userID, sportType string) ([]MatchableSegment, error) {
	sportGroup := sportGroupFromSportType(sportType)

	rows, err := db.Pool.Query(ctx, `
		SELECT s.id, sv.id, s.sport_group
		FROM segments s
		JOIN LATERAL (
			SELECT id, track FROM segment_versions
			WHERE segment_id = s.id
			ORDER BY version_number DESC
			LIMIT 1
		) sv ON true
		WHERE (s.visibility = 'public' OR s.created_by = $1)
		  AND (s.sport_group = $2 OR s.sport_group = 'generic')`,
		userID, sportGroup)
	if err != nil {
		return nil, fmt.Errorf("get matchable segments: %w", err)
	}
	defer rows.Close()

	var result []MatchableSegment
	for rows.Next() {
		var si MatchableSegment
		if err := rows.Scan(&si.SegmentID, &si.VersionID, &si.SportGroup); err != nil {
			return nil, fmt.Errorf("scan matchable segment: %w", err)
		}
		result = append(result, si)
	}
	return result, nil
}

// GetActivityTimingForSegment returns the elapsed time in seconds and started_at for an activity.
func (db *DB) GetActivityTimingForSegment(ctx context.Context, activityID string) (int, time.Time, error) {
	var elapsedS *int
	var startedAt time.Time
	err := db.Pool.QueryRow(ctx,
		`SELECT elapsed_time_s, started_at FROM activities WHERE id = $1`, activityID,
	).Scan(&elapsedS, &startedAt)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("get activity timing: %w", err)
	}
	elapsed := 0
	if elapsedS != nil {
		elapsed = *elapsedS
	}
	return elapsed, startedAt, nil
}

// MatchSegmentToActivity checks if an activity track matches a segment track using Hausdorff distance.
// Returns the distance in meters; a match is typically < 30m.
func (db *DB) MatchSegmentToActivity(ctx context.Context, activityID, segmentVersionID string) (*float64, error) {
	var distance *float64
	err := db.Pool.QueryRow(ctx, `
		SELECT ST_HausdorffDistance(
			ST_Transform(at.track_full, 3857),
			ST_Transform(sv.track, 3857)
		)
		FROM activity_tracks at,
		     segment_versions sv
		WHERE at.activity_id = $1
		  AND sv.id = $2
		  AND at.track_full IS NOT NULL`,
		activityID, segmentVersionID,
	).Scan(&distance)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("match segment: %w", err)
	}
	return distance, nil
}

func sportGroupFromSportType(sportType string) string {
	switch sportType {
	case "cycling", "e_biking", "mountain_biking", "gravel_cycling", "indoor_cycling":
		return "bike"
	case "running", "trail_running", "hiking", "walking", "treadmill_running":
		return "foot"
	case "swimming", "open_water_swimming", "lap_swimming":
		return "swim"
	case "alpine_skiing", "cross_country_skiing", "backcountry_skiing", "snowboarding":
		return "ski"
	case "kayaking", "stand_up_paddleboarding", "rowing":
		return "paddle"
	default:
		return "generic"
	}
}
