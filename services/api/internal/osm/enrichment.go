// services/api/internal/osm/enrichment.go
package osm

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"
)

// RunSpatialEnrichment runs all enrichment sub-phases on osm_ways using direct polygon joins.
// Must be called BEFORE intersection splitting (enrichment values are inherited by split segments).
// regionFilter: "" = global (all ways), "germany" = only DE + 50km buffer,
// "germany,austria" = union of both + buffer.
// Reports progress via the progressFn callback (0-100).
func RunSpatialEnrichment(ctx context.Context, pool *pgxpool.Pool, regionFilter string, progressFn func(int)) error {
	// Build BBox clause from region filter.
	// Used to restrict the reset + all enrichment sub-functions to the affected area only.
	bboxClause := ""
	if regionFilter != "" {
		regions := strings.Split(regionFilter, ",")
		for i := range regions {
			regions[i] = strings.TrimSpace(regions[i])
		}
		var bboxWKT string
		err := pool.QueryRow(ctx, `
			SELECT ST_AsText(ST_Expand(
				(SELECT ST_Extent(w.geom) FROM osm_ways w
				 JOIN osm_way_regions wr ON w.osm_id = wr.osm_id
				 WHERE wr.region_id = ANY($1::text[])), 0.5))`,
			regions).Scan(&bboxWKT)
		if err != nil || bboxWKT == "" {
			slog.Warn("enrichment: BBox computation failed, falling back to global", "regions", regionFilter, "error", err)
		} else {
			bboxClause = fmt.Sprintf(` AND ST_Intersects(w.geom, ST_GeomFromText('%s', 4326))`, bboxWKT)
			slog.Info("enrichment: BBox-filtered enrichment active", "regions", regionFilter, "bbox", bboxWKT)
		}
	}

	// PostgreSQL Enrichment-Tuning: work_mem + parallel workers hochsetzen.
	// ALTER SYSTEM wirkt global (alle neuen Connections), wird nach Enrichment zurückgesetzt.
	// Enrichment läuft als separater Job — alle Cores stehen zur Verfügung.
	// work_mem: 10% des verfügbaren RAM (min 64MB, max 512MB)
	workMemMB := availableRAMMB() * 10 / 100
	if workMemMB > 512 { workMemMB = 512 }
	if workMemMB < 64 { workMemMB = 64 }

	for _, stmt := range []string{
		fmt.Sprintf("ALTER SYSTEM SET work_mem = '%dMB'", workMemMB),
		"ALTER SYSTEM SET max_parallel_workers_per_gather = 6",
		"SELECT pg_reload_conf()",
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			slog.Warn("enrichment: tuning failed (non-fatal)", "stmt", stmt, "error", err)
		}
	}
	slog.Info("enrichment: tuning applied", "work_mem_mb", workMemMB, "parallel_workers", 6)
	defer func() {
		for _, stmt := range []string{
			"ALTER SYSTEM RESET work_mem",
			"ALTER SYSTEM RESET max_parallel_workers_per_gather",
			"SELECT pg_reload_conf()",
		} {
			pool.Exec(context.Background(), stmt)
		}
		slog.Info("enrichment: tuning reset")
	}()

	slog.Info("enrichment: resetting enrichment columns", "region_filter", regionFilter)
	resetSQL := fmt.Sprintf(`UPDATE osm_ways w SET forest_pct = NULL, protected_area = NULL,
		landuse_class = NULL, noise_score = NULL, traffic_class = NULL
		WHERE highway IS NOT NULL%s`, bboxClause)
	if _, err := pool.Exec(ctx, resetSQL); err != nil {
		return fmt.Errorf("enrichment: reset columns: %w", err)
	}

	report := func(pct int) {
		if progressFn != nil {
			progressFn(pct)
		}
	}

	// === Phase 1: Polygon-basierte Spatial Joins (parallel) ===
	slog.Info("enrichment: phase 1 — spatial joins (parallel)")
	g1, ctx1 := errgroup.WithContext(ctx)
	g1.Go(func() error { return EnrichForestPct(ctx1, pool, bboxClause) })
	g1.Go(func() error { return EnrichProtectedArea(ctx1, pool, bboxClause) })
	g1.Go(func() error { return EnrichLanduseFromPolygons(ctx1, pool, bboxClause) })
	if err := g1.Wait(); err != nil {
		return fmt.Errorf("enrichment phase 1 (spatial joins): %w", err)
	}
	report(40)

	// === Phase 2: Derived Enrichment (sequential — row locks prevent parallel UPDATEs) ===
	slog.Info("enrichment: phase 2 — derived enrichment (sequential)")
	phase2Steps := []struct {
		name string
		pct  int
		fn   func(context.Context, *pgxpool.Pool) error
	}{
		{"noise_score", 55, func(ctx context.Context, p *pgxpool.Pool) error {
			return EnrichNoiseScore(ctx, p, bboxClause)
		}},
		{"landuse_class", 65, EnrichLanduseClass},
		{"traffic_class", 75, func(ctx context.Context, p *pgxpool.Pool) error {
			return EnrichTrafficClass(ctx, p, bboxClause)
		}},
		{"route_membership", 85, EnrichRouteMembership},
	}
	for _, step := range phase2Steps {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("enrichment cancelled before %s: %w", step.name, err)
		}
		slog.Info("enrichment: step starting", "step", step.name)
		if err := step.fn(ctx, pool); err != nil {
			return fmt.Errorf("enrichment step %s: %w", step.name, err)
		}
		report(step.pct)
		slog.Info("enrichment: step complete", "step", step.name)
	}
	report(90)

	// === Phase 3: Final ANALYZE ===
	if _, err := pool.Exec(ctx, "ANALYZE osm_ways"); err != nil {
		slog.Warn("enrichment: final ANALYZE failed", "error", err)
	}
	report(100)

	slog.Info("enrichment: all phases complete")
	return nil
}

// EnrichNoiseScore calculates noise exposure score (0-100) for each way.
// Sources: road hierarchy proximity + aerodromes + optional railway lines.
// Uses pool.Acquire() for connection pinning (TEMP VIEW needs same connection).
// osm_ways.geom is SRID 4326 — cast directly to ::geography for meter distances.
func EnrichNoiseScore(ctx context.Context, pool *pgxpool.Pool, bboxClause string) error {
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
	// AND w2.noise_score IS NULL — nur resettete Ways, verhindert Überschreiben nicht-betroffener Regionen.
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
			WHERE w2.highway IS NOT NULL AND w2.noise_score IS NULL
			GROUP BY w2.way_id
		) sub
		WHERE w.way_id = sub.way_id
	`)
	if err != nil {
		return fmt.Errorf("enrichment noise_score: update: %w", err)
	}

	// Default 0 for ways with no nearby noise sources (restricted to BBox region if active)
	_, err = conn.Exec(ctx, `
		UPDATE osm_ways w SET noise_score = 0
		WHERE w.noise_score IS NULL AND w.highway IS NOT NULL`+bboxClause)
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
func EnrichTrafficClass(ctx context.Context, pool *pgxpool.Pool, bboxClause string) error {
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
				(SELECT ST_SetSRID(ST_Expand(ST_Extent(geom), 0.45)::geometry, 4326) FROM osm_ways WHERE highway IS NOT NULL)
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
		// AND w3.traffic_class IS NULL — nur resettete Ways, verhindert Überschreiben nicht-betroffener Regionen.
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
					  AND w3.traffic_class IS NULL
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
	// Restricted to BBox region if active, to avoid overwriting unrelated regions.
	if _, err = pool.Exec(ctx, `UPDATE osm_ways w SET traffic_class = 0
		WHERE w.highway IS NOT NULL AND w.traffic_class IS NULL`+bboxClause); err != nil {
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
	// source/target → NULL (set later by topology builder), all other columns forwarded.
	// Note: source/target may already exist in allCols after schema-restore — treat them
	// as NULL placeholders here (the split segments start without topology info).
	var selectParts []string
	for _, col := range allCols {
		switch col {
		case "way_id":
			selectParts = append(selectParts, "nextval('osm_ways_noded_way_id_seq')")
		case "geom":
			selectParts = append(selectParts, "(ST_Dump(ST_Split(w.geom, w.split_points))).geom")
		case "source", "target":
			selectParts = append(selectParts, "NULL::BIGINT")
		default:
			selectParts = append(selectParts, "w."+col)
		}
	}
	// Append noded-only column (parent_way_id is not in osm_ways)
	selectParts = append(selectParts, "w.way_id AS parent_way_id")
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
			SELECT w.*, w.way_id AS parent_way_id
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
		// TODO(pgRouting): pgr_createTopology wurde in pgRouting 4.0.1 entfernt.
		// Skadia's eigener A*-Router (routing/graph/) baut seinen CSR-Graphen direkt aus
		// osm_ways und benötigt die pgRouting-Topologie-Tabellen NICHT. Daher ist dieser
		// Schritt aktuell non-fatal (siehe Schleife unten).
		// Um pgr_createTopology wieder zu aktivieren: Entweder auf eine pgRouting-Version
		// <4.0 downgraden (nicht empfohlen) oder einen manuellen Topology-Builder
		// implementieren: ST_StartPoint/ST_EndPoint der Segmente + KNN-Vertex-Matching.
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
			// pgr_createTopology ist in pgRouting 4.0.1 nicht mehr vorhanden.
			// Da Skadia's A*-Router die pgRouting-Topologie nicht nutzt,
			// behandeln wir diesen Schritt als non-fatal.
			if step.name == "create_topology" {
				slog.Warn("enrichment: create_topology übersprungen (pgRouting 4.0+ hat pgr_createTopology entfernt)", "error", err)
				continue
			}
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
