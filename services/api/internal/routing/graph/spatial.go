package graph

import (
	"math"
	"sort"
)

// BuildSpatialIndex creates a Morton-sorted flat grid index.
func BuildSpatialIndex(g *Graph, cellSize float64) *SpatialGrid {
	if g.NodeCount == 0 {
		return &SpatialGrid{CellSize: cellSize, CellStart: []uint32{0}, NodeOrder: nil}
	}

	minLat := float64(g.NodeLat[0]) / 1e7
	maxLat := minLat
	minLon := float64(g.NodeLon[0]) / 1e7
	maxLon := minLon
	for i := uint32(1); i < g.NodeCount; i++ {
		lat := float64(g.NodeLat[i]) / 1e7
		lon := float64(g.NodeLon[i]) / 1e7
		if lat < minLat {
			minLat = lat
		}
		if lat > maxLat {
			maxLat = lat
		}
		if lon < minLon {
			minLon = lon
		}
		if lon > maxLon {
			maxLon = lon
		}
	}

	minLat -= cellSize
	minLon -= cellSize
	rows := int(math.Ceil((maxLat-minLat)/cellSize)) + 1
	cols := int(math.Ceil((maxLon-minLon)/cellSize)) + 1

	grid := &SpatialGrid{
		CellSize: cellSize,
		MinLat:   minLat,
		MinLon:   minLon,
		Rows:     rows,
		Cols:     cols,
	}

	type nodeSort struct {
		idx    uint32
		morton uint64
	}
	sorted := make([]nodeSort, g.NodeCount)
	for i := uint32(0); i < g.NodeCount; i++ {
		lat := float64(g.NodeLat[i]) / 1e7
		lon := float64(g.NodeLon[i]) / 1e7
		row := uint32((lat - minLat) / cellSize)
		col := uint32((lon - minLon) / cellSize)
		sorted[i] = nodeSort{idx: i, morton: mortonCode(row, col)}
	}

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].morton < sorted[j].morton
	})

	grid.NodeOrder = make([]uint32, g.NodeCount)
	for i, s := range sorted {
		grid.NodeOrder[i] = s.idx
	}

	totalCells := rows * cols
	grid.CellStart = make([]uint32, totalCells+1)

	cellIdx := func(nodeI uint32) int {
		lat := float64(g.NodeLat[nodeI]) / 1e7
		lon := float64(g.NodeLon[nodeI]) / 1e7
		r := int((lat - minLat) / cellSize)
		c := int((lon - minLon) / cellSize)
		return r*cols + c
	}

	cellCount := make([]int, totalCells)
	for _, nodeI := range grid.NodeOrder {
		ci := cellIdx(nodeI)
		if ci >= 0 && ci < totalCells {
			cellCount[ci]++
		}
	}

	for i := 0; i < totalCells; i++ {
		grid.CellStart[i+1] = grid.CellStart[i] + uint32(cellCount[i])
	}

	sort.Slice(grid.NodeOrder, func(i, j int) bool {
		return cellIdx(grid.NodeOrder[i]) < cellIdx(grid.NodeOrder[j])
	})

	return grid
}

// NearestNode finds the closest node to the given lat/lon.
func NearestNode(g *Graph, lat, lon float64) (uint32, uint16) {
	grid := g.Grid
	if grid == nil || g.NodeCount == 0 {
		return 0, 65535
	}

	centerRow := int((lat - grid.MinLat) / grid.CellSize)
	centerCol := int((lon - grid.MinLon) / grid.CellSize)

	bestNode := uint32(0)
	var bestDist uint16 = 65535

	for dr := -1; dr <= 1; dr++ {
		for dc := -1; dc <= 1; dc++ {
			r := centerRow + dr
			c := centerCol + dc
			if r < 0 || r >= grid.Rows || c < 0 || c >= grid.Cols {
				continue
			}
			ci := r*grid.Cols + c
			start := grid.CellStart[ci]
			end := grid.CellStart[ci+1]
			for idx := start; idx < end; idx++ {
				nodeI := grid.NodeOrder[idx]
				nodeLat := float64(g.NodeLat[nodeI]) / 1e7
				nodeLon := float64(g.NodeLon[nodeI]) / 1e7
				d := equirectangularDistDm(lat, lon, nodeLat, nodeLon)
				if d < bestDist {
					bestDist = d
					bestNode = nodeI
				}
			}
		}
	}

	return bestNode, bestDist
}

func mortonCode(row, col uint32) uint64 {
	return interleave(uint64(row)) | (interleave(uint64(col)) << 1)
}

func mortonDecode(code uint64) (row, col uint32) {
	return uint32(deinterleave(code)), uint32(deinterleave(code >> 1))
}

func interleave(x uint64) uint64 {
	x &= 0x00000000FFFFFFFF
	x = (x | (x << 16)) & 0x0000FFFF0000FFFF
	x = (x | (x << 8)) & 0x00FF00FF00FF00FF
	x = (x | (x << 4)) & 0x0F0F0F0F0F0F0F0F
	x = (x | (x << 2)) & 0x3333333333333333
	x = (x | (x << 1)) & 0x5555555555555555
	return x
}

func deinterleave(x uint64) uint64 {
	x &= 0x5555555555555555
	x = (x | (x >> 1)) & 0x3333333333333333
	x = (x | (x >> 2)) & 0x0F0F0F0F0F0F0F0F
	x = (x | (x >> 4)) & 0x00FF00FF00FF00FF
	x = (x | (x >> 8)) & 0x0000FFFF0000FFFF
	x = (x | (x >> 16)) & 0x00000000FFFFFFFF
	return x
}
