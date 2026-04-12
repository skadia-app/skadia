// reachable.go — Pareto-based findReachable + AlphaShape/ConvexHull utilities
// for isochrone + SuggestRoutes.
// Legacy scalar-cost Dijkstra lives in reachable_legacy.go.
package routing

import (
	"context"
	"math"
	"sort"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
	"github.com/fogleman/delaunay"
	"github.com/paulmach/orb"
)

// frontContainsState checks whether the given stateID is still active in the front.
func frontContainsState(front *paretoFront, stateID uint32) bool {
	for _, id := range front.activeIDs {
		if id == stateID {
			return true
		}
	}
	return false
}

// findReachable expands Pareto-Dijkstra from src until all reachable nodes within
// maxBudget are found. Uses 2D Pareto dominance over (cost, ascent) so each node
// can hold multiple non-dominated states. Expansion logic mirrors legacy
// (hysteresis, turn costs, way class penalties, coverage accumulation) but adds
// per-state ascent/descent/dist/qualitySum accumulators.
func findReachable(ctx context.Context, reader GraphReader, src uint64,
	cfg *CostConfig, maxBudget float32,
) (*reachableState, error) {

	rs := newReachableState()
	frontier := paretoFrontier{entries: make([]paretoFrontierEntry, 0, 8192)}

	// Insert root state.
	rootState := paretoState{
		cost:          0,
		ascent:        0,
		parentStateID: rootStateID,
		nodeID:        src,
	}
	rootID, _ := rs.tryInsert(src, rootState)
	rootFront := rs.fronts[src]
	frontier.Push(paretoFrontierEntry{
		nodeID:   src,
		stateID:  rootID,
		cost:     0,
		frontGen: rootFront.generation,
	})

	expansions := 0

	for !rs.truncated && frontier.Len() > 0 {
		if expansions&(reachableCheckCancel-1) == 0 {
			if err := ctx.Err(); err != nil {
				return rs, err
			}
		}
		expansions++
		if expansions > maxExpansions {
			rs.markExpansionTruncated()
			break
		}

		entry := frontier.Pop()

		// Stale-entry check: front generation changed or state no longer active.
		front, ok := rs.fronts[entry.nodeID]
		if !ok {
			continue
		}
		if front.generation != entry.frontGen || !frontContainsState(front, entry.stateID) {
			continue
		}

		curState := rs.slab.states[entry.stateID]
		if curState.cost > maxBudget {
			continue
		}

		curLat, curLon, _ := reader.NodeCoord(entry.nodeID)

		reader.ForEachEdge(entry.nodeID, func(ev EdgeView) {
			ecf := cfg.EdgeCost(ev.Tags1, ev.Tags2)
			if ecf.Flat >= costBlocked {
				return
			}

			// Save raw (unpenalized) flat factor before applying way penalty.
			rawFlat := ecf.Flat

			// Apply way penalty (monotone, >= 1.0)
			penalty := lookupWayPenalty(ev.ElevHandle, reader, cfg.WayPenalties)
			applyPenaltyToECF(&ecf, penalty)

			hr := propagateHysteresis(curState.pathState, ev.DeltaEle*100, int32(ev.Dist)*10, cfg)
			// Penalty scales elevCost too (critical for alpine avoidance)
			if penalty > 1.0 {
				hr.ElevCost *= penalty
			}

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
				curState.lastDX, curState.lastDY, cfg.TurnCost, ev.Dist)

			var initialCost float32
			if curState.lastWC != ecf.WayClass && curState.lastWC > 0 {
				if ecf.WayClass == 1 || curState.lastWC == 1 {
					initialCost = 10000
				} else {
					initialCost = cfg.TurnCost * 2
				}
			}

			newCost := curState.cost + distCost + hr.ElevCost + initialCost + turnCost

			if newCost > maxBudget {
				return
			}

			// Ascent / descent from DeltaEle (decimeters -> meters).
			deltaM := float32(ev.DeltaEle) / 10.0
			newAscent := curState.ascent
			newDescent := curState.descent
			if deltaM > 0 {
				newAscent += deltaM
			} else {
				newDescent += -deltaM
			}

			// Distance accumulation (centimeters -> meters).
			edgeDistM := float32(ev.Dist) / 100.0

			// Quality sum: unpenalised flat factor * distance.
			newQualitySum := curState.qualitySum + edgeDistM*rawFlat

			// Accumulate distance-weighted coverage scores.
			covDist := edgeDistM
			newCovForest := curState.covForestSum + float32(graph.DecodeForestPct(ev.Tags2))/100.0*covDist
			newCovNoise := curState.covNoiseSum + float32(graph.DecodeNoiseScore(ev.Tags2))/100.0*covDist
			newCovProtected := curState.covProtectedSum + float32(graph.DecodeProtectedArea(ev.Tags2))/3.0*covDist
			newCovUrban := curState.covUrbanSum + float32(graph.DecodeUrbanization(ev.Tags2))/7.0*covDist
			newCovRiver := curState.covRiverSum + float32(graph.DecodeRiverProximity(ev.Tags2))/7.0*covDist
			newCovTraffic := curState.covTrafficSum + float32(graph.DecodeTrafficClass(ev.Tags2))/6.0*covDist
			newCovDistSum := curState.covDistSum + covDist

			candidate := paretoState{
				cost:            newCost,
				ascent:          newAscent,
				descent:         newDescent,
				dist:            curState.dist + edgeDistM,
				qualitySum:      newQualitySum,
				pathState:       hr.State,
				lastDX:          newDX,
				lastDY:          newDY,
				handle:          ev.ElevHandle,
				nodeID:          ev.Target,
				parentStateID:   entry.stateID,
				lastWC:          ecf.WayClass,
				covForestSum:    newCovForest,
				covNoiseSum:     newCovNoise,
				covProtectedSum: newCovProtected,
				covUrbanSum:     newCovUrban,
				covRiverSum:     newCovRiver,
				covTrafficSum:   newCovTraffic,
				covDistSum:      newCovDistSum,
			}

			newID, inserted := rs.tryInsert(ev.Target, candidate)
			if inserted {
				targetFront := rs.fronts[ev.Target]
				frontier.Push(paretoFrontierEntry{
					nodeID:   ev.Target,
					stateID:  newID,
					cost:     newCost,
					frontGen: targetFront.generation,
				})
			}
		})
	}

	// Emit Prometheus metrics after expansion.
	for _, front := range rs.fronts {
		ParetoFrontSize.Observe(float64(len(front.activeIDs)))
	}
	if rs.truncated {
		ParetoTruncatedCount.WithLabelValues(rs.truncatedReason).Inc()
	}

	return rs, nil
}

const reachableCheckCancel = 4096

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
			if prev >= 0 && len(neighbors) > 1 {
				// Angle-based selection: pick neighbor with smallest CW turn
				// (right-hand rule traces outer boundary correctly at degree>2 nodes)
				inAngle := math.Atan2(dPoints[cur].Y-dPoints[prev].Y, dPoints[cur].X-dPoints[prev].X)
				reverseAngle := inAngle + math.Pi
				bestAngle := 4 * math.Pi // larger than any possible
				for _, n := range neighbors {
					if n == prev || visited[n] {
						continue
					}
					outAngle := math.Atan2(dPoints[n].Y-dPoints[cur].Y, dPoints[n].X-dPoints[cur].X)
					// CCW angle from reverse incoming to outgoing, normalized to [0, 2π)
					angle := outAngle - reverseAngle
					for angle < 0 {
						angle += 2 * math.Pi
					}
					for angle >= 2*math.Pi {
						angle -= 2 * math.Pi
					}
					if angle < bestAngle {
						bestAngle = angle
						next = n
					}
				}
			} else {
				// First node or single neighbor: pick any unvisited
				for _, n := range neighbors {
					if n != prev && !visited[n] {
						next = n
						break
					}
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
