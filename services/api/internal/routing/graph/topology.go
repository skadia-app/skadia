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
