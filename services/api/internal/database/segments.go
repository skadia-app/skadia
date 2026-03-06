package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

func (db *DB) ListSegments(ctx context.Context, userID string, sportGroup, segmentType, name, bbox string) ([]model.Segment, error) {
	query := `
		SELECT s.id, s.created_by, s.name, s.description, s.sport_group,
		       s.allowed_sport_types, s.allowed_sub_types,
		       s.segment_type, s.segment_type_meta,
		       s.source_activity_id, s.osm_lift_id,
		       s.distance_m, s.elevation_gain_m,
		       s.effort_count, s.athlete_count,
		       COALESCE((
		           SELECT COUNT(*)::int FROM segment_efforts se
		           JOIN activities a ON se.activity_id = a.id
		           WHERE se.segment_id = s.id AND a.user_id = $1
		       ), 0) AS ridden_count,
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
			&s.RiddenCount,
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

	// For lap-type segments: auto-close if start≈end (≤50m), error if >50m
	trackGeoJSON := create.GeoJSONTrack
	if create.SegmentType == "lap" {
		var gapM float64
		err = tx.QueryRow(ctx, `
			SELECT ST_Distance(
				ST_StartPoint(ST_SetSRID(ST_GeomFromGeoJSON($1), 4326))::geography,
				ST_EndPoint(ST_SetSRID(ST_GeomFromGeoJSON($1), 4326))::geography
			)`, create.GeoJSONTrack).Scan(&gapM)
		if err != nil {
			return nil, fmt.Errorf("check lap gap: %w", err)
		}
		if gapM > 50 {
			return nil, fmt.Errorf("lap segment must be a closed loop; start-end gap is %.0fm (max 50m)", gapM)
		}
		// Auto-close: snap end point to start point
		if gapM > 0.1 {
			err = tx.QueryRow(ctx, `
				SELECT ST_AsGeoJSON(
					ST_AddPoint(
						ST_SetSRID(ST_GeomFromGeoJSON($1), 4326),
						ST_StartPoint(ST_SetSRID(ST_GeomFromGeoJSON($1), 4326))
					)
				)`, create.GeoJSONTrack).Scan(&trackGeoJSON)
			if err != nil {
				return nil, fmt.Errorf("auto-close lap track: %w", err)
			}
		}
	}

	// Insert first version with track geometry
	var returnedTrackGeoJSON string
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
		s.ID, trackGeoJSON, userID,
	).Scan(&returnedTrackGeoJSON)
	if err != nil {
		return nil, fmt.Errorf("insert segment version: %w", err)
	}
	s.TrackGeoJSON = &returnedTrackGeoJSON

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
	// If new_version requested: copy latest segment_version to create a new one
	if update.NewVersion {
		if err := db.createNewSegmentVersion(ctx, segmentID); err != nil {
			return nil, err
		}
	}

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

// createNewSegmentVersion copies the latest segment_version into a new version row.
func (db *DB) createNewSegmentVersion(ctx context.Context, segmentID string) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO segment_versions (segment_id, version_number, track, start_point, end_point, valid_from, created_by, segment_type_meta)
		SELECT segment_id,
		       (SELECT COALESCE(MAX(version_number), 0) + 1 FROM segment_versions WHERE segment_id = $1),
		       track, start_point, end_point, CURRENT_DATE, created_by, segment_type_meta
		FROM segment_versions
		WHERE segment_id = $1
		ORDER BY version_number DESC
		LIMIT 1`, segmentID)
	if err != nil {
		return fmt.Errorf("create new segment version: %w", err)
	}
	return nil
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
		       g.name AS gear_name,
		       s.segment_type_meta
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
			&e.SegmentTypeMeta,
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

// CandidateSegment holds info for spatial pre-filtered segment matching.
type CandidateSegment struct {
	SegmentID   string
	VersionID   string
	SportGroup  string
	DistanceM   float64
	SegmentType string
	TypeMeta    map[string]interface{}
}

// GetSpatialCandidateSegments returns segments within 50m of the activity track,
// filtered by sport group. Uses ST_DWithin with GIST index for fast pre-filtering.
func (db *DB) GetSpatialCandidateSegments(ctx context.Context, activityID, userID, sportType string) ([]CandidateSegment, error) {
	sportGroup := sportGroupFromSportType(sportType)

	rows, err := db.Pool.Query(ctx, `
		SELECT s.id, sv.id, s.sport_group, COALESCE(s.distance_m, 0),
		       s.segment_type, COALESCE(sv.segment_type_meta, '{}')
		FROM segments s
		JOIN LATERAL (
			SELECT id, track, segment_type_meta FROM segment_versions
			WHERE segment_id = s.id
			ORDER BY version_number DESC
			LIMIT 1
		) sv ON true
		JOIN activity_tracks at ON at.activity_id = $1
		WHERE (s.visibility = 'public' OR s.created_by = $2)
		  AND (s.sport_group = $3 OR s.sport_group = 'generic')
		  AND at.track_full IS NOT NULL
		  AND sv.track IS NOT NULL
		  AND ST_DWithin(at.track_full::geography, sv.track::geography, 50)`,
		activityID, userID, sportGroup)
	if err != nil {
		return nil, fmt.Errorf("get spatial candidate segments: %w", err)
	}
	defer rows.Close()

	var result []CandidateSegment
	for rows.Next() {
		var c CandidateSegment
		var rawMeta json.RawMessage
		if err := rows.Scan(&c.SegmentID, &c.VersionID, &c.SportGroup, &c.DistanceM,
			&c.SegmentType, &rawMeta); err != nil {
			return nil, fmt.Errorf("scan candidate segment: %w", err)
		}
		if len(rawMeta) > 0 && string(rawMeta) != "{}" {
			_ = json.Unmarshal(rawMeta, &c.TypeMeta)
		}
		result = append(result, c)
	}
	return result, nil
}

// SegmentEndpoints holds start/end coordinates and length of a segment version.
type SegmentEndpoints struct {
	StartLat float64
	StartLng float64
	EndLat   float64
	EndLng   float64
	LengthM  float64
}

// GetSegmentEndpoints returns start/end point coordinates and geographic length of a segment version.
func (db *DB) GetSegmentEndpoints(ctx context.Context, versionID string) (*SegmentEndpoints, error) {
	var ep SegmentEndpoints
	err := db.Pool.QueryRow(ctx, `
		SELECT ST_Y(ST_StartPoint(track)), ST_X(ST_StartPoint(track)),
		       ST_Y(ST_EndPoint(track)), ST_X(ST_EndPoint(track)),
		       ST_Length(track::geography)
		FROM segment_versions
		WHERE id = $1 AND track IS NOT NULL`, versionID,
	).Scan(&ep.StartLat, &ep.StartLng, &ep.EndLat, &ep.EndLng, &ep.LengthM)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get segment endpoints: %w", err)
	}
	return &ep, nil
}

// GPSStreamPoint is a lightweight GPS point with timestamp for matching.
type GPSStreamPoint struct {
	Time time.Time
	Lat  float64
	Lng  float64
}

// GetActivityGPSStream returns all GPS stream points (time, lat, lng) for an activity,
// ordered chronologically. Only points with valid GPS coordinates are returned.
func (db *DB) GetActivityGPSStream(ctx context.Context, activityID string) ([]GPSStreamPoint, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT "time", lat, lng
		FROM activity_streams
		WHERE activity_id = $1 AND lat IS NOT NULL AND lng IS NOT NULL
		ORDER BY "time" ASC`, activityID)
	if err != nil {
		return nil, fmt.Errorf("get activity GPS stream: %w", err)
	}
	defer rows.Close()

	var points []GPSStreamPoint
	for rows.Next() {
		var p GPSStreamPoint
		if err := rows.Scan(&p.Time, &p.Lat, &p.Lng); err != nil {
			return nil, fmt.Errorf("scan GPS stream point: %w", err)
		}
		points = append(points, p)
	}
	return points, nil
}

// VerifySubTrackMatch computes the Hausdorff distance between a sub-track LineString
// (built from GPS points) and a segment version track. Returns distance in meters.
func (db *DB) VerifySubTrackMatch(ctx context.Context, versionID string, subTrack []GPSStreamPoint) (*float64, error) {
	if len(subTrack) < 2 {
		return nil, nil
	}

	// Build WKT LINESTRING from sub-track points
	var sb strings.Builder
	sb.WriteString("LINESTRING(")
	for i, p := range subTrack {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf("%.7f %.7f", p.Lng, p.Lat))
	}
	sb.WriteString(")")

	var distance *float64
	err := db.Pool.QueryRow(ctx, `
		SELECT ST_HausdorffDistance(
			ST_Transform(ST_SetSRID(ST_GeomFromText($1), 4326), 3857),
			ST_Transform(sv.track, 3857)
		)
		FROM segment_versions sv
		WHERE sv.id = $2 AND sv.track IS NOT NULL`,
		sb.String(), versionID,
	).Scan(&distance)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("verify sub-track match: %w", err)
	}
	return distance, nil
}

// SegmentEffortMetrics holds stream-based metrics for a segment effort.
type SegmentEffortMetrics struct {
	AvgSpeedMs   *float64
	AvgHeartRate *float64
	AvgPowerW    *float64
}

// GetSegmentEffortMetrics computes average speed, heart rate and power from
// the activity_streams table for a given time range within an activity.
func (db *DB) GetSegmentEffortMetrics(ctx context.Context, activityID string, startTime, endTime time.Time) (*SegmentEffortMetrics, error) {
	var m SegmentEffortMetrics
	err := db.Pool.QueryRow(ctx, `
		SELECT AVG(speed_ms), AVG(heart_rate::double precision), AVG(power_w::double precision)
		FROM activity_streams
		WHERE activity_id = $1 AND "time" >= $2 AND "time" <= $3`,
		activityID, startTime, endTime,
	).Scan(&m.AvgSpeedMs, &m.AvgHeartRate, &m.AvgPowerW)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get segment effort metrics: %w", err)
	}
	return &m, nil
}

// DeleteSegmentEffortsByActivity removes all segment efforts for a given activity.
func (db *DB) DeleteSegmentEffortsByActivity(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx,
		`DELETE FROM segment_efforts WHERE activity_id = $1`, activityID)
	if err != nil {
		return fmt.Errorf("delete segment efforts by activity: %w", err)
	}
	return nil
}

// DeleteSegmentEffortsBySegment removes all efforts for a segment by a specific user.
func (db *DB) DeleteSegmentEffortsBySegment(ctx context.Context, segmentID, userID string) error {
	_, err := db.Pool.Exec(ctx,
		`DELETE FROM segment_efforts WHERE segment_id = $1 AND user_id = $2`, segmentID, userID)
	if err != nil {
		return fmt.Errorf("delete segment efforts by segment: %w", err)
	}
	return nil
}

// GetAffectedSegmentIDs returns all segment IDs that have efforts for a given activity.
func (db *DB) GetAffectedSegmentIDs(ctx context.Context, activityID string) ([]string, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT DISTINCT segment_id FROM segment_efforts WHERE activity_id = $1`, activityID)
	if err != nil {
		return nil, fmt.Errorf("get affected segment IDs: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan segment ID: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// InsertDetectedLaps batch-inserts detected laps for an activity.
func (db *DB) InsertDetectedLaps(ctx context.Context, activityID string, laps []model.DetectedLap) error {
	if len(laps) == 0 {
		return nil
	}
	for _, lap := range laps {
		_, err := db.Pool.Exec(ctx, `
			INSERT INTO activity_detected_laps (
				activity_id, lap_number, start_time, end_time, elapsed_time_s,
				distance_m, avg_speed_ms, avg_heart_rate, avg_power_w, avg_cadence,
				total_ascent_m, total_descent_m, start_lat, start_lng
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
			activityID, lap.LapNumber, lap.StartTime, lap.EndTime, lap.ElapsedTimeS,
			lap.DistanceM, lap.AvgSpeedMs, lap.AvgHeartRate, lap.AvgPowerW, lap.AvgCadence,
			lap.TotalAscentM, lap.TotalDescentM, lap.StartLat, lap.StartLng,
		)
		if err != nil {
			return fmt.Errorf("insert detected lap %d: %w", lap.LapNumber, err)
		}
	}
	return nil
}

// GetDetectedLaps returns all detected laps for an activity, ordered by lap number.
func (db *DB) GetDetectedLaps(ctx context.Context, activityID string) ([]model.DetectedLap, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, activity_id, lap_number, start_time, end_time, elapsed_time_s,
		       distance_m, avg_speed_ms, avg_heart_rate, avg_power_w, avg_cadence,
		       total_ascent_m, total_descent_m, start_lat, start_lng
		FROM activity_detected_laps
		WHERE activity_id = $1
		ORDER BY lap_number ASC`, activityID)
	if err != nil {
		return nil, fmt.Errorf("get detected laps: %w", err)
	}
	defer rows.Close()

	var laps []model.DetectedLap
	for rows.Next() {
		var l model.DetectedLap
		if err := rows.Scan(
			&l.ID, &l.ActivityID, &l.LapNumber, &l.StartTime, &l.EndTime, &l.ElapsedTimeS,
			&l.DistanceM, &l.AvgSpeedMs, &l.AvgHeartRate, &l.AvgPowerW, &l.AvgCadence,
			&l.TotalAscentM, &l.TotalDescentM, &l.StartLat, &l.StartLng,
		); err != nil {
			return nil, fmt.Errorf("scan detected lap: %w", err)
		}
		laps = append(laps, l)
	}
	if laps == nil {
		laps = []model.DetectedLap{}
	}
	return laps, nil
}

// DeleteDetectedLaps removes all detected laps for an activity.
func (db *DB) DeleteDetectedLaps(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx,
		`DELETE FROM activity_detected_laps WHERE activity_id = $1`, activityID)
	if err != nil {
		return fmt.Errorf("delete detected laps: %w", err)
	}
	return nil
}

// IsLapDetectionEnabled checks if lap detection is enabled for a user.
func (db *DB) IsLapDetectionEnabled(ctx context.Context, userID string) bool {
	var enabled bool
	err := db.Pool.QueryRow(ctx,
		`SELECT enable_lap_detection FROM user_enrichment_settings WHERE user_id = $1`, userID,
	).Scan(&enabled)
	if err != nil {
		return true // default enabled
	}
	return enabled
}

func sportGroupFromSportType(sportType string) string {
	return sport.SegmentGroup(sportType)
}

// ImportLiftSegments creates segment_type='lift' segments from osm_lifts.
// Idempotent: skips lifts that already have a matching segment.
func (db *DB) ImportLiftSegments(ctx context.Context, userID string) (int, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT l.osm_id, l.name, ST_AsGeoJSON(l.geom) AS geojson
		FROM osm_lifts l
		WHERE NOT EXISTS (
			SELECT 1 FROM segments s WHERE s.osm_lift_id = l.osm_id
		)
	`)
	if err != nil {
		return 0, fmt.Errorf("query osm_lifts: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var osmID int64
		var name *string
		var geojson string
		if err := rows.Scan(&osmID, &name, &geojson); err != nil {
			return count, err
		}
		segName := "Lift"
		if name != nil && *name != "" {
			segName = *name
		}
		_, err := db.Pool.Exec(ctx, `
			INSERT INTO segments (created_by, name, sport_group, segment_type,
			                      segment_type_meta, osm_lift_id, visibility)
			VALUES ($1, $2, 'generic', 'lift', '{}', $3, 'private')
		`, userID, segName, osmID)
		if err != nil {
			continue
		}
		var segID string
		_ = db.Pool.QueryRow(ctx, `
			SELECT id FROM segments WHERE osm_lift_id = $1 LIMIT 1
		`, osmID).Scan(&segID)
		if segID != "" {
			_, _ = db.Pool.Exec(ctx, `
				INSERT INTO segment_versions (segment_id, version_number, track,
				                              start_point, end_point, valid_from, created_by)
				VALUES ($1, 1,
				        ST_SetSRID(ST_GeomFromGeoJSON($2), 4326),
				        ST_StartPoint(ST_SetSRID(ST_GeomFromGeoJSON($2), 4326)),
				        ST_EndPoint(ST_SetSRID(ST_GeomFromGeoJSON($2), 4326)),
				        CURRENT_DATE, $3)
			`, segID, geojson, userID)
		}
		count++
	}
	return count, rows.Err()
}

// ListActivityIDsForSegmentBackfill returns IDs of all GPS activities belonging
// to userID whose track passes within 50 m of the given segment's latest version track.
// This mirrors GetSpatialCandidateSegments but in the opposite direction:
// given a new segment, find which activities may traverse it.
func (db *DB) ListActivityIDsForSegmentBackfill(ctx context.Context, userID, segmentID string) ([]string, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT at.activity_id
		FROM activity_tracks at
		JOIN activities a ON a.id = at.activity_id
		JOIN LATERAL (
			SELECT track FROM segment_versions
			WHERE segment_id = $2
			ORDER BY version_number DESC
			LIMIT 1
		) sv ON true
		WHERE a.user_id = $1
		  AND at.track_full IS NOT NULL
		  AND sv.track IS NOT NULL
		  AND ST_DWithin(at.track_full::geography, sv.track::geography, 50)`,
		userID, segmentID)
	if err != nil {
		return nil, fmt.Errorf("list activity ids for segment backfill: %w", err)
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
	return ids, nil
}

// IsSegmentFavoriteByUser checks if a segment is favorited by a user.
func (db *DB) IsSegmentFavoriteByUser(ctx context.Context, userID, segmentID string) (bool, error) {
	var exists bool
	err := db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM segment_favorites WHERE user_id = $1 AND segment_id = $2)`,
		userID, segmentID).Scan(&exists)
	return exists, err
}

// AddSegmentFavorite adds a favorite record for a segment.
func (db *DB) AddSegmentFavorite(ctx context.Context, userID, segmentID string) error {
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO segment_favorites (user_id, segment_id) VALUES ($1, $2)
         ON CONFLICT DO NOTHING`,
		userID, segmentID)
	return err
}

// RemoveSegmentFavorite removes a favorite record for a segment.
func (db *DB) RemoveSegmentFavorite(ctx context.Context, userID, segmentID string) error {
	_, err := db.Pool.Exec(ctx,
		`DELETE FROM segment_favorites WHERE user_id = $1 AND segment_id = $2`,
		userID, segmentID)
	return err
}

// GetSegmentPerformances retrieves all segment efforts for a specific segment from a given user.
func (db *DB) GetSegmentPerformances(ctx context.Context, segmentID, userID string) ([]model.SegmentPerformance, error) {
	query := `
	SELECT
		se.id,
		se.activity_id,
		se.elapsed_time_s,
		a.moving_time_s,
		a.started_at,
		a.sport_type,
		a.title,
		a.gear_id
	FROM segment_efforts se
	JOIN activities a ON se.activity_id = a.id
	WHERE se.segment_id = $1 AND a.user_id = $2
	ORDER BY a.started_at DESC`

	rows, err := db.Pool.Query(ctx, query, segmentID, userID)
	if err != nil {
		return nil, fmt.Errorf("get segment performances: %w", err)
	}
	defer rows.Close()

	var performances []model.SegmentPerformance
	for rows.Next() {
		var perf model.SegmentPerformance
		if err := rows.Scan(&perf.SegmentEffortID, &perf.ActivityID, &perf.ElapsedTimeS,
			&perf.MovingTimeS, &perf.StartedAt, &perf.SportType, &perf.Title, &perf.GearID); err != nil {
			return nil, fmt.Errorf("scan performance: %w", err)
		}
		performances = append(performances, perf)
	}

	if performances == nil {
		performances = []model.SegmentPerformance{}
	}

	return performances, rows.Err()
}
