package graph

import (
	"fmt"
	"io"
	"log/slog"
	"math"
	"runtime"
	"runtime/debug"
	"slices"
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

	// === Read 1: Count edges/degrees + collect OSM IDs ===
	r, err := NewEdgeFileReader(edgeTmpPath)
	if err != nil {
		return nil, err
	}
	degree := make([]uint32, nodeCount)
	var osmIDs []int64 // sorted+deduped after Read 1 → becomes OSMIDPool directly
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
		osmIDs = append(osmIDs, osmID)
		for _, seg := range segments {
			degree[seg.FromID]++
			edgeCount++
			totalGeomPoints += len(seg.GeomLat)
			if !isOneway {
				degree[seg.ToID]++
				edgeCount++
				totalGeomPoints += len(seg.GeomLat)
			}
		}
	}

	g.EdgeCount = edgeCount

	// Build offsets from degree counts, then free degree array
	g.Offsets = make([]uint32, nodeCount+1)
	for i := uint32(0); i < nodeCount; i++ {
		g.Offsets[i+1] = g.Offsets[i] + degree[i]
	}
	degree = nil

	// Build OSM ID pool as sorted slice (142 MB) instead of map (1.8 GB).
	// Binary search during Read 2 for pool index.
	slices.Sort(osmIDs)
	osmIDs = slices.Compact(osmIDs)
	g.OSMIDPool = osmIDs
	slog.Info("CSR read 1 complete", "edges", edgeCount, "geom_points", totalGeomPoints,
		"unique_osm_ids", len(osmIDs))

	// Force GC to free Read 1 temporaries before allocating CSR arrays
	runtime.GC()
	debug.FreeOSMemory()

	// Allocate edge arrays
	g.Targets = make([]uint32, edgeCount)
	g.EdgeDist = make([]uint32, edgeCount)
	g.EdgeTags1 = make([]uint64, edgeCount)
	g.EdgeTags2 = make([]uint64, edgeCount)
	g.EdgeOSMIdx = make([]uint32, edgeCount)
	g.EdgeGeomOff = make([]uint32, edgeCount+1)

	// Pre-allocate geometry arrays (known size from Read 1 — no growth)
	allGeomLat := make([]int32, 0, totalGeomPoints)
	allGeomLon := make([]int32, 0, totalGeomPoints)

	// Track geometry spans per CSR edge: edges are placed in CSR order (by source
	// node) but geometry is appended in file order (by way). We record each edge's
	// geometry span in allGeomLat, then reorder to CSR order after Read 2.
	type geomSpan struct {
		start uint32
		size  uint32
	}
	edgeGeomSpans := make([]geomSpan, edgeCount)

	// === Read 2: Place edges in CSR order ===
	if err := r.Reset(); err != nil {
		r.Close()
		return nil, err
	}
	cursor := make([]uint32, nodeCount)
	copy(cursor, g.Offsets[:nodeCount])

	placeEdge := func(from, to, dist uint32, tags1, tags2 uint64, osmIdx uint32, geomLat, geomLon []int32, reversed bool) {
		pos := cursor[from]
		cursor[from]++
		g.Targets[pos] = to
		g.EdgeDist[pos] = dist
		g.EdgeTags1[pos] = tags1
		g.EdgeTags2[pos] = tags2
		g.EdgeOSMIdx[pos] = osmIdx
		startIdx := uint32(len(allGeomLat))
		if reversed {
			for j := len(geomLat) - 1; j >= 0; j-- {
				allGeomLat = append(allGeomLat, geomLat[j])
				allGeomLon = append(allGeomLon, geomLon[j])
			}
		} else {
			allGeomLat = append(allGeomLat, geomLat...)
			allGeomLon = append(allGeomLon, geomLon...)
		}
		edgeGeomSpans[pos] = geomSpan{start: startIdx, size: uint32(len(allGeomLat)) - startIdx}
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
		// Binary search in sorted pool — O(log n), no map needed
		osmIdx, _ := slices.BinarySearch(g.OSMIDPool, osmID)
		for _, seg := range segments {
			placeEdge(seg.FromID, seg.ToID, seg.Dist, tags1, tags2, uint32(osmIdx), seg.GeomLat, seg.GeomLon, false)
			if !isOneway {
				placeEdge(seg.ToID, seg.FromID, seg.Dist, tags1, tags2, uint32(osmIdx), seg.GeomLat, seg.GeomLon, true)
			}
		}
	}
	r.Close()

	// Reorder geometry from file order to CSR order so that EdgeGeomOff is
	// monotonically increasing (required by all consumers: A*, retrack, serialize).
	sortedGeomLat := make([]int32, 0, totalGeomPoints)
	sortedGeomLon := make([]int32, 0, totalGeomPoints)
	for ei := uint32(0); ei < edgeCount; ei++ {
		span := edgeGeomSpans[ei]
		g.EdgeGeomOff[ei] = uint32(len(sortedGeomLat))
		sortedGeomLat = append(sortedGeomLat, allGeomLat[span.start:span.start+span.size]...)
		sortedGeomLon = append(sortedGeomLon, allGeomLon[span.start:span.start+span.size]...)
	}
	g.EdgeGeomOff[edgeCount] = uint32(len(sortedGeomLat))
	g.EdgeGeomLat = sortedGeomLat
	g.EdgeGeomLon = sortedGeomLon

	// Free unsorted geometry arrays.
	allGeomLat = nil
	allGeomLon = nil
	edgeGeomSpans = nil

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
