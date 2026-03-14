package enrichment

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

// RunGeoEnrichment executes all 5 geo-enrichment queries directly in Go SQL.
// It replaces the Python modules peaks.py, settlements.py, forests.py,
// water.py and borders.py. Each module runs synchronously; failures are
// logged and do not abort subsequent modules.
//
// Track source: activity_tracks.track_full (identical to the Python code).
func RunGeoEnrichment(ctx context.Context, db *database.DB, activityID string) error {
	slog.Info("running geo enrichment (Go SQL)", "activity_id", activityID)

	modules := []struct {
		name string
		fn   func(context.Context, *database.DB, string) error
	}{
		{"peaks", enrichGeoTagsPeaks},
		{"settlements", enrichGeoTagsSettlements},
		{"forests", enrichGeoTagsForests},
		{"water", enrichGeoTagsWater},
		{"borders", enrichGeoTagsBorders},
	}

	for _, m := range modules {
		if err := m.fn(ctx, db, activityID); err != nil {
			slog.Warn("geo enrichment module failed",
				"module", m.name,
				"activity_id", activityID,
				"error", err,
			)
		}
	}

	// Surfaces: only active in Full mode (osm_ways must exist and be populated).
	// Writes to activity_surfaces; skips silently if osm_ways is not present.
	if err := EnrichSurfaces(ctx, db, activityID); err != nil {
		slog.Warn("geo enrichment module failed",
			"module", "surfaces",
			"activity_id", activityID,
			"error", err,
		)
	}

	return nil
}

// enrichGeoTagsPeaks finds peaks and passes within 100 m of the track.
// Categories: 'summit' or 'pass' (based on osm_pois.type within category 'peak'/'pass').
// Mirrors peaks.py.
func enrichGeoTagsPeaks(ctx context.Context, db *database.DB, activityID string) error {
	// Delete existing summit/pass tags to allow re-enrichment.
	if _, err := db.Pool.Exec(ctx,
		"DELETE FROM activity_geo_tags WHERE activity_id = $1 AND category IN ('summit', 'pass')",
		activityID,
	); err != nil {
		return fmt.Errorf("peaks: delete existing tags: %w", err)
	}

	rows, err := db.Pool.Query(ctx, `
		WITH route AS (
			SELECT track_full AS geom
			FROM activity_tracks WHERE activity_id = $1
		)
		SELECT p.osm_id, p.name, p.ele, p.category,
		       ROUND(ST_Distance(route.geom::geography, p.geom::geography))::int AS dist_m,
		       ST_Y(ST_Centroid(p.geom)) AS lat, ST_X(ST_Centroid(p.geom)) AS lon
		FROM osm_pois p, route
		WHERE ST_DWithin(route.geom::geography, p.geom::geography, 100)
		  AND p.category IN ('peak', 'pass')
		  AND p.name IS NOT NULL
		ORDER BY dist_m
	`, activityID)
	if err != nil {
		return fmt.Errorf("peaks: query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var osmID int64
		var name, poiCategory string
		var ele *float64
		var distM int
		var lat, lon float64

		if err := rows.Scan(&osmID, &name, &ele, &poiCategory, &distM, &lat, &lon); err != nil {
			return fmt.Errorf("peaks: scan: %w", err)
		}

		category := "summit"
		if poiCategory == "pass" {
			category = "pass"
		}

		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO activity_geo_tags
			    (activity_id, category, name, osm_id, map_type,
			     map_point_lat, map_point_lon, ele_m, dist_m)
			VALUES ($1, $2, $3, $4, 'point', $5, $6, $7, $8)
		`, activityID, category, name, osmID, lat, lon, ele, distM); err != nil {
			slog.Warn("peaks: insert failed", "osm_id", osmID, "error", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("peaks: rows iteration: %w", err)
	}
	return nil
}

// enrichGeoTagsSettlements finds settlements within 200 m of the track.
// Category: 'settlement'. Mirrors settlements.py.
func enrichGeoTagsSettlements(ctx context.Context, db *database.DB, activityID string) error {
	if _, err := db.Pool.Exec(ctx,
		"DELETE FROM activity_geo_tags WHERE activity_id = $1 AND category = 'settlement'",
		activityID,
	); err != nil {
		return fmt.Errorf("settlements: delete existing tags: %w", err)
	}

	rows, err := db.Pool.Query(ctx, `
		WITH route AS (
			SELECT track_full AS geom
			FROM activity_tracks WHERE activity_id = $1
		)
		SELECT s.osm_id, s.name, s.type,
		       ST_Y(ST_Centroid(s.geom)) AS center_lat,
		       ST_X(ST_Centroid(s.geom)) AS center_lon
		FROM osm_pois s, route
		WHERE ST_DWithin(route.geom::geography, s.geom::geography, 200)
		  AND s.category = 'settlement'
		ORDER BY s.type, s.name
	`, activityID)
	if err != nil {
		return fmt.Errorf("settlements: query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var osmID int64
		var name string
		var place *string
		var centerLat, centerLon float64

		if err := rows.Scan(&osmID, &name, &place, &centerLat, &centerLon); err != nil {
			return fmt.Errorf("settlements: scan: %w", err)
		}

		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO activity_geo_tags
			    (activity_id, category, name, osm_id, map_type,
			     map_point_lat, map_point_lon)
			VALUES ($1, 'settlement', $2, $3, 'point', $4, $5)
		`, activityID, name, osmID, centerLat, centerLon); err != nil {
			slog.Warn("settlements: insert failed", "osm_id", osmID, "error", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("settlements: rows iteration: %w", err)
	}
	return nil
}

// enrichGeoTagsForests finds named forests intersected by the track (min 200 m).
// Category: 'forest'. Mirrors forests.py.
func enrichGeoTagsForests(ctx context.Context, db *database.DB, activityID string) error {
	if _, err := db.Pool.Exec(ctx,
		"DELETE FROM activity_geo_tags WHERE activity_id = $1 AND category = 'forest'",
		activityID,
	); err != nil {
		return fmt.Errorf("forests: delete existing tags: %w", err)
	}

	rows, err := db.Pool.Query(ctx, `
		WITH route AS (
			SELECT track_full AS geom
			FROM activity_tracks WHERE activity_id = $1
		),
		intersected AS (
			SELECT f.osm_id, f.name,
			       ST_Intersection(route.geom, f.geom) AS isect,
			       ST_XMin(f.geom) AS bbox_west, ST_YMin(f.geom) AS bbox_south,
			       ST_XMax(f.geom) AS bbox_east, ST_YMax(f.geom) AS bbox_north
			FROM osm_forests f, route
			WHERE ST_Intersects(f.geom, route.geom)
		)
		SELECT osm_id, name,
		       ROUND(ST_Length(isect::geography))::int AS length_m,
		       bbox_west, bbox_south, bbox_east, bbox_north
		FROM intersected
		WHERE ST_Length(isect::geography) >= 200
		ORDER BY length_m DESC
	`, activityID)
	if err != nil {
		return fmt.Errorf("forests: query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var osmID int64
		var name *string
		var lengthM int
		var bboxWest, bboxSouth, bboxEast, bboxNorth float64

		if err := rows.Scan(&osmID, &name, &lengthM, &bboxWest, &bboxSouth, &bboxEast, &bboxNorth); err != nil {
			return fmt.Errorf("forests: scan: %w", err)
		}

		displayName := "Wald"
		if name != nil && *name != "" {
			displayName = *name
		}

		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO activity_geo_tags
			    (activity_id, category, name, osm_id, map_type,
			     map_bbox_west, map_bbox_south, map_bbox_east, map_bbox_north, length_m)
			VALUES ($1, 'forest', $2, $3, 'bbox', $4, $5, $6, $7, $8)
		`, activityID, displayName, osmID, bboxWest, bboxSouth, bboxEast, bboxNorth, lengthM); err != nil {
			slog.Warn("forests: insert failed", "osm_id", osmID, "error", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("forests: rows iteration: %w", err)
	}
	return nil
}

// enrichGeoTagsWater finds water bodies and waterways within 50 m of the track.
// Still water (osm_waters) → category 'still_water'.
// Flowing water (osm_waterways) → category 'flowing_water'.
// Mirrors water.py.
func enrichGeoTagsWater(ctx context.Context, db *database.DB, activityID string) error {
	if _, err := db.Pool.Exec(ctx,
		"DELETE FROM activity_geo_tags WHERE activity_id = $1 AND category IN ('still_water', 'flowing_water')",
		activityID,
	); err != nil {
		return fmt.Errorf("water: delete existing tags: %w", err)
	}

	// --- Still water (lakes, reservoirs) ---
	stillRows, err := db.Pool.Query(ctx, `
		WITH route AS (
			SELECT track_full AS geom
			FROM activity_tracks WHERE activity_id = $1
		)
		SELECT DISTINCT ON (w.osm_id) w.osm_id, w.name,
		       ROUND(ST_Distance(route.geom::geography, w.geom::geography))::int AS min_dist_m,
		       ST_XMin(w.geom) AS bbox_west, ST_YMin(w.geom) AS bbox_south,
		       ST_XMax(w.geom) AS bbox_east, ST_YMax(w.geom) AS bbox_north
		FROM osm_waters w, route
		WHERE ST_DWithin(route.geom::geography, w.geom::geography, 50)
		  AND w.name IS NOT NULL
		ORDER BY w.osm_id, min_dist_m
	`, activityID)
	if err != nil {
		return fmt.Errorf("water: still_water query: %w", err)
	}
	defer stillRows.Close()

	for stillRows.Next() {
		var osmID int64
		var name string
		var distM int
		var bboxWest, bboxSouth, bboxEast, bboxNorth float64

		if err := stillRows.Scan(&osmID, &name, &distM, &bboxWest, &bboxSouth, &bboxEast, &bboxNorth); err != nil {
			return fmt.Errorf("water: still_water scan: %w", err)
		}

		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO activity_geo_tags
			    (activity_id, category, name, osm_id, map_type,
			     map_bbox_west, map_bbox_south, map_bbox_east, map_bbox_north, dist_m)
			VALUES ($1, 'still_water', $2, $3, 'bbox', $4, $5, $6, $7, $8)
		`, activityID, name, osmID, bboxWest, bboxSouth, bboxEast, bboxNorth, distM); err != nil {
			slog.Warn("water: still_water insert failed", "osm_id", osmID, "error", err)
		}
	}
	if err := stillRows.Err(); err != nil {
		return fmt.Errorf("water: still_water rows iteration: %w", err)
	}

	// --- Flowing water (rivers, streams) ---
	flowRows, err := db.Pool.Query(ctx, `
		WITH route AS (
			SELECT track_full AS geom
			FROM activity_tracks WHERE activity_id = $1
		)
		SELECT DISTINCT ON (ww.osm_id) ww.osm_id, ww.name, ww.waterway,
		       ROUND(ST_Distance(route.geom::geography, ww.geom::geography))::int AS min_dist_m,
		       ST_Intersection(
		           ST_Buffer(route.geom::geography, 50)::geometry, ww.geom
		       ) AS nearby_section
		FROM osm_waterways ww, route
		WHERE ST_DWithin(route.geom::geography, ww.geom::geography, 50)
		  AND ww.name IS NOT NULL
		ORDER BY ww.osm_id, min_dist_m
	`, activityID)
	if err != nil {
		return fmt.Errorf("water: flowing_water query: %w", err)
	}
	defer flowRows.Close()

	for flowRows.Next() {
		var osmID int64
		var name string
		var waterway *string
		var distM int
		var nearbySection []byte // WKB geometry

		if err := flowRows.Scan(&osmID, &name, &waterway, &distM, &nearbySection); err != nil {
			return fmt.Errorf("water: flowing_water scan: %w", err)
		}

		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO activity_geo_tags
			    (activity_id, category, name, osm_id, map_type,
			     map_section, dist_m)
			VALUES ($1, 'flowing_water', $2, $3, 'section',
			        ST_GeomFromWKB($4, 4326), $5)
		`, activityID, name, osmID, nearbySection, distM); err != nil {
			slog.Warn("water: flowing_water insert failed", "osm_id", osmID, "error", err)
		}
	}
	if err := flowRows.Err(); err != nil {
		return fmt.Errorf("water: flowing_water rows iteration: %w", err)
	}
	return nil
}

// enrichGeoTagsBorders detects country/state border crossings along the track.
// Category: 'border'. Mirrors borders.py.
func enrichGeoTagsBorders(ctx context.Context, db *database.DB, activityID string) error {
	if _, err := db.Pool.Exec(ctx,
		"DELETE FROM activity_geo_tags WHERE activity_id = $1 AND category = 'border'",
		activityID,
	); err != nil {
		return fmt.Errorf("borders: delete existing tags: %w", err)
	}

	rows, err := db.Pool.Query(ctx, `
		WITH route AS (
			SELECT track_full AS geom
			FROM activity_tracks WHERE activity_id = $1
		)
		SELECT b.name, b.admin_level, b.country_code,
		       ST_Intersection(route.geom, ST_Boundary(b.geom)) AS crossing_geom
		FROM osm_boundaries b, route
		WHERE ST_Crosses(route.geom, ST_Boundary(b.geom))
		  AND b.admin_level IN (2, 4)
		ORDER BY b.admin_level
	`, activityID)
	if err != nil {
		return fmt.Errorf("borders: query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name *string
		var adminLevel int
		var countryCode *string
		var crossingGeom []byte // WKB geometry

		if err := rows.Scan(&name, &adminLevel, &countryCode, &crossingGeom); err != nil {
			return fmt.Errorf("borders: scan: %w", err)
		}

		displayName := "Grenze"
		if name != nil && *name != "" {
			displayName = *name
		}

		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO activity_geo_tags
			    (activity_id, category, name, map_type, map_section)
			VALUES ($1, 'border', $2, 'section', ST_GeomFromWKB($3, 4326))
		`, activityID, displayName, crossingGeom); err != nil {
			slog.Warn("borders: insert failed", "admin_level", adminLevel, "error", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("borders: rows iteration: %w", err)
	}
	return nil
}
