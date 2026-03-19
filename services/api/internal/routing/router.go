// router.go — UnifiedRouter backed by any GraphReader.
package routing

import (
	"context"
	"fmt"
	"time"
)

// UnifiedRouter is a Router backed by any GraphReader (e.g. TiledGraphReader).
// Uses a semaphore for concurrency control instead of a pre-allocated pool.
type UnifiedRouter struct {
	reader GraphReader
	sem    chan struct{}
}

// NewUnifiedRouter creates a router backed by any GraphReader.
// maxConcurrent limits simultaneous routing queries (semaphore-based).
func NewUnifiedRouter(reader GraphReader, maxConcurrent int) *UnifiedRouter {
	return &UnifiedRouter{
		reader: reader,
		sem:    make(chan struct{}, maxConcurrent),
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

	src, _, ok := r.reader.NearestNode(req.FromLat, req.FromLon)
	if !ok {
		return nil, fmt.Errorf("no graph node within 5km of start coordinate")
	}
	dst, _, ok := r.reader.NearestNode(req.ToLat, req.ToLon)
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

	// Pass 0: inadmissible A* (passCoeff > 1)
	res0, err := findRouteUnified(ctx, r.reader, src, dst, cfg,
		cfg.Pass0Coefficient, 0, nil)
	if err != nil {
		return nil, fmt.Errorf("pass0: %w", err)
	}

	// Build track index from pass0 path for pass1 cost-cutting
	trackIndex := buildTrackIndexUnified(res0.state, src, dst)

	// Pass 1: Dijkstra with track-index cost-cutting (passCoeff = 0)
	// Safety margin: pass0 uses DeltaEle which underestimates on hilly terrain.
	// DeltaEle sees net elevation change, full profile sees all ups+downs.
	// Multiply by Pass0Coefficient to give pass1 enough budget.
	pass1MaxCost := res0.totalCost * cfg.Pass0Coefficient
	res1, err := findRouteUnified(ctx, r.reader, src, dst, cfg,
		0, pass1MaxCost, trackIndex)
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
