package graph

import (
	"math"
	"slices"
)

// Coord is a lat/lon pair as float64.
type Coord struct {
	Lat float64
	Lon float64
}

// CoordKey is a deterministic map key for topology detection.
type CoordKey struct {
	Lat int32
	Lon int32
}

// NewCoordKey creates a CoordKey from float64 coordinates.
func NewCoordKey(lat, lon float64) CoordKey {
	return CoordKey{
		Lat: int32(math.Round(lat * 1e7)),
		Lon: int32(math.Round(lon * 1e7)),
	}
}

// RawWay represents a way loaded from PostGIS before topology processing.
type RawWay struct {
	OSMID         int64
	Coords        []Coord
	Tags          RawTags
	Enrichment    RawEnrichment
	ReverseOneway bool
	BarrierNodes  map[int]uint8
}

// TopologyEdge is a directed edge produced by topology building.
type TopologyEdge struct {
	From       uint32
	To         uint32
	OSMID      int64
	Tags       RawTags
	Enrichment RawEnrichment
}

// TopologyResult holds the output of BuildTopology.
type TopologyResult struct {
	Nodes     []Coord
	Edges     []TopologyEdge
	NodeIndex map[CoordKey]uint32
	NodeTags  []uint8
	NodeCount int
}

// CompactEdge is a memory-efficient directed edge with pre-encoded tags.
// 36 bytes vs ~300 bytes for TopologyEdge (no string copies).
type CompactEdge struct {
	From  uint32
	To    uint32
	OSMID int64
	Tags1 uint64 // EncodeOSMTags result
	Tags2 uint64 // EncodeEdgeTags2 result
	Dist  uint32 // distance in centimeters
}

// CompactTopologyResult holds topology with pre-encoded tags.
type CompactTopologyResult struct {
	Nodes     []Coord
	Edges     []CompactEdge
	NodeTags  []uint8
	NodeCount int
	// Edge geometry as int32 (lat/lon × 1e7) — half the memory of []Coord.
	// Geometry for edge i: EdgeGeomLat[EdgeGeomOffset[i]:EdgeGeomOffset[i+1]]
	EdgeGeomOffset []uint32
	EdgeGeomLat    []int32
	EdgeGeomLon    []int32
}

// VertexDegree tracks how many ways reference each coordinate.
// After AddWay phase, call Compact() to free ~95% of memory before Pass 2.
type VertexDegree struct {
	// Phase 1 (building): populated by AddWay, freed by Compact()
	counts    map[CoordKey]uint8
	endpoints map[CoordKey]bool
	// Phase 2 (querying): sorted slice for binary search (~200 MB vs ~1.4 GB for map)
	sortedNodes []CoordKey
}

// NewVertexDegree creates a new VertexDegree tracker.
func NewVertexDegree() *VertexDegree {
	return &VertexDegree{
		counts:    make(map[CoordKey]uint8, 1_000_000),
		endpoints: make(map[CoordKey]bool, 500_000),
	}
}

// AddWay processes all vertices of a way: increments degree for interior vertices
// that appear in multiple ways, marks first/last as endpoints.
func (vd *VertexDegree) AddWay(coords []Coord) {
	if len(coords) < 2 {
		return
	}
	vd.endpoints[NewCoordKey(coords[0].Lat, coords[0].Lon)] = true
	vd.endpoints[NewCoordKey(coords[len(coords)-1].Lat, coords[len(coords)-1].Lon)] = true
	for i := 1; i < len(coords)-1; i++ {
		key := NewCoordKey(coords[i].Lat, coords[i].Lon)
		if vd.counts[key] < 255 {
			vd.counts[key]++
		}
	}
}

// Compact collects all network node keys into a sorted slice for O(log n)
// binary-search lookups, then frees the large building-phase maps.
// Memory: sorted []CoordKey = 25M × 8 bytes = ~200 MB
// vs map[CoordKey]struct{} = 25M × ~56 bytes = ~1.4 GB
// Must be called between Pass 1 and Pass 2.
func (vd *VertexDegree) Compact() {
	// Collect all network node keys
	vd.sortedNodes = make([]CoordKey, 0, len(vd.endpoints)+len(vd.counts)/10)
	for key := range vd.endpoints {
		vd.sortedNodes = append(vd.sortedNodes, key)
	}
	for key, count := range vd.counts {
		if count >= 2 {
			vd.sortedNodes = append(vd.sortedNodes, key)
		}
	}
	// Free building-phase maps BEFORE sorting (so GC can reclaim during sort)
	vd.counts = nil
	vd.endpoints = nil
	// Sort for binary search
	sortCoordKeys(vd.sortedNodes)
}

// NetworkNodeCount returns the number of network nodes after Compact().
func (vd *VertexDegree) NetworkNodeCount() int {
	return len(vd.sortedNodes)
}

// IsNetworkNode returns true if the coordinate should be a graph node.
// Before Compact(): checks endpoints map + counts >= 2.
// After Compact(): binary search in sorted slice.
func (vd *VertexDegree) IsNetworkNode(key CoordKey) bool {
	if vd.sortedNodes != nil {
		return searchCoordKey(vd.sortedNodes, key)
	}
	if vd.endpoints[key] {
		return true
	}
	return vd.counts[key] >= 2
}

// sortCoordKeys sorts by Lat then Lon.
func sortCoordKeys(s []CoordKey) {
	slices.SortFunc(s, compareCoordKey)
}

func compareCoordKey(a, b CoordKey) int {
	if a.Lat != b.Lat {
		if a.Lat < b.Lat {
			return -1
		}
		return 1
	}
	if a.Lon != b.Lon {
		if a.Lon < b.Lon {
			return -1
		}
		return 1
	}
	return 0
}

// SearchNodeID returns the node ID for a network node key, or (0, false).
// The node ID is the position in the sorted slice (assigned during Compact).
func (vd *VertexDegree) SearchNodeID(key CoordKey) (uint32, bool) {
	lo, hi := 0, len(vd.sortedNodes)
	for lo < hi {
		mid := lo + (hi-lo)/2
		cmp := compareCoordKey(vd.sortedNodes[mid], key)
		if cmp == 0 {
			return uint32(mid), true
		}
		if cmp < 0 {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return 0, false
}

// searchCoordKey performs binary search in a sorted []CoordKey.
func searchCoordKey(s []CoordKey, key CoordKey) bool {
	lo, hi := 0, len(s)
	for lo < hi {
		mid := lo + (hi-lo)/2
		cmp := compareCoordKey(s[mid], key)
		if cmp == 0 {
			return true
		}
		if cmp < 0 {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return false
}

// BuildTopology computes the graph topology from raw ways.
func BuildTopology(ways []RawWay) TopologyResult {
	nodeIndex := make(map[CoordKey]uint32, len(ways)*10)
	var nodes []Coord
	var nodeTags []uint8

	assignNode := func(c Coord) uint32 {
		key := NewCoordKey(c.Lat, c.Lon)
		if id, ok := nodeIndex[key]; ok {
			return id
		}
		id := uint32(len(nodes))
		nodeIndex[key] = id
		nodes = append(nodes, c)
		nodeTags = append(nodeTags, BarrierNone)
		return id
	}

	for i := range ways {
		for j, c := range ways[i].Coords {
			id := assignNode(c)
			if ways[i].BarrierNodes != nil {
				if bt, ok := ways[i].BarrierNodes[j]; ok && bt != BarrierNone {
					nodeTags[id] = bt
				}
			}
		}
	}

	edges := make([]TopologyEdge, 0, len(ways)*4)

	for i := range ways {
		w := &ways[i]
		if len(w.Coords) < 2 {
			continue
		}

		isOneway := w.Tags.Oneway
		isReverse := w.ReverseOneway

		for j := 0; j < len(w.Coords)-1; j++ {
			fromKey := NewCoordKey(w.Coords[j].Lat, w.Coords[j].Lon)
			toKey := NewCoordKey(w.Coords[j+1].Lat, w.Coords[j+1].Lon)
			fromID := nodeIndex[fromKey]
			toID := nodeIndex[toKey]

			if fromID == toID {
				continue
			}

			if isOneway {
				if isReverse {
					edges = append(edges, TopologyEdge{
						From: toID, To: fromID, OSMID: w.OSMID,
						Tags: w.Tags, Enrichment: w.Enrichment,
					})
				} else {
					edges = append(edges, TopologyEdge{
						From: fromID, To: toID, OSMID: w.OSMID,
						Tags: w.Tags, Enrichment: w.Enrichment,
					})
				}
			} else {
				edges = append(edges, TopologyEdge{
					From: fromID, To: toID, OSMID: w.OSMID,
					Tags: w.Tags, Enrichment: w.Enrichment,
				})
				edges = append(edges, TopologyEdge{
					From: toID, To: fromID, OSMID: w.OSMID,
					Tags: w.Tags, Enrichment: w.Enrichment,
				})
			}
		}
	}

	return TopologyResult{
		Nodes:     nodes,
		Edges:     edges,
		NodeIndex: nodeIndex,
		NodeTags:  nodeTags,
		NodeCount: len(nodes),
	}
}
