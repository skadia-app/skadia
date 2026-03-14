// services/api/internal/osm/enrichment.go
package osm

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunSpatialEnrichment runs all enrichment sub-phases on osm_ways using raster-based sampling.
// Must be called BEFORE intersection splitting (enrichment values are inherited by split segments).
// forceRebuild: when true, truncates enrichment.raster_tiles before rasterizing.
// Reports progress via the progressFn callback (0-100).
func RunSpatialEnrichment(ctx context.Context, pool *pgxpool.Pool, forceRebuild bool, progressFn func(int)) error {
	if forceRebuild {
		slog.Info("enrichment: force rebuild — truncating enrichment.raster_tiles")
		if _, err := pool.Exec(ctx, "TRUNCATE enrichment.raster_tiles"); err != nil {
			slog.Warn("enrichment: truncate raster_tiles failed (may not exist yet)", "error", err)
		}
	}

	// Reset alle Enrichment-Spalten vor Neuberechnung
	slog.Info("enrichment: resetting enrichment columns")
	if _, err := pool.Exec(ctx, `UPDATE osm_ways SET forest_pct = NULL, protected_area = NULL,
		landuse_class = NULL, noise_score = NULL, traffic_class = NULL WHERE highway IS NOT NULL`); err != nil {
		return fmt.Errorf("enrichment: reset columns: %w", err)
	}

	type enrichStep struct {
		name string
		pct  int
		fn   func(context.Context, *pgxpool.Pool) error
	}
	steps := []enrichStep{
		// Phase 1: Rasterize
		{"rasterize_forest", 5, RasterizeForest},
		{"rasterize_protected", 12, RasterizeProtected},
		{"rasterize_landuse", 18, RasterizeLanduse},
		// Phase 1.5: ANALYZE
		{"analyze_rasters", 22, AnalyzeEnrichmentRasters},
		// Phase 2: Sample + NULL-Defaults
		{"sample_forest_pct", 32, SampleForestPct},
		{"sample_protected", 42, SampleProtectedArea},
		{"sample_landuse", 52, SampleLanduse},
		// Phase 3: Nicht-Raster-Enrichment
		{"noise_score", 62, EnrichNoiseScore},
		{"landuse_class", 70, EnrichLanduseClass},
		{"traffic_class", 82, EnrichTrafficClass},
		{"route_membership", 95, EnrichRouteMembership},
		// Phase 4: Cleanup
		{"analyze", 100, func(ctx context.Context, pool *pgxpool.Pool) error {
			_, err := pool.Exec(ctx, "ANALYZE osm_ways")
			return err
		}},
	}

	for _, step := range steps {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("enrichment cancelled before %s: %w", step.name, err)
		}
		slog.Info("enrichment: step starting", "step", step.name)
		if err := step.fn(ctx, pool); err != nil {
			return fmt.Errorf("enrichment step %s: %w", step.name, err)
		}
		if progressFn != nil {
			progressFn(step.pct)
		}
		slog.Info("enrichment: step complete", "step", step.name, "progress_pct", step.pct)
	}

	return nil
}

// EnrichNoiseScore calculates noise exposure score (0-100) for each way.
// Sources: road hierarchy proximity + aerodromes + optional railway lines.
// Uses pool.Acquire() for connection pinning (TEMP VIEW needs same connection).
// osm_ways.geom is SRID 4326 — cast directly to ::geography for meter distances.
func EnrichNoiseScore(ctx context.Context, pool *pgxpool.Pool) error {
	slog.Info("enrichment: starting noise_score")

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("enrichment noise_score: acquire conn: %w", err)
	}
	defer conn.Release()

	// Prüfe ob osm_railways Daten hat
	var railCount int64
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM osm_railways").Scan(&railCount); err != nil {
		slog.Warn("enrichment: osm_railways query failed (table may not exist)", "error", err)
		railCount = 0
	}

	railwaySQL := ""
	if railCount > 0 {
		railwaySQL = `
        UNION ALL
        SELECT geom::geometry, 'railway_heavy', 300, 0.7
        FROM osm_railways WHERE railway = 'rail'
        UNION ALL
        SELECT geom::geometry, 'railway_light', 150, 0.4
        FROM osm_railways WHERE railway IN ('light_rail', 'narrow_gauge', 'subway')`
	} else {
		slog.Warn("enrichment: osm_railways empty — skipping railway noise sources")
	}

	// Vollständiger View inkl. optionaler Railway-Quellen
	viewSQL := `CREATE TEMP VIEW noise_sources AS
    SELECT geom::geometry, 'motorway' AS source_type, 500 AS buffer_m, 1.0::float AS weight
    FROM osm_ways WHERE highway IN ('motorway', 'motorway_link', 'trunk', 'trunk_link')
    UNION ALL
    SELECT geom::geometry, 'primary', 200, 0.6
    FROM osm_ways WHERE highway IN ('primary', 'primary_link')
    UNION ALL
    SELECT geom::geometry, 'secondary', 100, 0.3
    FROM osm_ways WHERE highway IN ('secondary', 'secondary_link')
    UNION ALL
    SELECT geom::geometry, 'aerodrome', 3000, 0.9
    FROM osm_pois WHERE type = 'aerodrome'` + railwaySQL

	if _, err = conn.Exec(ctx, viewSQL); err != nil {
		return fmt.Errorf("enrichment noise_score: create view: %w", err)
	}
	defer conn.Exec(ctx, "DROP VIEW IF EXISTS noise_sources") //nolint:errcheck

	// Calculate noise score.
	// osm_ways.geom is already SRID 4326 — cast directly to ::geography for meter distances.
	_, err = conn.Exec(ctx, `
		UPDATE osm_ways w SET noise_score = sub.score
		FROM (
			SELECT w2.way_id,
				LEAST(100, GREATEST(0, ROUND(MAX(
					n.weight * 100.0 * (1.0 - ST_Distance(
						w2.geom::geography,
						n.geom::geography
					) / n.buffer_m)
				))))::SMALLINT AS score
			FROM osm_ways w2
			JOIN noise_sources n ON ST_DWithin(
				w2.geom::geography,
				n.geom::geography,
				n.buffer_m
			)
			WHERE w2.highway IS NOT NULL
			GROUP BY w2.way_id
		) sub
		WHERE w.way_id = sub.way_id
	`)
	if err != nil {
		return fmt.Errorf("enrichment noise_score: update: %w", err)
	}

	// Default 0 for ways with no nearby noise sources
	_, err = conn.Exec(ctx, `
		UPDATE osm_ways SET noise_score = 0
		WHERE noise_score IS NULL AND highway IS NOT NULL
	`)
	if err != nil {
		return fmt.Errorf("enrichment noise_score: default: %w", err)
	}

	_, err = pool.Exec(ctx, "ANALYZE osm_ways")
	if err != nil {
		slog.Warn("enrichment: ANALYZE after noise_score failed", "error", err)
	}
	slog.Info("enrichment: noise_score complete")
	return nil
}

// EnrichTrafficClass calculates traffic class (1-6) for primary/secondary/tertiary roads.
// Adapted from BRouter's brouter.sql formula.
// Uses pool.Acquire() for connection pinning (TEMP tables across steps).
// P3-TODO: industrial_factor returns 0 until Lua imports landuse=industrial.
func EnrichTrafficClass(ctx context.Context, pool *pgxpool.Pool) error {
	slog.Info("enrichment: starting traffic_class")

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("enrichment traffic_class: acquire conn: %w", err)
	}
	defer conn.Release()
	defer func() {
		for _, t := range []string{"cities_pop", "industrial_areas", "routing_grid",
			"motorway_density", "highway_density", "populate_factor", "industrial_factor"} {
			conn.Exec(ctx, "DROP TABLE IF EXISTS "+t)
		}
	}()

	steps := []struct {
		name string
		sql  string
	}{
		// osm_pois.geom is SRID 4326 — ST_Y returns latitude in degrees directly
		{"cities_pop", `
			CREATE TEMP TABLE cities_pop AS
			SELECT osm_id, name,
				(tags->>'population')::numeric AS population,
				geom,
				COS(RADIANS(ST_Y(ST_Centroid(geom)))) AS merca_coef
			FROM osm_pois
			WHERE category = 'settlement'
			  AND (tags->>'population') ~ '^\d+$'
			  AND (tags->>'population')::numeric > 200
		`},
		{"industrial_areas", `
			CREATE TEMP TABLE industrial_areas AS
			SELECT osm_id, geom,
				ST_Area(geom::geography) AS area_m2,
				ST_Centroid(geom) AS centroid
			FROM osm_landuse
			WHERE landuse IN ('industrial', 'retail', 'commercial')
			  AND ST_Area(geom::geography) > 20000
		`},
		// SRID 4326: degrees. 0.09 ≈ 10km, 0.45 ≈ 50km at 47°N.
		{"routing_grid", `
			CREATE TEMP TABLE routing_grid AS
			SELECT (ST_SquareGrid(0.09,
				(SELECT ST_Expand(ST_Extent(geom), 0.45) FROM osm_ways WHERE highway IS NOT NULL)
			)).geom
		`},
		// ST_Length via ::geography for accurate meter-based density
		{"motorway_density", `
			CREATE TEMP TABLE motorway_density AS
			SELECT g.geom AS cell,
				COALESCE(SUM(ST_Length(w.geom::geography) / 6430), 0) AS factor
			FROM routing_grid g
			LEFT JOIN osm_ways w ON ST_Intersects(w.geom, g.geom)
			  AND w.highway IN ('motorway', 'motorway_link', 'trunk', 'trunk_link')
			GROUP BY g.geom
		`},
		{"highway_density", `
			CREATE TEMP TABLE highway_density AS
			SELECT g.geom AS cell,
				COALESCE(SUM(ST_Length(w.geom::geography) / 6430), 0) AS factor
			FROM routing_grid g
			LEFT JOIN osm_ways w ON ST_Intersects(w.geom, g.geom)
			  AND w.highway IN ('primary', 'primary_link', 'secondary', 'secondary_link', 'tertiary')
			GROUP BY g.geom
		`},
		// Distance calculations use ::geography directly (geom is already 4326)
		{"populate_factor", `
			CREATE TEMP TABLE populate_factor AS
			SELECT g.geom AS cell,
				COALESCE(SUM(
					c.population * c.merca_coef / NULLIF(ST_Distance(
						ST_Centroid(g.geom)::geography,
						c.geom::geography
					) / 1000.0, 0)
				), 0) AS factor
			FROM routing_grid g
			LEFT JOIN cities_pop c ON ST_DWithin(
				ST_Centroid(g.geom)::geography,
				c.geom::geography, 50000)
			GROUP BY g.geom
		`},
		{"industrial_factor", `
			CREATE TEMP TABLE industrial_factor AS
			SELECT g.geom AS cell,
				COALESCE(SUM(
					i.area_m2 / NULLIF(ST_Distance(
						ST_Centroid(g.geom)::geography,
						i.centroid::geography
					) / 1000.0, 0)
				), 0) AS factor
			FROM routing_grid g
			LEFT JOIN industrial_areas i ON ST_DWithin(
				ST_Centroid(g.geom)::geography,
				i.centroid::geography, 20000)
			GROUP BY g.geom
		`},
		// osm_ways.geom and routing_grid are both 4326 — direct ST_Intersects
		{"classify", `
			UPDATE osm_ways w SET traffic_class = sub.tc
			FROM (
				SELECT inner_q.way_id,
					-- 3 classes to fit the 2-bit packing in EncodeEnrichmentTags (clamp 0-3).
					-- 1=low, 2=medium, 3=high; 0=default for ways without a grid cell match.
					(CASE
						WHEN inner_q.traffic_raw < 6  THEN 1
						WHEN inner_q.traffic_raw < 19 THEN 2
						ELSE 3
					END)::SMALLINT AS tc
				FROM (
					SELECT w3.way_id,
						COALESCE(p.factor, 0) * 0.3
						+ COALESCE(i.factor, 0) * 0.2
						+ COALESCE(md.factor, 0) * 30.0
						+ COALESCE(hd.factor, 0) * 5.0
						AS traffic_raw
					FROM osm_ways w3
					JOIN routing_grid g ON ST_Intersects(w3.geom, g.geom)
					LEFT JOIN populate_factor p ON p.cell = g.geom
					LEFT JOIN industrial_factor i ON i.cell = g.geom
					LEFT JOIN motorway_density md ON md.cell = g.geom
					LEFT JOIN highway_density hd ON hd.cell = g.geom
					WHERE w3.highway IN ('primary','primary_link','secondary','secondary_link','tertiary')
				) inner_q
			) sub
			WHERE w.way_id = sub.way_id
		`},
	}

	for _, step := range steps {
		slog.Info("enrichment: traffic_class step", "step", step.name)
		if _, err := conn.Exec(ctx, step.sql); err != nil {
			return fmt.Errorf("enrichment traffic_class step %s: %w", step.name, err)
		}
	}

	// Default 0 for ways not covered by the traffic grid (path, track, footway, etc.)
	if _, err = pool.Exec(ctx, `UPDATE osm_ways SET traffic_class = 0
		WHERE highway IS NOT NULL AND traffic_class IS NULL`); err != nil {
		slog.Warn("enrichment: traffic_class null default failed", "error", err)
	}

	_, err = pool.Exec(ctx, "ANALYZE osm_ways")
	if err != nil {
		slog.Warn("enrichment: ANALYZE after traffic_class failed", "error", err)
	}
	slog.Info("enrichment: traffic_class complete")
	return nil
}

// EnrichLanduseClass refines landuse_class after raster sampling:
// derives forest (1) from forest_pct and residential (3) from highway type.
// Raster sampling (SampleLanduse) runs first and sets the primary landuse class;
// this step fills in forest/residential for ways not covered by the landuse raster.
// 0=other, 1=forest, 2=agricultural, 3=residential, 4=industrial, 5=commercial, 6=quarry.
func EnrichLanduseClass(ctx context.Context, pool *pgxpool.Pool) error {
	slog.Info("enrichment: starting landuse_class (forest + residential derivation)")

	// 1. Forest aus forest_pct ableiten (präziserer 10m-Raster)
	_, err := pool.Exec(ctx, `
		UPDATE osm_ways SET landuse_class = 1
		WHERE highway IS NOT NULL AND forest_pct >= 60 AND landuse_class = 0
	`)
	if err != nil {
		return fmt.Errorf("enrichment landuse_class: forest: %w", err)
	}

	// 2. Residential-Fallback aus highway-Typ (nur für noch unklassifizierte Ways).
	// Wege mit forest_pct >= 60 wurden bereits als Wald (1) klassifiziert —
	// ein Waldweg (highway=residential durch Wald) bleibt Wald, nicht Wohngebiet.
	_, err = pool.Exec(ctx, `
		UPDATE osm_ways SET landuse_class = 3
		WHERE highway IN ('residential', 'living_street') AND landuse_class = 0
	`)
	if err != nil {
		return fmt.Errorf("enrichment landuse_class: residential: %w", err)
	}

	slog.Info("enrichment: landuse_class complete")
	return nil
}

// EnrichRouteMembership populates osm_route_way_members junction table
// and sets is_cycleroute/is_hikeroute flags with network level.
// NOTE: Uses spatial proximity (ST_DWithin 15m) as proxy for relation membership.
// This may produce false positives for nearby but unrelated ways.
// P3-TODO: Import explicit relation membership via Lua script for precise matching.
func EnrichRouteMembership(ctx context.Context, pool *pgxpool.Pool) error {
	slog.Info("enrichment: starting route_membership")

	// Check if Lua-extracted route members exist (source='relation')
	var luaCount int64
	pool.QueryRow(ctx,
		`SELECT count(*) FROM osm_route_way_members WHERE source = 'relation'`).Scan(&luaCount)

	if luaCount == 0 {
		// Fallback: populate junction table via spatial proximity
		slog.Info("enrichment: no Lua route members found, using spatial proximity fallback")
		_, err := pool.Exec(ctx, `
			INSERT INTO osm_route_way_members (route_osm_id, way_osm_id, source)
			SELECT DISTINCT r.osm_id, w.osm_id, 'proximity'
			FROM osm_routes r
			JOIN osm_ways w ON ST_DWithin(r.geom::geography, w.geom::geography, 15)
			WHERE w.highway IS NOT NULL
			ON CONFLICT DO NOTHING`)
		if err != nil {
			return fmt.Errorf("enrichment route_membership: proximity fallback: %w", err)
		}
	} else {
		slog.Info("enrichment: using Lua-extracted route members", "count", luaCount)
		// Clean up orphan members (way_osm_ids not in osm_ways due to tag filtering)
		tag, err := pool.Exec(ctx, `
			DELETE FROM osm_route_way_members m
			WHERE source = 'relation'
			  AND NOT EXISTS (SELECT 1 FROM osm_ways w WHERE w.osm_id = m.way_osm_id)`)
		if err != nil {
			slog.Warn("enrichment: cleanup orphan route members failed", "error", err)
		} else {
			slog.Info("enrichment: cleaned orphan route members", "deleted", tag.RowsAffected())
		}
	}

	// Set cycleroute flags with best network level
	_, err := pool.Exec(ctx, `
		UPDATE osm_ways w SET
			is_cycleroute = true,
			cycleroute_network = sub.best_network
		FROM (
			SELECT m.way_osm_id,
				(ARRAY['icn','ncn','rcn','lcn'])[MIN(
					CASE r.network
						WHEN 'icn' THEN 1 WHEN 'ncn' THEN 2
						WHEN 'rcn' THEN 3 WHEN 'lcn' THEN 4
						ELSE 5
					END
				)] AS best_network
			FROM osm_route_way_members m
			JOIN osm_routes r ON r.osm_id = m.route_osm_id
			WHERE r.route = 'bicycle'
			GROUP BY m.way_osm_id
		) sub
		WHERE w.osm_id = sub.way_osm_id
	`)
	if err != nil {
		return fmt.Errorf("enrichment route_membership: cycleroute: %w", err)
	}

	// Set hikeroute flags with best network level
	_, err = pool.Exec(ctx, `
		UPDATE osm_ways w SET
			is_hikeroute = true,
			hikeroute_network = sub.best_network
		FROM (
			SELECT m.way_osm_id,
				(ARRAY['iwn','nwn','rwn','lwn'])[MIN(
					CASE r.network
						WHEN 'iwn' THEN 1 WHEN 'nwn' THEN 2
						WHEN 'rwn' THEN 3 WHEN 'lwn' THEN 4
						ELSE 5
					END
				)] AS best_network
			FROM osm_route_way_members m
			JOIN osm_routes r ON r.osm_id = m.route_osm_id
			WHERE r.route IN ('hiking', 'foot')
			GROUP BY m.way_osm_id
		) sub
		WHERE w.osm_id = sub.way_osm_id
	`)
	if err != nil {
		return fmt.Errorf("enrichment route_membership: hikeroute: %w", err)
	}

	_, err = pool.Exec(ctx, "ANALYZE osm_ways")
	if err != nil {
		slog.Warn("enrichment: ANALYZE after route_membership failed", "error", err)
	}
	slog.Info("enrichment: route_membership complete")
	return nil
}

// getOSMWaysColumns queries information_schema.columns for the ordered column list of osm_ways.
// Returns column names in ordinal order. Never returns noded-only columns (parent_way_id, source, target).
func getOSMWaysColumns(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	rows, err := pool.Query(ctx, `
		SELECT column_name
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name = 'osm_ways'
		ORDER BY ordinal_position
	`)
	if err != nil {
		return nil, fmt.Errorf("getOSMWaysColumns: query: %w", err)
	}
	defer rows.Close()

	var cols []string
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			return nil, fmt.Errorf("getOSMWaysColumns: scan: %w", err)
		}
		cols = append(cols, col)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("getOSMWaysColumns: rows: %w", err)
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("getOSMWaysColumns: no columns found for osm_ways (table missing?)")
	}
	return cols, nil
}

// SplitAtIntersections creates osm_ways_noded by splitting ways at shared intermediate nodes.
// Uses ST_SnapToGrid for robust coordinate matching. Uses pool.Acquire() for TEMP table persistence.
// The noded table inherits all osm_ways columns plus parent_way_id, source, target.
// Reports progress via progressFn (0-100).
func SplitAtIntersections(ctx context.Context, pool *pgxpool.Pool, progressFn func(int)) error {
	slog.Info("enrichment: starting intersection splitting")

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("splitting: acquire conn: %w", err)
	}
	defer conn.Release()

	// Build dynamic column lists from information_schema to survive future migrations.
	allCols, err := getOSMWaysColumns(ctx, pool)
	if err != nil {
		return fmt.Errorf("splitting: %w", err)
	}

	// GROUP BY needs all columns (w.<col>). way_id is the primary key and must be included.
	groupByCols := make([]string, len(allCols))
	for i, col := range allCols {
		groupByCols[i] = "w." + col
	}
	groupByClause := strings.Join(groupByCols, ", ")

	// SELECT list for split_ways: way_id → nextval, geom → ST_Dump(ST_Split(...)),
	// all other columns forwarded as w.<col>.
	var selectParts []string
	for _, col := range allCols {
		switch col {
		case "way_id":
			selectParts = append(selectParts, "nextval('osm_ways_noded_way_id_seq')")
		case "geom":
			selectParts = append(selectParts, "(ST_Dump(ST_Split(w.geom, w.split_points))).geom")
		default:
			selectParts = append(selectParts, "w."+col)
		}
	}
	// Append noded-only columns
	selectParts = append(selectParts, "w.way_id AS parent_way_id", "NULL::BIGINT", "NULL::BIGINT")
	selectClause := strings.Join(selectParts, ",\n\t\t\t\t")

	splitWaysSQL := fmt.Sprintf(`
		INSERT INTO osm_ways_noded
		WITH ways_to_split AS (
			SELECT w.*, ST_Collect(sn.pt) AS split_points
			FROM osm_ways w
			JOIN shared_nodes sn ON ST_DWithin(w.geom, sn.pt, 0.00002)
			WHERE w.highway IS NOT NULL
			  AND NOT ST_Equals(ST_StartPoint(w.geom), sn.pt)
			  AND NOT ST_Equals(ST_EndPoint(w.geom), sn.pt)
			GROUP BY %s
		)
		SELECT
			%s
		FROM ways_to_split w
	`, groupByClause, selectClause)

	steps := []struct {
		name string
		pct  int
		sql  string
	}{
		{"create_noded_table", 5, `
			DROP TABLE IF EXISTS osm_ways_noded CASCADE;
			DROP TABLE IF EXISTS osm_ways_noded_vertices_pgr CASCADE;
			DROP SEQUENCE IF EXISTS osm_ways_noded_way_id_seq CASCADE;
			CREATE SEQUENCE osm_ways_noded_way_id_seq START WITH 100000000000;
			CREATE TABLE osm_ways_noded (LIKE osm_ways INCLUDING DEFAULTS INCLUDING CONSTRAINTS);
			ALTER TABLE osm_ways_noded ADD COLUMN IF NOT EXISTS parent_way_id BIGINT;
			ALTER TABLE osm_ways_noded ADD COLUMN IF NOT EXISTS source BIGINT;
			ALTER TABLE osm_ways_noded ADD COLUMN IF NOT EXISTS target BIGINT;
			-- way_id has no PK/UNIQUE on osm_ways; add one for pgr_createTopology
			CREATE UNIQUE INDEX idx_osm_ways_noded_way_id ON osm_ways_noded(way_id);
		`},
		{"find_shared_nodes", 20, `
			CREATE TEMP TABLE shared_nodes AS
			WITH node_points AS (
				SELECT way_id,
					ST_SnapToGrid((dp).geom, 0.000009) AS pt,
					(dp).path[1] AS pt_idx,
					ST_NPoints(geom) AS total_pts
				FROM (
					SELECT way_id, ST_DumpPoints(geom) AS dp, geom
					FROM osm_ways
					WHERE highway IS NOT NULL
				) dumped
			)
			SELECT DISTINCT pt
			FROM node_points
			WHERE pt_idx > 1 AND pt_idx < total_pts
			GROUP BY pt
			HAVING COUNT(DISTINCT way_id) > 1
		`},
		{"copy_unsplit_ways", 50, `
			INSERT INTO osm_ways_noded
			SELECT w.*, w.way_id AS parent_way_id, NULL::BIGINT AS source, NULL::BIGINT AS target
			FROM osm_ways w
			WHERE w.highway IS NOT NULL
			  AND NOT EXISTS (
				SELECT 1 FROM shared_nodes sn
				WHERE ST_DWithin(w.geom, sn.pt, 0.00002)
				  AND NOT ST_Equals(ST_StartPoint(w.geom), sn.pt)
				  AND NOT ST_Equals(ST_EndPoint(w.geom), sn.pt)
			)
		`},
		{"split_ways", 80, splitWaysSQL},
		{"cleanup_shared_nodes", 85, `
			DROP TABLE IF EXISTS shared_nodes;
		`},
		{"filter_valid_segments", 88, `
			DELETE FROM osm_ways_noded
			WHERE ST_GeometryType(geom) != 'ST_LineString'
			   OR ST_NPoints(geom) < 2;
		`},
		// SRID 4326: tolerance in degrees. 0.000009 ≈ 1 meter at 47°N.
		{"create_topology", 95, `
			SELECT pgr_createTopology('osm_ways_noded', 0.000009, 'geom', 'way_id', 'source', 'target');
		`},
		{"analyze", 100, `
			ANALYZE osm_ways_noded;
		`},
	}

	for _, step := range steps {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("splitting cancelled: %w", err)
		}
		slog.Info("enrichment: splitting step", "step", step.name)
		if _, err := conn.Exec(ctx, step.sql); err != nil {
			return fmt.Errorf("splitting step %s: %w", step.name, err)
		}
		if progressFn != nil {
			progressFn(step.pct)
		}
	}

	// Log stats
	var wayCount, nodedCount, nodeCount int64
	pool.QueryRow(ctx, "SELECT count(*) FROM osm_ways WHERE highway IS NOT NULL").Scan(&wayCount)
	pool.QueryRow(ctx, "SELECT count(*) FROM osm_ways_noded").Scan(&nodedCount)
	pool.QueryRow(ctx, "SELECT count(*) FROM osm_ways_noded_vertices_pgr").Scan(&nodeCount)
	slog.Info("enrichment: splitting complete",
		"original_ways", wayCount, "noded_segments", nodedCount, "nodes", nodeCount)

	return nil
}
