package routing

import (
	"context"
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

// SelectCrossingSeedsSQL selects up to 4 crossing seeds (one per quadrant) using
// road-proximity ranking. Passes near major roads (motorway/trunk/primary/secondary)
// are the most important geographic bottlenecks — they sit at the lowest crossing
// of mountain ridges where roads, trails, and hiking routes all converge.
//
// Ranking: major_roads DESC (nearby road count), then ele DESC (tiebreaker).
// Fallback: quadrants with no road-adjacent passes fall back to ele DESC.
//
// The flank landmarks placed from these seeds help ALL route types (road cyclist,
// MTB, hiker, mountaineer) because flanks are in the VALLEYS on both sides of the
// ridge, not at the pass itself.
func SelectCrossingSeedsSQL(ctx context.Context, pool *pgxpool.Pool,
	west, south, east, north float64) ([]OSMLandmark, error) {
	rows, err := pool.Query(ctx, `
		WITH candidates AS (
			SELECT DISTINCT ON (osm_id)
			       osm_id, COALESCE(name, '') AS name,
			       ST_Y(geom) AS lat, ST_X(geom) AS lon, ele, geom
			FROM osm_pois
			WHERE ele IS NOT NULL AND ele > 800
			  AND geom && ST_MakeEnvelope($1, $2, $3, $4, 4326)
			  AND (
			    category = 'pass'
			    OR (category = 'summit' AND subtype = 'saddle')
			  )
		),
		medians AS (
			SELECT
				percentile_cont(0.5) WITHIN GROUP (ORDER BY lat) AS med_lat,
				percentile_cont(0.5) WITHIN GROUP (ORDER BY lon) AS med_lon
			FROM candidates
		),
		scored AS (
			SELECT c.osm_id, c.name, c.lat, c.lon, c.ele,
				CASE WHEN c.lat >= m.med_lat THEN 1 ELSE 0 END AS lat_bin,
				CASE WHEN c.lon >= m.med_lon THEN 1 ELSE 0 END AS lon_bin,
				(SELECT count(DISTINCT w.osm_id) FROM osm_ways w
				 WHERE w.highway IN ('motorway','motorway_link','trunk','trunk_link','primary','secondary')
				 AND ST_DWithin(w.geom, c.geom, 0.005)) AS major_roads
			FROM candidates c, medians m
		),
		ranked AS (
			SELECT *, ROW_NUMBER() OVER (
				PARTITION BY lat_bin, lon_bin
				ORDER BY major_roads DESC, ele DESC
			) AS rn
			FROM scored
		)
		SELECT osm_id, name, lat, lon, ele
		FROM ranked WHERE rn = 1
		ORDER BY lat_bin, lon_bin
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

// SnappedLandmark holds a graph-snapped landmark candidate with its snap distance.
// SnapDistM is the distance from the OSM POI to the nearest graph node in meters.
// A small snap distance (<100m) means the crossing is directly on a trail.
// A large snap distance (>1000m) means the crossing is far from any trail (glacier, cliff).
type SnappedLandmark struct {
	graph.LandmarkCandidate
	SnapDistM float64
}

// SnapOSMLandmarksToGraph converts OSMLandmarks to SnappedLandmarks by snapping
// to the nearest graph node using the tile SpatialGrid (O(9 cells) per candidate).
// Returns snap distance alongside each candidate — used by the pipeline to filter
// crossings that aren't on the trail network (glacial saddles snap at >1km).
func SnapOSMLandmarksToGraph(landmarks []OSMLandmark, tileDir string) []SnappedLandmark {
	// Cache loaded tiles to avoid repeated mmap for candidates in the same tile.
	tileCache := make(map[uint32]*graph.Graph)
	// tileCache entries are released by GC finalizer (closeMmap).

	loadTile := func(tileID uint32) *graph.Graph {
		if g, ok := tileCache[tileID]; ok {
			return g
		}
		path := graph.TileFilePath(tileDir, tileID)
		g, err := graph.DeserializeMmap(path)
		if err != nil {
			return nil
		}
		tileCache[tileID] = g
		return g
	}

	var results []SnappedLandmark
	for _, lm := range landmarks {
		tileID := graph.TileIDFromCoord(lm.Lat, lm.Lon)
		g := loadTile(tileID)
		if g == nil {
			continue
		}
		localID, distDm := graph.NearestNode(g, lm.Lat, lm.Lon)
		if distDm == 65535 {
			// Try neighbor tiles (candidate near tile border)
			nnids, nncount := graph.NeighborTileIDs(tileID)
			for i := range nncount {
				gn := loadTile(nnids[i])
				if gn == nil {
					continue
				}
				candLocal, candDist := graph.NearestNode(gn, lm.Lat, lm.Lon)
				if candDist < distDm {
					localID = candLocal
					distDm = candDist
					tileID = nnids[i]
					g = gn
				}
			}
		}
		if distDm == 65535 {
			continue
		}
		results = append(results, SnappedLandmark{
			LandmarkCandidate: graph.LandmarkCandidate{
				GlobalNodeID: graph.MakeGlobalNodeID(tileID, localID),
				Lat:          g.NodeLat[localID],
				Lon:          g.NodeLon[localID],
				Ele:          g.NodeEle[localID],
			},
			SnapDistM: float64(distDm) / 10.0, // dm → m
		})
	}
	return results
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
