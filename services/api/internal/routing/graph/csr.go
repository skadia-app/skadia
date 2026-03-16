package graph

import (
	"fmt"
	"math"
)

// BuildCSR converts a TopologyResult into a CSR graph.
// Panics if any edge references an out-of-range node index.
func BuildCSR(topo TopologyResult) *Graph {
	nodeCount := uint32(topo.NodeCount)
	edgeCount := uint32(len(topo.Edges))

	// Validate edge indices before allocating.
	for i, e := range topo.Edges {
		if e.From >= nodeCount || e.To >= nodeCount {
			panic(fmt.Sprintf("BuildCSR: edge %d has out-of-range node index (from=%d, to=%d, nodeCount=%d)",
				i, e.From, e.To, nodeCount))
		}
	}

	g := &Graph{
		NodeCount: nodeCount,
		EdgeCount: edgeCount,
		Version:   FormatVersion,
	}

	g.NodeLat = make([]int32, nodeCount)
	g.NodeLon = make([]int32, nodeCount)
	g.NodeEle = make([]int32, nodeCount)
	g.NodeTags = make([]uint8, nodeCount)

	for i, c := range topo.Nodes {
		g.NodeLat[i] = int32(math.Round(c.Lat * 1e7))
		g.NodeLon[i] = int32(math.Round(c.Lon * 1e7))
	}
	copy(g.NodeTags, topo.NodeTags)

	degree := make([]uint32, nodeCount)
	for _, e := range topo.Edges {
		degree[e.From]++
	}

	g.Offsets = make([]uint32, nodeCount+1)
	for i := range nodeCount {
		g.Offsets[i+1] = g.Offsets[i] + degree[i]
	}

	g.Targets = make([]uint32, edgeCount)
	g.EdgeDist = make([]uint32, edgeCount)
	g.EdgeTags1 = make([]uint64, edgeCount)
	g.EdgeTags2 = make([]uint64, edgeCount)
	g.EdgeOSMIdx = make([]uint32, edgeCount)

	osmIDMap := make(map[int64]uint32)
	for _, e := range topo.Edges {
		if _, ok := osmIDMap[e.OSMID]; !ok {
			osmIDMap[e.OSMID] = uint32(len(osmIDMap))
		}
	}
	g.OSMIDPool = make([]int64, len(osmIDMap))
	for id, idx := range osmIDMap {
		g.OSMIDPool[idx] = id
	}

	cursor := make([]uint32, nodeCount)
	copy(cursor, g.Offsets[:nodeCount])

	for _, e := range topo.Edges {
		pos := cursor[e.From]
		cursor[e.From]++

		g.Targets[pos] = e.To
		g.EdgeTags1[pos] = EncodeOSMTags(&e.Tags)
		g.EdgeTags2[pos] = EncodeEdgeTags2(&e.Tags, &e.Enrichment)
		g.EdgeOSMIdx[pos] = osmIDMap[e.OSMID]

		fromLat := topo.Nodes[e.From].Lat
		fromLon := topo.Nodes[e.From].Lon
		toLat := topo.Nodes[e.To].Lat
		toLon := topo.Nodes[e.To].Lon
		g.EdgeDist[pos] = uint32(equirectangularDistDm(fromLat, fromLon, toLat, toLon))
	}

	g.EdgeGeomOff = make([]uint32, edgeCount+1)
	g.EdgeGeomLat = nil
	g.EdgeGeomLon = nil
	g.EdgeElevOff = make([]uint32, edgeCount+1)
	g.EdgeElevProf = nil

	g.BBox = computeBBox(topo.Nodes)

	return g
}

// BuildCSRCompact converts a CompactTopologyResult into a CSR graph.
// CompactEdge already has pre-encoded Tags1/Tags2 and pre-computed Dist in centimeters.
// Panics if any edge references an out-of-range node index.
func BuildCSRCompact(topo *CompactTopologyResult) *Graph {
	nodeCount := uint32(topo.NodeCount)
	edgeCount := uint32(len(topo.Edges))

	for i, e := range topo.Edges {
		if e.From >= nodeCount || e.To >= nodeCount {
			panic(fmt.Sprintf("BuildCSRCompact: edge %d out-of-range (from=%d, to=%d, nodeCount=%d)",
				i, e.From, e.To, nodeCount))
		}
	}

	g := &Graph{
		NodeCount: nodeCount,
		EdgeCount: edgeCount,
		Version:   FormatVersion,
	}

	g.NodeLat = make([]int32, nodeCount)
	g.NodeLon = make([]int32, nodeCount)
	g.NodeEle = make([]int32, nodeCount)
	g.NodeTags = make([]uint8, nodeCount)
	for i, c := range topo.Nodes {
		g.NodeLat[i] = int32(math.Round(c.Lat * 1e7))
		g.NodeLon[i] = int32(math.Round(c.Lon * 1e7))
	}
	copy(g.NodeTags, topo.NodeTags)

	degree := make([]uint32, nodeCount)
	for _, e := range topo.Edges {
		degree[e.From]++
	}

	g.Offsets = make([]uint32, nodeCount+1)
	for i := uint32(0); i < nodeCount; i++ {
		g.Offsets[i+1] = g.Offsets[i] + degree[i]
	}

	g.Targets = make([]uint32, edgeCount)
	g.EdgeDist = make([]uint32, edgeCount)
	g.EdgeTags1 = make([]uint64, edgeCount)
	g.EdgeTags2 = make([]uint64, edgeCount)
	g.EdgeOSMIdx = make([]uint32, edgeCount)
	g.EdgeGeomOff = make([]uint32, edgeCount+1)

	osmIDMap := make(map[int64]uint32)
	for _, e := range topo.Edges {
		if _, ok := osmIDMap[e.OSMID]; !ok {
			osmIDMap[e.OSMID] = uint32(len(osmIDMap))
		}
	}
	g.OSMIDPool = make([]int64, len(osmIDMap))
	for id, idx := range osmIDMap {
		g.OSMIDPool[idx] = id
	}

	cursor := make([]uint32, nodeCount)
	copy(cursor, g.Offsets[:nodeCount])

	type geomEntry struct {
		coords []Coord
	}
	geomByPos := make([]geomEntry, edgeCount)

	for i, e := range topo.Edges {
		pos := cursor[e.From]
		cursor[e.From]++

		g.Targets[pos] = e.To
		g.EdgeDist[pos] = e.Dist
		g.EdgeTags1[pos] = e.Tags1
		g.EdgeTags2[pos] = e.Tags2
		g.EdgeOSMIdx[pos] = osmIDMap[e.OSMID]

		geomStart := topo.EdgeGeomOffset[i]
		geomEnd := topo.EdgeGeomOffset[i+1]
		if geomEnd > geomStart {
			geomByPos[pos].coords = topo.EdgeGeomCoords[geomStart:geomEnd]
		}
	}

	var allGeomLat, allGeomLon []int32
	for i := uint32(0); i < edgeCount; i++ {
		g.EdgeGeomOff[i] = uint32(len(allGeomLat))
		for _, c := range geomByPos[i].coords {
			allGeomLat = append(allGeomLat, int32(math.Round(c.Lat*1e7)))
			allGeomLon = append(allGeomLon, int32(math.Round(c.Lon*1e7)))
		}
	}
	g.EdgeGeomOff[edgeCount] = uint32(len(allGeomLat))
	g.EdgeGeomLat = allGeomLat
	g.EdgeGeomLon = allGeomLon

	g.EdgeElevOff = make([]uint32, edgeCount+1)
	g.EdgeElevProf = nil

	g.BBox = computeBBox(topo.Nodes)
	return g
}

// equirectangularDistDm calculates flat-earth distance in decimeters.
func equirectangularDistDm(lat1, lon1, lat2, lon2 float64) uint16 {
	const earthRadius = 6371000.0
	const degToRad = math.Pi / 180.0

	dLat := (lat2 - lat1) * degToRad
	dLon := (lon2 - lon1) * degToRad
	meanLat := (lat1 + lat2) / 2.0 * degToRad
	x := dLon * math.Cos(meanLat)
	dist := math.Sqrt(dLat*dLat+x*x) * earthRadius
	dm := min(int(math.Round(dist*10)), 65535)
	if dm < 0 {
		dm = 0
	}
	return uint16(dm)
}

func computeBBox(nodes []Coord) [4]float64 {
	if len(nodes) == 0 {
		return [4]float64{}
	}
	minLat, minLon := nodes[0].Lat, nodes[0].Lon
	maxLat, maxLon := nodes[0].Lat, nodes[0].Lon
	for _, n := range nodes[1:] {
		if n.Lat < minLat {
			minLat = n.Lat
		}
		if n.Lat > maxLat {
			maxLat = n.Lat
		}
		if n.Lon < minLon {
			minLon = n.Lon
		}
		if n.Lon > maxLon {
			maxLon = n.Lon
		}
	}
	return [4]float64{minLat, minLon, maxLat, maxLon}
}
