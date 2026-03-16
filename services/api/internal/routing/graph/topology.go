package graph

import "math"

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
	NodeIndex map[CoordKey]uint32
	NodeTags  []uint8
	NodeCount int
	// Edge geometry: intermediate coordinates for each edge.
	// EdgeGeom[EdgeGeomOffset[i]:EdgeGeomOffset[i+1]] gives the geometry for edge i.
	EdgeGeomOffset []uint32
	EdgeGeomCoords []Coord
}

// VertexDegree tracks how many ways reference each coordinate.
type VertexDegree struct {
	counts    map[CoordKey]uint8
	endpoints map[CoordKey]bool
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

// IsNetworkNode returns true if the coordinate should be a graph node.
func (vd *VertexDegree) IsNetworkNode(key CoordKey) bool {
	if vd.endpoints[key] {
		return true
	}
	return vd.counts[key] >= 2
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
