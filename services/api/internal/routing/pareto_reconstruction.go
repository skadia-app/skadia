package routing

import (
	"errors"
	"fmt"
	"slices"
)

// EdgeHandle is an opaque edge identifier used during path reconstruction.
type EdgeHandle uint64

// ReconstructPath rebuilds the edge sequence from the root state to targetStateID
// by walking the parent chain through the slab.
// Returns edges in start-to-target order, plus a pointer to the target state.
func ReconstructPath(
	rs *reachableState,
	targetStateID uint32,
) ([]EdgeHandle, *paretoState, error) {
	const maxHops = 10000

	if int(targetStateID) >= len(rs.slab.states) {
		return nil, nil, fmt.Errorf("reconstruction: targetStateID %d out of range (slab len %d)",
			targetStateID, len(rs.slab.states))
	}
	targetState := &rs.slab.states[targetStateID]

	var edges []EdgeHandle
	curID := targetStateID

	for i := 0; i < maxHops; i++ {
		if curID == rootStateID {
			slices.Reverse(edges)
			return edges, targetState, nil
		}
		if int(curID) >= len(rs.slab.states) {
			return nil, nil, fmt.Errorf("reconstruction: stateID %d out of range", curID)
		}
		state := &rs.slab.states[curID]
		if state.handle != 0 {
			edges = append(edges, EdgeHandle(state.handle))
		}
		curID = state.parentStateID
	}
	return nil, nil, errors.New("reconstruction: max hops exceeded (corrupt parent chain)")
}

// BuildRouteResultFromEdges constructs a RouteResult from a reconstructed
// edge sequence. Metrics come from the target paretoState (path-accumulated
// during findReachable). Geometry is re-read from the graph.
//
// Note: Segments are NOT populated here (that requires EdgeTags which
// is not yet on GraphReader). Segment population happens when the
// handler merges leg1+leg2 via the normal routing path.
func BuildRouteResultFromEdges(
	edges []EdgeHandle,
	targetState *paretoState,
	reader GraphReader,
) *RouteResult {
	result := &RouteResult{
		DistanceM: float64(targetState.dist),
		AscentM:   float64(targetState.ascent),
		DescentM:  float64(targetState.descent),
		Cost:      float64(targetState.cost),

		DistSum:    targetState.dist,
		QualitySum: targetState.qualitySum,
		AscentSum:  targetState.ascent,
		DescentSum: targetState.descent,
	}

	// Walk edges to build geometry
	for _, handle := range edges {
		lats, lons := reader.EdgeGeometry(uint64(handle))
		elevsDm, _ := reader.EdgeElevProfile(uint64(handle))

		for i := range lats {
			ele := 0.0
			if i < len(elevsDm) {
				ele = float64(elevsDm[i]) / 10.0
			}
			result.Coordinates = append(result.Coordinates, LatLonEle{
				Lat: float64(lats[i]) / 1e7,
				Lon: float64(lons[i]) / 1e7,
				Ele: ele,
			})
		}
	}

	return result
}
