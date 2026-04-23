package routing

import (
	"log/slog"

	"github.com/skadia-app/skadia/services/api/internal/geo"
)

// buildElevationProfile iterates edge handles and returns the full-resolution
// elevation profile plus per-edge distance anchors. Both retrackUnified and
// BuildRouteResultFromEdges call this. Algorithm per design spec §6.5.
//
// Returns:
//   - profile: cumulative-distance-anchored elevation points, RAW resolution
//   - edgeStartDistM: cumulative distance at the START of edge i (before any of its points)
//   - edgeEndDistM:   cumulative distance at the END of edge i (after its last point)
//
// Both anchors are populated for every edge, including DEM-less edges.
// edgeStartDistM[i] / edgeEndDistM[i] feed buildSegmentsUnified for the
// FromDistM / ToDistM segment fields.
func buildElevationProfile(
	reader graphReaderForElev,
	edgeHandles []uint64,
) (profile []ElevationPoint, edgeStartDistM, edgeEndDistM []float64) {
	// NOTE: reader types are VERIFIED against graphreader.go:191/258 —
	//   EdgeElevProfile returns ([]int32, []uint16)  — elevs in decimeters, segDists in cm
	//   EdgeDistanceCm   returns uint32              — edge length in cm
	nEdges := len(edgeHandles)
	edgeStartDistM = make([]float64, nEdges)
	edgeEndDistM = make([]float64, nEdges)

	var distAccum float64

	for i, edgeHandle := range edgeHandles {
		edgeStartDistM[i] = distAccum

		elevs, segDists := reader.EdgeElevProfile(edgeHandle)
		edgeDistM := float64(reader.EdgeDistanceCm(edgeHandle)) / 100.0

		// Guard 1: no DEM data → still advance distance for §P5 axis continuity
		if len(elevs) == 0 {
			distAccum += edgeDistM
			edgeEndDistM[i] = distAccum
			continue
		}
		// Guard 2: single-point profile
		if len(elevs) == 1 {
			if i == 0 {
				profile = append(profile, ElevationPoint{
					DistM: distAccum,
					EleM:  float64(elevs[0]) / 10.0,
				})
			}
			distAccum += edgeDistM
			edgeEndDistM[i] = distAccum
			continue
		}
		// Guard 3: segDists missing despite >1 elev points
		if segDists == nil || len(segDists) < len(elevs)-1 {
			slog.Warn("buildElevationProfile: nil/short segDists with elevs>1",
				"edgeHandle", edgeHandle, "nElevs", len(elevs))
			distAccum += edgeDistM
			edgeEndDistM[i] = distAccum
			continue
		}

		// First point of edge:
		//   i==0 → emit (route start)
		//   i>0  → suppress (Junction-Dedup with previous edge's last point)
		if i == 0 {
			profile = append(profile, ElevationPoint{
				DistM: distAccum,
				EleM:  float64(elevs[0]) / 10.0,
			})
		}
		// All subsequent points: emit. Distance ALWAYS accumulates including
		// segDists[0] for follow-edges (the dropped junction-point's outgoing
		// segment still belongs to the route distance).
		for k := 1; k < len(elevs); k++ {
			distAccum += float64(segDists[k-1]) / 100.0
			profile = append(profile, ElevationPoint{
				DistM: distAccum,
				EleM:  float64(elevs[k]) / 10.0,
			})
		}
		edgeEndDistM[i] = distAccum
	}
	return profile, edgeStartDistM, edgeEndDistM
}

// InterpolateProfileEleAt returns the elevation at a given cumulative distance
// along the route, linearly interpolating between the two nearest profile points.
//
// EXPORTED so callers in other packages (e.g. handler/route_export.go for the
// GPX-Export defensive path, T2.1) can reuse this lookup without duplicating
// the algorithm.
//
// Behavior (Spec Rev 2.3 §5.2):
//   - len(profile)==0  → returns 0 (callers should null-check the profile first)
//   - distM <= profile[0].DistM           → profile[0].EleM (clamp-start)
//   - distM >= profile[last].DistM        → profile[last].EleM (clamp-end)
//   - otherwise: O(log n) binary-search + linear interpolation between the two surrounding samples.
func InterpolateProfileEleAt(profile []ElevationPoint, distM float64) float64 {
	if len(profile) == 0 {
		return 0
	}
	if distM <= profile[0].DistM {
		return profile[0].EleM
	}
	last := len(profile) - 1
	if distM >= profile[last].DistM {
		return profile[last].EleM
	}
	// Binary search: find largest lo with profile[lo].DistM <= distM
	lo, hi := 0, last
	for lo < hi-1 {
		mid := (lo + hi) / 2
		if profile[mid].DistM <= distM {
			lo = mid
		} else {
			hi = mid
		}
	}
	d0, e0 := profile[lo].DistM, profile[lo].EleM
	d1, e1 := profile[lo+1].DistM, profile[lo+1].EleM
	span := d1 - d0
	if span <= 0 {
		// Defensive: equal/non-monotonic adjacent samples → no interpolation,
		// pick the lower-anchor's elevation (consistent with clamp semantics).
		return e0
	}
	t := (distM - d0) / span
	return e0*(1-t) + e1*t
}

// applyProfileToCoords overwrites coord[i].Ele in-place using
// InterpolateProfileEleAt against cumulative-Haversine distance along coords.
// Distance helper: `geo.DistanceHaversine(lat, lon, lat, lon)` from
// `services/api/internal/geo/distance.go:17` — the only accepted distance
// helper in the plan (Critic-Finding #8).
//
// This is THE single source of truth for inline `coord[2]` after Phase 1
// (Spec Rev 2.3 §5.2, §6.5): both retrackUnified and BuildRouteResultFromEdges
// MUST call this after the profile is built and BEFORE returning the result.
//
// Distance-axis drift caveat (Spec §5.1): coord-side uses Haversine of
// DP-simplified coords, profile-side uses EdgeElevSegDist cumulative sum
// (equirectangular of RAW coords). Drift is typically <1-10m horizontally
// (= <50cm vertically at 5% gradient) — orders of magnitude better than
// the carriedEle bug.
//
// No-op when len(coords)==0 OR len(profile)==0 (callers can pass nil safely).
func applyProfileToCoords(coords []LatLonEle, profile []ElevationPoint) {
	if len(profile) == 0 || len(coords) == 0 {
		return
	}
	var distAccum float64
	for i := range coords {
		if i > 0 {
			// Argument order: (lat1, lon1, lat2, lon2). Verified in
			// services/api/internal/geo/distance.go:17.
			distAccum += geo.DistanceHaversine(
				coords[i-1].Lat, coords[i-1].Lon,
				coords[i].Lat, coords[i].Lon,
			)
		}
		coords[i].Ele = InterpolateProfileEleAt(profile, distAccum)
	}
}

// graphReaderForElev is the minimal subset of graph-reader methods needed
// here. Defined locally so tests can supply a small fake without satisfying
// the full GraphReader interface.
//
// Signatures VERIFIED against graphreader.go:191/258 on 2026-04-23.
type graphReaderForElev interface {
	EdgeElevProfile(handle uint64) ([]int32, []uint16) // decimeters, cm
	EdgeDistanceCm(handle uint64) uint32               // cm
}
