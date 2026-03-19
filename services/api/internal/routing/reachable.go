// reachable.go — Dijkstra with cost budget for isochrone + SuggestRoutes.
// No destination, no A* heuristic. Expands until budget exhausted or maxExpansions hit.
// AlphaShape via fogleman/delaunay for concave isochrone polygons.
package routing

import (
	"context"
	"math"
	"sort"

	"github.com/fogleman/delaunay"
	"github.com/paulmach/orb"
)

// reachableNode stores per-node data from the Dijkstra expansion.
type reachableNode struct {
	Cost       float32 // accumulated cost to reach this node
	EdgeHandle uint64  // handle of the incoming edge (for EdgeOSMID lookup)
}

const reachableCheckCancel = 4096

// findReachable expands Dijkstra from src until all reachable nodes within
// maxBudget are found. Uses DeltaEle for hysteresis (speed > accuracy).
// Unlike findRouteUnified, elevationCorrection is intentionally omitted:
// the latent buffer state slightly shifts the isochrone boundary but the
// shape remains correct, and skipping it halves per-node bookkeeping.
// Returns map[globalNodeID]reachableNode and truncated flag.
func findReachable(ctx context.Context, reader GraphReader, src uint64,
	cfg *CostConfig, maxBudget float32, maxExpansions int,
) (map[uint64]reachableNode, bool) {

	type dijkState struct {
		cost      float32
		pathState PathState
		lastDX    float32
		lastDY    float32
		lastWC    uint8
		handle    uint64 // incoming edge handle
	}

	state := make(map[uint64]dijkState, 8192)
	frontier := Frontier{entries: make([]FrontierEntry, 0, 8192)}

	state[src] = dijkState{cost: 0}
	frontier.Push(FrontierEntry{NodeID: src, Cost: 0})

	expansions := 0
	truncated := false

	for frontier.Len() > 0 {
		if expansions&(reachableCheckCancel-1) == 0 {
			if err := ctx.Err(); err != nil {
				break
			}
		}
		expansions++
		if expansions > maxExpansions {
			truncated = true
			break
		}

		entry := frontier.Pop()
		ns, ok := state[entry.NodeID]
		if !ok {
			continue
		}
		if entry.Cost > ns.cost+0.001 {
			continue // stale
		}
		if ns.cost > maxBudget {
			continue // over budget
		}

		curLat, curLon, _ := reader.NodeCoord(entry.NodeID)

		reader.ForEachEdge(entry.NodeID, func(ev EdgeView) {
			ecf := cfg.EdgeCost(ev.Tags1, ev.Tags2)
			if ecf.Flat >= costBlocked {
				return
			}

			hr := propagateHysteresis(ns.pathState, ev.DeltaEle*100, int32(ev.Dist)*10, cfg)

			cfUp := ecf.Up
			cfDown := ecf.Down
			if cfUp == 0 {
				cfUp = ecf.Flat
			}
			if cfDown == 0 {
				cfDown = ecf.Flat
			}
			flatW := float32(1.0) - hr.UpWeight - hr.DownWeight
			if flatW < 0 {
				flatW = 0
			}
			cf := cfUp*hr.UpWeight + ecf.Flat*flatW + cfDown*hr.DownWeight
			distCost := float32(ev.Dist) / 100.0 * cf

			turnCost, newDX, newDY := computeTurnCost(
				curLon, curLat, ev.TargetLon, ev.TargetLat,
				ns.lastDX, ns.lastDY, cfg.TurnCost, ev.Dist)

			var initialCost float32
			if ns.lastWC != ecf.WayClass && ns.lastWC > 0 {
				if ecf.WayClass == 1 || ns.lastWC == 1 {
					initialCost = 10000
				} else {
					initialCost = cfg.TurnCost * 2
				}
			}

			newCost := ns.cost + distCost + hr.ElevCost + initialCost + turnCost

			if newCost > maxBudget {
				return
			}

			existing, exists := state[ev.Target]
			if !exists || newCost < existing.cost {
				state[ev.Target] = dijkState{
					cost:      newCost,
					pathState: hr.State,
					lastDX:    newDX,
					lastDY:    newDY,
					lastWC:    ecf.WayClass,
					handle:    ev.ElevHandle,
				}
				frontier.Push(FrontierEntry{NodeID: ev.Target, Cost: newCost})
			}
		})
	}

	// Convert to exported type
	result := make(map[uint64]reachableNode, len(state))
	for nodeID, ds := range state {
		if ds.cost <= maxBudget {
			result[nodeID] = reachableNode{Cost: ds.cost, EdgeHandle: ds.handle}
		}
	}
	return result, truncated
}

// AlphaShape computes a concave boundary using Delaunay triangulation.
// Triangles with circumradius > alpha are removed. Boundary edges of remaining
// triangles form the alpha shape. If alpha <= 0, auto-calculate from avg edge length.
// Falls back to ConvexHull (via fogleman/delaunay) if too few points.
func AlphaShape(points []orb.Point, alpha float64) orb.Ring {
	if len(points) < 3 {
		return nil
	}

	// Convert to delaunay.Point
	dPoints := make([]delaunay.Point, len(points))
	for i, p := range points {
		dPoints[i] = delaunay.Point{X: p[0], Y: p[1]}
	}

	tri, err := delaunay.Triangulate(dPoints)
	if err != nil || len(tri.Triangles) < 3 {
		return nil
	}

	// Auto-alpha: average edge length x 2
	if alpha <= 0 {
		totalLen := 0.0
		numEdges := 0
		for i := 0; i < len(tri.Triangles); i += 3 {
			for j := range 3 {
				a := dPoints[tri.Triangles[i+j]]
				b := dPoints[tri.Triangles[i+(j+1)%3]]
				dx := a.X - b.X
				dy := a.Y - b.Y
				totalLen += math.Sqrt(dx*dx + dy*dy)
				numEdges++
			}
		}
		alpha = (totalLen / float64(numEdges)) * 2.0
	}

	// Filter triangles by circumradius
	type edge struct{ a, b int }
	edgeCount := make(map[edge]int)

	for i := 0; i < len(tri.Triangles); i += 3 {
		ia, ib, ic := tri.Triangles[i], tri.Triangles[i+1], tri.Triangles[i+2]
		pa, pb, pc := dPoints[ia], dPoints[ib], dPoints[ic]

		// Circumradius
		cr := circumradius(pa, pb, pc)
		if cr > alpha {
			continue // remove this triangle
		}

		// Count boundary edges (edges appearing in only 1 kept triangle)
		edges := [3]edge{
			{min(ia, ib), max(ia, ib)},
			{min(ib, ic), max(ib, ic)},
			{min(ic, ia), max(ic, ia)},
		}
		for _, e := range edges {
			edgeCount[e]++
		}
	}

	// Boundary edges: count == 1
	boundaryEdges := make(map[int][]int) // node -> connected boundary nodes
	for e, cnt := range edgeCount {
		if cnt == 1 {
			boundaryEdges[e.a] = append(boundaryEdges[e.a], e.b)
			boundaryEdges[e.b] = append(boundaryEdges[e.b], e.a)
		}
	}

	if len(boundaryEdges) < 3 {
		return nil
	}

	// Walk boundary to form ring (take the largest connected component)
	visited := make(map[int]bool)
	var bestRing orb.Ring

	for startNode := range boundaryEdges {
		if visited[startNode] {
			continue
		}
		var ring orb.Ring
		cur := startNode
		prev := -1
		for {
			visited[cur] = true
			ring = append(ring, orb.Point{dPoints[cur].X, dPoints[cur].Y})
			neighbors := boundaryEdges[cur]
			next := -1
			for _, n := range neighbors {
				if n != prev && !visited[n] {
					next = n
					break
				}
			}
			if next == -1 {
				break
			}
			prev = cur
			cur = next
		}
		// Close ring
		if len(ring) > 2 {
			ring = append(ring, ring[0])
		}
		if len(ring) > len(bestRing) {
			bestRing = ring
		}
	}

	if len(bestRing) < 4 {
		return nil
	}
	return bestRing
}

func circumradius(a, b, c delaunay.Point) float64 {
	ax, ay := b.X-a.X, b.Y-a.Y
	bx, by := c.X-a.X, c.Y-a.Y
	D := 2 * (ax*by - ay*bx)
	if math.Abs(D) < 1e-10 {
		return math.Inf(1) // degenerate (collinear)
	}
	ux := (by*(ax*ax+ay*ay) - ay*(bx*bx+by*by)) / D
	uy := (ax*(bx*bx+by*by) - bx*(ax*ax+ay*ay)) / D
	return math.Sqrt(ux*ux + uy*uy)
}

// ConvexHull computes the convex hull using Andrew's monotone chain algorithm.
// Returns an orb.Ring (closed, CCW) or nil if fewer than 3 unique points.
// Kept as utility (POI hull, degenerate fallback).
func ConvexHull(points []orb.Point) orb.Ring {
	if len(points) < 3 {
		return nil
	}

	// Sort by X, then Y
	sorted := make([]orb.Point, len(points))
	copy(sorted, points)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i][0] != sorted[j][0] {
			return sorted[i][0] < sorted[j][0]
		}
		return sorted[i][1] < sorted[j][1]
	})

	// Remove exact duplicates
	unique := sorted[:1]
	for i := 1; i < len(sorted); i++ {
		if sorted[i] != sorted[i-1] {
			unique = append(unique, sorted[i])
		}
	}
	if len(unique) < 3 {
		// Degenerate: return all points as a closed ring
		ring := make(orb.Ring, len(unique)+1)
		copy(ring, unique)
		ring[len(unique)] = unique[0]
		return ring
	}

	n := len(unique)
	hull := make(orb.Ring, 0, 2*n)

	// Lower hull
	for _, p := range unique {
		for len(hull) >= 2 && cross(hull[len(hull)-2], hull[len(hull)-1], p) <= 0 {
			hull = hull[:len(hull)-1]
		}
		hull = append(hull, p)
	}

	// Upper hull
	lower := len(hull) + 1
	for i := n - 2; i >= 0; i-- {
		p := unique[i]
		for len(hull) >= lower && cross(hull[len(hull)-2], hull[len(hull)-1], p) <= 0 {
			hull = hull[:len(hull)-1]
		}
		hull = append(hull, p)
	}

	return hull
}

// cross returns the cross product of vectors OA and OB where O=o, A=a, B=b.
// Positive = counter-clockwise, negative = clockwise, zero = collinear.
func cross(o, a, b orb.Point) float64 {
	return (a[0]-o[0])*(b[1]-o[1]) - (a[1]-o[1])*(b[0]-o[0])
}
