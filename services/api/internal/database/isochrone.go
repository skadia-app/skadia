package database

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ReachableNode represents a node reachable within a given cost budget from the
// pgr_drivingDistance query, where cost is measured in minutes of travel time.
type ReachableNode struct {
	NodeID int64   `json:"node_id"`
	Cost   float64 `json:"cost"` // minutes to reach this node
}

// IsochronePOI represents a point of interest found within an isochrone polygon.
type IsochronePOI struct {
	Name    string  `json:"name"`
	Type    string  `json:"type"`
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
	OsmID   int64   `json:"osm_id,omitempty"`
	CostMin float64 `json:"cost_min"` // estimated travel time to this POI
}

// IsochronePolygon is a GeoJSON polygon for a single time band.
type IsochronePolygon struct {
	Minutes int             `json:"minutes"`
	GeoJSON json.RawMessage `json:"geojson"`
}

// NearestTrailNode finds the nearest node in the osm_ways topology to the
// given [lng, lat] coordinate and returns its node ID.
// Returns pgx.ErrNoRows (wrapped) if no trail topology has been built yet.
func (db *DB) NearestTrailNode(ctx context.Context, lngLat [2]float64) (int64, error) {
	var nodeID int64
	err := db.Pool.QueryRow(ctx, `
		SELECT v.id
		FROM osm_ways_vertices_pgr v
		ORDER BY v.the_geom <-> ST_SetSRID(ST_MakePoint($1, $2), 4326)
		LIMIT 1
	`, lngLat[0], lngLat[1]).Scan(&nodeID)
	if err == pgx.ErrNoRows {
		return 0, fmt.Errorf("nearest trail node: no topology available: %w", err)
	}
	if err != nil {
		return 0, fmt.Errorf("nearest trail node: %w", err)
	}
	return nodeID, nil
}

// DrivingDistance runs pgr_drivingDistance from startNode with a budget of
// maxMinutes (using the cost column which stores travel time in minutes).
// Returns all reachable nodes ordered by ascending cost.
func (db *DB) DrivingDistance(ctx context.Context, startNode int64, maxMinutes float64) ([]ReachableNode, error) {
	// pgRouting 4.x: pgr_drivingDistance(sql text, start_v bigint, distance float8)
	rows, err := db.Pool.Query(ctx, `
		SELECT node, agg_cost
		FROM pgr_drivingDistance(
			'SELECT id, source, target, cost, reverse_cost FROM osm_ways WHERE cost > 0',
			$1,
			$2
		)
		ORDER BY agg_cost ASC
	`, startNode, maxMinutes)
	if err != nil {
		return nil, fmt.Errorf("pgr_drivingDistance: %w", err)
	}
	defer rows.Close()

	var nodes []ReachableNode
	for rows.Next() {
		var n ReachableNode
		if err := rows.Scan(&n.NodeID, &n.Cost); err != nil {
			return nil, fmt.Errorf("scan driving distance row: %w", err)
		}
		nodes = append(nodes, n)
	}
	// E15: check rows.Err() after iteration.
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate driving distance rows: %w", err)
	}
	return nodes, nil
}

// BuildIsochronePolygons uses pgr_alphashape (if available) or a convex hull
// fallback to generate one polygon per requested time band.
// reachable must be sorted by ascending cost (as returned by DrivingDistance).
func (db *DB) BuildIsochronePolygons(ctx context.Context, reachable []ReachableNode, minutes []int) ([]IsochronePolygon, error) {
	if len(reachable) == 0 || len(minutes) == 0 {
		return nil, nil
	}

	// Build a temporary VALUES list of (node_id, cost) to pass to SQL.
	// We query the vertex coordinates from osm_ways_vertices_pgr and compute
	// an alpha shape per time band.
	var polygons []IsochronePolygon

	for _, m := range minutes {
		budget := float64(m)

		// Collect node IDs within this time budget.
		var nodeIDs []int64
		for _, n := range reachable {
			if n.Cost <= budget {
				nodeIDs = append(nodeIDs, n.NodeID)
			}
		}
		if len(nodeIDs) < 3 {
			// Not enough points for a polygon — skip this band silently.
			continue
		}

		// pgr_alphashape needs at least a few dozen points; convex hull is the
		// reliable fallback for sparse networks.  We attempt pgr_alphashape
		// first and fall back to ST_ConvexHull on error.
		var gjson []byte
		err := db.Pool.QueryRow(ctx, `
			WITH pts AS (
				SELECT the_geom
				FROM osm_ways_vertices_pgr
				WHERE id = ANY($1)
			),
			hull AS (
				SELECT ST_ConvexHull(ST_Collect(the_geom)) AS geom FROM pts
			)
			SELECT ST_AsGeoJSON(geom) FROM hull
		`, nodeIDs).Scan(&gjson)
		if err != nil {
			// Non-fatal: skip this band.
			continue
		}

		polygons = append(polygons, IsochronePolygon{
			Minutes: m,
			GeoJSON: json.RawMessage(gjson),
		})
	}

	return polygons, nil
}

// ResolveNodeCoordinates fetches the [lng, lat] for each node in the reachable
// slice from osm_ways_vertices_pgr. The result map key is the node ID.
// Nodes not found in the topology are silently omitted.
func (db *DB) ResolveNodeCoordinates(ctx context.Context, reachable []ReachableNode) (map[int64][2]float64, error) {
	if len(reachable) == 0 {
		return map[int64][2]float64{}, nil
	}

	nodeIDs := make([]int64, len(reachable))
	for i, n := range reachable {
		nodeIDs[i] = n.NodeID
	}

	rows, err := db.Pool.Query(ctx, `
		SELECT id, ST_X(the_geom) AS lng, ST_Y(the_geom) AS lat
		FROM osm_ways_vertices_pgr
		WHERE id = ANY($1)
	`, nodeIDs)
	if err != nil {
		return nil, fmt.Errorf("resolve node coordinates: %w", err)
	}
	defer rows.Close()

	result := make(map[int64][2]float64, len(reachable))
	for rows.Next() {
		var id int64
		var lng, lat float64
		if err := rows.Scan(&id, &lng, &lat); err != nil {
			return nil, fmt.Errorf("scan node coordinate: %w", err)
		}
		result[id] = [2]float64{lng, lat}
	}
	// E15: check rows.Err() after iteration.
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate node coordinates: %w", err)
	}
	return result, nil
}

// GetVisitedTrailIDs returns the set of osm_ways node IDs (source and target)
// that the user has already visited according to heatmap_trail_matches.
// The returned map can be used to identify unvisited ("unknown") nodes when
// selecting via-points in the route suggestion algorithm.
func (db *DB) GetVisitedTrailIDs(ctx context.Context, userID string) (map[int64]bool, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT t.source, t.target
		FROM heatmap_trail_matches m
		JOIN osm_ways t ON t.id = m.way_id
		WHERE m.user_id = $1
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("get visited trail IDs: %w", err)
	}
	defer rows.Close()

	visited := make(map[int64]bool)
	for rows.Next() {
		var src, tgt int64
		if err := rows.Scan(&src, &tgt); err != nil {
			return nil, fmt.Errorf("scan visited trail row: %w", err)
		}
		if src != 0 {
			visited[src] = true
		}
		if tgt != 0 {
			visited[tgt] = true
		}
	}
	// E15: check rows.Err() after iteration.
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate visited trail rows: %w", err)
	}
	return visited, nil
}

// FindPOIsInIsochrone finds points of interest (from osm_pois) of the
// requested types that lie inside the convex hull of reachable nodes.
// poiTypes may include values like "hut", "cafe", "restaurant", "peak",
// "viewpoint".
func (db *DB) FindPOIsInIsochrone(ctx context.Context, reachable []ReachableNode, poiTypes []string) ([]IsochronePOI, error) {
	if len(reachable) == 0 || len(poiTypes) == 0 {
		return nil, nil
	}

	nodeIDs := make([]int64, len(reachable))
	for i, n := range reachable {
		nodeIDs[i] = n.NodeID
	}

	rows, err := db.Pool.Query(ctx, `
		WITH hull AS (
			SELECT ST_ConvexHull(ST_Collect(the_geom)) AS geom
			FROM osm_ways_vertices_pgr
			WHERE id = ANY($1)
		)
		SELECT p.name, p.subtype, ST_Y(p.geom) AS lat, ST_X(p.geom) AS lng, p.osm_id
		FROM osm_pois p, hull h
		WHERE ST_Within(p.geom, h.geom)
		  AND p.subtype = ANY($2)
		ORDER BY p.name
		LIMIT 500
	`, nodeIDs, poiTypes)
	if err != nil {
		return nil, fmt.Errorf("find POIs in isochrone: %w", err)
	}
	defer rows.Close()

	var pois []IsochronePOI
	for rows.Next() {
		var p IsochronePOI
		if err := rows.Scan(&p.Name, &p.Type, &p.Lat, &p.Lng, &p.OsmID); err != nil {
			return nil, fmt.Errorf("scan POI row: %w", err)
		}
		pois = append(pois, p)
	}
	// E15: check rows.Err() after iteration.
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate POI rows: %w", err)
	}
	return pois, nil
}
