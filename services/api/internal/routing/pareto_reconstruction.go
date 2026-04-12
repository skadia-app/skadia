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

// reconstructPathWithNodes rebuilds the edge sequence from root to targetStateID,
// also collecting the nodeID for each state in the path.
// Returns edges in start-to-target order, the corresponding nodeID sequence
// (length == len(edges)+1: source node IDs for each edge plus the final destination),
// and a pointer to the target state.
func reconstructPathWithNodes(
	rs *reachableState,
	targetStateID uint32,
) ([]EdgeHandle, []uint64, *paretoState, error) {
	const maxHops = 10000

	if int(targetStateID) >= len(rs.slab.states) {
		return nil, nil, nil, fmt.Errorf("reconstruction: targetStateID %d out of range (slab len %d)",
			targetStateID, len(rs.slab.states))
	}
	targetState := &rs.slab.states[targetStateID]

	type hop struct {
		handle EdgeHandle
		nodeID uint64 // the node this state belongs to
	}
	var hops []hop
	curID := targetStateID

	for i := 0; i < maxHops; i++ {
		if curID == rootStateID {
			slices.Reverse(hops)
			// Build separate edge and nodeID slices.
			// hops[0].nodeID is the root node (no incoming edge),
			// hops[i].handle (i>0) is the edge arriving at hops[i].nodeID.
			edges := make([]EdgeHandle, 0, len(hops))
			nodeIDs := make([]uint64, 0, len(hops))
			for _, h := range hops {
				nodeIDs = append(nodeIDs, h.nodeID)
				if h.handle != 0 {
					edges = append(edges, h.handle)
				}
			}
			return edges, nodeIDs, targetState, nil
		}
		if int(curID) >= len(rs.slab.states) {
			return nil, nil, nil, fmt.Errorf("reconstruction: stateID %d out of range", curID)
		}
		state := &rs.slab.states[curID]
		hops = append(hops, hop{
			handle: EdgeHandle(state.handle),
			nodeID: state.nodeID,
		})
		curID = state.parentStateID
	}
	return nil, nil, nil, errors.New("reconstruction: max hops exceeded (corrupt parent chain)")
}

// BuildRouteResultFromEdges constructs a RouteResult from a reconstructed
// edge sequence. Metrics come from the target paretoState (path-accumulated
// during findReachable). Geometry is re-read from the graph.
//
// nodeIDs must have length == len(edges)+1: nodeIDs[i] is the source node of
// edges[i], and nodeIDs[len(edges)] is the final destination node.
// Source-node coordinates are emitted before each edge's intermediate geometry
// so the output matches the retrack format used by unified.go.
//
// Note: Segments are NOT populated here (that requires EdgeTags which
// is not yet on GraphReader). Segment population happens when the
// handler merges leg1+leg2 via the normal routing path.
func BuildRouteResultFromEdges(
	edges []EdgeHandle,
	nodeIDs []uint64,
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

	// Walk edges to build geometry: emit source node coord, then intermediate points.
	for i, handle := range edges {
		// Emit source node coordinate.
		if i < len(nodeIDs) {
			lat, lon, ele := reader.NodeCoord(nodeIDs[i])
			result.Coordinates = append(result.Coordinates, LatLonEle{
				Lat: float64(lat) / 1e7,
				Lon: float64(lon) / 1e7,
				Ele: float64(ele) / 10.0,
			})
		}

		// Emit DP-simplified intermediate geometry (does NOT include start/end nodes).
		lats, lons := reader.EdgeGeometry(uint64(handle))
		elevsDm, _ := reader.EdgeElevProfile(uint64(handle))
		for j := range lats {
			ele := 0.0
			if j < len(elevsDm) {
				ele = float64(elevsDm[j]) / 10.0
			}
			result.Coordinates = append(result.Coordinates, LatLonEle{
				Lat: float64(lats[j]) / 1e7,
				Lon: float64(lons[j]) / 1e7,
				Ele: ele,
			})
		}
	}

	// Emit the final destination node coordinate.
	if len(nodeIDs) > len(edges) {
		lastNodeID := nodeIDs[len(nodeIDs)-1]
		lat, lon, ele := reader.NodeCoord(lastNodeID)
		result.Coordinates = append(result.Coordinates, LatLonEle{
			Lat: float64(lat) / 1e7,
			Lon: float64(lon) / 1e7,
			Ele: float64(ele) / 10.0,
		})
	}

	return result
}

// ExtractWayIDsFromRouteResult returns the set of OSM way IDs used by this route.
func ExtractWayIDsFromRouteResult(r *RouteResult) map[int64]bool {
	set := make(map[int64]bool, len(r.Segments))
	for _, seg := range r.Segments {
		if seg.OsmWayID != 0 {
			set[seg.OsmWayID] = true
		}
	}
	return set
}

// ExtractWayIDsFromEdges returns the set of OSM way IDs for an edge-handle list.
func ExtractWayIDsFromEdges(edges []EdgeHandle, reader GraphReader) map[int64]bool {
	set := make(map[int64]bool, len(edges))
	for _, h := range edges {
		id := reader.EdgeOSMID(uint64(h))
		if id != 0 {
			set[id] = true
		}
	}
	return set
}

// ReconstructAndBuildRoute reconstructs the edge path from root to targetStateID
// and builds a full RouteResult. This avoids exporting reachableState/paretoState
// to callers outside the routing package.
func ReconstructAndBuildRoute(rs *reachableState, targetStateID uint32, reader GraphReader) (*RouteResult, []EdgeHandle, error) {
	edges, nodeIDs, targetState, err := reconstructPathWithNodes(rs, targetStateID)
	if err != nil {
		return nil, nil, err
	}
	result := BuildRouteResultFromEdges(edges, nodeIDs, targetState, reader)
	return result, edges, nil
}

// ComputeWayIDOverlap returns the overlap of two WayID sets:
// |A ∩ B| / min(|A|, |B|). Returns 0 if either set is empty.
func ComputeWayIDOverlap(a, b map[int64]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	intersection := 0
	for id := range a {
		if b[id] {
			intersection++
		}
	}
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	return float64(intersection) / float64(minLen)
}
