// Legacy scalar-cost Dijkstra kept for feature-flag fallback.
// DO NOT EXTEND — new development should use the Pareto-based findReachable
// in reachable.go. This file will be removed after 2 weeks of stable Pareto
// production traffic.
package routing

import (
	"context"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// reachableNodeLegacy stores per-node data from the legacy Dijkstra expansion.
type reachableNodeLegacy struct {
	Cost       float32 // accumulated cost to reach this node
	EdgeHandle uint64  // handle of the incoming edge (for EdgeOSMID lookup)
	// Path-averaged coverage scores, normalized [0, 1]
	ForestPct      float32
	NoiseScore     float32
	ProtectedArea  float32
	Urbanization   float32
	RiverProximity float32
	TrafficClass   float32
}

const reachableCheckCancelLegacy = 4096

// findReachableLegacy expands Dijkstra from src until all reachable nodes within
// maxBudget are found. Uses DeltaEle for hysteresis (speed > accuracy).
// Unlike findRouteUnified, elevationCorrection is intentionally omitted:
// the latent buffer state slightly shifts the isochrone boundary but the
// shape remains correct, and skipping it halves per-node bookkeeping.
// Returns map[globalNodeID]reachableNodeLegacy and truncated flag.
func findReachableLegacy(ctx context.Context, reader GraphReader, src uint64,
	cfg *CostConfig, maxBudget float32, maxExpansions int,
) (map[uint64]reachableNodeLegacy, bool) {

	type dijkState struct {
		cost      float32
		pathState PathState
		lastDX    float32
		lastDY    float32
		lastWC    uint8
		handle    uint64 // incoming edge handle
		// Coverage accumulators: distance-weighted sums (normalized_value * meters)
		// Divide by covDistSum for path-average [0, 1]
		covForestSum    float32
		covNoiseSum     float32
		covProtectedSum float32
		covUrbanSum     float32
		covRiverSum     float32 // includes lake proximity
		covTrafficSum   float32
		covDistSum      float32
	}

	state := make(map[uint64]dijkState, 8192)
	frontier := Frontier{entries: make([]FrontierEntry, 0, 8192)}

	state[src] = dijkState{cost: 0}
	frontier.Push(FrontierEntry{NodeID: src, Cost: 0})

	expansions := 0
	truncated := false

	for frontier.Len() > 0 {
		if expansions&(reachableCheckCancelLegacy-1) == 0 {
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

			// Accumulate distance-weighted coverage scores
			covDist := float32(ev.Dist) / 100.0 // centimeters to meters
			newCovForest := ns.covForestSum + float32(graph.DecodeForestPct(ev.Tags2))/100.0*covDist
			newCovNoise := ns.covNoiseSum + float32(graph.DecodeNoiseScore(ev.Tags2))/100.0*covDist
			newCovProtected := ns.covProtectedSum + float32(graph.DecodeProtectedArea(ev.Tags2))/3.0*covDist
			newCovUrban := ns.covUrbanSum + float32(graph.DecodeUrbanization(ev.Tags2))/7.0*covDist
			newCovRiver := ns.covRiverSum + float32(graph.DecodeRiverProximity(ev.Tags2))/7.0*covDist
			newCovTraffic := ns.covTrafficSum + float32(graph.DecodeTrafficClass(ev.Tags2))/6.0*covDist
			newCovDistSum := ns.covDistSum + covDist

			existing, exists := state[ev.Target]
			if !exists || newCost < existing.cost {
				state[ev.Target] = dijkState{
					cost:            newCost,
					pathState:       hr.State,
					lastDX:          newDX,
					lastDY:          newDY,
					lastWC:          ecf.WayClass,
					handle:          ev.ElevHandle,
					covForestSum:    newCovForest,
					covNoiseSum:     newCovNoise,
					covProtectedSum: newCovProtected,
					covUrbanSum:     newCovUrban,
					covRiverSum:     newCovRiver,
					covTrafficSum:   newCovTraffic,
					covDistSum:      newCovDistSum,
				}
				frontier.Push(FrontierEntry{NodeID: ev.Target, Cost: newCost})
			}
		})
	}

	// Convert to exported type with path-averaged coverage scores
	result := make(map[uint64]reachableNodeLegacy, len(state))
	for nodeID, ds := range state {
		if ds.cost <= maxBudget {
			var forest, noise, prot, urban, river, traffic float32
			if ds.covDistSum > 0 {
				forest = ds.covForestSum / ds.covDistSum
				noise = ds.covNoiseSum / ds.covDistSum
				prot = ds.covProtectedSum / ds.covDistSum
				urban = ds.covUrbanSum / ds.covDistSum
				river = ds.covRiverSum / ds.covDistSum
				traffic = ds.covTrafficSum / ds.covDistSum
			}
			result[nodeID] = reachableNodeLegacy{
				Cost:           ds.cost,
				EdgeHandle:     ds.handle,
				ForestPct:      forest,
				NoiseScore:     noise,
				ProtectedArea:  prot,
				Urbanization:   urban,
				RiverProximity: river,
				TrafficClass:   traffic,
			}
		}
	}
	return result, truncated
}
