package database

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PassedSummit describes a named summit or pass encountered within a given
// radius of a route geometry. EleM is nullable because osm_pois.ele is nullable.
type PassedSummit struct {
	OsmID    int64    `json:"osm_id"`
	Name     string   `json:"name"`
	EleM     *float64 `json:"ele_m"`
	Category string   `json:"category"` // "summit" | "pass"
	DistM    int      `json:"dist_m"`
	Lat      float64  `json:"lat"`
	Lng      float64  `json:"lng"`
}

// expandDegForRadius converts a meter-radius into a conservative bbox-expand
// amount in degrees (1.5× safety factor, calibrated at ~60°N).
//
//	expandDeg = radiusM * 1.5 / 55660.0
func expandDegForRadius(radiusM float64) float64 {
	return (radiusM * 1.5) / 55660.0
}

// FindSummitsAlongGeom finds summits and passes within radiusM of an arbitrary
// SRID-4326 LineString or MultiLineString supplied as WKB bytes. Returns nil,
// nil if geomWKB is empty or radiusM is ≤ 0.
//
// Intended for route-suggest pipelines where no activity row exists yet.
func FindSummitsAlongGeom(
	ctx context.Context,
	pool *pgxpool.Pool,
	geomWKB []byte,
	radiusM float64,
) ([]PassedSummit, error) {
	if len(geomWKB) == 0 || radiusM <= 0 {
		return nil, nil
	}
	// Safety-net clamp: handler-layer enforces 10–150 m range before reaching
	// this function; the secondary guard here protects direct-Go-test callers
	// that bypass the handler.
	if radiusM <= 0 {
		radiusM = 80
	}
	expandDeg := expandDegForRadius(radiusM)

	const q = `
		WITH route AS (
			SELECT ST_GeomFromWKB($1, 4326) AS geom
		)
		SELECT DISTINCT ON (p.osm_id)
		       p.osm_id, p.name, p.ele, p.category,
		       ROUND(ST_Distance(route.geom::geography, p.geom::geography))::int AS dist_m,
		       ST_Y(ST_Centroid(p.geom)) AS lat,
		       ST_X(ST_Centroid(p.geom)) AS lng
		FROM osm_pois p, route
		WHERE p.geom && ST_Expand(route.geom, $3)
		  AND ST_DWithin(route.geom::geography, p.geom::geography, $2)
		  AND p.category IN ('summit', 'pass')
		  AND p.name IS NOT NULL
		ORDER BY p.osm_id, dist_m
	`

	rows, err := pool.Query(ctx, q, geomWKB, radiusM, expandDeg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PassedSummit
	for rows.Next() {
		var s PassedSummit
		if err := rows.Scan(&s.OsmID, &s.Name, &s.EleM, &s.Category, &s.DistM, &s.Lat, &s.Lng); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// FindSummitsAlongActivityTrack returns summits and passes within radiusM of
// the stored track for the given activity. The track is fetched inside the
// query CTE (no Go-side round-trip) and lightly simplified to reduce the
// point-count used by the spatial predicates.
//
// Returns nil, nil if radiusM ≤ 0. Returns an empty slice (no error) when the
// activity has no track row or the track has no nearby summits.
func FindSummitsAlongActivityTrack(
	ctx context.Context,
	pool *pgxpool.Pool,
	activityID string,
	radiusM float64,
) ([]PassedSummit, error) {
	if radiusM <= 0 {
		return nil, nil
	}
	expandDeg := expandDegForRadius(radiusM)

	const q = `
		WITH route AS (
			SELECT ST_Simplify(track_full, 0.00005) AS geom
			FROM activity_tracks
			WHERE activity_id = $1
		)
		SELECT DISTINCT ON (p.osm_id)
		       p.osm_id, p.name, p.ele, p.category,
		       ROUND(ST_Distance(route.geom::geography, p.geom::geography))::int AS dist_m,
		       ST_Y(ST_Centroid(p.geom)) AS lat,
		       ST_X(ST_Centroid(p.geom)) AS lng
		FROM osm_pois p, route
		WHERE p.geom && ST_Expand(route.geom, $3)
		  AND ST_DWithin(route.geom::geography, p.geom::geography, $2)
		  AND p.category IN ('summit', 'pass')
		  AND p.name IS NOT NULL
		ORDER BY p.osm_id, dist_m
	`

	rows, err := pool.Query(ctx, q, activityID, radiusM, expandDeg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PassedSummit
	for rows.Next() {
		var s PassedSummit
		if err := rows.Scan(&s.OsmID, &s.Name, &s.EleM, &s.Category, &s.DistM, &s.Lat, &s.Lng); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
