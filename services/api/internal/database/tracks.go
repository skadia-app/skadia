package database

import (
	"context"
	"fmt"
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

// RebuildHeatmapData deletes and re-inserts both heatmap_lines and heatmap_points
// for a single activity after its track geometry has changed (e.g. following a
// GPS merge). Uses a transaction with a shared temp table so the expensive track
// decomposition (ST_DumpPoints + ST_Distance) runs only once.
func (db *DB) RebuildHeatmapData(ctx context.Context, activityID string) error {
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
