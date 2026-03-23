// Package graph implements an in-memory CSR routing graph for BadRouter.
package graph

import (
	"os"
	"syscall"
	"time"
)

// Graph is the immutable in-memory CSR routing graph.
type Graph struct {
	// CSR adjacency: edges for node i are Targets[Offsets[i]:Offsets[i+1]].
	Offsets []uint32 // len = NodeCount+1
	Targets []uint32 // len = EdgeCount

	// Edge data (parallel to Targets).
	EdgeDist      []uint32 // distance in centimeters (was uint16 decimeters)
	EdgeTags1     []uint64 // OSM tags bit-packed (first uint64)
	EdgeTags2     []uint64 // enrichment tags bit-packed (second uint64)
	EdgeOSMIdx    []uint32
	OSMIDPool     []int64

	// Edge geometry: intermediate coordinates (transfer nodes).
	// Geometry for edge i: EdgeGeomLat[EdgeGeomOff[i]:EdgeGeomOff[i+1]]
	EdgeGeomOff []uint32 // len = EdgeCount + 1
	EdgeGeomLat []int32  // latitude × 1e7
	EdgeGeomLon []int32  // longitude × 1e7

	// Edge elevation profile: DEM elevation at each geometry point.
	// Profile for edge i: EdgeElevProf[EdgeElevOff[i]:EdgeElevOff[i+1]]
	EdgeElevOff     []uint32 // len = EdgeCount + 1
	EdgeElevProf    []int32  // elevation in decimeters (int32: supports >3276.7m)
	EdgeElevSegDist []uint16 // segment distances in cm between ElevProf points (N-1 per edge)

	// Node data (Morton-sorted).
	NodeLat  []int32 // latitude × 1e7 (fixed-point, ~1cm precision)
	NodeLon  []int32 // longitude × 1e7
	NodeEle  []int32 // elevation in decimeters (0.1m, worldwide range)
	NodeTags []uint8 // barrier type enum

	// Spatial index for nearest-node lookups.
	Grid *SpatialGrid

	// Tile border data (tiled graph engine).
	BorderIndex *BorderNodeIndex
	IsBorder    []bool

	// Metadata.
	TileID    uint32
	NodeCount uint32
	EdgeCount uint32
	Version   uint32
	BuildTime time.Time
	BBox      [4]float64 // minLat, minLon, maxLat, maxLon

	// Landmark metadata (from Section 26, loaded eagerly — only 276 bytes)
	LandmarkMeta *LandmarkMetaData

	// Pending landmark data for serialization (not kept after Serialize)
	pendingLandmarkSections map[uint16][]float32

	// Reverse adjacency (built at load time, not serialized).
	// RevOffsets[i] = start index in RevTargets for incoming edges of node i.
	// RevTargets[j] = source node of the j-th reverse edge.
	// RevEdgeIdx[j] = original edge index in forward CSR (for Tags/Dist lookup).
	RevOffsets []uint32
	RevTargets []uint32
	RevEdgeIdx []uint32

	// Lazy I/O for cold sections. The backing file is kept open;
	// cold section data (geometry, elevation profiles, OSM IDs) is read
	// on demand via ReadAt. No mmap, no page faults, no RSS inflation.
	backingFile  *os.File                 // graph.bin, kept open (nil for heap-only)
	coldSections map[uint16]sectionEntry  // section type → file offset+length

	// mmap resources (set by DeserializeMmap, nil for heap-only or ReadAt graphs).
	mmapData []byte   // mmap'd file region; released by closeMmap finalizer
	mmapFile *os.File // underlying file; closed after Munmap
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
	FormatVersion uint32 = 4 // v4: separated EdgeGeomOff/EdgeElevOff after DP + SectionEdgeElevSegDist for hysteresis
	HeaderSize    int    = 64
	EleChunkSize  uint32 = 65536
)

// Section type constants for the binary section directory.
const (
	SectionOffsets     uint16 = 1
	SectionTargets     uint16 = 2
	SectionEdgeDist    uint16 = 3  // now uint32 centimeters (was uint16 decimeters)
	SectionEdgeTags1   uint16 = 4
	SectionEdgeTags2   uint16 = 5
	SectionEdgeOSMIdx  uint16 = 6
	SectionOSMIDPool   uint16 = 7
	SectionNodeLat     uint16 = 8
	SectionNodeLon     uint16 = 9
	SectionNodeTags    uint16 = 10
	SectionNodeEle     uint16 = 11
	SectionSpatialGrid uint16 = 12
	// v2 sections
	SectionEdgeGeomOff  uint16 = 13
	SectionEdgeGeomLat  uint16 = 14
	SectionEdgeGeomLon  uint16 = 15
	SectionEdgeElevOff  uint16 = 16
	SectionEdgeElevProf uint16 = 17
	// v3 sections (tiled graph engine)
	SectionBorderNodeIndex uint16 = 18
	SectionTileMeta        uint16 = 19
	SectionIsBorder        uint16 = 20
	SectionEdgeElevSegDist uint16 = 21 // []uint16 cm, segment distances for hysteresis

	// Landmark ALT-Heuristik Sections (optional, FormatVersion bleibt 4)
	SectionLandmarkDistA uint16 = 22
	SectionLandmarkDistB uint16 = 23
	SectionLandmarkDistC uint16 = 24
	SectionLandmarkDistD uint16 = 25
	SectionLandmarkMeta  uint16 = 26
)

// LandmarkSetID identifies one of the 4 landmark cost-config sets.
type LandmarkSetID uint8

const (
	LandmarkSetA    LandmarkSetID = 0 // uphillcost=7, downhillcost=7
	LandmarkSetB    LandmarkSetID = 1 // uphillcost=7, downhillcost=0
	LandmarkSetC    LandmarkSetID = 2 // uphillcost=0, downhillcost=7
	LandmarkSetD    LandmarkSetID = 3 // uphillcost=0, downhillcost=0
	NumLandmarkSets               = 4
	LandmarksPerSet               = 16
)

// OutDegree returns the number of outgoing edges for node i.
func (g *Graph) OutDegree(i uint32) int {
	return int(g.Offsets[i+1] - g.Offsets[i])
}

// Edges returns the target node indices for outgoing edges of node i.
func (g *Graph) Edges(i uint32) []uint32 {
	return g.Targets[g.Offsets[i]:g.Offsets[i+1]]
}

// LandmarkMetaData stores the 16 global landmark positions + thresholds.
type LandmarkMetaData struct {
	NodeIDs           [16]uint64
	Lats              [16]int32 // ×1e7
	Lons              [16]int32 // ×1e7
	UphillThreshold   float32   // 7.0
	DownhillThreshold float32   // 7.0
	LandmarkCount     int
	PreprocessingTime time.Time
}

// closeMmap releases mmap resources. Called by GC finalizer, not directly.
func (g *Graph) closeMmap() {
	if g.mmapData != nil {
		syscall.Munmap(g.mmapData)
		g.mmapData = nil
	}
	mf := g.mmapFile
	bf := g.backingFile
	g.mmapFile = nil
	g.backingFile = nil
	if mf != nil {
		mf.Close()
	}
	// backingFile may alias mmapFile (set by DeserializeMmap) or be independent.
	// Only close if distinct to avoid double-close.
	if bf != nil && bf != mf {
		bf.Close()
	}
}
