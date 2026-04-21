package database

import (
	"context"
	"fmt"
	"log/slog"

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

// findSummitsCore is the shared implementation for FindSummitsAlongGeom and
// FindSummitsAlongActivityTrack. The caller provides a routeCTE that defines
// a CTE named "route" yielding a single "geom" column.
//
// simplifyTolerance note:
//   - Activity tracks use ST_Simplify(track_full, 0.00005) to reduce GPS noise
//     before the spatial predicates are evaluated.
//   - Route geometries from the Dijkstra pipeline are already smooth; no
//     simplification is applied (simplifyTolerance == 0 → caller omits it from
//     the CTE).
//
// DISTINCT ON deduplication: uses a prefixed CASE expression to namespace
// osm_id=0 rows ('id:<serial>') separately from real OSM nodes ('osm:<osm_id>').
// This prevents ID-space collisions between osm_pois.id (BIGSERIAL, starts at 1)
// and osm_pois.osm_id (OSM node IDs, also start at 1) — without the prefix, a
// row with osm_id=0,id=5 and a row with osm_id=5,id=99 would deduplicate to one
// result (F1 fix, 2026-04-21). Real summits are still deduplicated by osm_id
// (each osm_id appears at most once — closest passage wins). The ORDER BY must
// match the DISTINCT ON expression exactly (Postgres requirement).
func findSummitsCore(
	ctx context.Context,
	pool *pgxpool.Pool,
	routeCTE string, // full "WITH route AS (...)" fragment
	args []any,      // query args matching $1..$N already used in routeCTE, plus $radius, $expandDeg at N+1 / N+2
	radiusArgIdx int, // 1-based index of the radius parameter in args
	expandArgIdx int, // 1-based index of the expandDeg parameter in args
) ([]PassedSummit, error) {
	q := fmt.Sprintf(`%s
		SELECT DISTINCT ON (CASE WHEN p.osm_id = 0 THEN 'id:' || p.id::text ELSE 'osm:' || p.osm_id::text END)
		       p.osm_id, p.name, p.ele, p.category,
		       ROUND(ST_Distance(route.geom::geography, p.geom::geography))::int AS dist_m,
		       ST_Y(ST_Centroid(p.geom)) AS lat,
		       ST_X(ST_Centroid(p.geom)) AS lng
		FROM osm_pois p, route
		WHERE p.geom && ST_Expand(route.geom, $%d)
		  AND ST_DWithin(route.geom::geography, p.geom::geography, $%d)
		  AND p.category IN ('summit', 'pass')
		  AND p.name IS NOT NULL
		ORDER BY CASE WHEN p.osm_id = 0 THEN 'id:' || p.id::text ELSE 'osm:' || p.osm_id::text END, dist_m
	`, routeCTE, expandArgIdx, radiusArgIdx)

	rows, err := pool.Query(ctx, q, args...)
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
	// Early-return for empty geometry or non-positive radius (defensive — e.g.
	// approach-only routes, or mis-configured test fixtures). Handler layer
	// already clamps user input to [10, 150] via clampSummitDetectionRadius;
	// this check protects against internal mis-uses (e.g. direct Go tests).
	if len(geomWKB) == 0 || radiusM <= 0 {
		if radiusM <= 0 {
			slog.Warn("summit detection skipped: radius_m must be > 0 (check user_enrichment_settings.summit_detection_radius_m migration)",
				"radius_m", radiusM)
		}
		return nil, nil
	}
	expandDeg := expandDegForRadius(radiusM)

	// routeCTE: route geometry comes from the WKB parameter ($1).
	// Subsequent parameters: $2 = radiusM, $3 = expandDeg.
	const routeCTE = `WITH route AS (
		SELECT ST_GeomFromWKB($1, 4326) AS geom
	)`

	return findSummitsCore(ctx, pool, routeCTE,
		[]any{geomWKB, radiusM, expandDeg},
		2, // $2 = radiusM
		3, // $3 = expandDeg
	)
}

// FindSummitsAlongActivityTrack returns summits and passes within radiusM of
// the stored track for the given activity. The track is fetched inside the
// query CTE (no Go-side round-trip) and lightly simplified to reduce the
// point-count used by the spatial predicates.
//
// ST_Simplify(track_full, 0.00005) is applied to the activity track because
// GPS noise can otherwise create spurious distance artefacts. Route geometries
// from FindSummitsAlongGeom are Dijkstra-smooth and need no simplification.
//
// Returns nil, nil if radiusM ≤ 0. Returns an empty slice (no error) when the
// activity has no track row or the track has no nearby summits.
func FindSummitsAlongActivityTrack(
	ctx context.Context,
	pool *pgxpool.Pool,
	activityID string,
	radiusM float64,
) ([]PassedSummit, error) {
	// Early-return for non-positive radius (defensive — same rationale as
	// FindSummitsAlongGeom; handler layer already clamps to [10, 150]).
	if radiusM <= 0 {
		slog.Warn("summit detection skipped: radius_m must be > 0 (check user_enrichment_settings.summit_detection_radius_m migration)",
			"radius_m", radiusM,
			"activity_id", activityID)
		return nil, nil
	}
	expandDeg := expandDegForRadius(radiusM)

	// routeCTE: activity track fetched by $1 = activityID; simplified to reduce GPS noise.
	// Subsequent parameters: $2 = radiusM, $3 = expandDeg.
	const routeCTE = `WITH route AS (
		SELECT ST_Simplify(track_full, 0.00005) AS geom
		FROM activity_tracks
		WHERE activity_id = $1
	)`

	return findSummitsCore(ctx, pool, routeCTE,
		[]any{activityID, radiusM, expandDeg},
		2, // $2 = radiusM
		3, // $3 = expandDeg
	)
}
