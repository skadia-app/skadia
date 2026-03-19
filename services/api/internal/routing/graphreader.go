// Package routing — GraphReader interface and SingleGraphReader implementation.
package routing

import "github.com/skadia-app/skadia/services/api/internal/routing/graph"

// EdgeView contains all data needed per edge expansion in A*.
// Stack-allocated in ForEachEdge callback — no heap allocation.
type EdgeView struct {
	Target     uint64 // global node ID: (tileID << 32 | localNodeID)
	TargetLat  int32  // target node latitude × 1e7 (avoids NodeCoord call in hot path)
	TargetLon  int32  // target node longitude × 1e7
	Dist       uint32 // distance in centimeters
	Tags1      uint64 // OSM tags (highway, surface, smoothness, access, ...)
	Tags2      uint64 // enrichment + extended tags (forest, noise, traffic, ...)
	ElevHandle uint64 // opaque: (tileID << 32 | edgeIdx) for profile/geometry
	DeltaEle   int32  // net elevation change in decimeters (target.Ele - source.Ele)
}

// GraphReader abstracts graph access for all routing consumers.
// One A* implementation works with any GraphReader.
type GraphReader interface {
	// === A* Hot Path ===

	// ForEachEdge calls fn for each outgoing edge of nodeID.
	// For border nodes in TiledGraphReader: automatically includes
	// cross-tile edges from neighbor tiles (transparent for A*).
	ForEachEdge(nodeID uint64, fn func(EdgeView))

	// NodeCoord returns the coordinate and elevation of a node.
	NodeCoord(nodeID uint64) (lat, lon, ele int32)

	// === Snapping ===

	// NearestNode finds the closest graph node to the given coordinates.
	NearestNode(lat, lon float64) (nodeID uint64, dist float64, ok bool)

	// === Hysteresis (pass1) ===

	// EdgeElevProfile returns the full-resolution elevation profile and
	// pre-computed segment distances for an edge.
	// elevsDm: elevation in decimeters at each original geometry point.
	// segDistsCm: distance in centimeters between consecutive points (N-1 values).
	EdgeElevProfile(handle uint64) (elevsDm []int32, segDistsCm []uint16)

	// === Retrack ===

	// EdgeGeometry returns the DP-simplified intermediate geometry points
	// for route display. Does NOT include start/end node coordinates.
	EdgeGeometry(handle uint64) (lats, lons []int32)

	// EdgeOSMID returns the OSM way ID for an edge identified by its handle.
	// Used for Terra Incognita matching in SuggestRoutes.
	// Returns 0 if not available.
	EdgeOSMID(handle uint64) int64
}

// --- Global Node ID helpers are in cross.go ---

// --- SingleGraphReader: wraps *graph.Graph for tests and single-tile ---

// SingleGraphReader implements GraphReader for a single in-memory graph.
// No border-node expansion. Used for unit tests and monolithic graphs.
type SingleGraphReader struct {
	G      *graph.Graph
	TileID uint32 // fixed (0 for tests)
}

func (r *SingleGraphReader) ForEachEdge(nodeID uint64, fn func(EdgeView)) {
	_, localID := SplitGlobalNodeID(nodeID)
	g := r.G
	for ei := g.Offsets[localID]; ei < g.Offsets[localID+1]; ei++ {
		target := g.Targets[ei]
		fn(EdgeView{
			Target:     MakeGlobalNodeID(r.TileID, target),
			TargetLat:  g.NodeLat[target],
			TargetLon:  g.NodeLon[target],
			Dist:       g.EdgeDist[ei],
			Tags1:      g.EdgeTags1[ei],
			Tags2:      g.EdgeTags2[ei],
			ElevHandle: uint64(r.TileID)<<32 | uint64(ei),
			DeltaEle:   g.NodeEle[target] - g.NodeEle[localID],
		})
	}
}

func (r *SingleGraphReader) NodeCoord(nodeID uint64) (lat, lon, ele int32) {
	_, localID := SplitGlobalNodeID(nodeID)
	return r.G.NodeLat[localID], r.G.NodeLon[localID], r.G.NodeEle[localID]
}

func (r *SingleGraphReader) NearestNode(lat, lon float64) (uint64, float64, bool) {
	nodeID, distDm := graph.NearestNode(r.G, lat, lon)
	if distDm == 65535 {
		return 0, 0, false
	}
	return MakeGlobalNodeID(r.TileID, nodeID), float64(distDm) / 10.0, true
}

func (r *SingleGraphReader) EdgeElevProfile(handle uint64) ([]int32, []uint16) {
	_, edgeIdx := SplitGlobalNodeID(handle)
	g := r.G

	elevStart := g.EdgeElevOff[edgeIdx]
	elevEnd := g.EdgeElevOff[edgeIdx+1]
	nPoints := elevEnd - elevStart

	elevs := g.EdgeElevProf[elevStart:elevEnd]

	if nPoints <= 1 || len(g.EdgeElevSegDist) == 0 {
		return elevs, nil
	}

	// C05: ElevSegDist offset = elevStart - edgeIdx
	segDistStart := elevStart - edgeIdx
	segDists := g.EdgeElevSegDist[segDistStart : segDistStart+nPoints-1]

	return elevs, segDists
}

func (r *SingleGraphReader) EdgeGeometry(handle uint64) ([]int32, []int32) {
	_, edgeIdx := SplitGlobalNodeID(handle)
	g := r.G

	geomStart := g.EdgeGeomOff[edgeIdx]
	geomEnd := g.EdgeGeomOff[edgeIdx+1]
	nPts := geomEnd - geomStart

	// A04: return only intermediate points (exclude start/end node coords)
	if nPts <= 2 {
		return nil, nil
	}
	return g.EdgeGeomLat[geomStart+1 : geomEnd-1], g.EdgeGeomLon[geomStart+1 : geomEnd-1]
}

func (r *SingleGraphReader) EdgeOSMID(handle uint64) int64 {
	_, edgeIdx := SplitGlobalNodeID(handle)
	g := r.G
	if len(g.EdgeOSMIdx) == 0 || len(g.OSMIDPool) == 0 {
		return 0
	}
	osmIdx := g.EdgeOSMIdx[edgeIdx]
	if int(osmIdx) >= len(g.OSMIDPool) {
		return 0
	}
	return g.OSMIDPool[osmIdx]
}
