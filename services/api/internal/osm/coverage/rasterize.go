// rasterize.go — Scan-line polygon fill with Active Edge List (AEL) optimisation.
//
// Supports even-odd fill (all rings toggled), which naturally handles holes in
// GeoJSON/OGC polygons where outer rings and inner rings (holes) wind in opposite
// directions.  The ring winding is irrelevant: even-odd counting handles both.
package coverage

import (
	"sort"

	"github.com/paulmach/orb"
)

// edge is an entry in the global edge table (GET).
type edge struct {
	yMin    float64 // scan latitude where this edge first becomes active
	yMax    float64 // scan latitude where this edge expires (exclusive)
	xAtYMin float64 // x (longitude) at yMin
	dxPerDy float64 // Δx per Δy step
}

// RasterizePolygon fills poly into g with val using even-odd scan-line fill.
// Existing cell values are overwritten (set semantics).
func RasterizePolygon(g *CoverageGrid, poly orb.Polygon, val uint8) {
	rasterizeInternal(g, poly, val, false)
}

// RasterizePolygonMax fills poly into g with val using max semantics:
// a cell is only updated if val is greater than its current value.
func RasterizePolygonMax(g *CoverageGrid, poly orb.Polygon, val uint8) {
	rasterizeInternal(g, poly, val, true)
}

func rasterizeInternal(g *CoverageGrid, poly orb.Polygon, val uint8, useMax bool) {
	if len(poly) == 0 || len(poly[0]) < 4 {
		return
	}

	// Build the Global Edge Table from all rings (outer + holes).
	var edges []edge
	for _, ring := range poly {
		n := len(ring)
		for i := 0; i < n-1; i++ {
			a, b := ring[i], ring[i+1]
			ay, by := a[1], b[1]
			if ay == by {
				continue // horizontal edges do not contribute crossings
			}
			e := edge{}
			if ay < by {
				e.yMin, e.yMax, e.xAtYMin = ay, by, a[0]
				e.dxPerDy = (b[0] - a[0]) / (by - ay)
			} else {
				e.yMin, e.yMax, e.xAtYMin = by, ay, b[0]
				e.dxPerDy = (a[0] - b[0]) / (ay - by)
			}
			edges = append(edges, e)
		}
	}
	if len(edges) == 0 {
		return
	}

	// Sort GET by yMin so we can sweep rows top-to-bottom (south-to-north).
	sort.Slice(edges, func(i, j int) bool { return edges[i].yMin < edges[j].yMin })

	// Active Edge List: indices into edges[] that are live for the current scan line.
	active := make([]int, 0, 16)
	nextEdgeIdx := 0

	for row := 0; row < g.Rows; row++ {
		// Scan latitude at the vertical centre of this row.
		scanLat := g.Bound.Min[1] + (float64(row)+0.5)*g.LatStep

		// Promote edges from GET whose yMin has been reached.
		for nextEdgeIdx < len(edges) && edges[nextEdgeIdx].yMin <= scanLat {
			active = append(active, nextEdgeIdx)
			nextEdgeIdx++
		}

		// Remove expired edges (yMax <= scanLat).
		w := 0
		for _, idx := range active {
			if edges[idx].yMax > scanLat {
				active[w] = idx
				w++
			}
		}
		active = active[:w]

		if len(active) < 2 {
			continue
		}

		// Compute x-crossings for every active edge at scanLat.
		xCrossings := make([]float64, 0, len(active))
		for _, idx := range active {
			e := &edges[idx]
			x := e.xAtYMin + (scanLat-e.yMin)*e.dxPerDy
			xCrossings = append(xCrossings, x)
		}

		// Even-odd fill: pair up sorted crossings.
		sort.Float64s(xCrossings)
		for i := 0; i+1 < len(xCrossings); i += 2 {
			colStart := max(0, int((xCrossings[i]-g.Bound.Min[0])/g.LonStep))
			colEnd := min(g.Cols-1, int((xCrossings[i+1]-g.Bound.Min[0])/g.LonStep))
			for col := colStart; col <= colEnd; col++ {
				idx := row*g.Cols + col
				if useMax {
					if val > g.Data[idx] {
						g.Data[idx] = val
					}
				} else {
					g.Data[idx] = val
				}
			}
		}
	}
}
