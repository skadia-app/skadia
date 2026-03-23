package routing

import (
	"context"
	"log/slog"
	"math"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OSMLandmark holds a geographic landmark from OSM data.
type OSMLandmark struct {
	OSMID int64
	Name  string
	Lat   float64 // degrees
	Lon   float64 // degrees
	Ele   float64 // meters (from osm_pois.ele, NOT decimeters)
}

// QueryOSMPassLandmarks returns top-N highest passes/saddles within bbox.
// Covers category='pass' (mountain_pass=yes) AND category='summit' with natural=saddle.
// Uses the ele column (float, set by skadia-osm.lua), NOT tags->>'ele'.
func QueryOSMPassLandmarks(ctx context.Context, pool *pgxpool.Pool,
	west, south, east, north float64, limit int) ([]OSMLandmark, error) {
	rows, err := pool.Query(ctx, `
		SELECT osm_id, COALESCE(name, ''), ST_Y(geom), ST_X(geom), ele
		FROM osm_pois
		WHERE (category = 'pass' OR (category = 'summit' AND subtype = 'saddle'))
		  AND ST_Within(geom, ST_MakeEnvelope($1, $2, $3, $4, 4326))
		  AND ele IS NOT NULL AND ele > 800
		ORDER BY ele DESC
		LIMIT $5
	`, west, south, east, north, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []OSMLandmark
	for rows.Next() {
		var lm OSMLandmark
		if err := rows.Scan(&lm.OSMID, &lm.Name, &lm.Lat, &lm.Lon, &lm.Ele); err != nil {
			return nil, err
		}
		results = append(results, lm)
	}
	return results, rows.Err()
}

// QueryOutdoorCrossings returns all outdoor crossings (passes, saddles, named cols)
// within the given bounding box. These are candidates for landmark seed selection.
// Uses: category='pass' (mountain passes), subtype='saddle' (geographical saddles),
// and name-based matching for Joch/Scharte/Sattel/Forcella/etc.
// Requires ele > 500m to include Mittelgebirge crossings.
// NOTE: ele > 500 excludes coastal crossings (Cinque Terre ~200-400m, Norwegian
// fjord crossings ~100-300m). Acceptable for v1: coastal regions have no
// bottleneck problem (low relief). For global rollout, consider lowering to 300m
// or making the threshold adaptive based on BBox elevation range.
func QueryOutdoorCrossings(ctx context.Context, pool *pgxpool.Pool,
	west, south, east, north float64) ([]OSMLandmark, error) {
	rows, err := pool.Query(ctx, `
		SELECT osm_id, COALESCE(name, ''), ST_Y(geom), ST_X(geom), ele
		FROM osm_pois
		WHERE ele IS NOT NULL AND ele > 500
		  AND geom && ST_MakeEnvelope($1, $2, $3, $4, 4326)
		  AND (
		    category = 'pass'
		    OR (category = 'summit' AND subtype = 'saddle')
		    OR (category = 'summit' AND (
		      name ILIKE '%joch%' OR name ILIKE '%scharte%'
		      OR name ILIKE '%sattel%' OR name ILIKE '%forcella%'
		      OR name ILIKE '%bocchetta%' OR name ILIKE '%col %'
		      OR name ILIKE '%colle %' OR name ILIKE '%fuorcla%'
		      OR name ILIKE '%luecke%' OR name ILIKE '%toerl%'
		    ))
		  )
	`, west, south, east, north)
	// NOTE: 10 ILIKE patterns on name column. Performance is acceptable because:
	// (a) geom && BBox narrows to ~1-5K rows before ILIKE runs
	// (b) this runs only during preprocessing (~1/month), not in hot path
	// If query exceeds 1s on large BBoxes, consider pg_trgm GIN index on osm_pois.name
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []OSMLandmark
	for rows.Next() {
		var lm OSMLandmark
		if err := rows.Scan(&lm.OSMID, &lm.Name, &lm.Lat, &lm.Lon, &lm.Ele); err != nil {
			return nil, err
		}
		results = append(results, lm)
	}
	return results, rows.Err()
}

// SelectCrossingSeedsSQL selects up to 12 crossing seeds (top-3 per quadrant)
// via median-split. Returns seeds ordered by quadrant then elevation.
// Uses ele BETWEEN 500 AND 3000 to exclude glacial saddles (>3000m) which are
// poorly connected in the trail graph and produce useless ALT landmarks.
// Returns top-3 per quadrant so the pipeline can skip disconnected seeds
// and try the next candidate.
func SelectCrossingSeedsSQL(ctx context.Context, pool *pgxpool.Pool,
	west, south, east, north float64) ([]OSMLandmark, error) {
	rows, err := pool.Query(ctx, `
		WITH crossings AS (
			SELECT osm_id, COALESCE(name, '') AS name,
			       ST_Y(geom) AS lat, ST_X(geom) AS lon, ele
			FROM osm_pois
			WHERE ele IS NOT NULL AND ele > 500 AND ele < 3000
			  AND geom && ST_MakeEnvelope($1, $2, $3, $4, 4326)
			  AND (
			    category = 'pass'
			    OR (category = 'summit' AND subtype = 'saddle')
			    OR (category = 'summit' AND (
			      name ILIKE '%joch%' OR name ILIKE '%scharte%'
			      OR name ILIKE '%sattel%' OR name ILIKE '%forcella%'
			      OR name ILIKE '%bocchetta%' OR name ILIKE '%col %'
			      OR name ILIKE '%colle %' OR name ILIKE '%fuorcla%'
			      OR name ILIKE '%luecke%' OR name ILIKE '%toerl%'
			    ))
			  )
		),
		medians AS (
			SELECT
				percentile_cont(0.5) WITHIN GROUP (ORDER BY lat) AS med_lat,
				percentile_cont(0.5) WITHIN GROUP (ORDER BY lon) AS med_lon
			FROM crossings
		),
		ranked AS (
			SELECT c.osm_id, c.name, c.lat, c.lon, c.ele,
				CASE WHEN c.lat >= m.med_lat THEN 1 ELSE 0 END AS lat_bin,
				CASE WHEN c.lon >= m.med_lon THEN 1 ELSE 0 END AS lon_bin,
				ROW_NUMBER() OVER (
					PARTITION BY
						CASE WHEN c.lat >= m.med_lat THEN 1 ELSE 0 END,
						CASE WHEN c.lon >= m.med_lon THEN 1 ELSE 0 END
					ORDER BY c.ele DESC
				) AS rn
			FROM crossings c, medians m
		)
		SELECT osm_id, name, lat, lon, ele
		FROM ranked WHERE rn <= 3
		ORDER BY lat_bin, lon_bin, rn
	`, west, south, east, north)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []OSMLandmark
	for rows.Next() {
		var lm OSMLandmark
		if err := rows.Scan(&lm.OSMID, &lm.Name, &lm.Lat, &lm.Lon, &lm.Ele); err != nil {
			return nil, err
		}
		results = append(results, lm)
	}
	return results, rows.Err()
}

// SnapOSMLandmarksToGraph converts OSMLandmarks to LandmarkCandidates by snapping to nearest graph node.
func SnapOSMLandmarksToGraph(landmarks []OSMLandmark, tiles []graph.TileScanResult) []graph.LandmarkCandidate {
	var candidates []graph.LandmarkCandidate
	for _, lm := range landmarks {
		latE7 := int32(lm.Lat * 1e7)
		lonE7 := int32(lm.Lon * 1e7)
		best, found := graph.FindNearestInTiles(tiles, latE7, lonE7)
		if !found {
			slog.Warn("OSM landmark snap failed", "osm_id", lm.OSMID, "name", lm.Name)
			continue
		}
		candidates = append(candidates, best)
	}
	return candidates
}

// BBoxFromTiles computes bounding box from tile scan results.
func BBoxFromTiles(tiles []graph.TileScanResult) (south, west, north, east float64) {
	south, west = 90.0, 180.0
	north, east = -90.0, -180.0
	for _, t := range tiles {
		for i, lat32 := range t.NodeLats {
			lat := float64(lat32) / 1e7
			lon := float64(t.NodeLons[i]) / 1e7
			south = math.Min(south, lat)
			north = math.Max(north, lat)
			west = math.Min(west, lon)
			east = math.Max(east, lon)
		}
	}
	return
}
