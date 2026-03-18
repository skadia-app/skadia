// services/api/internal/osm/enrichment.go
package osm

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// validWKTRe validates that a WKT string is a safe POLYGON (defense in depth against SQL injection).
var validWKTRe = regexp.MustCompile(`^POLYGON\(\([0-9\s.,eE+-]+\)\)$`)

const defaultChunkSize int64 = 50000 // way_ids per chunk

// wayIDRange returns the min/max way_id for ways matching "highway IS NOT NULL AND <column> IS NULL".
// bboxClause is an optional SQL fragment (e.g. " AND ST_Intersects(w.geom, ...)") that restricts
// the range to a geographic bbox; pass "" for global (all ways).
// Returns (0, 0) if no matching rows exist.
func wayIDRange(ctx context.Context, conn *pgxpool.Conn, column, bboxClause string) (minID, maxID int64, err error) {
	err = conn.QueryRow(ctx, fmt.Sprintf(
		"SELECT COALESCE(min(way_id),0), COALESCE(max(way_id),0) FROM osm_ways w WHERE w.highway IS NOT NULL AND w.%s IS NULL%s", column, bboxClause),
	).Scan(&minID, &maxID)
	return
}

// chunkedExec runs a parameterized SQL in way_id range chunks.
// The SQL MUST use $1 and $2 as placeholders for the way_id range (lo, hi).
// Additional args can be passed after chunkSize — they become $3, $4, etc.
// Calls progressFn with total affected rows after each chunk.
func chunkedExec(ctx context.Context, conn *pgxpool.Conn, sql string,
	minID, maxID, chunkSize int64, name string, progressFn func(int64), args ...any) (int64, error) {
	var total int64
	for lo := minID; lo <= maxID; lo += chunkSize {
		if ctx.Err() != nil {
			return total, ctx.Err()
		}
		allArgs := append([]any{lo, lo + chunkSize - 1}, args...)
		tag, err := conn.Exec(ctx, sql, allArgs...)
		if err != nil {
			return total, fmt.Errorf("chunk [%d-%d]: %w", lo, lo+chunkSize-1, err)
		}
		affected := tag.RowsAffected()
		total += affected
		if progressFn != nil && affected > 0 {
			progressFn(affected) // delta, not cumulative
		}
	}
	if total > 0 {
		slog.Info("enrichment: chunked step complete", "step", name, "rows", total)
	}
	return total, nil
}

// RunSpatialEnrichment runs all enrichment sub-phases on osm_ways using direct polygon joins.
// Must be called BEFORE intersection splitting (enrichment values are inherited by split segments).
// regionFilter: "" = global (all ways), "germany" = only DE + 50km buffer,
// "germany,austria" = union of both + buffer.
// Reports progress via progressFn(current, total) with absolute row counts.
func RunSpatialEnrichment(ctx context.Context, pool *pgxpool.Pool, regionFilter string, progressFn func(current, total int64)) error {
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
				 WHERE wr.region_id = ANY($1::text[])
				   AND w.highway != 'ferry'), 0.5))`,
			regions).Scan(&bboxWKT)
		if err != nil || bboxWKT == "" {
			slog.Warn("enrichment: BBox computation failed, falling back to global", "regions", regionFilter, "error", err)
		} else if !validWKTRe.MatchString(bboxWKT) {
			slog.Warn("enrichment: invalid BBox WKT, falling back to global", "wkt", bboxWKT)
		} else {
			bboxClause = fmt.Sprintf(` AND ST_Intersects(w.geom, ST_GeomFromText('%s', 4326))`, bboxWKT)
			slog.Info("enrichment: BBox-filtered enrichment active", "regions", regionFilter, "bbox", bboxWKT)
		}
	}

	// Each enrichment sub-function acquires its own pinned connection and applies
	// connection-level work_mem tuning. No top-level tuning connection needed here.

	// Resume support: skip reset if enrichment was interrupted mid-flight.
	// Completed chunks set columns to non-NULL; the reset would destroy that progress.
	// Only reset on fresh start (no enriched ways in the target BBox).
	var alreadyEnriched int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM osm_ways w
		WHERE w.highway IS NOT NULL AND w.forest_pct IS NOT NULL`+bboxClause).Scan(&alreadyEnriched)
	if alreadyEnriched > 0 {
		slog.Info("enrichment: resuming — skipping column reset", "already_enriched", alreadyEnriched)
	} else {
		slog.Info("enrichment: fresh start — resetting enrichment columns", "region_filter", regionFilter)
		resetSQL := fmt.Sprintf(`UPDATE osm_ways w SET forest_pct = NULL, protected_area = NULL,
			landuse_class = NULL, noise_score = NULL, traffic_class = NULL
			WHERE highway IS NOT NULL%s`, bboxClause)
		if _, err := pool.Exec(ctx, resetSQL); err != nil {
			return fmt.Errorf("enrichment: reset columns: %w", err)
		}
	}

	// Count total ways needing enrichment for real progress reporting.
	var totalWays int64
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM osm_ways w
		WHERE w.highway IS NOT NULL AND w.forest_pct IS NULL`+bboxClause).Scan(&totalWays)
	slog.Info("enrichment: ways to process", "total", totalWays)
	if progressFn != nil {
		progressFn(0, totalWays)
	}

	// Cumulative progress across all enrichment steps.
	// Each chunkedExec callback reports a delta; we accumulate here.
	var cumulative int64
	chunkCb := func(delta int64) {
		cumulative += delta
		if progressFn != nil {
			progressFn(cumulative, totalWays)
		}
	}

	// === Phase 1: Polygon-basierte Spatial Joins (sequential, chunked) ===
	slog.Info("enrichment: phase 1 — spatial joins (sequential, chunked)")
	phase1Steps := []struct {
		name string
		fn   func(context.Context, *pgxpool.Pool, string, func(int64)) error
	}{
		{"forest_pct", EnrichForestPct},
		{"protected_area", EnrichProtectedArea},
		{"landuse_from_polygons", EnrichLanduseFromPolygons},
	}
	for _, step := range phase1Steps {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("enrichment cancelled before %s: %w", step.name, err)
		}
		slog.Info("enrichment: starting " + step.name)
		if err := step.fn(ctx, pool, bboxClause, chunkCb); err != nil {
			return fmt.Errorf("enrichment phase 1 (%s): %w", step.name, err)
		}
		slog.Info("enrichment: " + step.name + " done")
	}

	// === Phase 2: Derived Enrichment (sequential, chunked) ===
	slog.Info("enrichment: phase 2 — derived enrichment (sequential, chunked)")
	phase2Steps := []struct {
		name string
		fn   func(context.Context, *pgxpool.Pool) error
	}{
		{"noise_score", func(ctx context.Context, p *pgxpool.Pool) error {
			return EnrichNoiseScore(ctx, p, bboxClause, chunkCb)
		}},
		{"landuse_class", func(ctx context.Context, p *pgxpool.Pool) error {
			return EnrichLanduseClass(ctx, p, bboxClause, chunkCb)
		}},
		{"traffic_class", func(ctx context.Context, p *pgxpool.Pool) error {
			return EnrichTrafficClass(ctx, p, bboxClause, chunkCb)
		}},
		{"route_membership", func(ctx context.Context, p *pgxpool.Pool) error {
			return EnrichRouteMembership(ctx, p, bboxClause)
		}},
	}
	for _, step := range phase2Steps {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("enrichment cancelled before %s: %w", step.name, err)
		}
		slog.Info("enrichment: step starting", "step", step.name)
		if err := step.fn(ctx, pool); err != nil {
			return fmt.Errorf("enrichment step %s: %w", step.name, err)
		}
		slog.Info("enrichment: step complete", "step", step.name)
	}

	// === Final ANALYZE ===
	if _, err := pool.Exec(ctx, "ANALYZE osm_ways"); err != nil {
		slog.Warn("enrichment: final ANALYZE failed", "error", err)
	}
	if progressFn != nil {
		progressFn(totalWays, totalWays)
	}

	slog.Info("enrichment: all phases complete")
	return nil
}

// EnrichNoiseScore calculates noise exposure score (0-100) for each way.
// Sources: road hierarchy proximity + aerodromes + optional railway lines.
// Uses pool.Acquire() for connection pinning (TEMP VIEW needs same connection).
// osm_ways.geom is SRID 4326 — cast directly to ::geography for meter distances.
func EnrichNoiseScore(ctx context.Context, pool *pgxpool.Pool, bboxClause string, progressFn func(int64)) error {
	slog.Info("enrichment: starting noise_score")

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("enrichment noise_score: acquire conn: %w", err)
	}
	defer conn.Release()

	workMemMB := availableRAMMB() * 10 / 100
	if workMemMB > 512 { workMemMB = 512 }
	if workMemMB < 64 { workMemMB = 64 }
	if _, err := conn.Exec(ctx, fmt.Sprintf("SET work_mem = '%dMB'", workMemMB)); err != nil {
		slog.Warn("enrichment noise_score: SET work_mem failed (non-fatal)", "error", err)
	}

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

	// Wind turbines
	windSQL := ""
	var windCount int64
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM osm_pois WHERE category = 'wind_turbine'").Scan(&windCount); err == nil && windCount > 0 {
		windSQL = `
        UNION ALL
        SELECT geom::geometry, 'wind_turbine', 800, 0.5
        FROM osm_pois WHERE category = 'wind_turbine'`
		slog.Info("enrichment: wind turbines found", "count", windCount)
	}

	// Shooting ranges
	shootingSQL := ""
	var shootingCount int64
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM osm_pois WHERE category = 'shooting_range'").Scan(&shootingCount); err == nil && shootingCount > 0 {
		shootingSQL = `
        UNION ALL
        SELECT geom::geometry, 'shooting_range', 400, 0.4
        FROM osm_pois WHERE category = 'shooting_range'`
	}

	// Military areas — polygon directly (distance to nearest edge, not centroid)
	militarySQL := ""
	var militaryCount int64
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM osm_landuse WHERE landuse = 'military'").Scan(&militaryCount); err == nil && militaryCount > 0 {
		militarySQL = `
        UNION ALL
        SELECT geom::geometry, 'military', 500, 0.6
        FROM osm_landuse WHERE landuse = 'military'`
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
    FROM osm_pois WHERE category = 'aerodrome' AND subtype NOT IN ('gliding', 'hang_gliding')` + windSQL + shootingSQL + militarySQL + railwaySQL

	if _, err = conn.Exec(ctx, viewSQL); err != nil {
		return fmt.Errorf("enrichment noise_score: create view: %w", err)
	}
	defer conn.Exec(ctx, "DROP VIEW IF EXISTS noise_sources") //nolint:errcheck

	// Calculate noise score (chunked).
	// osm_ways.geom is already SRID 4326 — cast directly to ::geography for meter distances.
	// AND w2.noise_score IS NULL — nur resettete Ways, verhindert Überschreiben nicht-betroffener Regionen.
	minID, maxID, err := wayIDRange(ctx, conn, "noise_score", bboxClause)
	if err != nil || maxID == 0 {
		slog.Info("enrichment: noise_score — no NULL rows, skipping")
		return nil
	}
	slog.Info("enrichment: noise_score way_id range", "min", minID, "max", maxID)

	if _, err := chunkedExec(ctx, conn, `
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
			WHERE w2.way_id BETWEEN $1 AND $2
			  AND w2.highway IS NOT NULL AND w2.noise_score IS NULL
			GROUP BY w2.way_id
		) sub
		WHERE w.way_id = sub.way_id
	`, minID, maxID, defaultChunkSize, "noise_score:calculate", progressFn); err != nil {
		return fmt.Errorf("enrichment noise_score: update: %w", err)
	}

	// Default 0 for ways with no nearby noise sources (chunked)
	minID, maxID, _ = wayIDRange(ctx, conn, "noise_score", bboxClause)
	if maxID > 0 {
		if _, err := chunkedExec(ctx, conn, `
			UPDATE osm_ways w SET noise_score = 0
			WHERE w.way_id BETWEEN $1 AND $2
			  AND w.noise_score IS NULL AND w.highway IS NOT NULL
		`, minID, maxID, defaultChunkSize, "noise_score:default", progressFn); err != nil {
			return fmt.Errorf("enrichment noise_score: default: %w", err)
		}
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
func EnrichTrafficClass(ctx context.Context, pool *pgxpool.Pool, bboxClause string, progressFn func(int64)) error {
	slog.Info("enrichment: starting traffic_class")

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("enrichment traffic_class: acquire conn: %w", err)
	}
	defer conn.Release()

	workMemMB := availableRAMMB() * 10 / 100
	if workMemMB > 512 { workMemMB = 512 }
	if workMemMB < 64 { workMemMB = 64 }
	if _, err := conn.Exec(ctx, fmt.Sprintf("SET work_mem = '%dMB'", workMemMB)); err != nil {
		slog.Warn("enrichment traffic_class: SET work_mem failed (non-fatal)", "error", err)
	}

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

	// Execute temp table creation steps (global grid data)
	for _, step := range steps {
		if step.name == "classify" {
			break // classify step is chunked separately below
		}
		slog.Info("enrichment: traffic_class step", "step", step.name)
		if _, err := conn.Exec(ctx, step.sql); err != nil {
			return fmt.Errorf("enrichment traffic_class step %s: %w", step.name, err)
		}
	}

	// Classify step — chunked
	minID, maxID, err := wayIDRange(ctx, conn, "traffic_class", bboxClause)
	if err != nil || maxID == 0 {
		slog.Info("enrichment: traffic_class — no NULL rows, skipping classify")
	} else {
		slog.Info("enrichment: traffic_class way_id range", "min", minID, "max", maxID)
		if _, err := chunkedExec(ctx, conn, `
			UPDATE osm_ways w SET traffic_class = sub.tc
			FROM (
				SELECT inner_q.way_id,
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
					WHERE w3.way_id BETWEEN $1 AND $2
					  AND w3.highway IN ('primary','primary_link','secondary','secondary_link','tertiary')
					  AND w3.traffic_class IS NULL
				) inner_q
			) sub
			WHERE w.way_id = sub.way_id
		`, minID, maxID, defaultChunkSize, "traffic_class:classify", progressFn); err != nil {
			return fmt.Errorf("enrichment traffic_class: classify: %w", err)
		}
	}

	// Default 0 for ways not covered by the traffic grid (path, track, footway, etc.) — chunked
	minID, maxID, _ = wayIDRange(ctx, conn, "traffic_class", bboxClause)
	if maxID > 0 {
		if _, err := chunkedExec(ctx, conn, `
			UPDATE osm_ways w SET traffic_class = 0
			WHERE w.way_id BETWEEN $1 AND $2
			  AND w.highway IS NOT NULL AND w.traffic_class IS NULL
		`, minID, maxID, defaultChunkSize, "traffic_class:default", progressFn); err != nil {
			slog.Warn("enrichment: traffic_class null default failed", "error", err)
		}
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
func EnrichLanduseClass(ctx context.Context, pool *pgxpool.Pool, bboxClause string, progressFn func(int64)) error {
	slog.Info("enrichment: starting landuse_class (forest + residential derivation, chunked)")

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("enrichment landuse_class: acquire conn: %w", err)
	}
	defer conn.Release()

	// Use landuse_class = 0 rows as targets — wayIDRange checks IS NULL,
	// but landuse_class is already set to 0 by EnrichLanduseFromPolygons default step.
	// We need a custom range query for rows where landuse_class = 0.
	var minID, maxID int64
	_ = conn.QueryRow(ctx,
		"SELECT COALESCE(min(way_id),0), COALESCE(max(way_id),0) FROM osm_ways WHERE highway IS NOT NULL AND landuse_class = 0",
	).Scan(&minID, &maxID)
	if maxID == 0 {
		slog.Info("enrichment: landuse_class — no rows with class=0, skipping")
		return nil
	}
	slog.Info("enrichment: landuse_class way_id range", "min", minID, "max", maxID)

	// 1. Forest aus forest_pct ableiten (präziserer 10m-Raster)
	if _, err := chunkedExec(ctx, conn, `
		UPDATE osm_ways w SET landuse_class = 1
		WHERE w.way_id BETWEEN $1 AND $2
		  AND w.highway IS NOT NULL AND w.forest_pct >= 60 AND w.landuse_class = 0
	`, minID, maxID, defaultChunkSize, "landuse_class:forest", progressFn); err != nil {
		return fmt.Errorf("enrichment landuse_class: forest: %w", err)
	}

	// 2. Residential-Fallback aus highway-Typ (nur für noch unklassifizierte Ways).
	// Wege mit forest_pct >= 60 wurden bereits als Wald (1) klassifiziert —
	// ein Waldweg (highway=residential durch Wald) bleibt Wald, nicht Wohngebiet.
	if _, err := chunkedExec(ctx, conn, `
		UPDATE osm_ways w SET landuse_class = 3
		WHERE w.way_id BETWEEN $1 AND $2
		  AND w.highway IN ('residential', 'living_street') AND w.landuse_class = 0
	`, minID, maxID, defaultChunkSize, "landuse_class:residential", progressFn); err != nil {
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
func EnrichRouteMembership(ctx context.Context, pool *pgxpool.Pool, bboxClause string) error {
	slog.Info("enrichment: starting route_membership")

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("enrichment route_membership: acquire conn: %w", err)
	}
	defer conn.Release()

	workMemMB := availableRAMMB() * 10 / 100
	if workMemMB > 512 { workMemMB = 512 }
	if workMemMB < 64 { workMemMB = 64 }
	if _, err := conn.Exec(ctx, fmt.Sprintf("SET work_mem = '%dMB'", workMemMB)); err != nil {
		slog.Warn("enrichment route_membership: SET work_mem failed (non-fatal)", "error", err)
	}

	// Check if Lua-extracted route members exist (source='relation')
	var luaCount int64
	conn.QueryRow(ctx,
		`SELECT count(*) FROM osm_route_way_members WHERE source = 'relation'`).Scan(&luaCount)

	if luaCount == 0 {
		// Fallback: populate junction table via spatial proximity
		slog.Info("enrichment: no Lua route members found, using spatial proximity fallback")
		_, err := conn.Exec(ctx, `
			INSERT INTO osm_route_way_members (route_osm_id, way_osm_id, source)
			SELECT DISTINCT r.osm_id, w.osm_id, 'proximity'
			FROM osm_routes r
			JOIN osm_ways w ON ST_DWithin(r.geom::geography, w.geom::geography, 15)
			WHERE w.highway IS NOT NULL`+bboxClause+`
			ON CONFLICT DO NOTHING`)
		if err != nil {
			return fmt.Errorf("enrichment route_membership: proximity fallback: %w", err)
		}
	} else {
		slog.Info("enrichment: using Lua-extracted route members", "count", luaCount)
		// Clean up orphan members (way_osm_ids not in osm_ways due to tag filtering).
		// Uses LEFT JOIN instead of NOT EXISTS — PostgreSQL can hash-join which is
		// faster than nested-loop NOT EXISTS on large tables.
		tag, err := conn.Exec(ctx, `
			DELETE FROM osm_route_way_members m
			USING (
				SELECT m2.route_osm_id, m2.way_osm_id FROM osm_route_way_members m2
				LEFT JOIN osm_ways w ON w.osm_id = m2.way_osm_id
				WHERE m2.source = 'relation' AND w.osm_id IS NULL
			) orphans
			WHERE m.route_osm_id = orphans.route_osm_id AND m.way_osm_id = orphans.way_osm_id`)
		if err != nil {
			slog.Warn("enrichment: cleanup orphan route members failed", "error", err)
		} else {
			slog.Info("enrichment: cleaned orphan route members", "deleted", tag.RowsAffected())
		}
	}

	// Set cycleroute flags with best network level
	_, err = conn.Exec(ctx, `
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
	_, err = conn.Exec(ctx, `
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

	_, err = conn.Exec(ctx, "ANALYZE osm_ways")
	if err != nil {
		slog.Warn("enrichment: ANALYZE after route_membership failed", "error", err)
	}
	slog.Info("enrichment: route_membership complete")
	return nil
}

// getOSMWaysColumns and SplitAtIntersections were removed:
// osm_ways_noded is no longer consumed. BadRouter's CSR graph builder
// (routing/graph/topology.go) computes topology from osm_ways directly.
// pgr_createTopology was removed in pgRouting 4.0.1.

// withDeadlockRetry retries a function up to 3 times on PostgreSQL deadlock (40P01).
// The enrichment functions are idempotent (WHERE col IS NULL guards), so retry is safe.
// This is the standard PostgreSQL recommendation for deadlock handling.
func withDeadlockRetry(ctx context.Context, name string, fn func() error) error {
	const maxRetries = 3
	for attempt := range maxRetries + 1 {
		err := fn()
		if err == nil {
			return nil
		}
		if !strings.Contains(err.Error(), "40P01") || attempt == maxRetries {
			return err
		}
		backoff := time.Duration(50+rand.IntN(200)) * time.Millisecond
		slog.Warn("enrichment: deadlock detected, retrying", "step", name, "attempt", attempt+1, "backoff", backoff)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}
	return nil // unreachable
}
