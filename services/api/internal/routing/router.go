// router.go — UnifiedRouter backed by any GraphReader.
package routing

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"
)

// altDisabled disables the ALT heuristic when SKADIA_ALT_DISABLED=true.
// Useful for A/B comparisons and debugging.
var altDisabled = os.Getenv("SKADIA_ALT_DISABLED") == "true"

// UnifiedRouter is a Router backed by any GraphReader (e.g. TiledGraphReader).
// Uses a semaphore for concurrency control instead of a pre-allocated pool.
type UnifiedRouter struct {
	reader GraphReader
	sem    chan struct{}
	isoSem chan struct{} // separate semaphore for isochrone (2 concurrent)
}

// NewUnifiedRouter creates a router backed by any GraphReader.
// maxConcurrent limits simultaneous routing queries (semaphore-based).
func NewUnifiedRouter(reader GraphReader, maxConcurrent int) *UnifiedRouter {
	return &UnifiedRouter{
		reader:  reader,
		sem:     make(chan struct{}, maxConcurrent),
		isoSem:  make(chan struct{}, 2),
	}
}

// Route computes a route from start to destination using findRouteUnified
// (pass0 -> buildTrackIndexUnified -> pass1 -> retrackUnified).
func (r *UnifiedRouter) Route(ctx context.Context, req RouteRequest,
	cfg *CostConfig) (*RouteResult, error) {

	select {
	case r.sem <- struct{}{}:
		defer func() { <-r.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	totalStart := time.Now()

	src, _, ok := r.snapWithFallback(req.FromLat, req.FromLon, cfg)
	if !ok {
		return nil, fmt.Errorf("no graph node within 5km of start coordinate")
	}
	dst, _, ok := r.snapWithFallback(req.ToLat, req.ToLon, cfg)
	if !ok {
		return nil, fmt.Errorf("no graph node within 5km of destination coordinate")
	}

	if src == dst {
		lat, lon, ele := r.reader.NodeCoord(src)
		return &RouteResult{
			Coordinates: []LatLonEle{{
				Lat: float64(lat) / 1e7,
				Lon: float64(lon) / 1e7,
				Ele: float64(ele) / 10.0,
			}},
		}, nil
	}

	// Landmark-Heuristik initialisieren
	meta := r.reader.LandmarkMeta()
	setID := SelectLandmarkSet(cfg, meta)
	var landmarkH *LandmarkHeuristic
	if !altDisabled && meta != nil && meta.LandmarkCount > 0 {
		landmarkH = NewLandmarkHeuristic(r.reader, dst, setID)
	}

	// Pass 0: inadmissible A* (passCoeff > 1), MIT Landmark-Heuristik
	res0, err := findRouteUnified(ctx, r.reader, src, dst, cfg,
		cfg.Pass0Coefficient, 0, nil, landmarkH)
	if err != nil {
		return nil, fmt.Errorf("pass0: %w", err)
	}

	// Build track index from pass0 path for pass1 cost-cutting
	trackIndex := buildTrackIndexUnified(res0.state, src, dst)

	// Pass 1: Dijkstra with track-index cost-cutting (passCoeff = 0), OHNE Heuristik
	// Safety margin: pass0 uses DeltaEle which underestimates on hilly terrain.
	// DeltaEle sees net elevation (e.g. +150m), full profile sees actual work (e.g. 250m).
	// Underestimation can be up to ~1.7x on hilly edges. Single Pass0Coefficient (1.5x)
	// is insufficient: 0.6 * 1.5 = 0.9 < 1.0. Squared coefficient gives 2.25x headroom.
	pass1MaxCost := res0.totalCost * cfg.Pass0Coefficient * cfg.Pass0Coefficient
	res1, err := findRouteUnified(ctx, r.reader, src, dst, cfg,
		0, pass1MaxCost, trackIndex, nil)
	if err != nil {
		return nil, fmt.Errorf("pass1: %w", err)
	}

	// Retrack: reconstruct path with full geometry
	result := retrackUnified(r.reader, res1.state, src, dst)
	if result == nil {
		return nil, fmt.Errorf("retrack failed: broken parent chain")
	}
	result.PassStats = PassStats{
		Pass0Expansions: res0.expansions,
		Pass0DurationMs: res0.durationMs,
		Pass1Expansions: res1.expansions,
		Pass1DurationMs: res1.durationMs,
		TotalDurationMs: time.Since(totalStart).Milliseconds(),
	}
	return result, nil
}

// RouteVia computes a route through multiple waypoints.
// Hysteresis state is NOT transferred between legs (reset at via-points, like BRouter).
func (r *UnifiedRouter) RouteVia(ctx context.Context, waypoints []LatLon,
	cfg *CostConfig) (*RouteResult, error) {

	if len(waypoints) < 2 {
		return nil, fmt.Errorf("need at least 2 waypoints")
	}

	var legs []*RouteResult
	for i := range len(waypoints) - 1 {
		leg, err := r.Route(ctx, RouteRequest{
			FromLat: waypoints[i].Lat, FromLon: waypoints[i].Lon,
			ToLat: waypoints[i+1].Lat, ToLon: waypoints[i+1].Lon,
		}, cfg)
		if err != nil {
			return nil, fmt.Errorf("leg %d: %w", i, err)
		}
		legs = append(legs, leg)
	}
	return mergeLegs(legs), nil
}

// FindReachable finds all nodes reachable within the given cost budget.
// Uses the isochrone semaphore (2 concurrent) to limit resource usage.
func (r *UnifiedRouter) FindReachable(ctx context.Context,
	lat, lon float64, cfg *CostConfig, maxBudget float32, maxExpansions int,
) (map[uint64]reachableNode, bool, error) {

	select {
	case r.isoSem <- struct{}{}:
		defer func() { <-r.isoSem }()
	case <-ctx.Done():
		return nil, false, ctx.Err()
	}

	src, _, ok := r.snapWithFallback(lat, lon, cfg)
	if !ok {
		return nil, false, fmt.Errorf("no graph node within 5km of coordinate")
	}

	nodes, truncated := findReachable(ctx, r.reader, src, cfg, maxBudget, maxExpansions)
	return nodes, truncated, nil
}

// ReachableNode is the exported type for isochrone/suggest consumers.
type ReachableNode struct {
	NodeID  uint64
	Cost    float32
	Lat     float64
	Lon     float64
	Unknown bool // Terra Incognita: incoming edge OSM-ID not in visited set
}

// Reader returns the underlying GraphReader.
func (r *UnifiedRouter) Reader() GraphReader { return r.reader }

// snapWithFallback snaps (lat, lon) to the nearest graph node that has at
// least one non-blocked outgoing edge for cfg. If the closest node lies on a
// highway that cfg blocks (e.g. motorway with hiking profile), it tries 8
// offsets of ~50 m in cardinal and diagonal directions before giving up.
//
// This prevents coordinates placed 3–4 m beside a motorway from being snapped
// onto a blocked node, which would cause Pass0 to expand millions of nodes
// without ever reaching the destination (30 s timeout).
func (r *UnifiedRouter) snapWithFallback(lat, lon float64, cfg *CostConfig) (uint64, float64, bool) {
	if id, dist, ok := r.trySnap(lat, lon, cfg); ok {
		return id, dist, true
	}

	// Try 8 offsets of ~50 m (0.0005° ≈ 55 m) in cardinal + diagonal directions.
	const d = 0.0005
	offsets := [8][2]float64{
		{d, 0}, {-d, 0}, {0, d}, {0, -d},
		{d, d}, {d, -d}, {-d, d}, {-d, -d},
	}
	for _, off := range offsets {
		if id, dist, ok := r.trySnap(lat+off[0], lon+off[1], cfg); ok {
			slog.Info("routing: snap fallback used",
				"orig_lat", lat, "orig_lon", lon,
				"offset_lat", off[0], "offset_lon", off[1],
				"node_id", id, "dist_m", dist)
			return id, dist, true
		}
	}

	return 0, 0, false
}

// trySnap returns the nearest node to (lat, lon) only when it has at least one
// outgoing edge whose cost is below costBlocked for cfg.
func (r *UnifiedRouter) trySnap(lat, lon float64, cfg *CostConfig) (uint64, float64, bool) {
	nodeID, dist, ok := r.reader.NearestNode(lat, lon)
	if !ok {
		return 0, 0, false
	}

	usable := false
	r.reader.ForEachEdge(nodeID, func(ev EdgeView) {
		if usable {
			return
		}
		if cfg.EdgeCost(ev.Tags1, ev.Tags2).Flat < costBlocked {
			usable = true
		}
	})
	if usable {
		return nodeID, dist, true
	}
	return 0, 0, false
}

// FindReachableWithCoords returns reachable nodes with coordinates and
// Terra Incognita flags. Used by SuggestRoutes and Isochrone.
// Returns (nodes, truncated, error). truncated is true when the expansion
// limit (500k) was hit before the budget was exhausted.
func (r *UnifiedRouter) FindReachableWithCoords(ctx context.Context,
	lat, lon float64, cfg *CostConfig, maxBudget float32,
	visitedWayIDs map[int64]bool,
) ([]ReachableNode, bool, error) {

	nodes, truncated, err := r.FindReachable(ctx, lat, lon, cfg, maxBudget, 500000)
	if err != nil {
		return nil, false, err
	}

	result := make([]ReachableNode, 0, len(nodes))
	for nodeID, rn := range nodes {
		nodeLat, nodeLon, _ := r.reader.NodeCoord(nodeID)
		unknown := false
		if visitedWayIDs != nil && rn.EdgeHandle != 0 {
			osmID := r.reader.EdgeOSMID(rn.EdgeHandle)
			if osmID != 0 {
				unknown = !visitedWayIDs[osmID]
			}
		}
		result = append(result, ReachableNode{
			NodeID:  nodeID,
			Cost:    rn.Cost,
			Lat:     float64(nodeLat) / 1e7,
			Lon:     float64(nodeLon) / 1e7,
			Unknown: unknown,
		})
	}
	return result, truncated, nil
}

