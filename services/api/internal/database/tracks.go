package database

import (
	"context"
	"fmt"
	"math"
)

// RebuildActivityTrack rebuilds track_full, track_simplified, and bbox
// from the activity_streams coordinates.
func (db *DB) RebuildActivityTrack(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE activity_tracks SET
			track_full = sub.full_track,
			track_simplified = ST_Simplify(sub.full_track, 0.0001),
			bbox = ST_Envelope(sub.full_track)
		FROM (
			SELECT ST_MakeLine(
				ST_SetSRID(ST_MakePoint(lng, lat), 4326) ORDER BY time
			) AS full_track
			FROM activity_streams
			WHERE activity_id = $1 AND lat IS NOT NULL AND lng IS NOT NULL
		) sub
		WHERE activity_id = $1`, activityID)
	return err
}

// EnsureActivityTrack creates or updates the activity track from GPS streams.
// Unlike RebuildActivityTrack (which only UPDATEs an existing row), this uses
// INSERT ON CONFLICT to handle activities that have streams but no
// activity_tracks row yet (e.g. GPX imports where TrackPoints was empty).
// The HAVING COUNT(*) >= 2 ensures we don't create degenerate single-point
// tracks. Returns nil if no GPS points exist (no row created).
func (db *DB) EnsureActivityTrack(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx, `
		WITH pts AS (
			SELECT ST_SetSRID(ST_MakePoint(lng, lat), 4326) AS geom,
			       time AS ts
			FROM activity_streams
			WHERE activity_id = $1
			  AND lat IS NOT NULL AND lng IS NOT NULL
			ORDER BY time
		),
		track AS (
			SELECT ST_MakeLine(geom ORDER BY ts) AS full_track,
			       COUNT(*) AS cnt
			FROM pts
			HAVING COUNT(*) >= 2
		)
		INSERT INTO activity_tracks (activity_id, track_full, track_simplified, bbox, point_count)
		SELECT $1, full_track,
		       ST_Simplify(full_track, 0.0001),
		       ST_Envelope(full_track),
		       cnt
		FROM track
		ON CONFLICT (activity_id) DO UPDATE SET
		    track_full = EXCLUDED.track_full,
		    track_simplified = EXCLUDED.track_simplified,
		    bbox = EXCLUDED.bbox,
		    point_count = EXCLUDED.point_count`,
		activityID)
	return err
}

// RebuildHeatmapData deletes and re-inserts both heatmap_lines and heatmap_points
// for a single activity after its track geometry has changed (e.g. following a
// GPS merge). Uses a transaction with a shared temp table so the expensive track
// decomposition (ST_DumpPoints + ST_Distance) runs only once.
func (db *DB) RebuildHeatmapData(ctx context.Context, activityID string) error {
	var excludeHeatmap, isGroupSecondary bool
	_ = db.Pool.QueryRow(ctx, "SELECT exclude_from_heatmap, is_group_secondary FROM activities WHERE id = $1", activityID).Scan(&excludeHeatmap, &isGroupSecondary)
	if excludeHeatmap || isGroupSecondary {
		return db.DeleteHeatmapData(ctx, activityID)
	}

	// Remove existing heatmap entries for this activity.
	if err := db.DeleteHeatmapData(ctx, activityID); err != nil {
		return err
	}

	// Look up user's heatmap sampling interval.
	var userID string
	_ = db.Pool.QueryRow(ctx,
		`SELECT user_id FROM activities WHERE id = $1`, activityID,
	).Scan(&userID)
	intervalM := 20 // default
	if userID != "" {
		if v, err := db.GetUserHeatmapInterval(ctx, userID); err == nil {
			intervalM = v
		}
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("rebuild heatmap data: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Step 1: Shared track decomposition into temp table.
	_, err = tx.Exec(ctx, `
		CREATE TEMP TABLE _heatmap_rebuild ON COMMIT DROP AS
		WITH pts AS (
			SELECT (ST_DumpPoints(COALESCE(t.track_public, t.track_full))).path[1] AS idx,
			       (ST_DumpPoints(COALESCE(t.track_public, t.track_full))).geom  AS pt
			FROM activity_tracks t
			WHERE t.activity_id = $1
			  AND t.track_full IS NOT NULL
		),
		with_dist AS (
			SELECT idx, pt,
				COALESCE(
					ST_Distance(pt::geography, LAG(pt) OVER (ORDER BY idx)::geography),
					0
				) AS prev_dist
			FROM pts
		),
		grouped AS (
			SELECT idx, pt,
				SUM(CASE WHEN prev_dist > 2000 THEN 1 ELSE 0 END)
					OVER (ORDER BY idx) AS grp
			FROM with_dist
		)
		SELECT grp, ST_MakeLine(pt ORDER BY idx) AS geom
		FROM grouped
		GROUP BY grp
		HAVING COUNT(*) >= 2`,
		activityID,
	)
	if err != nil {
		return fmt.Errorf("rebuild heatmap data: create temp table: %w", err)
	}

	// Step 2: Insert into heatmap_lines.
	_, err = tx.Exec(ctx, `
		INSERT INTO heatmap_lines (user_id, activity_id, sport_type, sub_type, geom, geom_3857, activity_date)
		SELECT a.user_id, $1, a.sport_type, a.sub_type, r.geom, ST_Transform(r.geom, 3857), a.started_at::date
		FROM _heatmap_rebuild r
		CROSS JOIN activities a
		WHERE a.id = $1
		  AND r.geom IS NOT NULL`,
		activityID,
	)
	if err != nil {
		return fmt.Errorf("rebuild heatmap data: insert lines: %w", err)
	}

	// Step 3: Insert into heatmap_points (with ST_Segmentize).
	_, err = tx.Exec(ctx, `
		INSERT INTO heatmap_points (user_id, activity_id, sport_type, sub_type, activity_date, geom)
		SELECT a.user_id, $1, a.sport_type, a.sub_type, a.started_at::date,
		       (ST_DumpPoints(ST_Segmentize(r.geom::geography, $2)::geometry)).geom
		FROM _heatmap_rebuild r
		CROSS JOIN activities a
		WHERE a.id = $1
		  AND r.geom IS NOT NULL`,
		activityID, intervalM,
	)
	if err != nil {
		return fmt.Errorf("rebuild heatmap data: insert points: %w", err)
	}

	return tx.Commit(ctx)
}

// BatchTrackIntersectsCircle checks which activities have tracks intersecting a circle.
func (db *DB) BatchTrackIntersectsCircle(ctx context.Context, activityIDs []string, lat, lon, radiusM float64) (map[string]bool, error) {
	if len(activityIDs) == 0 {
		return nil, nil
	}
	rows, err := db.Pool.Query(ctx, `
		SELECT activity_id, ST_DWithin(
			track_simplified::geography,
			ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography,
			$3
		) FROM activity_tracks WHERE activity_id = ANY($4)`,
		lon, lat, radiusM, activityIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]bool, len(activityIDs))
	for rows.Next() {
		var id string
		var intersects bool
		if err := rows.Scan(&id, &intersects); err != nil {
			return nil, err
		}
		result[id] = intersects
	}
	return result, rows.Err()
}

// BatchDwellInCircle checks which activities have GPS points dwelling within a circle
// for at least minDwellSeconds. Dwell is determined by: time span of low-speed
// (< 3 m/s ≈ 10 km/h) GPS points within the geofence exceeds the threshold.
func (db *DB) BatchDwellInCircle(ctx context.Context, activityIDs []string, lat, lon, radiusM float64, minDwellSeconds int) (map[string]bool, error) {
	if len(activityIDs) == 0 {
		return nil, nil
	}
	// Bounding box pre-filter: ~0.002° lat ≈ 220m, adjust by radius
	degLat := radiusM / 111000.0
	degLon := radiusM / (111000.0 * math.Cos(lat*math.Pi/180))

	rows, err := db.Pool.Query(ctx, `
		SELECT activity_id,
			MAX(time) - MIN(time) >= make_interval(secs => $5::float)
		FROM activity_streams
		WHERE activity_id = ANY($6)
		AND lat IS NOT NULL AND lng IS NOT NULL
		AND lat BETWEEN ($1 - $3) AND ($1 + $3)
		AND lng BETWEEN ($2 - $4) AND ($2 + $4)
		AND (speed_ms IS NULL OR speed_ms < 3.0)
		GROUP BY activity_id`,
		lat, lon, degLat, degLon, float64(minDwellSeconds), activityIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]bool, len(activityIDs))
	for rows.Next() {
		var id string
		var dwells bool
		if err := rows.Scan(&id, &dwells); err != nil {
			return nil, err
		}
		result[id] = dwells
	}
	return result, rows.Err()
}

// BatchTrackIntersectsPolygon checks which activities have tracks intersecting a GeoJSON polygon.
func (db *DB) BatchTrackIntersectsPolygon(ctx context.Context, activityIDs []string, geoJSONPolygon string) (map[string]bool, error) {
	if len(activityIDs) == 0 {
		return nil, nil
	}
	rows, err := db.Pool.Query(ctx, `
		SELECT activity_id, ST_Intersects(
			track_simplified,
			ST_SetSRID(ST_GeomFromGeoJSON($1), 4326)
		) FROM activity_tracks WHERE activity_id = ANY($2)`,
		geoJSONPolygon, activityIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]bool, len(activityIDs))
	for rows.Next() {
		var id string
		var intersects bool
		if err := rows.Scan(&id, &intersects); err != nil {
			return nil, err
		}
		result[id] = intersects
	}
	return result, rows.Err()
}
