// Package graph implements an in-memory CSR routing graph for FITroute.
package graph

import "time"

// Graph is the immutable in-memory CSR routing graph.
type Graph struct {
	// CSR adjacency: edges for node i are Targets[Offsets[i]:Offsets[i+1]].
	Offsets []uint32 // len = NodeCount+1
	Targets []uint32 // len = EdgeCount

	// Edge data (parallel to Targets).
	EdgeDist  []uint16 // distance in decimeters (max 6553.5m)
	EdgeTags1 []uint64 // OSM tags bit-packed (first uint64)
	EdgeTags2 []uint64 // enrichment tags bit-packed (second uint64)

	// Reverse mapping: EdgeOSMIdx[i] indexes into OSMIDPool.
	EdgeOSMIdx []uint32
	OSMIDPool  []int64

	// Node data.
	NodeLat  []int32 // latitude × 1e7 (fixed-point, ~1cm precision)
	NodeLon  []int32 // longitude × 1e7
	NodeEle  []int32 // elevation in decimeters (0.1m, worldwide range)
	NodeTags []uint8 // barrier type enum

	// Spatial index for nearest-node lookups.
	Grid *SpatialGrid

	// Metadata.
	NodeCount uint32
	EdgeCount uint32
	Version   uint32
	BuildTime time.Time
	BBox      [4]float64 // minLat, minLon, maxLat, maxLon
}

// SpatialGrid is a flat grid index for O(1) cell lookup.
// Nodes are grouped by row-major cell index (r*Cols+c).
type SpatialGrid struct {
	CellSize  float64  // in degrees (~0.01 = ~1.1km)
	MinLat    float64
	MinLon    float64
	Cols      int
	Rows      int
	CellStart []uint32 // len = Rows*Cols+1, CSR-style offsets
	NodeOrder []uint32 // cell-grouped node indices
}

// Node barrier type constants.
const (
	BarrierNone           uint8 = 0
	BarrierGate           uint8 = 1
	BarrierBollard        uint8 = 2
	BarrierCycleBarrier   uint8 = 3
	BarrierLiftGate       uint8 = 4
	BarrierStile          uint8 = 5
	BarrierTrafficSignals uint8 = 6
)

// Binary format constants.
const (
	Magic         = "FRGR"
	FormatVersion uint32 = 1
	HeaderSize    int    = 64
	EleChunkSize  uint32 = 65536
)

// Section type constants for the binary section directory.
const (
	SectionOffsets     uint16 = 1
	SectionTargets     uint16 = 2
	SectionEdgeDist    uint16 = 3
	SectionEdgeTags1   uint16 = 4
	SectionEdgeTags2   uint16 = 5
	SectionEdgeOSMIdx  uint16 = 6
	SectionOSMIDPool   uint16 = 7
	SectionNodeLat     uint16 = 8
	SectionNodeLon     uint16 = 9
	SectionNodeTags    uint16 = 10
	SectionNodeEle     uint16 = 11
	SectionSpatialGrid uint16 = 12
	// 13+ reserved for Phase 3b (Reverse-CSR, etc.)
)

// OutDegree returns the number of outgoing edges for node i.
func (g *Graph) OutDegree(i uint32) int {
	return int(g.Offsets[i+1] - g.Offsets[i])
}

// Edges returns the target node indices for outgoing edges of node i.
func (g *Graph) Edges(i uint32) []uint32 {
	return g.Targets[g.Offsets[i]:g.Offsets[i+1]]
}
