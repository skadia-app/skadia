// Package routing — GraphReader interface and SingleGraphReader implementation.
package routing

import (
	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

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

	// ForEachInEdge iterates over incoming edges of nodeID (reverse adjacency).
	// Used by backward search for correct lower bounds on directed graphs.
	ForEachInEdge(nodeID uint64, fn func(EdgeView))

	// NodeCoord returns the coordinate and elevation of a node.
	NodeCoord(nodeID uint64) (lat, lon, ele int32)

	// === Snapping ===

	// NearestNode finds the closest graph node to the given coordinates.
	NearestNode(lat, lon float64) (nodeID uint64, dist float64, ok bool)

	// FindNearestEdge finds the nearest graph edge to the given coordinates.
	// If filter is non-nil, edges where filter(tags1) returns false are skipped.
	// Returns the edge projection and true, or zero-value and false if none found.
	FindNearestEdge(lat, lon float64, filter func(tags1 uint64) bool) (graph.EdgeProjection, bool)

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

	// EdgeTags2 returns the extended tag uint64 for an edge handle.
	// Returns 0 if handle is invalid.
	EdgeTags2(handle uint64) uint64

	// EdgeDistanceCm returns the edge length in centimeters for a handle.
	// Returns 0 if handle is invalid.
	// Note: graph.Graph.EdgeDist stores centimeters (uint32); this method
	// exposes that value directly.
	EdgeDistanceCm(handle uint64) uint32

	// === ALT Heuristic ===

	// LandmarkDists returns precomputed distances from nodeID to all 16 landmarks
	// for the given set. Returns nil if not available.
	LandmarkDists(setID LandmarkSetID, nodeID uint64) []float32

	// LandmarkDistsInto reads landmark distances into pre-allocated buffers,
	// avoiding allocation in the A* hot path.
	// dst must have length >= LandmarksPerSet; byteBuf must have length >= LandmarksPerSet*4.
	// Returns the filled slice or nil if no data is available.
	LandmarkDistsInto(setID LandmarkSetID, nodeID uint64, dst []float32, byteBuf []byte) []float32

	// LandmarkDistsPageInto reads landmark distances for a page of nodes in one I/O call.
	// tileID identifies the tile; startLocalNode is the first local node in the page;
	// nodeCount is the number of nodes to read (will be clamped to tile bounds).
	// dst must have capacity >= nodeCount * LandmarksPerSet; byteBuf >= nodeCount * LandmarksPerSet * 4.
	// Returns the filled slice (len = actualCount * LandmarksPerSet) or nil if unavailable.
	LandmarkDistsPageInto(setID LandmarkSetID, tileID uint32, startLocalNode, nodeCount uint32, dst []float32, byteBuf []byte) []float32

	// LandmarkMeta returns the landmark metadata (positions, thresholds).
	// Returns nil if no landmarks are loaded.
	LandmarkMeta() *graph.LandmarkMetaData

	// === Surface Enrichment ===

	// GraphAt returns the graph tile containing the given coordinates,
	// or nil if no tile covers that area. Used by surface enrichment
	// to run FindNearEdges on the underlying graph data.
	GraphAt(lat, lon float64) *graph.Graph

	// HasTilesFor checks whether tiles exist that cover the entire
	// bounding box. Returns false if any part is uncovered.
	HasTilesFor(minLat, minLon, maxLat, maxLon float64) bool
}

// --- Global Node ID helpers are in cross.go ---

// --- SingleGraphReader: wraps *graph.Graph for tests and single-tile ---

// SingleGraphReader implements GraphReader for a single in-memory graph.
// No border-node expansion. Used for unit tests and monolithic graphs.
type SingleGraphReader struct {
	G      *graph.Graph
	TileID uint32 // fixed (0 for tests)
}

func (r *SingleGraphReader) ForEachInEdge(nodeID uint64, fn func(EdgeView)) {
	_, localID := SplitGlobalNodeID(nodeID)
	g := r.G
	if g == nil || localID >= g.NodeCount {
		return
	}
	g.EnsureReverseAdj()
	if g.RevOffsets == nil { // empty graph
		return
	}
	for ri := g.RevOffsets[localID]; ri < g.RevOffsets[localID+1]; ri++ {
		srcLocal := g.RevTargets[ri]
		ei := g.RevEdgeIdx[ri]
		fn(EdgeView{
			Target:     MakeGlobalNodeID(r.TileID, srcLocal),
			TargetLat:  g.NodeLat[srcLocal],
			TargetLon:  g.NodeLon[srcLocal],
			Dist:       g.EdgeDist[ei],
			Tags1:      g.EdgeTags1[ei],
			Tags2:      g.EdgeTags2[ei],
			ElevHandle: uint64(r.TileID)<<32 | uint64(ei),
			DeltaEle:   g.NodeEle[srcLocal] - g.NodeEle[localID],
		})
	}
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

func (r *SingleGraphReader) FindNearestEdge(lat, lon float64, filter func(tags1 uint64) bool) (graph.EdgeProjection, bool) {
	if r.G == nil {
		return graph.EdgeProjection{}, false
	}
	return r.G.FindNearestEdgeLocal(lat, lon, filter)
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

func (r *SingleGraphReader) EdgeTags2(handle uint64) uint64 {
	_, edgeIdx := SplitGlobalNodeID(handle)
	g := r.G
	if int(edgeIdx) >= len(g.EdgeTags2) {
		return 0
	}
	return g.EdgeTags2[edgeIdx]
}

func (r *SingleGraphReader) EdgeDistanceCm(handle uint64) uint32 {
	_, edgeIdx := SplitGlobalNodeID(handle)
	g := r.G
	if int(edgeIdx) >= len(g.EdgeDist) {
		return 0
	}
	return g.EdgeDist[edgeIdx]
}

func (r *SingleGraphReader) LandmarkDists(setID LandmarkSetID, nodeID uint64) []float32 {
	_, localID := SplitGlobalNodeID(nodeID)
	return r.G.LandmarkDists(graph.LandmarkSetID(setID), localID)
}

func (r *SingleGraphReader) LandmarkDistsInto(setID LandmarkSetID, nodeID uint64, dst []float32, byteBuf []byte) []float32 {
	_, localID := SplitGlobalNodeID(nodeID)
	return r.G.LandmarkDistsInto(graph.LandmarkSetID(setID), localID, dst, byteBuf)
}

func (r *SingleGraphReader) LandmarkDistsPageInto(setID LandmarkSetID, tileID uint32, startLocalNode, nodeCount uint32, dst []float32, byteBuf []byte) []float32 {
	// tileID must match the single graph's tile; silently return nil if not.
	if tileID != r.TileID {
		return nil
	}
	return r.G.LandmarkDistsPageInto(graph.LandmarkSetID(setID), startLocalNode, nodeCount, dst, byteBuf)
}

func (r *SingleGraphReader) LandmarkMeta() *graph.LandmarkMetaData {
	return r.G.LandmarkMeta
}

func (r *SingleGraphReader) GraphAt(lat, lon float64) *graph.Graph {
	return r.G
}

func (r *SingleGraphReader) HasTilesFor(minLat, minLon, maxLat, maxLon float64) bool {
	return r.G != nil
}
