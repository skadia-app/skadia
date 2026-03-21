package graph

import (
	"math"
	"slices"
)

// EdgeCandidate describes an edge that is close to a query point, with the
// projected foot-point and decoded OSM tag strings.
type EdgeCandidate struct {
	EdgeIdx  uint32
	NodeFrom uint32
	NodeTo   uint32
	DistM    float64
	ProjLat  float64
	ProjLng  float64
	Surface  string
	Highway  string
}

const (
	edgeLookupNodeRadiusM = 75.0
	edgeLookupMaxDistM    = 25.0
	degreesToMeters       = 111320.0
)

// geoPoint is a lat/lng coordinate pair used internally in edge geometry.
type geoPoint struct{ lat, lng float64 }

// FindNearEdges returns all edges whose full geometry passes within
// edgeLookupMaxDistM metres of the query point (lat, lng).
// Results are sorted by ascending distance.
// Returns nil when the graph has no spatial index or no nodes.
func FindNearEdges(g *Graph, lat, lng float64) []EdgeCandidate {
	if g.Grid == nil || g.NodeCount == 0 {
		return nil
	}

	grid := g.Grid
	cosLat := math.Cos(lat * math.Pi / 180)
	nodeRadiusDeg := edgeLookupNodeRadiusM / degreesToMeters
	cellsNeeded := int(math.Ceil(nodeRadiusDeg / grid.CellSize))

	centerRow := int((lat - grid.MinLat) / grid.CellSize)
	centerCol := int((lng - grid.MinLon) / grid.CellSize)

	seen := make(map[uint32]bool)
	var candidates []EdgeCandidate

	for dr := -cellsNeeded; dr <= cellsNeeded; dr++ {
		for dc := -cellsNeeded; dc <= cellsNeeded; dc++ {
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
				// Iterate over all outgoing edges of this node.
				for eIdx := g.Offsets[node]; eIdx < g.Offsets[node+1]; eIdx++ {
					if seen[eIdx] {
						continue
					}
					seen[eIdx] = true

					toNode := g.Targets[eIdx]
					pts := getEdgeGeometry(g, eIdx, node, toNode)
					projLat, projLng, distM := closestPointOnPolyline(lat, lng, pts, cosLat)
					if distM > edgeLookupMaxDistM {
						continue
					}

					tags := g.EdgeTags1[eIdx]
					hw := HighwayName(DecodeHighway(tags))
					sf := SurfaceName(DecodeSurface(tags))

					candidates = append(candidates, EdgeCandidate{
						EdgeIdx:  eIdx,
						NodeFrom: node,
						NodeTo:   toNode,
						DistM:    distM,
						ProjLat:  projLat,
						ProjLng:  projLng,
						Surface:  sf,
						Highway:  hw,
					})
				}
			}
		}
	}

	slices.SortFunc(candidates, func(a, b EdgeCandidate) int {
		if a.DistM < b.DistM {
			return -1
		}
		if a.DistM > b.DistM {
			return 1
		}
		return 0
	})
	return candidates
}

// getEdgeGeometry assembles the full polyline for an edge:
// [fromNode, intermediate..., toNode].
func getEdgeGeometry(g *Graph, edgeIdx, fromNode, toNode uint32) []geoPoint {
	fromLat := float64(g.NodeLat[fromNode]) / 1e7
	fromLng := float64(g.NodeLon[fromNode]) / 1e7
	toLat := float64(g.NodeLat[toNode]) / 1e7
	toLng := float64(g.NodeLon[toNode]) / 1e7

	// Build result with pre-allocated capacity for the common case.
	pts := make([]geoPoint, 0, 4)
	pts = append(pts, geoPoint{fromLat, fromLng})

	if int(edgeIdx) < len(g.EdgeGeomOff)-1 {
		geomStart := g.EdgeGeomOff[edgeIdx]
		geomEnd := g.EdgeGeomOff[edgeIdx+1]
		for i := geomStart; i < geomEnd; i++ {
			if int(i) < len(g.EdgeGeomLat) {
				iLat := float64(g.EdgeGeomLat[i]) / 1e7
				iLng := float64(g.EdgeGeomLon[i]) / 1e7
				pts = append(pts, geoPoint{iLat, iLng})
			}
		}
	}

	pts = append(pts, geoPoint{toLat, toLng})
	return pts
}

// closestPointOnPolyline finds the foot-point on the polyline that is nearest
// to (lat, lng) using equirectangular projection. Falls back to the first
// point when the polyline is empty.
func closestPointOnPolyline(lat, lng float64, pts []geoPoint, cosLat float64) (projLat, projLng, distM float64) {
	if len(pts) == 0 {
		return lat, lng, 0
	}
	projLat = pts[0].lat
	projLng = pts[0].lng
	distM = math.MaxFloat64

	for i := range len(pts) - 1 {
		pLat, pLng, d := perpendicularDist(lat, lng,
			pts[i].lat, pts[i].lng,
			pts[i+1].lat, pts[i+1].lng,
			cosLat)
		if d < distM {
			distM = d
			projLat = pLat
			projLng = pLng
		}
	}
	return projLat, projLng, distM
}

// perpendicularDist computes the closest point on segment (lat1,lng1)-(lat2,lng2)
// to the query point (lat,lng) and the Euclidean distance to it, using an
// equirectangular approximation with the provided cosLat correction factor.
func perpendicularDist(lat, lng, lat1, lng1, lat2, lng2, cosLat float64) (projLat, projLng, distM float64) {
	x := (lng - lng1) * cosLat * degreesToMeters
	y := (lat - lat1) * degreesToMeters

	dx := (lng2 - lng1) * cosLat * degreesToMeters
	dy := (lat2 - lat1) * degreesToMeters

	segLen2 := dx*dx + dy*dy
	if segLen2 < 1e-10 {
		// Degenerate segment: both endpoints coincide.
		distM = math.Sqrt(x*x + y*y)
		projLat = lat1
		projLng = lng1
		return projLat, projLng, distM
	}

	t := (x*dx + y*dy) / segLen2
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}

	px := t * dx
	py := t * dy

	distM = math.Sqrt((x-px)*(x-px) + (y-py)*(y-py))
	projLat = lat1 + py/degreesToMeters
	projLng = lng1 + px/(cosLat*degreesToMeters)
	return projLat, projLng, distM
}
