package graph

import (
	"math"

	"github.com/skadia-app/skadia/services/api/internal/geo"
)

// BuildSpatialIndex creates a flat grid index for O(1) nearest-node lookups.
// Nodes are grouped by row-major cell index with CSR-style CellStart offsets.
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

	totalCells := rows * cols

	cellIdx := func(nodeI uint32) int {
		lat := float64(g.NodeLat[nodeI]) / 1e7
		lon := float64(g.NodeLon[nodeI]) / 1e7
		r := int((lat - minLat) / cellSize)
		c := int((lon - minLon) / cellSize)
		return r*cols + c
	}

	// Count nodes per cell, then prefix-sum for CellStart.
	cellCount := make([]int, totalCells)
	for i := uint32(0); i < g.NodeCount; i++ {
		ci := cellIdx(i)
		if ci >= 0 && ci < totalCells {
			cellCount[ci]++
		}
	}

	grid.CellStart = make([]uint32, totalCells+1)
	for i := range totalCells {
		grid.CellStart[i+1] = grid.CellStart[i] + uint32(cellCount[i])
	}

	// Place nodes into NodeOrder at correct cell positions.
	grid.NodeOrder = make([]uint32, g.NodeCount)
	cursor := make([]uint32, totalCells)
	copy(cursor, grid.CellStart[:totalCells])
	for i := uint32(0); i < g.NodeCount; i++ {
		ci := cellIdx(i)
		if ci >= 0 && ci < totalCells {
			grid.NodeOrder[cursor[ci]] = i
			cursor[ci]++
		}
	}

	return grid
}

// NearestNode finds the closest node to the given lat/lon within a 3×3 cell
// neighbourhood (~3.3km). Returns (0, 65535) when no node is found — callers
// must check bestDist against a threshold (e.g. maxSnapDist) to detect this.
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

// NearestNodeFiltered finds the closest node that passes the filter function.
// Scans the 3×3 cell neighbourhood like NearestNode but skips nodes where
// filter returns false. Used to avoid snapping to motorway nodes when the
// profile blocks them.
func NearestNodeFiltered(g *Graph, lat, lon float64, filter func(nodeID uint32) bool) (uint32, uint16) {
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
				if !filter(nodeI) {
					continue
				}
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

// ---------------------------------------------------------------------------
// EdgeProjection — snap a click point to the nearest graph edge
// ---------------------------------------------------------------------------

// EdgeProjection describes the closest point on a graph edge to a query point.
type EdgeProjection struct {
	TileID     uint32  // Tile containing this edge
	EdgeIdx    uint32  // local CSR edge index within the tile
	EdgeHandle uint64  // tileID<<32 | edgeIdx (for retrack/geometry lookups)
	EdgeTags1  uint64  // Tags of the edge (for CostConfig)
	NodeA      uint32  // source node (local ID)
	NodeB      uint32  // target node (local ID)
	ProjLat    float64 // projected point latitude
	ProjLon    float64 // projected point longitude
	ProjEleDm  int32   // interpolated elevation in decimeters
	DistanceM  float64 // click point → projected point distance in meters
	FractionAB float64 // 0.0 = at NodeA, 1.0 = at NodeB
	EdgeDistM  float64 // total edge length in meters
	EleA_dm    int32   // elevation of NodeA in decimeters
	EleB_dm    int32   // elevation of NodeB in decimeters
}

// FindNearestEdgeLocal finds the closest graph edge to (lat, lon) using the
// spatial grid. It first searches a 5×5 cell neighbourhood; if nothing is found,
// it expands to 9×9. An optional filter can reject edges by tags1.
func (g *Graph) FindNearestEdgeLocal(lat, lon float64, filter func(tags1 uint64) bool) (EdgeProjection, bool) {
	grid := g.Grid
	if grid == nil || g.NodeCount == 0 {
		return EdgeProjection{}, false
	}

	cosLat := math.Cos(lat * math.Pi / 180)
	centerRow := int((lat - grid.MinLat) / grid.CellSize)
	centerCol := int((lon - grid.MinLon) / grid.CellSize)

	// Try 5×5 first (radius 2).
	if proj, ok := g.searchEdgesInRadius(lat, lon, cosLat, centerRow, centerCol, 2, filter); ok {
		return proj, true
	}
	// Expand to 9×9 (radius 4).
	return g.searchEdgesInRadius(lat, lon, cosLat, centerRow, centerCol, 4, filter)
}

// searchEdgesInRadius iterates grid cells within the given radius around
// (centerRow, centerCol) and finds the edge with the smallest projection distance.
func (g *Graph) searchEdgesInRadius(lat, lon, cosLat float64, centerRow, centerCol, radius int,
	filter func(tags1 uint64) bool) (EdgeProjection, bool) {

	grid := g.Grid
	best := EdgeProjection{DistanceM: math.MaxFloat64}
	found := false
	seen := make(map[uint32]bool)

	for dr := -radius; dr <= radius; dr++ {
		for dc := -radius; dc <= radius; dc++ {
			r := centerRow + dr
			c := centerCol + dc
			if r < 0 || r >= grid.Rows || c < 0 || c >= grid.Cols {
				continue
			}
			ci := r*grid.Cols + c
			start := grid.CellStart[ci]
			end := grid.CellStart[ci+1]

			for idx := start; idx < end; idx++ {
				node := grid.NodeOrder[idx]
				for eIdx := g.Offsets[node]; eIdx < g.Offsets[node+1]; eIdx++ {
					if seen[eIdx] {
						continue
					}
					seen[eIdx] = true

					tags1 := g.EdgeTags1[eIdx]
					if filter != nil && !filter(tags1) {
						continue
					}

					toNode := g.Targets[eIdx]
					pts := getEdgeGeometry(g, eIdx, node, toNode)
					projLat, projLon, distM := closestPointOnPolyline(lat, lon, pts, cosLat)

					if distM < best.DistanceM {
						totalLen, fracLen := computeFractionAlongPolyline(pts, projLat, projLon, cosLat)
						frac := 0.0
						if totalLen > 0 {
							frac = fracLen / totalLen
						}

						eleA := g.NodeEle[node]
						eleB := g.NodeEle[toNode]
						projEle := int32(float64(eleA) + frac*float64(eleB-eleA))

						best = EdgeProjection{
							TileID:     g.TileID,
							EdgeIdx:    eIdx,
							EdgeHandle: uint64(g.TileID)<<32 | uint64(eIdx),
							EdgeTags1:  tags1,
							NodeA:      node,
							NodeB:      toNode,
							ProjLat:    projLat,
							ProjLon:    projLon,
							ProjEleDm:  projEle,
							DistanceM:  distM,
							FractionAB: frac,
							EdgeDistM:  float64(g.EdgeDist[eIdx]) / 100.0,
							EleA_dm:    eleA,
							EleB_dm:    eleB,
						}
						found = true

						// Early termination: if within 10m, no need to keep searching.
						if distM < 10 {
							return best, true
						}
					}
				}
			}
		}
	}

	return best, found
}

// computeFractionAlongPolyline determines where along the polyline the
// projected point falls. Returns (totalLength, lengthToProjection) in meters.
func computeFractionAlongPolyline(pts []geoPoint, projLat, projLon, cosLat float64) (totalLen, fracLen float64) {
	if len(pts) < 2 {
		return 0, 0
	}

	bestSegIdx := 0
	bestSegDist := math.MaxFloat64

	// Find which segment the projection falls on.
	for i := range len(pts) - 1 {
		_, _, d := perpendicularDist(projLat, projLon,
			pts[i].lat, pts[i].lng,
			pts[i+1].lat, pts[i+1].lng,
			cosLat)
		if d < bestSegDist {
			bestSegDist = d
			bestSegIdx = i
		}
	}

	// Compute total polyline length and length up to the projection point.
	for i := range len(pts) - 1 {
		dx := (pts[i+1].lng - pts[i].lng) * cosLat * geo.MetersPerDegreeLat
		dy := (pts[i+1].lat - pts[i].lat) * geo.MetersPerDegreeLat
		segLen := math.Sqrt(dx*dx + dy*dy)

		totalLen += segLen
		if i < bestSegIdx {
			fracLen += segLen
		} else if i == bestSegIdx {
			// Distance from segment start to projection point.
			px := (projLon - pts[i].lng) * cosLat * geo.MetersPerDegreeLat
			py := (projLat - pts[i].lat) * geo.MetersPerDegreeLat
			fracLen += math.Sqrt(px*px + py*py)
		}
	}

	return totalLen, fracLen
}
