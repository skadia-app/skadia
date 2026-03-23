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
	Lat   float64
	Lon   float64
	Ele   float64
}

// QueryOSMPassLandmarks returns top-N highest passes/saddles within bbox.
// Covers category='pass' (mountain_pass=yes) AND category='summit' with natural=saddle.
// Note: OSM ele tags can contain non-numeric values ("~1370", "1370 m").
// The regex filter ensures only clean numeric values are cast to float.
func QueryOSMPassLandmarks(ctx context.Context, pool *pgxpool.Pool,
	west, south, east, north float64, limit int) ([]OSMLandmark, error) {
	rows, err := pool.Query(ctx, `
		SELECT osm_id, COALESCE(name, ''), ST_Y(geom), ST_X(geom),
		       (tags->>'ele')::float
		FROM osm_pois
		WHERE (category = 'pass' OR (category = 'summit' AND tags->>'natural' = 'saddle'))
		  AND ST_Within(geom, ST_MakeEnvelope($1, $2, $3, $4, 4326))
		  AND (tags->>'ele') ~ '^\d+\.?\d*$'
		  AND (tags->>'ele')::float > 800
		ORDER BY (tags->>'ele')::float DESC
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

// QueryOSMPeakLandmarks returns top-N highest peaks (excluding saddles) within bbox.
func QueryOSMPeakLandmarks(ctx context.Context, pool *pgxpool.Pool,
	west, south, east, north float64, limit int) ([]OSMLandmark, error) {
	rows, err := pool.Query(ctx, `
		SELECT osm_id, COALESCE(name, ''), ST_Y(geom), ST_X(geom),
		       (tags->>'ele')::float
		FROM osm_pois
		WHERE category = 'summit'
		  AND tags->>'natural' IS DISTINCT FROM 'saddle'
		  AND ST_Within(geom, ST_MakeEnvelope($1, $2, $3, $4, 4326))
		  AND (tags->>'ele') ~ '^\d+\.?\d*$'
		ORDER BY (tags->>'ele')::float DESC
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

// SnapOSMLandmarksToGraph converts OSMLandmarks to LandmarkCandidates by snapping to nearest graph node.
func SnapOSMLandmarksToGraph(landmarks []OSMLandmark, tiles []graph.TileScanResult) []graph.LandmarkCandidate {
	var candidates []graph.LandmarkCandidate
	for _, lm := range landmarks {
		latE7 := int32(lm.Lat * 1e7)
		lonE7 := int32(lm.Lon * 1e7)
		best := graph.FindNearestInTiles(tiles, latE7, lonE7)
		if best.GlobalNodeID == 0 {
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
		for i := range t.NodeCount {
			lat := float64(t.NodeLats[i]) / 1e7
			lon := float64(t.NodeLons[i]) / 1e7
			south = math.Min(south, lat)
			north = math.Max(north, lat)
			west = math.Min(west, lon)
			east = math.Max(east, lon)
		}
	}
	return
}
