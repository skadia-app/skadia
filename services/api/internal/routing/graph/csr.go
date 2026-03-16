package graph

import (
	"fmt"
	"io"
	"log/slog"
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

// BuildCSRFromFile builds a CSR graph by reading edges from a temp file written
// by EdgeFileWriter. Two sequential passes over the file:
//
// Pass 1: Count edges per node (degree) + collect OSM IDs + count total geometry points.
// Pass 2: Place edges in CSR arrays using cursor approach.
func BuildCSRFromFile(nodes []Coord, nodeTags []uint8, edgeTmpPath string) (*Graph, error) {
	nodeCount := uint32(len(nodes))

	g := &Graph{
		NodeCount: nodeCount,
		Version:   FormatVersion,
	}

	// Copy node data
	g.NodeLat = make([]int32, nodeCount)
	g.NodeLon = make([]int32, nodeCount)
	g.NodeEle = make([]int32, nodeCount)
	g.NodeTags = make([]uint8, nodeCount)
	for i, c := range nodes {
		g.NodeLat[i] = int32(math.Round(c.Lat * 1e7))
		g.NodeLon[i] = int32(math.Round(c.Lon * 1e7))
	}
	copy(g.NodeTags, nodeTags)

	// === Read 1: Count edges and degrees ===
	r, err := NewEdgeFileReader(edgeTmpPath)
	if err != nil {
		return nil, err
	}
	degree := make([]uint32, nodeCount)
	osmIDSet := make(map[int64]struct{})
	var edgeCount uint32
	var totalGeomPoints int

	for {
		osmID, _, _, isOneway, segments, rerr := r.ReadWay()
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			r.Close()
			return nil, fmt.Errorf("CSR read 1: %w", rerr)
		}
		osmIDSet[osmID] = struct{}{}
		for _, seg := range segments {
			degree[seg.FromID]++
			edgeCount++
			totalGeomPoints += len(seg.GeomLat)
			if !isOneway {
				degree[seg.ToID]++
				edgeCount++
				totalGeomPoints += len(seg.GeomLat) // reverse gets same point count
			}
		}
	}

	g.EdgeCount = edgeCount
	slog.Info("CSR read 1 complete", "edges", edgeCount, "geom_points", totalGeomPoints)

	// Build offsets
	g.Offsets = make([]uint32, nodeCount+1)
	for i := uint32(0); i < nodeCount; i++ {
		g.Offsets[i+1] = g.Offsets[i] + degree[i]
	}

	// Build OSM ID pool
	osmIDMap := make(map[int64]uint32, len(osmIDSet))
	g.OSMIDPool = make([]int64, 0, len(osmIDSet))
	for id := range osmIDSet {
		osmIDMap[id] = uint32(len(g.OSMIDPool))
		g.OSMIDPool = append(g.OSMIDPool, id)
	}
	osmIDSet = nil

	// Allocate edge arrays
	g.Targets = make([]uint32, edgeCount)
	g.EdgeDist = make([]uint32, edgeCount)
	g.EdgeTags1 = make([]uint64, edgeCount)
	g.EdgeTags2 = make([]uint64, edgeCount)
	g.EdgeOSMIdx = make([]uint32, edgeCount)
	g.EdgeGeomOff = make([]uint32, edgeCount+1)

	// Pre-allocate geometry arrays
	allGeomLat := make([]int32, 0, totalGeomPoints)
	allGeomLon := make([]int32, 0, totalGeomPoints)

	// === Read 2: Place edges in CSR order ===
	if err := r.Reset(); err != nil {
		r.Close()
		return nil, err
	}
	cursor := make([]uint32, nodeCount)
	copy(cursor, g.Offsets[:nodeCount])

	placeEdge := func(from, to, dist uint32, tags1, tags2 uint64, osmIdx uint32, geomLat, geomLon []int32) {
		pos := cursor[from]
		cursor[from]++
		g.Targets[pos] = to
		g.EdgeDist[pos] = dist
		g.EdgeTags1[pos] = tags1
		g.EdgeTags2[pos] = tags2
		g.EdgeOSMIdx[pos] = osmIdx
		g.EdgeGeomOff[pos] = uint32(len(allGeomLat))
		allGeomLat = append(allGeomLat, geomLat...)
		allGeomLon = append(allGeomLon, geomLon...)
	}

	for {
		osmID, tags1, tags2, isOneway, segments, rerr := r.ReadWay()
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			r.Close()
			return nil, fmt.Errorf("CSR read 2: %w", rerr)
		}
		osmIdx := osmIDMap[osmID]
		for _, seg := range segments {
			placeEdge(seg.FromID, seg.ToID, seg.Dist, tags1, tags2, osmIdx, seg.GeomLat, seg.GeomLon)
			if !isOneway {
				// Reverse edge — geometry in reverse order
				n := len(seg.GeomLat)
				revLat := make([]int32, n)
				revLon := make([]int32, n)
				for j := 0; j < n; j++ {
					revLat[j] = seg.GeomLat[n-1-j]
					revLon[j] = seg.GeomLon[n-1-j]
				}
				placeEdge(seg.ToID, seg.FromID, seg.Dist, tags1, tags2, osmIdx, revLat, revLon)
			}
		}
	}
	r.Close()

	g.EdgeGeomOff[edgeCount] = uint32(len(allGeomLat))
	g.EdgeGeomLat = allGeomLat
	g.EdgeGeomLon = allGeomLon

	g.EdgeElevOff = make([]uint32, edgeCount+1)
	g.EdgeElevProf = nil

	g.BBox = computeBBox(nodes)
	slog.Info("CSR build complete", "nodes", nodeCount, "edges", edgeCount, "geom_points", len(allGeomLat))
	return g, nil
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
