package database

import (
	"context"
	"fmt"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// InsertHeatmapData inserts both heatmap_lines and heatmap_points for an
// activity in a single transaction, sharing the expensive track decomposition
// (ST_DumpPoints + ST_Distance teleportation detection + ST_MakeLine grouping).
// Uses COALESCE(track_public, track_full) so privacy zones are respected.
// Segments longer than 2000 m (GPS teleportation jumps) are treated as group
// boundaries: each contiguous group of points is stored as a separate row so
// that straight "teleportation lines" never appear in the heatmap.
func (db *DB) InsertHeatmapData(ctx context.Context, userID, activityID, sportType string, subType *string, startedAt time.Time, intervalM int) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("insert heatmap data: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Step 1: Shared track decomposition into temp table (dropped on commit).
	_, err = tx.Exec(ctx, `
		CREATE TEMP TABLE _heatmap_groups ON COMMIT DROP AS
		WITH pts AS (
		  SELECT (ST_DumpPoints(COALESCE(track_public, track_full))).path[1] AS idx,
		         (ST_DumpPoints(COALESCE(track_public, track_full))).geom  AS pt
		  FROM activity_tracks
		  WHERE activity_id = $1
		    AND track_full IS NOT NULL
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
		return fmt.Errorf("insert heatmap data: create temp table: %w", err)
	}

	// Step 2: Insert into heatmap_lines from the shared decomposition.
	_, err = tx.Exec(ctx, `
		INSERT INTO heatmap_lines (user_id, activity_id, sport_type, sub_type, geom, geom_3857, activity_date)
		SELECT $1, $2, $3, $5, geom, ST_Transform(geom, 3857), $4::date
		FROM _heatmap_groups
		WHERE geom IS NOT NULL`,
		userID, activityID, sportType, startedAt, subType,
	)
	if err != nil {
		return fmt.Errorf("insert heatmap data: insert lines: %w", err)
	}

	// Step 3: Insert into heatmap_points (with ST_Segmentize) from the shared decomposition.
	_, err = tx.Exec(ctx, `
		INSERT INTO heatmap_points (user_id, activity_id, sport_type, sub_type, activity_date, geom)
		SELECT $1, $2, $3, $5, $4::date,
		       (ST_DumpPoints(ST_Segmentize(geom::geography, $6)::geometry)).geom
		FROM _heatmap_groups
		WHERE geom IS NOT NULL`,
		userID, activityID, sportType, startedAt, subType, intervalM,
	)
	if err != nil {
		return fmt.Errorf("insert heatmap data: insert points: %w", err)
	}

	return tx.Commit(ctx)
}

// DeleteHeatmapData removes both heatmap_lines and heatmap_points for an activity.
func (db *DB) DeleteHeatmapData(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM heatmap_lines WHERE activity_id = $1`, activityID)
	if err != nil {
		return fmt.Errorf("delete heatmap lines: %w", err)
	}
	_, err = db.Pool.Exec(ctx, `DELETE FROM heatmap_points WHERE activity_id = $1`, activityID)
	if err != nil {
		return fmt.Errorf("delete heatmap points: %w", err)
	}
	return nil
}

// BackfillHeatmapAll populates both heatmap_lines and heatmap_points for all
// existing activities of a user that are missing from EITHER table, sharing
// the expensive track decomposition. Returns the number of activities processed.
func (db *DB) BackfillHeatmapAll(ctx context.Context, userID string, intervalM int) (int, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("backfill heatmap: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Step 1: Find activities missing from either table and decompose tracks.
	_, err = tx.Exec(ctx, `
		CREATE TEMP TABLE _backfill_groups ON COMMIT DROP AS
		WITH activities_to_fill AS (
		  SELECT a.id AS activity_id, a.user_id, a.sport_type, a.sub_type, a.started_at
		  FROM activities a
		  JOIN activity_tracks t ON t.activity_id = a.id
		  WHERE a.user_id = $1
		    AND t.track_full IS NOT NULL
		    AND NOT a.exclude_from_heatmap
		    AND (
		        NOT EXISTS (SELECT 1 FROM heatmap_lines h WHERE h.activity_id = a.id)
		        OR NOT EXISTS (SELECT 1 FROM heatmap_points h WHERE h.activity_id = a.id)
		    )
		),
		pts AS (
		  SELECT af.activity_id, af.user_id, af.sport_type, af.sub_type, af.started_at,
		         (ST_DumpPoints(COALESCE(t.track_public, t.track_full))).path[1] AS idx,
		         (ST_DumpPoints(COALESCE(t.track_public, t.track_full))).geom  AS pt
		  FROM activities_to_fill af
		  JOIN activity_tracks t ON t.activity_id = af.activity_id
		),
		with_dist AS (
		  SELECT activity_id, user_id, sport_type, sub_type, started_at, idx, pt,
		    COALESCE(
		      ST_Distance(pt::geography,
		        LAG(pt) OVER (PARTITION BY activity_id ORDER BY idx)::geography),
		      0
		    ) AS prev_dist
		  FROM pts
		),
		grouped AS (
		  SELECT activity_id, user_id, sport_type, sub_type, started_at, idx, pt,
		    SUM(CASE WHEN prev_dist > 2000 THEN 1 ELSE 0 END)
		      OVER (PARTITION BY activity_id ORDER BY idx) AS grp
		  FROM with_dist
		)
		SELECT activity_id, user_id, sport_type, sub_type, started_at, grp,
		       ST_MakeLine(pt ORDER BY idx) AS geom
		FROM grouped
		GROUP BY activity_id, user_id, sport_type, sub_type, started_at, grp
		HAVING COUNT(*) >= 2`,
		userID,
	)
	if err != nil {
		return 0, fmt.Errorf("backfill heatmap: create temp table: %w", err)
	}

	// Step 2: Insert into heatmap_lines (skip activities already present).
	_, err = tx.Exec(ctx, `
		INSERT INTO heatmap_lines (user_id, activity_id, sport_type, sub_type, geom, geom_3857, activity_date)
		SELECT user_id, activity_id, sport_type, sub_type, geom, ST_Transform(geom, 3857), started_at::date
		FROM _backfill_groups bg
		WHERE geom IS NOT NULL
		  AND NOT EXISTS (SELECT 1 FROM heatmap_lines h WHERE h.activity_id = bg.activity_id)`,
	)
	if err != nil {
		return 0, fmt.Errorf("backfill heatmap: insert lines: %w", err)
	}

	// Step 3: Insert into heatmap_points (skip activities already present).
	result, err := tx.Exec(ctx, `
		INSERT INTO heatmap_points (user_id, activity_id, sport_type, sub_type, activity_date, geom)
		SELECT user_id, activity_id, sport_type, sub_type, started_at::date,
		       (ST_DumpPoints(ST_Segmentize(geom::geography, $1)::geometry)).geom
		FROM _backfill_groups bg
		WHERE geom IS NOT NULL
		  AND NOT EXISTS (SELECT 1 FROM heatmap_points h WHERE h.activity_id = bg.activity_id)`,
		intervalM,
	)
	if err != nil {
		return 0, fmt.Errorf("backfill heatmap: insert points: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("backfill heatmap: commit: %w", err)
	}

	// Return distinct activity count from the backfill (approximate via points rows).
	return int(result.RowsAffected()), nil
}

// GetHeatmapSettings returns all heatmap-related settings for a user.
func (db *DB) GetHeatmapSettings(ctx context.Context, userID string) (*model.HeatmapSettings, error) {
	var s model.HeatmapSettings
	err := db.Pool.QueryRow(ctx, `
		SELECT COALESCE(heatmap_sample_interval_m, 20),
		       COALESCE(teleport_threshold_m, 2000),
		       COALESCE(teleport_filter_map, true),
		       COALESCE(teleport_filter_activity, true)
		FROM users WHERE id = $1`, userID,
	).Scan(&s.SamplingIntervalM, &s.TeleportThresholdM, &s.TeleportFilterMap, &s.TeleportFilterActivity)
	if err != nil {
		// Defaults falls User nicht gefunden
		return &model.HeatmapSettings{SamplingIntervalM: 20, TeleportThresholdM: 2000, TeleportFilterMap: true, TeleportFilterActivity: true}, nil
	}
	return &s, nil
}

// UpdateHeatmapSettings writes all heatmap-related settings for a user.
func (db *DB) UpdateHeatmapSettings(ctx context.Context, userID string, s *model.HeatmapSettings) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE users SET
		    heatmap_sample_interval_m = $2,
		    teleport_threshold_m = $3,
		    teleport_filter_map = $4,
		    teleport_filter_activity = $5
		WHERE id = $1`,
		userID, s.SamplingIntervalM, s.TeleportThresholdM, s.TeleportFilterMap, s.TeleportFilterActivity,
	)
	return err
}

// GetUserHeatmapInterval returns the user's heatmap sampling interval in meters.
func (db *DB) GetUserHeatmapInterval(ctx context.Context, userID string) (int, error) {
	s, err := db.GetHeatmapSettings(ctx, userID)
	if err != nil {
		return 20, nil
	}
	return s.SamplingIntervalM, nil
}

// UpdateUserHeatmapInterval is a compatibility wrapper (updates only the sampling interval).
func (db *DB) UpdateUserHeatmapInterval(ctx context.Context, userID string, intervalM int) error {
	s, err := db.GetHeatmapSettings(ctx, userID)
	if err != nil {
		s = &model.HeatmapSettings{SamplingIntervalM: intervalM, TeleportThresholdM: 2000, TeleportFilterMap: true, TeleportFilterActivity: true}
	} else {
		s.SamplingIntervalM = intervalM
	}
	return db.UpdateHeatmapSettings(ctx, userID, s)
}
