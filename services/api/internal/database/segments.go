package database

import (
	"errors"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

// Sentinel errors for segment operations — use errors.Is() in handlers.
var (
	ErrSegmentNotFound      = errors.New("segment not found")
	ErrSegmentNotAuthorized = errors.New("segment not authorized")
	ErrSegmentOSMLift       = errors.New("OSM-imported lift segments cannot be deleted")
)

func (db *DB) ListSegments(ctx context.Context, p model.ListSegmentsParams) (*model.ListSegmentsResult, error) {
	// args: $1 = userID, $2 = isAdmin (fixed positions for WHERE clause)
	args := []any{p.UserID, p.IsAdmin}
	argIdx := 3 // next dynamic param starts at $3

	// --- CTE: user_efforts ---
	cte := `
WITH user_efforts AS (
    SELECT se.segment_id,
           COUNT(*)::int AS ridden_count,
           COUNT(*) FILTER (WHERE a.visibility != 'public')::int AS private_ridden_count
    FROM segment_efforts se
    JOIN activities a ON se.activity_id = a.id
    WHERE a.user_id = $1
    GROUP BY se.segment_id
),
user_favs AS (
    SELECT segment_id FROM segment_favorites WHERE user_id = $1
)`

	// --- SELECT ---
	selectCols := `
SELECT s.id, s.created_by, s.name, s.description, s.sport_group,
       s.allowed_sport_types, s.allowed_sub_types,
       s.segment_type, s.segment_type_meta,
       s.source_activity_id, s.osm_lift_id,
       s.distance_m, s.elevation_gain_m,
       s.effort_count + COALESCE(ue.private_ridden_count, 0) AS effort_count, s.athlete_count,
       COALESCE(ue.ridden_count, 0) AS ridden_count,
       (uf.segment_id IS NOT NULL) AS is_favorite,
       s.visibility, s.route_hash, s.created_at,`

	if p.IncludeGeoJSON {
		selectCols += `
       ST_AsGeoJSON(sv.track) AS track_geojson,`
	} else {
		selectCols += `
       NULL::text AS track_geojson,`
	}
	selectCols += `
       s.terrain_type, s.climb_type`

	// --- FROM + JOINs ---
	from := `
FROM segments s
LEFT JOIN user_efforts ue ON ue.segment_id = s.id
LEFT JOIN user_favs uf ON uf.segment_id = s.id`

	if p.IncludeGeoJSON {
		from += `
LEFT JOIN LATERAL (
    SELECT track FROM segment_versions
    WHERE segment_id = s.id
    ORDER BY version_number DESC
    LIMIT 1
) sv ON true`
	}

	// --- WHERE ---
	// Admin sees all segments (public + private from all users).
	// Segments contain only geometry/metadata — no personal health data.
	// Admin access is covered by the existing audit log.
	where := `
WHERE (s.visibility = 'public' OR s.created_by = $1 OR $2 = true)`

	if p.SportGroup != "" {
		where += fmt.Sprintf(" AND s.sport_group = $%d", argIdx)
		args = append(args, p.SportGroup)
		argIdx++
	}
	if p.SegmentTypes != "" {
		types := strings.Split(p.SegmentTypes, ",")
		placeholders := make([]string, len(types))
		for i, t := range types {
			placeholders[i] = fmt.Sprintf("$%d", argIdx)
			args = append(args, strings.TrimSpace(t))
			argIdx++
		}
		where += " AND s.segment_type IN (" + strings.Join(placeholders, ", ") + ")"
	} else if p.SegmentType != "" {
		where += fmt.Sprintf(" AND s.segment_type = $%d", argIdx)
		args = append(args, p.SegmentType)
		argIdx++
	}
	if p.Name != "" {
		where += fmt.Sprintf(" AND s.name ILIKE $%d", argIdx)
		args = append(args, "%"+p.Name+"%")
		argIdx++
	}
	if p.BBox != "" {
		parts := strings.Split(p.BBox, ",")
		if len(parts) == 4 {
			if p.IncludeGeoJSON {
				where += fmt.Sprintf(
					` AND sv.track IS NOT NULL AND ST_Intersects(sv.track, ST_MakeEnvelope($%d, $%d, $%d, $%d, 4326))`,
					argIdx, argIdx+1, argIdx+2, argIdx+3,
				)
			} else {
				// Ohne sv brauchen wir einen eigenen LATERAL fuer BBox
				from += fmt.Sprintf(`
LEFT JOIN LATERAL (
    SELECT track FROM segment_versions
    WHERE segment_id = s.id
    ORDER BY version_number DESC
    LIMIT 1
) sv_bbox ON true`)
				where += fmt.Sprintf(
					` AND sv_bbox.track IS NOT NULL AND ST_Intersects(sv_bbox.track, ST_MakeEnvelope($%d, $%d, $%d, $%d, 4326))`,
					argIdx, argIdx+1, argIdx+2, argIdx+3,
				)
			}
			args = append(args, parts[0], parts[1], parts[2], parts[3])
			argIdx += 4
		}
	}
	if p.OnlyRidden {
		where += " AND COALESCE(ue.ridden_count, 0) > 0"
	}

	// --- Cursor ---
	if p.Cursor != "" {
		cursorTS, cursorID, err := decodeCursor(p.Cursor)
		if err == nil {
			where += fmt.Sprintf(" AND (s.created_at, s.id) < ($%d, $%d)", argIdx, argIdx+1)
			args = append(args, cursorTS, cursorID)
			argIdx += 2
		}
	}

	// --- ORDER + LIMIT ---
	orderLimit := "\nORDER BY s.created_at DESC, s.id DESC"
	limit := p.Limit
	if limit == 0 {
		limit = 50 // default
	}
	if limit > 0 {
		// limit+1 fuer has_more Erkennung
		orderLimit += fmt.Sprintf(" LIMIT %d", limit+1)
	}

	query := cte + selectCols + from + where + orderLimit

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
			&s.RiddenCount, &s.IsFavorite,
			&s.Visibility, &s.RouteHash, &s.CreatedAt,
			&s.TrackGeoJSON,
			&s.TerrainType, &s.ClimbType,
		)
		if err != nil {
			return nil, fmt.Errorf("scan segment: %w", err)
		}
		segments = append(segments, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list segments: %w", err)
	}
	if segments == nil {
		segments = []model.Segment{}
	}

	// has_more + cursor berechnen
	hasMore := limit > 0 && len(segments) > limit
	var nextCursor string
	if hasMore {
		segments = segments[:limit] // ueberzaehliges Element abschneiden
		last := segments[len(segments)-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
	}

	return &model.ListSegmentsResult{
		Segments:   segments,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
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
		INSERT INTO segments (created_by, name, description, sport_group, segment_type, segment_type_meta, source_activity_id, terrain_type, climb_type, visibility)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_by, name, description, sport_group,
		          allowed_sport_types, allowed_sub_types,
		          segment_type, segment_type_meta,
		          source_activity_id, osm_lift_id,
		          distance_m, elevation_gain_m,
		          effort_count, athlete_count,
		          visibility, route_hash, created_at,
		          terrain_type, climb_type`,
		userID, create.Name, create.Description, create.SportGroup,
		create.SegmentType, segmentTypeMeta, create.SourceActivityID,
		create.TerrainType, create.ClimbType, create.Visibility,
	).Scan(
		&s.ID, &s.CreatedBy, &s.Name, &s.Description, &s.SportGroup,
		&s.AllowedSportTypes, &s.AllowedSubTypes,
		&s.SegmentType, &s.SegmentTypeMeta,
		&s.SourceActivityID, &s.OsmLiftID,
		&s.DistanceM, &s.ElevationGainM,
		&s.EffortCount, &s.AthleteCount,
		&s.Visibility, &s.RouteHash, &s.CreatedAt,
		&s.TerrainType, &s.ClimbType,
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
		WITH g AS (SELECT ST_SetSRID(ST_GeomFromGeoJSON($2), 4326) AS geom)
		INSERT INTO segment_versions (segment_id, version_number, track, start_point, end_point, valid_from, created_by)
		SELECT $1, 1, g.geom, ST_StartPoint(g.geom), ST_EndPoint(g.geom), CURRENT_DATE, $3
		FROM g
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
			SELECT ST_Length(track::geography)
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

func (db *DB) GetSegment(ctx context.Context, segmentID, userID string, isAdmin bool) (*model.Segment, error) {
	var s model.Segment
	err := db.Pool.QueryRow(ctx, `
		SELECT s.id, s.created_by, s.name, s.description, s.sport_group,
		       s.allowed_sport_types, s.allowed_sub_types,
		       s.segment_type, s.segment_type_meta,
		       s.source_activity_id, s.osm_lift_id,
		       s.distance_m, s.elevation_gain_m,
		       s.effort_count + COALESCE((
		           SELECT COUNT(*)::int FROM segment_efforts se
		           JOIN activities a ON se.activity_id = a.id
		           WHERE se.segment_id = s.id AND a.user_id = NULLIF($2::text, '')::uuid AND a.visibility != 'public'
		       ), 0) AS effort_count, s.athlete_count,
		       COALESCE((
		           SELECT COUNT(*)::int FROM segment_efforts se
		           JOIN activities a ON se.activity_id = a.id
		           WHERE se.segment_id = s.id AND a.user_id = NULLIF($2::text, '')::uuid
		       ), 0) AS ridden_count,
		       EXISTS(
		           SELECT 1 FROM segment_favorites sf
		           WHERE sf.segment_id = s.id AND sf.user_id = NULLIF($2::text, '')::uuid
		       ) AS is_favorite,
		       s.visibility, s.route_hash, s.created_at,
		       ST_AsGeoJSON(sv.track) AS track_geojson,
		       s.terrain_type, s.climb_type
		FROM segments s
		LEFT JOIN LATERAL (
			SELECT track FROM segment_versions
			WHERE segment_id = s.id
			ORDER BY version_number DESC
			LIMIT 1
		) sv ON true
		WHERE s.id = $1
		  AND (s.visibility = 'public' OR s.created_by = NULLIF($2::text, '')::uuid OR $3 = true)`, segmentID, userID, isAdmin,
	).Scan(
		&s.ID, &s.CreatedBy, &s.Name, &s.Description, &s.SportGroup,
		&s.AllowedSportTypes, &s.AllowedSubTypes,
		&s.SegmentType, &s.SegmentTypeMeta,
		&s.SourceActivityID, &s.OsmLiftID,
		&s.DistanceM, &s.ElevationGainM,
		&s.EffortCount, &s.AthleteCount,
		&s.RiddenCount, &s.IsFavorite,
		&s.Visibility, &s.RouteHash, &s.CreatedAt,
		&s.TrackGeoJSON,
		&s.TerrainType, &s.ClimbType,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get segment: %w", err)
	}
	return &s, nil
}

func (db *DB) UpdateSegment(ctx context.Context, segmentID, userID string, update *model.SegmentUpdate) (*model.Segment, error) {
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
	if update.TerrainType != nil {
		setClauses = append(setClauses, fmt.Sprintf("terrain_type = $%d", argIdx))
		args = append(args, *update.TerrainType)
		argIdx++
	}
	if update.ClimbType != nil {
		setClauses = append(setClauses, fmt.Sprintf("climb_type = $%d", argIdx))
		args = append(args, *update.ClimbType)
		argIdx++
	}
	// Note: private→public MUST go through PublishSegment (handler blocks it via 400).
	// This SET clause only handles public→private or same-value updates.
	if update.Visibility != nil {
		setClauses = append(setClauses, fmt.Sprintf("visibility = $%d", argIdx))
		args = append(args, *update.Visibility)
		argIdx++
	}

	if len(setClauses) == 0 {
		return db.GetSegment(ctx, segmentID, userID, false)
	}

	query := fmt.Sprintf("UPDATE segments SET %s WHERE id = $%d AND created_by = $%d",
		strings.Join(setClauses, ", "), argIdx, argIdx+1)
	args = append(args, segmentID, userID)

	tag, err := db.Pool.Exec(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("update segment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, nil
	}

	return db.GetSegment(ctx, segmentID, userID, false)
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

func (db *DB) DeleteSegment(ctx context.Context, userID, segmentID string, isAdmin bool) error {
	var segType string
	var osmLiftID *int64
	var createdBy string
	err := db.Pool.QueryRow(ctx,
		`SELECT segment_type, osm_lift_id, created_by FROM segments WHERE id = $1`,
		segmentID).Scan(&segType, &osmLiftID, &createdBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrSegmentNotFound
	}
	if err != nil {
		return fmt.Errorf("delete segment: %w", err)
	}
	if !isAdmin && createdBy != userID {
		return ErrSegmentNotAuthorized
	}
	if segType == "lift" && osmLiftID != nil {
		return ErrSegmentOSMLift
	}

	tag, err := db.Pool.Exec(ctx, `DELETE FROM segments WHERE id = $1`, segmentID)
	if err != nil {
		return fmt.Errorf("delete segment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSegmentNotFound
	}

	// Audit: log admin deletions of other users' segments (DSGVO Art. 5(2) Rechenschaftspflicht).
	if isAdmin && createdBy != userID {
		slog.Info("admin deleted segment", "admin_id", userID, "segment_id", segmentID, "segment_owner", createdBy)
	}

	return nil
}

// DeleteSegments bulk-deletes segments owned by userID.
// OSM-imported lift segments (segment_type='lift' AND osm_lift_id IS NOT NULL) are silently skipped.
// Returns the number of rows actually deleted.
func (db *DB) DeleteSegments(ctx context.Context, ids []string, userID string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res, err := db.Pool.Exec(ctx,
		`DELETE FROM segments
		 WHERE id = ANY($1::uuid[])
		   AND created_by = $2
		   AND NOT (segment_type = 'lift' AND osm_lift_id IS NOT NULL)`,
		ids, userID)
	if err != nil {
		return 0, fmt.Errorf("delete segments: %w", err)
	}
	return res.RowsAffected(), nil
}

// PublishSegment sets a private segment to public.
// Only the creator may publish. Returns false if segment not found, not owned, or already public.
func (db *DB) PublishSegment(ctx context.Context, segmentID, creatorID string) (bool, error) {
	tag, err := db.Pool.Exec(ctx,
		`UPDATE segments SET visibility = 'public' WHERE id = $1 AND created_by = $2 AND visibility = 'private'`,
		segmentID, creatorID)
	if err != nil {
		return false, fmt.Errorf("publish segment: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (db *DB) GetSegmentLeaderboard(ctx context.Context, segmentID, sportType string, isReverse *bool, gearID string, onlyUserID string) ([]model.SegmentEffort, error) {
	query := `
		SELECT se.id, se.segment_id, se.segment_version_id, se.activity_id,
		       se.user_id, se.started_at, se.elapsed_time_s,
		       se.is_reverse, se.sport_type, se.sub_type, se.gear_id,
		       se.avg_power_w, se.avg_heart_rate, se.avg_speed_ms,
		       se.rank_overall, se.is_personal_best, se.personal_rank, se.is_segment_record,
		       se.in_leaderboard, se.created_at,
		       s.name AS segment_name,
		       u.display_name,
		       g.name AS gear_name
		FROM segment_efforts se
		JOIN segments s ON s.id = se.segment_id
		JOIN users u ON u.id = se.user_id
		LEFT JOIN gear g ON g.id = se.gear_id`

	args := []any{segmentID}
	argIdx := 2

	if onlyUserID != "" {
		// "Nur ich": show all own efforts including from private activities, but exclude group-secondary.
		query += `
		JOIN activities act ON act.id = se.activity_id AND NOT act.is_group_secondary
		WHERE se.segment_id = $1 AND se.in_leaderboard = true AND se.user_id = $` + fmt.Sprint(argIdx)
		args = append(args, onlyUserID)
		argIdx++
	} else {
		// "Alle": JOIN activities to exclude efforts from private or group-secondary activities.
		query += `
		JOIN activities act ON act.id = se.activity_id AND act.visibility = 'public' AND NOT act.is_group_secondary
		WHERE se.segment_id = $1 AND se.in_leaderboard = true`
	}

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
			&e.RankOverall, &e.IsPersonalBest, &e.PersonalRank, &e.IsSegmentRecord,
			&e.InLeaderboard, &e.CreatedAt,
			&e.SegmentName, &e.DisplayName, &e.GearName,
		)
		if err != nil {
			return nil, fmt.Errorf("scan effort: %w", err)
		}
		efforts = append(efforts, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get segment efforts: %w", err)
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
		       se.rank_overall, se.is_personal_best, se.personal_rank, se.is_segment_record,
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
			&e.RankOverall, &e.IsPersonalBest, &e.PersonalRank, &e.IsSegmentRecord,
			&e.InLeaderboard, &e.CreatedAt,
			&e.SegmentName, &e.DisplayName, &e.GearName,
			&e.SegmentTypeMeta,
		)
		if err != nil {
			return nil, fmt.Errorf("scan effort: %w", err)
		}
		efforts = append(efforts, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get activity segment efforts: %w", err)
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
			gear_id, avg_power_w, avg_heart_rate, avg_speed_ms,
			distance_m, elevation_gain_m
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
		effort.SegmentID, effort.SegmentVersionID, effort.ActivityID, effort.UserID,
		effort.StartedAt, effort.ElapsedTimeS, effort.IsReverse, effort.SportType, effort.SubType,
		effort.GearID, effort.AvgPowerW, effort.AvgHeartRate, effort.AvgSpeedMs,
		effort.DistanceM, effort.ElevationGainM,
	)
	if err != nil {
		return fmt.Errorf("insert segment effort: %w", err)
	}
	return nil
}

func (db *DB) UpdateSegmentEffortRanks(ctx context.Context, segmentID string) error {
	// Two CTEs:
	// global_ranked: ranks efforts from non-private activities only (for public leaderboard).
	// personal_ranked: ranks ALL of a user's efforts, including private activities.
	//   Intentional: a user should see their own personal best even if the activity is private.
	//   The global rank (rank_overall) will be NULL for private-activity efforts — that's correct.
	//
	// TimescaleDB note: both CTEs include started_at so the UPDATE WHERE clause can use the
	// composite PK (id, started_at) — required for efficient chunk pruning on the hypertable.
	_, err := db.Pool.Exec(ctx, `
		WITH
		global_ranked AS (
			SELECT se.id, se.started_at,
			       ROW_NUMBER() OVER (
			           PARTITION BY se.sport_type, se.is_reverse
			           ORDER BY se.elapsed_time_s ASC
			       ) AS rk,
			       (ROW_NUMBER() OVER (
			           PARTITION BY se.sport_type, se.is_reverse
			           ORDER BY se.elapsed_time_s ASC
			       ) = 1) AS is_record
			FROM segment_efforts se
			JOIN activities a ON a.id = se.activity_id AND a.visibility = 'public' AND NOT a.is_group_secondary
			WHERE se.segment_id = $1 AND se.in_leaderboard = true
		),
		personal_ranked AS (
			SELECT id, started_at,
			       ROW_NUMBER() OVER (
			           PARTITION BY sport_type, is_reverse, user_id
			           ORDER BY elapsed_time_s ASC
			       ) AS user_rk
			FROM segment_efforts
			WHERE segment_id = $1 AND in_leaderboard = true
		)
		UPDATE segment_efforts se
		SET rank_overall      = gr.rk,
		    is_personal_best  = (pr.user_rk = 1),
		    personal_rank     = CASE WHEN pr.user_rk <= 3 THEN pr.user_rk::smallint ELSE NULL END,
		    is_segment_record = COALESCE(gr.is_record, false)
		FROM personal_ranked pr
		LEFT JOIN global_ranked gr ON gr.id = pr.id AND gr.started_at = pr.started_at
		WHERE se.id = pr.id AND se.started_at = pr.started_at`, segmentID)
	if err != nil {
		return fmt.Errorf("update effort ranks: %w", err)
	}

	// Update effort_count and athlete_count: public, non-group-secondary activities only.
	_, err = db.Pool.Exec(ctx, `
		UPDATE segments SET
			effort_count = (
				SELECT COUNT(*) FROM segment_efforts se
				JOIN activities a ON a.id = se.activity_id AND a.visibility = 'public' AND NOT a.is_group_secondary
				WHERE se.segment_id = $1
			),
			athlete_count = (
				SELECT COUNT(DISTINCT se.user_id) FROM segment_efforts se
				JOIN activities a ON a.id = se.activity_id AND a.visibility = 'public' AND NOT a.is_group_secondary
				WHERE se.segment_id = $1
			)
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
	TypeMeta    map[string]any
	TrackWKB    []byte // NEW: Segment track as WKB for in-memory Hausdorff
}

// GetSpatialCandidateSegments returns segments within 50m of the activity track,
// filtered by sport group. Uses ST_DWithin with GIST index for fast pre-filtering.
func (db *DB) GetSpatialCandidateSegments(ctx context.Context, activityID, userID, sportType string, activityDate time.Time) ([]CandidateSegment, error) {
	sportGroup := sportGroupFromSportType(sportType)

	rows, err := db.Pool.Query(ctx, `
		SELECT s.id, sv.id, s.sport_group, COALESCE(s.distance_m, 0),
		       s.segment_type, COALESCE(sv.segment_type_meta, '{}'),
		       ST_AsBinary(sv.track)
		FROM segments s
		JOIN LATERAL (
			SELECT id, track, segment_type_meta FROM (
				(SELECT id, track, segment_type_meta FROM segment_versions
				 WHERE segment_id = s.id AND valid_from <= $4
				 ORDER BY valid_from DESC, version_number DESC LIMIT 1)
				UNION ALL
				(SELECT id, track, segment_type_meta FROM segment_versions
				 WHERE segment_id = s.id
				 ORDER BY version_number ASC LIMIT 1)
			) versioned LIMIT 1
		) sv ON true
		JOIN activity_tracks at ON at.activity_id = $1
		WHERE (s.visibility = 'public' OR s.created_by = $2)
		  AND (s.sport_group = $3 OR s.sport_group = 'generic')
		  AND at.track_full IS NOT NULL
		  AND sv.track IS NOT NULL
		  AND ST_DWithin(at.track_full::geography, sv.track::geography, 100)`,
		activityID, userID, sportGroup, activityDate)
	if err != nil {
		return nil, fmt.Errorf("get spatial candidate segments: %w", err)
	}
	defer rows.Close()

	var result []CandidateSegment
	for rows.Next() {
		var c CandidateSegment
		var rawMeta json.RawMessage
		if err := rows.Scan(&c.SegmentID, &c.VersionID, &c.SportGroup, &c.DistanceM,
			&c.SegmentType, &rawMeta, &c.TrackWKB); err != nil {
			return nil, fmt.Errorf("scan candidate segment: %w", err)
		}
		if len(rawMeta) > 0 && string(rawMeta) != "{}" {
			_ = json.Unmarshal(rawMeta, &c.TypeMeta)
		}
		result = append(result, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get spatial candidate segments: %w", err)
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
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get segment endpoints: %w", err)
	}
	return &ep, nil
}

// GetSegmentEndpointsBatch returns endpoints for multiple segment versions in a single query.
func (db *DB) GetSegmentEndpointsBatch(ctx context.Context, versionIDs []string) (map[string]*SegmentEndpoints, error) {
	if len(versionIDs) == 0 {
		return map[string]*SegmentEndpoints{}, nil
	}

	rows, err := db.Pool.Query(ctx, `
		SELECT id,
		       ST_Y(ST_StartPoint(track)), ST_X(ST_StartPoint(track)),
		       ST_Y(ST_EndPoint(track)), ST_X(ST_EndPoint(track)),
		       ST_Length(track::geography)
		FROM segment_versions
		WHERE id = ANY($1) AND track IS NOT NULL`, versionIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("get segment endpoints batch: %w", err)
	}
	defer rows.Close()

	result := make(map[string]*SegmentEndpoints, len(versionIDs))
	for rows.Next() {
		var id string
		var ep SegmentEndpoints
		if err := rows.Scan(&id, &ep.StartLat, &ep.StartLng, &ep.EndLat, &ep.EndLng, &ep.LengthM); err != nil {
			return nil, fmt.Errorf("get segment endpoints batch scan: %w", err)
		}
		result[id] = &ep
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get segment endpoints batch rows: %w", err)
	}
	return result, nil
}

// GPSStreamPoint is a lightweight GPS point with timestamp for matching.
type GPSStreamPoint struct {
	Time      time.Time
	Lat       float64
	Lng       float64
	SpeedMs   *float64 // NEW: speed in m/s (nullable)
	HeartRate *int     // NEW: heart rate in bpm (nullable)
	PowerW    *int     // NEW: power in watts (nullable)
	AltitudeM *float64 // NEW: altitude in meters (nullable)
}

// GetActivityGPSStream returns all GPS stream points (time, lat, lng) for an activity,
// ordered chronologically. Only points with valid GPS coordinates are returned.
func (db *DB) GetActivityGPSStream(ctx context.Context, activityID string) ([]GPSStreamPoint, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT "time", lat, lng, speed_ms, heart_rate, power_w, altitude_m
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
		if err := rows.Scan(&p.Time, &p.Lat, &p.Lng, &p.SpeedMs, &p.HeartRate, &p.PowerW, &p.AltitudeM); err != nil {
			return nil, fmt.Errorf("scan GPS stream point: %w", err)
		}
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get GPS stream points: %w", err)
	}
	return points, nil
}

// NOTE: GetSegmentEffortMetrics, GetSegmentEffortMetricsAndElevation, and
// GetSubTrackElevationGain have been replaced by in-memory computeEffortMetrics
// in handler/segment_matching.go (eliminates per-candidate DB round-trips).

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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list segment IDs: %w", err)
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get detected laps: %w", err)
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
			VALUES ($1, $2, 'generic', 'lift', '{}', $3, 'public')
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
				WITH g AS (SELECT ST_SetSRID(ST_GeomFromGeoJSON($2), 4326) AS geom)
				INSERT INTO segment_versions (segment_id, version_number, track,
				                              start_point, end_point, valid_from, created_by)
				SELECT $1, 1, g.geom, ST_StartPoint(g.geom), ST_EndPoint(g.geom), CURRENT_DATE, $3
				FROM g
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
		  AND ST_DWithin(at.track_full::geography, sv.track::geography, 100)`,
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list activity ids for segment backfill: %w", err)
	}
	return ids, nil
}

// ListAllActivityIDsForSegmentBackfill returns activity IDs from ALL users
// whose track passes within 100m of the given segment. Used when publishing
// a previously private segment (full recompute across all users).
func (db *DB) ListAllActivityIDsForSegmentBackfill(ctx context.Context, segmentID string) ([]string, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT at.activity_id
		FROM activity_tracks at
		JOIN LATERAL (
			SELECT track FROM segment_versions
			WHERE segment_id = $1
			ORDER BY version_number DESC
			LIMIT 1
		) sv ON true
		WHERE at.track_full IS NOT NULL
		  AND sv.track IS NOT NULL
		  AND ST_DWithin(at.track_full::geography, sv.track::geography, 100)`,
		segmentID)
	if err != nil {
		return nil, fmt.Errorf("list all activity ids for segment backfill: %w", err)
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
	return ids, rows.Err()
}

// DeleteAllSegmentEffortsBySegment removes ALL efforts for a segment across all users.
// Used when publishing a previously private segment (full recompute).
func (db *DB) DeleteAllSegmentEffortsBySegment(ctx context.Context, segmentID string) error {
	_, err := db.Pool.Exec(ctx,
		`DELETE FROM segment_efforts WHERE segment_id = $1`, segmentID)
	if err != nil {
		return fmt.Errorf("delete all segment efforts by segment: %w", err)
	}
	return nil
}

// InsertSegmentEffortsBatch inserts multiple segment efforts in a single multi-row INSERT.
func (db *DB) InsertSegmentEffortsBatch(ctx context.Context, efforts []*model.SegmentEffort) error {
	return insertSegmentEffortsBatch(ctx, db.Pool, efforts)
}

// InsertSegmentEffortsBatchTx inserts segment efforts inside an existing transaction.
// The caller is responsible for committing or rolling back the transaction.
// This is the transactional counterpart of InsertSegmentEffortsBatch, used by
// recomputeSegmentEffortsForActivity to atomically DELETE + re-INSERT efforts.
func InsertSegmentEffortsBatchTx(ctx context.Context, tx pgx.Tx, efforts []*model.SegmentEffort) error {
	return insertSegmentEffortsBatch(ctx, tx, efforts)
}

// insertSegmentEffortsBatch is the shared implementation for InsertSegmentEffortsBatch
// and InsertSegmentEffortsBatchTx. It accepts any Querier (pool or transaction).
func insertSegmentEffortsBatch(ctx context.Context, conn Querier, efforts []*model.SegmentEffort) error {
	if len(efforts) == 0 {
		return nil
	}

	const cols = 15
	valueParts := make([]string, 0, len(efforts))
	args := make([]any, 0, len(efforts)*cols)

	for i, e := range efforts {
		base := i * cols
		valueParts = append(valueParts, fmt.Sprintf(
			"($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7,
			base+8, base+9, base+10, base+11, base+12, base+13, base+14, base+15,
		))
		args = append(args,
			e.SegmentID, e.SegmentVersionID, e.ActivityID, e.UserID,
			e.StartedAt, e.ElapsedTimeS, e.IsReverse,
			e.SportType, e.SubType, e.GearID,
			e.AvgSpeedMs, e.AvgHeartRate, e.AvgPowerW,
			e.DistanceM, e.ElevationGainM,
		)
	}

	query := `INSERT INTO segment_efforts
		(segment_id, segment_version_id, activity_id, user_id,
		 started_at, elapsed_time_s, is_reverse,
		 sport_type, sub_type, gear_id,
		 avg_speed_ms, avg_heart_rate, avg_power_w,
		 distance_m, elevation_gain_m)
		VALUES ` + strings.Join(valueParts, ", ") + `
		ON CONFLICT DO NOTHING`

	_, err := conn.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("batch insert segment efforts: %w", err)
	}
	return nil
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
// isReverse: nil = all directions, false = forward only, true = reverse only.
func (db *DB) GetSegmentPerformances(ctx context.Context, segmentID, userID string, isReverse *bool) ([]model.SegmentPerformance, error) {
	query := `
	SELECT
		se.id,
		se.activity_id,
		se.elapsed_time_s,
		a.moving_time_s,
		a.started_at,
		a.sport_type,
		COALESCE(a.title, '') AS title,
		se.is_reverse,
		a.gear_id,
		g.name AS gear_name,
		g.type AS gear_type,
		a.timezone
	FROM segment_efforts se
	JOIN activities a ON se.activity_id = a.id
	LEFT JOIN gear g ON a.gear_id = g.id
	WHERE se.segment_id = $1 AND a.user_id = $2`

	args := []any{segmentID, userID}
	if isReverse != nil {
		args = append(args, *isReverse)
		query += fmt.Sprintf(" AND se.is_reverse = $%d", len(args))
	}
	query += " ORDER BY a.started_at DESC"

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get segment performances: %w", err)
	}
	defer rows.Close()

	var performances []model.SegmentPerformance
	for rows.Next() {
		var perf model.SegmentPerformance
		if err := rows.Scan(&perf.SegmentEffortID, &perf.ActivityID, &perf.ElapsedTimeS,
			&perf.MovingTimeS, &perf.StartedAt, &perf.SportType, &perf.Title, &perf.IsReverse,
			&perf.GearID, &perf.GearName, &perf.GearType, &perf.Timezone); err != nil {
			return nil, fmt.Errorf("scan performance: %w", err)
		}
		performances = append(performances, perf)
	}

	if performances == nil {
		performances = []model.SegmentPerformance{}
	}

	return performances, rows.Err()
}

// GetUserSegmentPRs returns all segment efforts where the user achieved personal_rank 1, 2, or 3,
// one row per (segment, personal_rank), ordered by segment name then rank.
func (db *DB) GetUserSegmentPRs(ctx context.Context, userID string) ([]model.SegmentEffort, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT se.id, se.segment_id, se.segment_version_id, se.activity_id,
		       se.user_id, se.started_at, se.elapsed_time_s,
		       se.is_reverse, se.sport_type, se.sub_type, se.gear_id,
		       se.avg_power_w, se.avg_heart_rate, se.avg_speed_ms,
		       se.rank_overall, se.is_personal_best, se.personal_rank, se.is_segment_record,
		       se.in_leaderboard, se.created_at,
		       s.name AS segment_name,
		       NULL::text AS display_name,
		       NULL::text AS gear_name
		FROM segment_efforts se
		JOIN segments s ON s.id = se.segment_id
		WHERE se.user_id = $1
		  AND se.personal_rank IS NOT NULL
		  AND se.personal_rank <= 3
		ORDER BY s.name ASC, se.personal_rank ASC`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("get user segment prs: %w", err)
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
			&e.RankOverall, &e.IsPersonalBest, &e.PersonalRank, &e.IsSegmentRecord,
			&e.InLeaderboard, &e.CreatedAt,
			&e.SegmentName, &e.DisplayName, &e.GearName,
		)
		if err != nil {
			return nil, fmt.Errorf("scan segment pr: %w", err)
		}
		efforts = append(efforts, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get user segment prs: %w", err)
	}
	if efforts == nil {
		efforts = []model.SegmentEffort{}
	}
	return efforts, nil
}

// GlobalTopSegmentEffort wraps a SegmentEffort with the segment's current athlete count,
// so the frontend can render "#rank von N" labels.
type GlobalTopSegmentEffort struct {
	model.SegmentEffort
	AthleteCount int `json:"athlete_count"`
}

// GetUserGlobalTopSegmentEfforts returns all segment efforts where the user is ranked
// rank_overall <= 10 and the effort is in the leaderboard.
// Ordered by rank_overall ASC, then segment name ASC.
func (db *DB) GetUserGlobalTopSegmentEfforts(ctx context.Context, userID string) ([]GlobalTopSegmentEffort, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT se.id, se.segment_id, se.segment_version_id, se.activity_id,
		       se.user_id, se.started_at, se.elapsed_time_s,
		       se.is_reverse, se.sport_type, se.sub_type, se.gear_id,
		       se.avg_power_w, se.avg_heart_rate, se.avg_speed_ms,
		       se.rank_overall, se.is_personal_best, se.personal_rank, se.is_segment_record,
		       se.in_leaderboard, se.created_at,
		       s.name AS segment_name,
		       NULL::text AS display_name,
		       NULL::text AS gear_name,
		       s.athlete_count
		FROM segment_efforts se
		JOIN segments s ON s.id = se.segment_id
		WHERE se.user_id = $1
		  AND se.rank_overall IS NOT NULL
		  AND se.rank_overall <= 10
		  AND se.in_leaderboard = true
		ORDER BY se.rank_overall ASC, s.name ASC`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("get user global top segment efforts: %w", err)
	}
	defer rows.Close()

	var efforts []GlobalTopSegmentEffort
	for rows.Next() {
		var e GlobalTopSegmentEffort
		err := rows.Scan(
			&e.ID, &e.SegmentID, &e.SegmentVersionID, &e.ActivityID,
			&e.UserID, &e.StartedAt, &e.ElapsedTimeS,
			&e.IsReverse, &e.SportType, &e.SubType, &e.GearID,
			&e.AvgPowerW, &e.AvgHeartRate, &e.AvgSpeedMs,
			&e.RankOverall, &e.IsPersonalBest, &e.PersonalRank, &e.IsSegmentRecord,
			&e.InLeaderboard, &e.CreatedAt,
			&e.SegmentName, &e.DisplayName, &e.GearName,
			&e.AthleteCount,
		)
		if err != nil {
			return nil, fmt.Errorf("scan global top segment effort: %w", err)
		}
		efforts = append(efforts, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get user global top segment efforts: %w", err)
	}
	if efforts == nil {
		efforts = []GlobalTopSegmentEffort{}
	}
	return efforts, nil
}

func encodeCursor(t time.Time, id string) string {
	raw := t.Format(time.RFC3339Nano) + "|" + id
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

// GetSegmentsForExport returns minimal segment data needed for GPX/GeoJSON export.
// Only returns segments the user can access (own or public).
func (db *DB) GetSegmentsForExport(ctx context.Context, ids []string, userID string) ([]model.Segment, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT s.id, s.name, s.distance_m, s.elevation_gain_m,
		       ST_AsGeoJSON(sv.track) AS track_geojson
		FROM segments s
		LEFT JOIN LATERAL (
		    SELECT track FROM segment_versions
		    WHERE segment_id = s.id
		    ORDER BY version_number DESC LIMIT 1
		) sv ON true
		WHERE s.id = ANY($1)
		  AND (s.visibility = 'public' OR s.created_by = $2)
		  AND sv.track IS NOT NULL`, ids, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []model.Segment
	for rows.Next() {
		var seg model.Segment
		if err := rows.Scan(&seg.ID, &seg.Name, &seg.DistanceM, &seg.ElevationGainM, &seg.TrackGeoJSON); err != nil {
			return nil, err
		}
		result = append(result, seg)
	}
	return result, rows.Err()
}

func decodeCursor(cursor string) (time.Time, string, error) {
	raw, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", err
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, "", fmt.Errorf("invalid cursor format")
	}
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", err
	}
	return t, parts[1], nil
}
