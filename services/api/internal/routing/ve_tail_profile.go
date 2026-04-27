// ve_tail_profile.go — Builds high-resolution elevation profiles for the partial
// edge between a VirtualEndpoint's projection point and the seed node A* used.
// Replaces the previous linear NodeA↔NodeB interpolation with a DEM-accurate
// slice from EdgeElevProf via SliceEdgeElevProf.
package routing

import (
	"github.com/skadia-app/skadia/services/api/internal/dem"
)

// veTailProfileReader is the minimal subset of GraphReader needed by
// buildVeTailProfile. Defined locally so tests can supply a small fake.
type veTailProfileReader interface {
	EdgeElevProfile(handle uint64) ([]int32, []uint16) // decimeters, cm
}

// buildVeTailProfile constructs the elevation profile for the partial edge
// between a VirtualEndpoint's projection point and the seed node used by A*.
// The returned profile has DistM[0]==0 and ascending DistM, oriented in the
// same direction as the coordinate sequence produced by extractPartialEdgeGeometry:
//
//   - isSource=true  → projection → seed direction (matches src-VE prepend)
//   - isSource=false → seed → projection direction (matches dst-VE append)
//
// Returns nil when the edge has no DEM profile.
//
// Direction logic (verified against extractPartialEdgeGeometry, unified.go:769):
//
//	             |  isSource=true       |  isSource=false
//	  seedIsA    |  proj(f)→A(0): rev   |  A(0)→proj(f): natural
//	  seedIsB    |  proj(f)→B(1): nat   |  B(1)→proj(f): rev
//
// "Natural" = SliceEdgeElevProf direction (DistM ascends with edge fraction).
// "Reverse" = traversal goes from high fraction to low fraction; we flip the
// slice and re-base DistM so it ascends from 0 in the traversal direction.
func buildVeTailProfile(reader veTailProfileReader, ve *VirtualEndpoint,
	seedNode uint64, isSource bool) []ElevationPoint {

	if ve == nil {
		return nil
	}

	elevs, segDistsCm := reader.EdgeElevProfile(ve.Proj.EdgeHandle)
	if len(elevs) == 0 {
		return nil
	}

	// Build a full-edge ElevationPoint profile from the raw EdgeElevProf
	// arrays. Cumulative distance from segDistsCm (cm → m). NoData sentinel
	// elevations propagate as nil EleM (Spec Rev 2.3 §5.3 — gap-fill phase D
	// will resolve these; for now keep them nil so applyProfileToCoords leaves
	// the coord's prior elevation untouched).
	prof := make([]ElevationPoint, len(elevs))
	var dM float64
	for i, e := range elevs {
		if i > 0 {
			// Defensive: segDistsCm may be shorter than expected on malformed
			// profiles. Treat missing entries as zero so we never index OOB —
			// the slice will then collapse to a degenerate single-point profile
			// but won't crash.
			if i-1 < len(segDistsCm) {
				dM += float64(segDistsCm[i-1]) / 100.0
			}
		}
		prof[i].DistM = dM
		if e == dem.NoData {
			prof[i].EleM = nil
		} else {
			prof[i].EleM = PtrEle(float64(e) / 10.0)
		}
	}

	// Single-point edge → return as-is; reversal/slicing has no effect.
	if len(prof) == 1 {
		return prof
	}

	f := ve.Proj.FractionAB
	seedIsA := seedNode == ve.GlobalA

	var fStart, fEnd float64
	var reverse bool
	switch {
	case isSource && seedIsA:
		// proj(f) → A(0): slice [0, f], then reverse so DistM=0 at proj.
		fStart, fEnd = 0.0, f
		reverse = true
	case isSource && !seedIsA:
		// proj(f) → B(1): slice [f, 1], natural direction.
		fStart, fEnd = f, 1.0
		reverse = false
	case !isSource && seedIsA:
		// A(0) → proj(f): slice [0, f], natural direction.
		fStart, fEnd = 0.0, f
		reverse = false
	default: // !isSource && !seedIsA
		// B(1) → proj(f): slice [f, 1], then reverse so DistM=0 at B=seed.
		fStart, fEnd = f, 1.0
		reverse = true
	}

	sliced := SliceEdgeElevProf(prof, fStart, fEnd)
	if reverse {
		sliced = reverseElevSlice(sliced)
	}
	return sliced
}

// reverseElevSlice flips an ElevationPoint slice and re-bases DistM so it
// ascends from 0 in the reversed direction. Preserves nil EleM samples.
//
// Example: [{0, 100}, {50, 110}, {120, 130}]  →  [{0, 130}, {70, 110}, {120, 100}]
func reverseElevSlice(prof []ElevationPoint) []ElevationPoint {
	n := len(prof)
	if n == 0 {
		return prof
	}
	last := prof[n-1].DistM
	out := make([]ElevationPoint, n)
	for i, p := range prof {
		out[n-1-i] = ElevationPoint{
			DistM: last - p.DistM,
			EleM:  p.EleM,
		}
	}
	return out
}

// concatProfilesWithJunctionDedup concatenates per-segment elevation profiles
// into a single route-wide profile, advancing the cumulative DistM axis and
// dropping the duplicated junction point at each boundary (the previous
// segment's last point and the next segment's first point coincide at the
// shared network node).
//
// Empty parts are skipped without consuming a junction-dedup. The very first
// non-empty part is emitted in full; every subsequent non-empty part has its
// first sample dropped (the junction is owned by the previous part).
func concatProfilesWithJunctionDedup(parts ...[]ElevationPoint) []ElevationPoint {
	// Pre-size: sum of lengths is an upper bound; we trim by len(parts)-1
	// junction drops at most.
	var total int
	for _, p := range parts {
		total += len(p)
	}
	if total == 0 {
		return nil
	}
	out := make([]ElevationPoint, 0, total)

	var distAccum float64
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		startIdx := 0
		if len(out) > 0 {
			startIdx = 1 // junction-dedup
		}
		for j := startIdx; j < len(p); j++ {
			out = append(out, ElevationPoint{
				DistM: distAccum + p[j].DistM,
				EleM:  p[j].EleM,
			})
		}
		distAccum += p[len(p)-1].DistM
	}
	return out
}
