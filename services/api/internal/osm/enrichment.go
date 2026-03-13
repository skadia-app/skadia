// services/api/internal/osm/enrichment.go
package osm

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunSpatialEnrichment runs all 6 enrichment sub-phases on osm_ways.
// Must be called BEFORE intersection splitting (enrichment values are inherited by split segments).
// Reports progress via the progressFn callback (0-100).
func RunSpatialEnrichment(ctx context.Context, pool *pgxpool.Pool, progressFn func(int)) error {
	type enrichStep struct {
		name string
		pct  int
		fn   func(context.Context, *pgxpool.Pool) error
	}
	steps := []enrichStep{
		{"forest_pct", 15, EnrichForestPct},
		{"noise_score", 35, EnrichNoiseScore},
		{"traffic_class", 60, EnrichTrafficClass},
		{"protected_area", 70, EnrichProtectedArea},
		{"landuse_class", 80, EnrichLanduseClass},
		{"route_membership", 100, EnrichRouteMembership},
	}
	for _, s := range steps {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("enrichment cancelled: %w", err)
		}
		slog.Info("enrichment: running step", "step", s.name)
		if err := s.fn(ctx, pool); err != nil {
			return fmt.Errorf("enrichment step %s failed: %w", s.name, err)
		}
		if progressFn != nil {
			progressFn(s.pct)
		}
	}
	return nil
}

// EnrichForestPct calculates forest coverage percentage for each way in osm_ways.
// Uses batched 50km tiles (SRID 3857 = meters) for memory control.
// ST_Length ratio in 3857 is acceptable — Mercator distortion cancels out in numerator/denominator.
// Requires: osm_ways.forest_pct column (migration 000157), osm_forests table.
func EnrichForestPct(ctx context.Context, pool *pgxpool.Pool) error {
	slog.Info("enrichment: starting forest_pct")
	_, err := pool.Exec(ctx, `
		DO $$
		DECLARE
			tile RECORD;
			data_extent GEOMETRY;
			processed INT := 0;
		BEGIN
			-- SRID 3857: units are meters. 10000 = 10km buffer.
			SELECT ST_Expand(ST_Extent(geom), 10000) INTO data_extent
			FROM osm_ways WHERE highway IS NOT NULL;

			IF data_extent IS NULL THEN
				RAISE NOTICE 'No osm_ways with highway found';
				RETURN;
			END IF;

			-- 50000 = 50km grid cells (3857 meters)
			FOR tile IN SELECT * FROM ST_SquareGrid(50000, data_extent) LOOP
				UPDATE osm_ways w SET forest_pct = COALESCE(sub.pct, 0)
				FROM (
					SELECT w2.way_id,
						LEAST(100, ROUND(
							100.0 * SUM(ST_Length(ST_Intersection(w2.geom, f.geom)))
							/ NULLIF(ST_Length(w2.geom), 0)
						))::SMALLINT AS pct
					FROM osm_ways w2
					LEFT JOIN osm_forests f ON ST_Intersects(w2.geom, f.geom)
					WHERE ST_Intersects(w2.geom, tile.geom)
					  AND w2.highway IS NOT NULL
					GROUP BY w2.way_id, w2.geom
				) sub
				WHERE w.way_id = sub.way_id;
				processed := processed + 1;
			END LOOP;
			RAISE NOTICE 'forest_pct: processed % tiles', processed;
		END $$;
	`)
	if err != nil {
		return fmt.Errorf("enrichment forest_pct: %w", err)
	}
	_, err = pool.Exec(ctx, "ANALYZE osm_ways")
	if err != nil {
		slog.Warn("enrichment: ANALYZE after forest_pct failed", "error", err)
	}
	slog.Info("enrichment: forest_pct complete")
	return nil
}

// EnrichNoiseScore calculates noise exposure score (0-100) for each way.
// Sources: road hierarchy proximity + aerodromes.
// Uses pool.Acquire() for connection pinning (TEMP VIEW needs same connection).
// SRID 3857: must ST_Transform to 4326 before ::geography cast.
// P3-TODO: Add railway lines and industrial areas when Lua importer is extended.
func EnrichNoiseScore(ctx context.Context, pool *pgxpool.Pool) error {
	slog.Info("enrichment: starting noise_score")

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("enrichment noise_score: acquire conn: %w", err)
	}
	defer conn.Release()

	// Create temp view for noise sources (all geometry cast to untyped for UNION compat)
	_, err = conn.Exec(ctx, `
		CREATE TEMP VIEW noise_sources AS
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
			FROM osm_pois WHERE type = 'aerodrome'
	`)
	if err != nil {
		return fmt.Errorf("enrichment noise_score: create view: %w", err)
	}

	// Calculate noise score
	// SRID 3857: must ST_Transform to 4326 before ::geography cast for correct distances
	_, err = conn.Exec(ctx, `
		UPDATE osm_ways w SET noise_score = sub.score
		FROM (
			SELECT w2.way_id,
				LEAST(100, GREATEST(0, ROUND(MAX(
					n.weight * 100.0 * (1.0 - ST_Distance(
						ST_Transform(w2.geom, 4326)::geography,
						ST_Transform(n.geom, 4326)::geography
					) / n.buffer_m)
				))))::SMALLINT AS score
			FROM osm_ways w2
			JOIN noise_sources n ON ST_DWithin(
				ST_Transform(w2.geom, 4326)::geography,
				ST_Transform(n.geom, 4326)::geography,
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

	conn.Exec(ctx, "DROP VIEW IF EXISTS noise_sources")

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

	steps := []struct {
		name string
		sql  string
	}{
		// SRID 3857: ST_Y returns meters, need ST_Transform for lat in degrees
		{"cities_pop", `
			CREATE TEMP TABLE cities_pop AS
			SELECT osm_id, name,
				(tags->>'population')::numeric AS population,
				geom,
				COS(RADIANS(ST_Y(ST_Transform(geom, 4326)))) AS merca_coef
			FROM osm_pois
			WHERE category = 'settlement'
			  AND (tags->>'population') ~ '^\d+$'
			  AND (tags->>'population')::numeric > 200
		`},
		{"industrial_areas", `
			CREATE TEMP TABLE industrial_areas AS
			SELECT osm_id, geom,
				ST_Area(ST_Transform(geom, 4326)::geography) AS area_m2,
				ST_Centroid(geom) AS centroid
			FROM osm_boundaries
			WHERE boundary_type IN ('industrial', 'retail')
			  AND ST_Area(ST_Transform(geom, 4326)::geography) > 20000
		`},
		// Data is already in 3857 (meters), 10km grid cells, 50km buffer
		{"routing_grid", `
			CREATE TEMP TABLE routing_grid AS
			SELECT (ST_SquareGrid(10000,
				(SELECT ST_Expand(ST_Extent(geom), 50000) FROM osm_ways WHERE highway IS NOT NULL)
			)).geom
		`},
		// ST_Length via geography for accurate meter-based density
		{"motorway_density", `
			CREATE TEMP TABLE motorway_density AS
			SELECT g.geom AS cell,
				COALESCE(SUM(ST_Length(ST_Transform(w.geom, 4326)::geography) / 6430), 0) AS factor
			FROM routing_grid g
			LEFT JOIN osm_ways w ON ST_Intersects(w.geom, g.geom)
			  AND w.highway IN ('motorway', 'motorway_link', 'trunk', 'trunk_link')
			GROUP BY g.geom
		`},
		{"highway_density", `
			CREATE TEMP TABLE highway_density AS
			SELECT g.geom AS cell,
				COALESCE(SUM(ST_Length(ST_Transform(w.geom, 4326)::geography) / 6430), 0) AS factor
			FROM routing_grid g
			LEFT JOIN osm_ways w ON ST_Intersects(w.geom, g.geom)
			  AND w.highway IN ('primary', 'primary_link', 'secondary', 'secondary_link', 'tertiary')
			GROUP BY g.geom
		`},
		// Distance calculations need ST_Transform to 4326 for geography
		{"populate_factor", `
			CREATE TEMP TABLE populate_factor AS
			SELECT g.geom AS cell,
				COALESCE(SUM(
					c.population * c.merca_coef / NULLIF(ST_Distance(
						ST_Transform(ST_Centroid(g.geom), 4326)::geography,
						ST_Transform(c.geom, 4326)::geography
					) / 1000.0, 0)
				), 0) AS factor
			FROM routing_grid g
			LEFT JOIN cities_pop c ON ST_DWithin(
				ST_Transform(ST_Centroid(g.geom), 4326)::geography,
				ST_Transform(c.geom, 4326)::geography, 50000)
			GROUP BY g.geom
		`},
		{"industrial_factor", `
			CREATE TEMP TABLE industrial_factor AS
			SELECT g.geom AS cell,
				COALESCE(SUM(
					i.area_m2 / NULLIF(ST_Distance(
						ST_Transform(ST_Centroid(g.geom), 4326)::geography,
						ST_Transform(i.centroid, 4326)::geography
					) / 1000.0, 0)
				), 0) AS factor
			FROM routing_grid g
			LEFT JOIN industrial_areas i ON ST_DWithin(
				ST_Transform(ST_Centroid(g.geom), 4326)::geography,
				ST_Transform(i.centroid, 4326)::geography, 20000)
			GROUP BY g.geom
		`},
		// osm_ways.geom is already in 3857, grid is in 3857 — direct ST_Intersects
		{"classify", `
			UPDATE osm_ways w SET traffic_class = sub.tc
			FROM (
				SELECT inner_q.way_id,
					(CASE
						WHEN inner_q.traffic_raw < 6  THEN 1
						WHEN inner_q.traffic_raw < 10 THEN 2
						WHEN inner_q.traffic_raw < 19 THEN 3
						WHEN inner_q.traffic_raw < 35 THEN 4
						WHEN inner_q.traffic_raw < 70 THEN 5
						ELSE 6
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

	// Cleanup temp tables (on the pinned connection)
	for _, t := range []string{"cities_pop", "industrial_areas", "routing_grid",
		"motorway_density", "highway_density", "populate_factor", "industrial_factor"} {
		conn.Exec(ctx, "DROP TABLE IF EXISTS "+t)
	}

	_, err = pool.Exec(ctx, "ANALYZE osm_ways")
	if err != nil {
		slog.Warn("enrichment: ANALYZE after traffic_class failed", "error", err)
	}
	slog.Info("enrichment: traffic_class complete")
	return nil
}

// EnrichProtectedArea sets protection level (0-3) based on osm_boundaries.
func EnrichProtectedArea(ctx context.Context, pool *pgxpool.Pool) error {
	slog.Info("enrichment: starting protected_area")
	_, err := pool.Exec(ctx, `
		UPDATE osm_ways w SET protected_area = sub.level
		FROM (
			SELECT w2.way_id,
				MAX(CASE
					WHEN b.protect_class IN ('1a', '1b', '1') THEN 3
					WHEN b.protect_class IN ('2', '3', '4') THEN 2
					WHEN b.boundary_type = 'protected_area' THEN 1
					ELSE 0
				END)::SMALLINT AS level
			FROM osm_ways w2
			JOIN osm_boundaries b ON ST_Intersects(w2.geom, b.geom)
			WHERE w2.highway IS NOT NULL
			  AND (b.boundary_type = 'protected_area' OR b.protect_class IS NOT NULL)
			GROUP BY w2.way_id
		) sub
		WHERE w.way_id = sub.way_id
	`)
	if err != nil {
		return fmt.Errorf("enrichment protected_area: %w", err)
	}
	slog.Info("enrichment: protected_area complete")
	return nil
}

// EnrichLanduseClass sets landuse class using forest_pct + highway type as proxy.
// 0=other, 1=forest, 3=residential, 4=industrial.
// P3-TODO: Import landuse polygons for class 2 (agricultural).
func EnrichLanduseClass(ctx context.Context, pool *pgxpool.Pool) error {
	slog.Info("enrichment: starting landuse_class")
	_, err := pool.Exec(ctx, `
		UPDATE osm_ways SET landuse_class = (
			CASE
				WHEN forest_pct >= 60 THEN 1
				WHEN highway IN ('residential', 'living_street') THEN 3
				WHEN EXISTS (
					SELECT 1 FROM osm_boundaries b
					WHERE ST_Within(ST_LineInterpolatePoint(osm_ways.geom, 0.5), b.geom)
					  AND b.boundary_type IN ('industrial')
				) THEN 4
				ELSE 0
			END
		)::SMALLINT
		WHERE highway IS NOT NULL
	`)
	if err != nil {
		return fmt.Errorf("enrichment landuse_class: %w", err)
	}
	slog.Info("enrichment: landuse_class complete")
	return nil
}

// EnrichRouteMembership populates osm_route_way_members junction table
// and sets is_cycleroute/is_hikeroute flags with network level.
func EnrichRouteMembership(ctx context.Context, pool *pgxpool.Pool) error {
	slog.Info("enrichment: starting route_membership")

	// Populate junction table via spatial proximity
	// SRID 3857: must ST_Transform to 4326 before ::geography for correct meter distances
	_, err := pool.Exec(ctx, `
		INSERT INTO osm_route_way_members (route_osm_id, way_osm_id)
		SELECT DISTINCT r.osm_id, w.osm_id
		FROM osm_routes r
		JOIN osm_ways w ON ST_DWithin(
			ST_Transform(r.geom, 4326)::geography,
			ST_Transform(w.geom, 4326)::geography, 15
		)
		WHERE w.highway IS NOT NULL
		ON CONFLICT DO NOTHING
	`)
	if err != nil {
		return fmt.Errorf("enrichment route_membership: junction: %w", err)
	}

	// Set cycleroute flags with best network level
	_, err = pool.Exec(ctx, `
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
