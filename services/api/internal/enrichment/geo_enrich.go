package enrichment

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/jackc/pgx/v5"
	"golang.org/x/sync/errgroup"
)

// GeoEnrichmentSettings contains the per-user flags that gate individual
// geo-enrichment modules. All flags default to true so that callers which
// don't have settings available can pass a zero-value struct (all false
// would skip everything, so always use allGeoEnabled or populate from
// model.EnrichmentSettings).
type GeoEnrichmentSettings struct {
	EnableSummitDetection     bool
	EnableSettlementDetection bool
	EnableForestDetection     bool
	EnableWaterDetection      bool
	EnableBorderDetection     bool
	EnableSurfaceDetection    bool
}

// geoSettingsFromModel converts a model.EnrichmentSettings into the smaller
// GeoEnrichmentSettings struct used by RunGeoEnrichment.
func geoSettingsFromModel(s *model.EnrichmentSettings) GeoEnrichmentSettings {
	return GeoEnrichmentSettings{
		EnableSummitDetection:     s.EnableSummitDetection,
		EnableSettlementDetection: s.EnableSettlementDetection,
		EnableForestDetection:     s.EnableForestDetection,
		EnableWaterDetection:      s.EnableWaterDetection,
		EnableBorderDetection:     s.EnableBorderDetection,
		EnableSurfaceDetection:    s.EnableSurfaceDetection,
	}
}

// allGeoEnabled returns a GeoEnrichmentSettings with every flag set to true,
// used as a fallback when no user settings are available.
func allGeoEnabled() GeoEnrichmentSettings {
	return GeoEnrichmentSettings{
		EnableSummitDetection:     true,
		EnableSettlementDetection: true,
		EnableForestDetection:     true,
		EnableWaterDetection:      true,
		EnableBorderDetection:     true,
		EnableSurfaceDetection:    true,
	}
}

// geoEnrichSem is a global semaphore that limits concurrent geo-enrichment
// runs to 1. Without this limit two simultaneous enrichments (e.g. bulk
// re-enrichment) each spawn up to 6 parallel PostGIS queries, totalling 12+
// heavy geometry operations that saturate the CPU and freeze the WebUI.
var geoEnrichSem = make(chan struct{}, 1)

// RunGeoEnrichment executes geo-enrichment queries directly in Go SQL.
// It covers peaks, settlements, forests, water, and borders.
// Lifts are handled by the Python ML service (segment matching + heuristic detection).
// Each module is gated by the corresponding flag in settings; pass
// allGeoEnabled() to run every module regardless of user preferences.
// Failures are logged per module and do not abort subsequent modules.
//
// All enabled modules run in parallel (max 3 concurrent) via errgroup to
// prevent DB connection pool exhaustion.
//
// At most 1 activity is geo-enriched at a time (geoEnrichSem) to prevent
// CPU saturation when multiple enrichment jobs run concurrently.
//
// Track source: activity_tracks.track_full (identical to the Python code).
// Queries use ST_Simplify(track_full, 0.00005) (~5.5 m tolerance) to reduce
// 40k-point tracks to ~4-6k points for spatial lookups — the stored track is
// never modified. Tolerance chosen to be safe for tightest DWithin (water: 50m).
func RunGeoEnrichment(ctx context.Context, db *database.DB, activityID string, settings GeoEnrichmentSettings) error {
	// Acquire global slot — blocks until the currently running enrichment
	// (if any) finishes, or until ctx is cancelled.
	select {
	case geoEnrichSem <- struct{}{}:
		defer func() { <-geoEnrichSem }()
	case <-ctx.Done():
		return ctx.Err()
	}
	slog.Info("running geo enrichment (Go SQL)", "activity_id", activityID)

	type module struct {
		name    string
		enabled bool
		fn      func(context.Context, *database.DB, string) error
	}
	modules := []module{
		{"summits", settings.EnableSummitDetection, enrichGeoTagsSummits},
		{"settlements", settings.EnableSettlementDetection, enrichGeoTagsSettlements},
		{"forests", settings.EnableForestDetection, enrichGeoTagsForests},
		{"water", settings.EnableWaterDetection, enrichGeoTagsWater},
		{"borders", settings.EnableBorderDetection, enrichGeoTagsBorders},
	}

	g, gCtx := errgroup.WithContext(ctx)
	sem := make(chan struct{}, 3) // max 3 parallel — prevent connection pool exhaustion

	for _, m := range modules {
		if !m.enabled {
			slog.Debug("geo enrichment module disabled, skipping",
				"module", m.name,
				"activity_id", activityID,
			)
			continue
		}
		g.Go(func() error {
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := m.fn(gCtx, db, activityID); err != nil {
				slog.Warn("geo enrichment module failed",
					"module", m.name,
					"activity_id", activityID,
					"error", err,
				)
			}
			return nil // individual module failures are NOT fatal
		})
	}

	return g.Wait()
}

// enrichGeoTagsSummits finds summits and passes within 100 m of the track.
// Categories: 'summit' or 'pass' (based on osm_pois.type within category 'summit'/'pass').
// Mirrors peaks.py.
func enrichGeoTagsSummits(ctx context.Context, db *database.DB, activityID string) error {
	// Delete existing summit/pass tags to allow re-enrichment.
	if _, err := db.Pool.Exec(ctx,
		"DELETE FROM activity_geo_tags WHERE activity_id = $1 AND category IN ('summit', 'pass')",
		activityID,
	); err != nil {
		return fmt.Errorf("summits: delete existing tags: %w", err)
	}

	rows, err := db.Pool.Query(ctx, `
		WITH route AS (
			SELECT ST_Simplify(track_full, 0.00005) AS geom
			FROM activity_tracks WHERE activity_id = $1
		)
		SELECT p.osm_id, p.name, p.ele, p.category,
		       ROUND(ST_Distance(route.geom::geography, p.geom::geography))::int AS dist_m,
		       ST_Y(ST_Centroid(p.geom)) AS lat, ST_X(ST_Centroid(p.geom)) AS lon
		FROM osm_pois p, route
		WHERE p.geom && ST_Expand(route.geom, 0.002)
		  AND ST_DWithin(route.geom::geography, p.geom::geography, 100)
		  AND p.category IN ('summit', 'pass')
		  AND p.name IS NOT NULL
		ORDER BY dist_m
	`, activityID)
	if err != nil {
		return fmt.Errorf("summits: query: %w", err)
	}
	defer rows.Close()

	batch := &pgx.Batch{}
	for rows.Next() {
		var osmID int64
		var name, poiCategory string
		var ele *float64
		var distM int
		var lat, lon float64

		if err := rows.Scan(&osmID, &name, &ele, &poiCategory, &distM, &lat, &lon); err != nil {
			return fmt.Errorf("summits: scan: %w", err)
		}

		category := "summit"
		if poiCategory == "pass" {
			category = "pass"
		}

		batch.Queue(`
			INSERT INTO activity_geo_tags
			    (activity_id, category, name, osm_id, map_type,
			     map_point_lat, map_point_lon, ele_m, dist_m)
			VALUES ($1, $2, $3, $4, 'point', $5, $6, $7, $8)
		`, activityID, category, name, osmID, lat, lon, ele, distM)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("summits: rows iteration: %w", err)
	}

	if batch.Len() > 0 {
		if err := db.Pool.SendBatch(ctx, batch).Close(); err != nil {
			slog.Warn("summits: batch insert failed", "activity_id", activityID, "error", err)
		}
	}
	return nil
}

// enrichGeoTagsSettlements finds settlements using a hybrid approach:
// 1. Boundaries (admin_level=8, ST_Intersects with >=200m track) for cities/municipalities
// 2. POI fallback (500m ST_DWithin) for hamlets/isolated dwellings not covered by boundaries
// Deduplication is geometry-based: a POI is excluded if its centroid falls inside
// any admin_level=8 boundary (handles same-named cities in different regions correctly).
func enrichGeoTagsSettlements(ctx context.Context, db *database.DB, activityID string) error {
	if _, err := db.Pool.Exec(ctx,
		"DELETE FROM activity_geo_tags WHERE activity_id = $1 AND category = 'settlement'",
		activityID,
	); err != nil {
		return fmt.Errorf("settlements: delete existing tags: %w", err)
	}

	rows, err := db.Pool.Query(ctx, `
		WITH route AS (
			SELECT ST_Simplify(track_full, 0.00005) AS geom
			FROM activity_tracks WHERE activity_id = $1
		),
		from_boundaries AS (
			SELECT b.osm_id, b.name, 'boundary'::text AS source, 8::smallint AS admin_level,
			       ST_Y(ST_Centroid(b.geom)) AS center_lat,
			       ST_X(ST_Centroid(b.geom)) AS center_lon
			FROM osm_boundaries b, route
			WHERE ST_Intersects(route.geom, b.geom)
			  AND b.boundary_type = 'administrative'
			  AND b.admin_level = 8
			  AND ST_Length(ST_Intersection(route.geom, b.geom)::geography) >= 200
		),
		from_poi AS (
			SELECT p.osm_id, p.name, 'poi'::text AS source, NULL::smallint AS admin_level,
			       ST_Y(ST_Centroid(p.geom)) AS center_lat,
			       ST_X(ST_Centroid(p.geom)) AS center_lon
			FROM osm_pois p, route
			WHERE p.geom && ST_Expand(route.geom, 0.007)
			  AND ST_DWithin(route.geom::geography, p.geom::geography, 500)
			  AND p.category = 'settlement'
			  AND NOT EXISTS (
			      SELECT 1 FROM osm_boundaries b
			      WHERE b.admin_level = 8
			        AND b.boundary_type = 'administrative'
			        AND ST_Contains(b.geom, ST_Centroid(p.geom))
			  )
		)
		SELECT osm_id, name, source, admin_level, center_lat, center_lon
		FROM from_boundaries
		UNION ALL
		SELECT osm_id, name, source, admin_level, center_lat, center_lon
		FROM from_poi
		ORDER BY name
	`, activityID)
	if err != nil {
		return fmt.Errorf("settlements: query: %w", err)
	}
	defer rows.Close()

	batch := &pgx.Batch{}
	for rows.Next() {
		var osmID int64
		var name, source string
		var adminLevel *int16
		var centerLat, centerLon float64

		if err := rows.Scan(&osmID, &name, &source, &adminLevel, &centerLat, &centerLon); err != nil {
			return fmt.Errorf("settlements: scan: %w", err)
		}

		batch.Queue(`
			INSERT INTO activity_geo_tags
			    (activity_id, category, name, osm_id, map_type,
			     map_point_lat, map_point_lon, source, admin_level)
			VALUES ($1, 'settlement', $2, $3, 'point', $4, $5, $6, $7)
		`, activityID, name, osmID, centerLat, centerLon, source, adminLevel)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("settlements: rows iteration: %w", err)
	}

	if batch.Len() > 0 {
		if err := db.Pool.SendBatch(ctx, batch).Close(); err != nil {
			slog.Warn("settlements: batch insert failed", "activity_id", activityID, "error", err)
		}
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
			SELECT ST_Simplify(track_full, 0.00005) AS geom
			FROM activity_tracks WHERE activity_id = $1
		),
		intersected AS (
			SELECT f.osm_id, f.name,
			       ROUND(ST_Length(ST_Intersection(route.geom, f.geom)::geography))::int AS length_m,
			       ST_XMin(f.geom) AS bbox_west, ST_YMin(f.geom) AS bbox_south,
			       ST_XMax(f.geom) AS bbox_east, ST_YMax(f.geom) AS bbox_north
			FROM osm_forests f, route
			WHERE ST_Intersects(f.geom, route.geom)
			  AND f.name IS NOT NULL
			  AND f.name != ''
		),
		filtered AS (
			SELECT osm_id, name, length_m,
			       bbox_west, bbox_south, bbox_east, bbox_north
			FROM intersected
			WHERE length_m >= 200
		),
		deduped AS (
			SELECT DISTINCT ON (name) osm_id, name, length_m,
			       bbox_west, bbox_south, bbox_east, bbox_north
			FROM filtered
			ORDER BY name, length_m DESC
		)
		SELECT osm_id, name, length_m,
		       bbox_west, bbox_south, bbox_east, bbox_north
		FROM deduped
		ORDER BY length_m DESC
	`, activityID)
	if err != nil {
		return fmt.Errorf("forests: query: %w", err)
	}
	defer rows.Close()

	batch := &pgx.Batch{}
	for rows.Next() {
		var osmID int64
		var name string
		var lengthM int
		var bboxWest, bboxSouth, bboxEast, bboxNorth float64

		if err := rows.Scan(&osmID, &name, &lengthM, &bboxWest, &bboxSouth, &bboxEast, &bboxNorth); err != nil {
			return fmt.Errorf("forests: scan: %w", err)
		}

		batch.Queue(`
			INSERT INTO activity_geo_tags
			    (activity_id, category, name, osm_id, map_type,
			     map_bbox_west, map_bbox_south, map_bbox_east, map_bbox_north, length_m)
			VALUES ($1, 'forest', $2, $3, 'bbox', $4, $5, $6, $7, $8)
		`, activityID, name, osmID, bboxWest, bboxSouth, bboxEast, bboxNorth, lengthM)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("forests: rows iteration: %w", err)
	}

	if batch.Len() > 0 {
		if err := db.Pool.SendBatch(ctx, batch).Close(); err != nil {
			slog.Warn("forests: batch insert failed", "activity_id", activityID, "error", err)
		}
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
			SELECT ST_Simplify(track_full, 0.00005) AS geom
			FROM activity_tracks WHERE activity_id = $1
		)
		SELECT DISTINCT ON (w.name) w.osm_id, w.name,
		       ROUND(ST_Distance(route.geom::geography, w.geom::geography))::int AS min_dist_m,
		       ST_XMin(w.geom) AS bbox_west, ST_YMin(w.geom) AS bbox_south,
		       ST_XMax(w.geom) AS bbox_east, ST_YMax(w.geom) AS bbox_north
		FROM osm_waters w, route
		WHERE w.geom && ST_Expand(route.geom, 0.001)
		  AND ST_DWithin(route.geom::geography, w.geom::geography, 50)
		  AND w.name IS NOT NULL
		ORDER BY w.name, min_dist_m
	`, activityID)
	if err != nil {
		return fmt.Errorf("water: still_water query: %w", err)
	}
	defer stillRows.Close()

	batch := &pgx.Batch{}
	for stillRows.Next() {
		var osmID int64
		var name string
		var distM int
		var bboxWest, bboxSouth, bboxEast, bboxNorth float64

		if err := stillRows.Scan(&osmID, &name, &distM, &bboxWest, &bboxSouth, &bboxEast, &bboxNorth); err != nil {
			return fmt.Errorf("water: still_water scan: %w", err)
		}

		batch.Queue(`
			INSERT INTO activity_geo_tags
			    (activity_id, category, name, osm_id, map_type,
			     map_bbox_west, map_bbox_south, map_bbox_east, map_bbox_north, dist_m)
			VALUES ($1, 'still_water', $2, $3, 'bbox', $4, $5, $6, $7, $8)
		`, activityID, name, osmID, bboxWest, bboxSouth, bboxEast, bboxNorth, distM)
	}
	if err := stillRows.Err(); err != nil {
		return fmt.Errorf("water: still_water rows iteration: %w", err)
	}

	// Flush still_water batch before querying flowing water.
	// Keeping the two batches separate ensures that a geometry-type error in a
	// flowing_water row (MULTILINESTRING vs. LINESTRING constraint) does not
	// roll back the already-collected still_water inserts.
	if batch.Len() > 0 {
		if err := db.Pool.SendBatch(ctx, batch).Close(); err != nil {
			slog.Warn("water: still_water batch insert failed", "activity_id", activityID, "error", err)
		}
	}

	// --- Flowing water (rivers, streams) ---
	flowRows, err := db.Pool.Query(ctx, `
		WITH route AS (
			SELECT ST_Simplify(track_full, 0.00005) AS geom
			FROM activity_tracks WHERE activity_id = $1
		)
		SELECT DISTINCT ON (ww.name) ww.osm_id, ww.name, ww.waterway,
		       ROUND(ST_Distance(route.geom::geography, ww.geom::geography))::int AS min_dist_m,
		       ST_AsBinary(ST_Intersection(
		           ST_Buffer(route.geom::geography, 50)::geometry, ww.geom
		       )) AS nearby_section
		FROM osm_waterways ww, route
		WHERE ww.geom && ST_Expand(route.geom, 0.001)
		  AND ST_DWithin(route.geom::geography, ww.geom::geography, 50)
		  AND ww.name IS NOT NULL
		ORDER BY ww.name, min_dist_m
	`, activityID)
	if err != nil {
		return fmt.Errorf("water: flowing_water query: %w", err)
	}
	defer flowRows.Close()

	flowBatch := &pgx.Batch{}
	for flowRows.Next() {
		var osmID int64
		var name string
		var waterway *string
		var distM int
		var nearbySection []byte // WKB geometry

		if err := flowRows.Scan(&osmID, &name, &waterway, &distM, &nearbySection); err != nil {
			return fmt.Errorf("water: flowing_water scan: %w", err)
		}

		flowBatch.Queue(`
			INSERT INTO activity_geo_tags
			    (activity_id, category, name, osm_id, map_type,
			     map_section, dist_m)
			VALUES ($1, 'flowing_water', $2, $3, 'section',
			        ST_LineMerge(ST_GeomFromWKB($4, 4326)), $5)
		`, activityID, name, osmID, nearbySection, distM)
	}
	if err := flowRows.Err(); err != nil {
		return fmt.Errorf("water: flowing_water rows iteration: %w", err)
	}

	if flowBatch.Len() > 0 {
		if err := db.Pool.SendBatch(ctx, flowBatch).Close(); err != nil {
			slog.Warn("water: flowing_water batch insert failed", "activity_id", activityID, "error", err)
		}
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
			SELECT ST_Simplify(track_full, 0.00005) AS geom
			FROM activity_tracks WHERE activity_id = $1
		)
		SELECT b.name, b.admin_level,
		       CASE
		           WHEN NOT ST_IsEmpty(ST_Intersection(route.geom, ST_Boundary(b.geom)))
		               THEN ST_AsBinary(ST_Intersection(route.geom, ST_Boundary(b.geom)))
		           ELSE ST_AsBinary(ST_ClosestPoint(ST_Boundary(b.geom), route.geom))
		       END AS crossing_geom
		FROM osm_boundaries b, route
		WHERE ST_Intersects(route.geom, b.geom)
		  AND b.admin_level IN (2, 4)
		ORDER BY b.admin_level
	`, activityID)
	if err != nil {
		return fmt.Errorf("borders: query: %w", err)
	}
	defer rows.Close()

	batch := &pgx.Batch{}
	for rows.Next() {
		var name *string
		var adminLevel int
		var crossingGeom []byte // WKB geometry

		if err := rows.Scan(&name, &adminLevel, &crossingGeom); err != nil {
			return fmt.Errorf("borders: scan: %w", err)
		}

		displayName := "Grenze"
		if name != nil && *name != "" {
			displayName = *name
		}

		batch.Queue(`
			INSERT INTO activity_geo_tags
			    (activity_id, category, name, map_type, map_section)
			VALUES ($1, 'border', $2, 'section', ST_GeomFromWKB($3, 4326))
		`, activityID, displayName, crossingGeom)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("borders: rows iteration: %w", err)
	}

	if batch.Len() > 0 {
		if err := db.Pool.SendBatch(ctx, batch).Close(); err != nil {
			slog.Warn("borders: batch insert failed", "activity_id", activityID, "error", err)
		}
	}
	return nil
}
