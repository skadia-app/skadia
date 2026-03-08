package database

import "context"

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

// RebuildHeatmapData deletes and re-inserts heatmap_lines for a single activity
// after its track geometry has changed (e.g. following a GPS merge).
func (db *DB) RebuildHeatmapData(ctx context.Context, activityID string) error {
	// Remove existing heatmap entries for this activity.
	if err := db.DeleteHeatmapLines(ctx, activityID); err != nil {
		return err
	}

	// Re-insert using the same teleportation-gap logic as InsertHeatmapLine,
	// pulling user_id, sport_type, sub_type, and started_at from the activities table.
	_, err := db.Pool.Exec(ctx, `
		WITH act AS (
			SELECT user_id, sport_type, sub_type, started_at
			FROM activities
			WHERE id = $1
		),
		pts AS (
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
		),
		line_groups AS (
			SELECT grp, ST_MakeLine(pt ORDER BY idx) AS geom
			FROM grouped
			GROUP BY grp
			HAVING COUNT(*) >= 2
		)
		INSERT INTO heatmap_lines (user_id, activity_id, sport_type, sub_type, geom, geom_3857, activity_date)
		SELECT a.user_id, $1, a.sport_type, a.sub_type, lg.geom, ST_Transform(lg.geom, 3857), a.started_at::date
		FROM line_groups lg
		CROSS JOIN act a
		WHERE lg.geom IS NOT NULL`,
		activityID)
	return err
}
