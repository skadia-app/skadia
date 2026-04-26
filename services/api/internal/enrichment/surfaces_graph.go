package enrichment

import (
	"math"

	"github.com/skadia-app/skadia/services/api/internal/geo"
	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

type resampledPoint = struct {
	lat float64
	lng float64
}

// resampleStreamPoints filters GPS-valid points from stream, then resamples
// them to uniform spacing of spacingM metres using linear interpolation.
// Returns at least 2 points when the total track distance is >= spacingM,
// otherwise an empty/single-point slice (caller must guard with len < 2).
func resampleStreamPoints(stream []EnrichmentStreamPoint, spacingM float64) []resampledPoint {
	// Collect GPS-valid raw points.
	type rawPt struct{ lat, lng float64 }
	var raw []rawPt
	for _, sp := range stream {
		if sp.Lat == nil || sp.Lng == nil {
			continue
		}
		raw = append(raw, rawPt{*sp.Lat, *sp.Lng})
	}
	if len(raw) < 2 {
		return nil
	}

	// Build cumulative distance array.
	cumDist := make([]float64, len(raw))
	cumDist[0] = 0
	for i := 1; i < len(raw); i++ {
		cumDist[i] = cumDist[i-1] + equirectDistMeters(raw[i-1].lat, raw[i-1].lng, raw[i].lat, raw[i].lng)
	}
	totalDist := cumDist[len(cumDist)-1]
	if totalDist < 1e-6 {
		return nil
	}

	numPts := max(2, int(totalDist/spacingM)+1)
	out := make([]resampledPoint, numPts)

	// Linear interpolation along cumulative distance.
	j := 0 // segment pointer into raw[]
	for i := range numPts {
		target := float64(i) * (totalDist / float64(numPts-1))
		if target >= totalDist {
			out[i] = resampledPoint{raw[len(raw)-1].lat, raw[len(raw)-1].lng}
			continue
		}
		// Advance j so that cumDist[j] <= target < cumDist[j+1].
		for j < len(raw)-2 && cumDist[j+1] < target {
			j++
		}
		segLen := cumDist[j+1] - cumDist[j]
		var t float64
		if segLen > 1e-10 {
			t = (target - cumDist[j]) / segLen
		}
		out[i] = resampledPoint{
			lat: raw[j].lat + t*(raw[j+1].lat-raw[j].lat),
			lng: raw[j].lng + t*(raw[j+1].lng-raw[j].lng),
		}
	}

	return out
}

// equirectDistMeters returns an equirectangular approximation of the distance
// in metres between two lat/lng points. Accurate to ~0.1% for distances
// under 100 km.
func equirectDistMeters(lat1, lng1, lat2, lng2 float64) float64 {
	dLat := (lat2 - lat1) * geo.MetersPerDegreeLat
	dLng := (lng2 - lng1) * math.Cos(lat1*math.Pi/180.0) * geo.MetersPerDegreeLat
	return math.Sqrt(dLat*dLat + dLng*dLng)
}

// viterbiMatch runs a Viterbi algorithm over the resampled track points and
// the per-point edge candidate lists, returning one matched EdgeCandidate per
// point (zero-value when offroad / no candidates).
//
// Emission model:
//
//	log P(obs | edge) = -0.5 * (distM² / sigma²),  sigma = 10 m
//
// Transition model (log probabilities):
//
//	same edge                     →  0
//	shared node (to/from overlap) → -0.5
//	unconnected                   → -max(gcDist/10, 3.0)
//
// Offroad handling:
//   - When candidates[t] is nil/empty the point is considered offroad and
//     its entry in the output is a zero-value EdgeCandidate.
//   - The algorithm tracks lastOnRoad; when the offroad gap exceeds 5 points
//     (~50 m) it re-initialises from emission probabilities only (no
//     transition from the pre-gap state), preventing stale path propagation.
func viterbiMatch(
	pts []resampledPoint,
	candidates [][]graph.EdgeCandidate,
) []graph.EdgeCandidate {
	n := len(pts)
	if n == 0 {
		return nil
	}

	const sigma = 10.0 // emission Gaussian stddev in metres
	const sigma2 = sigma * sigma

	// vProb[i] = best log-prob reaching candidates[t][i] at step t.
	// vPrev[i] = index into candidates[t-1] that led to candidates[t][i].
	type dpState struct {
		prob float64
		prev int // -1 = no predecessor (re-init or first step)
	}

	// Per-step DP tables (reused each step).
	// We keep full arrays: dpCurr[candidateIndex], dpPrev[candidateIndex].
	// For offroad steps we store nothing (nil slice).
	steps := make([][]dpState, n)

	lastOnRoad := -1

	// --- Forward pass ---
	for t := range n {
		cands := candidates[t]
		if len(cands) == 0 {
			// Offroad point — leave steps[t] nil.
			continue
		}

		steps[t] = make([]dpState, len(cands))
		gapSinceLast := t - lastOnRoad // == t+1 when lastOnRoad==-1

		if lastOnRoad < 0 || gapSinceLast > 5 {
			// First on-road point or re-init after long offroad gap:
			// emission-only initialisation.
			for k, ec := range cands {
				steps[t][k] = dpState{
					prob: -0.5 * (ec.DistM * ec.DistM / sigma2),
					prev: -1,
				}
			}
		} else {
			// Normal step: emission + best transition from previous on-road step.
			prevCands := candidates[lastOnRoad]
			prevStates := steps[lastOnRoad]

			for k, ec := range cands {
				emit := -0.5 * (ec.DistM * ec.DistM / sigma2)
				best := math.Inf(-1)
				bestPrev := -1

				for pk, pec := range prevCands {
					trans := transitionLogProb(pec, ec, pts[lastOnRoad], pts[t])
					score := prevStates[pk].prob + trans
					if score > best {
						best = score
						bestPrev = pk
					}
				}

				steps[t][k] = dpState{
					prob: emit + best,
					prev: bestPrev,
				}
			}
		}

		lastOnRoad = t
	}

	// --- Backtrack ---
	result := make([]graph.EdgeCandidate, n)

	// Find the last on-road point.
	lastT := -1
	for t := n - 1; t >= 0; t-- {
		if len(steps[t]) > 0 {
			lastT = t
			break
		}
	}
	if lastT < 0 {
		// All offroad — return zero-value slice.
		return result
	}

	// Pick best candidate at lastT.
	bestK := 0
	for k := 1; k < len(steps[lastT]); k++ {
		if steps[lastT][k].prob > steps[lastT][bestK].prob {
			bestK = k
		}
	}

	// Trace back, filling result[].
	// We walk backwards through on-road steps only.
	curT := lastT
	curK := bestK
	for curT >= 0 {
		if len(steps[curT]) == 0 {
			// Offroad — leave result[curT] zero.
			curT--
			continue
		}

		result[curT] = candidates[curT][curK]
		prevK := steps[curT][curK].prev

		// Find previous on-road step.
		prevT := curT - 1
		for prevT >= 0 && len(steps[prevT]) == 0 {
			prevT--
		}

		if prevT < 0 {
			break
		}
		if prevK < 0 {
			// Re-init boundary: current step has no backpointer to prevT.
			// Start a fresh local backtrack using the best candidate at prevT.
			bestLocalK := 0
			for k := 1; k < len(steps[prevT]); k++ {
				if steps[prevT][k].prob > steps[prevT][bestLocalK].prob {
					bestLocalK = k
				}
			}
			curT = prevT
			curK = bestLocalK
			continue
		}

		// Check for re-init boundary: gap > 5.
		if curT-prevT > 5 {
			// Re-init boundary: start a fresh local backtrack at prevT using
			// the local maximum (emission only is the best choice here).
			bestLocalK := 0
			for k := 1; k < len(steps[prevT]); k++ {
				if steps[prevT][k].prob > steps[prevT][bestLocalK].prob {
					bestLocalK = k
				}
			}
			curT = prevT
			curK = bestLocalK
		} else {
			curT = prevT
			curK = prevK
		}
	}

	return result
}

// transitionLogProb returns the log-probability of transitioning from edge pec
// to edge ec given the two resampled track positions. Three cases:
//
//   - Same edge (EdgeIdx matches)         →  0
//   - Shared node                         → -0.5
//   - Disconnected (great-circle penalty) → -max(gcDist/10, 3.0)
func transitionLogProb(
	pec, ec graph.EdgeCandidate,
	from, to resampledPoint,
) float64 {
	if pec.EdgeIdx == ec.EdgeIdx {
		return 0
	}
	// Shared node check.
	if pec.NodeTo == ec.NodeFrom || pec.NodeTo == ec.NodeTo ||
		pec.NodeFrom == ec.NodeFrom || pec.NodeFrom == ec.NodeTo {
		return -0.5
	}
	// Disconnected: penalise by great-circle distance between the projected
	// foot-points (approximated with equirectangular).
	gcDist := equirectDistMeters(from.lat, from.lng, to.lat, to.lng)
	return -max(gcDist/10.0, 3.0)
}
