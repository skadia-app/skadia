// edge_profile_slice.go — Fraction-based slicing of EdgeElevProf profiles
// and unified coord+profile slicing for the same-edge routing special case.
package routing

import (
	"github.com/skadia-app/skadia/services/api/internal/dem"
	"github.com/skadia-app/skadia/services/api/internal/geo"
)

// SliceEdgeElevProf returns the sub-profile for the fraction range [fStart, fEnd]
// of a full-edge profile. Endpoints are linearly interpolated from surrounding
// profile points when they fall between recorded samples. The distance axis of
// the returned slice starts at 0.
//
// Behaviour:
//   - nil/empty input → nil output
//   - fStart/fEnd are clamped to [0, 1]
//   - fStart > fEnd → swapped (not an error)
//   - fStart == fEnd → single interpolated point at DistM 0
//   - nil EleM on either neighbour of an interpolated point → nil result (NoData propagation)
func SliceEdgeElevProf(prof []ElevationPoint, fStart, fEnd float64) []ElevationPoint {
	if len(prof) == 0 {
		return nil
	}

	// Clamp and normalise range.
	if fStart < 0 {
		fStart = 0
	}
	if fEnd > 1 {
		fEnd = 1
	}
	if fStart > fEnd {
		fStart, fEnd = fEnd, fStart
	}

	totalDist := prof[len(prof)-1].DistM
	distStart := fStart * totalDist
	distEnd := fEnd * totalDist

	// interpEle performs linear interpolation at absolute distance d.
	// Returns nil when either bounding sample has nil EleM (NoData propagation).
	interpEle := func(d float64) *float64 {
		// Handle edge cases: before first point or at/beyond last point.
		if d <= prof[0].DistM {
			return prof[0].EleM
		}
		if d >= prof[len(prof)-1].DistM {
			return prof[len(prof)-1].EleM
		}
		for i := 1; i < len(prof); i++ {
			if prof[i].DistM >= d {
				d0 := prof[i-1].DistM
				d1 := prof[i].DistM
				e0 := prof[i-1].EleM
				e1 := prof[i].EleM
				if e0 == nil || e1 == nil {
					return nil // NoData propagation
				}
				if d1 == d0 {
					return e0
				}
				t := (d - d0) / (d1 - d0)
				v := *e0 + t*(*e1-*e0)
				return &v
			}
		}
		return prof[len(prof)-1].EleM
	}

	var out []ElevationPoint

	// Always emit the interpolated start point at DistM 0.
	out = append(out, ElevationPoint{DistM: 0, EleM: interpEle(distStart)})

	// Emit interior profile points that fall strictly inside (distStart, distEnd).
	for _, p := range prof {
		if p.DistM > distStart && p.DistM < distEnd {
			out = append(out, ElevationPoint{
				DistM: p.DistM - distStart,
				EleM:  p.EleM,
			})
		}
	}

	// Emit the interpolated end point only for non-degenerate ranges.
	if distEnd > distStart {
		out = append(out, ElevationPoint{
			DistM: distEnd - distStart,
			EleM:  interpEle(distEnd),
		})
	}

	return out
}

// buildEdgeProfFromRaw converts the raw (elevsDm, segDistsCm) arrays returned
// by GraphReader.EdgeElevProfile into a []ElevationPoint with cumulative DistM
// in meters and EleM in meters. dem.NoData decimeters propagate as nil EleM
// (Spec Rev 2.3 §5.3 — gap-fill phase D resolves these later). Mirrors the
// conversion in buildVeTailProfile so both paths share identical NoData
// handling and defensive segDist length tolerance.
func buildEdgeProfFromRaw(elevsDm []int32, segDistsCm []uint16) []ElevationPoint {
	if len(elevsDm) == 0 {
		return nil
	}
	prof := make([]ElevationPoint, len(elevsDm))
	var dM float64
	for i, e := range elevsDm {
		if i > 0 && i-1 < len(segDistsCm) {
			dM += float64(segDistsCm[i-1]) / 100.0
		}
		prof[i].DistM = dM
		if e == dem.NoData {
			prof[i].EleM = nil
		} else {
			prof[i].EleM = PtrEle(float64(e) / 10.0)
		}
	}
	return prof
}

// sliceEdgeCoordsAndProfile slices an edge's geometry and elevation profile to
// the fraction range [fStart, fEnd]. Both outputs use a unified vertex set
// derived from the geometry vertices in the range, plus interpolated endpoints,
// so len(slicedCoords) == len(slicedProfile) by construction. Profile elevations
// are interpolated from the input profile at each unified vertex's fraction.
//
// coords is ([2]int32{lat, lon} ×1e7) representing the FULL edge polyline,
// i.e. [NodeA, ...intermediate geom..., NodeB]. The fraction axis is built by
// haversine cumulative distance over coords (the displayed-geometry truth).
// prof is the pre-built []ElevationPoint from EdgeElevProfile (cumulative DistM
// from segDistsCm; EleM = nil for dem.NoData decimeters). prof's distance axis
// may differ from the haversine cumulative axis (independent DP simplification),
// so prof is sampled by *fraction* — fr = profDist / profTotal — at each
// unified vertex's coord-fraction.
//
// Returns nil/nil for empty input. fStart/fEnd are clamped to [0, 1] and
// swapped if fStart > fEnd. Caller handles reversed-traversal externally.
//
// LatLonEle invariants:
//   - Lat/Lon are populated from the geometry (interpolated linearly between
//     two surrounding geom vertices for the two endpoints).
//   - Ele = *EleM (in meters) when the corresponding profile sample is non-nil,
//     EleValid = true. Otherwise Ele = 0.0, EleValid = false. This mirrors the
//     contract that applyProfileToCoords would establish for normal routes.
func sliceEdgeCoordsAndProfile(
	coords [][2]int32, prof []ElevationPoint, fStart, fEnd float64,
) (slicedCoords []LatLonEle, slicedProfile []ElevationPoint) {
	if len(coords) == 0 {
		return nil, nil
	}

	// Clamp + normalise range.
	if fStart < 0 {
		fStart = 0
	}
	if fEnd > 1 {
		fEnd = 1
	}
	if fStart > fEnd {
		fStart, fEnd = fEnd, fStart
	}

	// Single-vertex coord polyline — degenerate edge. Emit one point.
	if len(coords) == 1 {
		latF := float64(coords[0][0]) / 1e7
		lonF := float64(coords[0][1]) / 1e7
		var (
			ele      float64
			eleValid bool
			eleM     *float64
		)
		if interp := interpEleAtFraction(prof, fStart); interp != nil {
			ele = *interp
			eleValid = true
			eleM = interp
		}
		return []LatLonEle{{Lat: latF, Lon: lonF, Ele: ele, EleValid: eleValid}},
			[]ElevationPoint{{DistM: 0, EleM: eleM}}
	}

	// Build coord-axis cumulative distances (haversine).
	coordDists := make([]float64, len(coords))
	for i := 1; i < len(coords); i++ {
		coordDists[i] = coordDists[i-1] + geo.DistanceHaversine(
			float64(coords[i-1][0])/1e7, float64(coords[i-1][1])/1e7,
			float64(coords[i][0])/1e7, float64(coords[i][1])/1e7,
		)
	}
	coordTotal := coordDists[len(coordDists)-1]
	if coordTotal == 0 {
		// All coords colocated — degenerate. Emit single point.
		latF := float64(coords[0][0]) / 1e7
		lonF := float64(coords[0][1]) / 1e7
		var (
			ele      float64
			eleValid bool
			eleM     *float64
		)
		if interp := interpEleAtFraction(prof, fStart); interp != nil {
			ele = *interp
			eleValid = true
			eleM = interp
		}
		return []LatLonEle{{Lat: latF, Lon: lonF, Ele: ele, EleValid: eleValid}},
			[]ElevationPoint{{DistM: 0, EleM: eleM}}
	}

	distStart := fStart * coordTotal
	distEnd := fEnd * coordTotal

	// Find segment index containing distStart and a vertex range strictly
	// inside (distStart, distEnd). interpolateLatLon at a target distance:
	//   - locate i such that coordDists[i-1] <= d <= coordDists[i]
	//   - linearly interpolate lat/lon (×1e7) between coords[i-1] and coords[i]
	interpolateLatLon := func(d float64) (float64, float64) {
		if d <= 0 {
			return float64(coords[0][0]) / 1e7, float64(coords[0][1]) / 1e7
		}
		if d >= coordTotal {
			n := len(coords) - 1
			return float64(coords[n][0]) / 1e7, float64(coords[n][1]) / 1e7
		}
		for i := 1; i < len(coords); i++ {
			if coordDists[i] >= d {
				d0 := coordDists[i-1]
				d1 := coordDists[i]
				if d1 == d0 {
					return float64(coords[i-1][0]) / 1e7, float64(coords[i-1][1]) / 1e7
				}
				t := (d - d0) / (d1 - d0)
				lat := float64(coords[i-1][0])/1e7 +
					t*(float64(coords[i][0])/1e7-float64(coords[i-1][0])/1e7)
				lon := float64(coords[i-1][1])/1e7 +
					t*(float64(coords[i][1])/1e7-float64(coords[i-1][1])/1e7)
				return lat, lon
			}
		}
		n := len(coords) - 1
		return float64(coords[n][0]) / 1e7, float64(coords[n][1]) / 1e7
	}

	// Build the unified master vertex list.
	type vertex struct {
		lat, lon float64
		dist     float64 // cumulative distance from distStart along the coord axis
		fraction float64 // coord-fraction (dist / coordTotal) used to sample prof
	}
	var vs []vertex

	// Start vertex.
	startLat, startLon := interpolateLatLon(distStart)
	vs = append(vs, vertex{lat: startLat, lon: startLon, dist: 0, fraction: fStart})

	// Interior original geom vertices strictly between distStart and distEnd.
	for i, d := range coordDists {
		if d > distStart && d < distEnd {
			vs = append(vs, vertex{
				lat:      float64(coords[i][0]) / 1e7,
				lon:      float64(coords[i][1]) / 1e7,
				dist:     d - distStart,
				fraction: d / coordTotal,
			})
		}
	}

	// End vertex (skipped on degenerate range — fStart == fEnd).
	if distEnd > distStart {
		endLat, endLon := interpolateLatLon(distEnd)
		vs = append(vs, vertex{
			lat: endLat, lon: endLon,
			dist: distEnd - distStart, fraction: fEnd,
		})
	}

	slicedCoords = make([]LatLonEle, len(vs))
	slicedProfile = make([]ElevationPoint, len(vs))
	for i, v := range vs {
		eleM := interpEleAtFraction(prof, v.fraction)
		slicedProfile[i] = ElevationPoint{DistM: v.dist, EleM: eleM}
		var (
			eleVal   float64
			eleValid bool
		)
		if eleM != nil {
			eleVal = *eleM
			eleValid = true
		}
		slicedCoords[i] = LatLonEle{
			Lat:      v.lat,
			Lon:      v.lon,
			Ele:      eleVal,
			EleValid: eleValid,
		}
	}
	return slicedCoords, slicedProfile
}

// interpEleAtFraction linearly interpolates an EleM value out of prof at the
// given fraction fr ∈ [0, 1] of prof's own distance axis. Returns nil when
// either bounding sample has nil EleM (NoData propagation), when prof is empty,
// or when prof's distance axis is degenerate.
func interpEleAtFraction(prof []ElevationPoint, fr float64) *float64 {
	n := len(prof)
	if n == 0 {
		return nil
	}
	if fr < 0 {
		fr = 0
	}
	if fr > 1 {
		fr = 1
	}
	if n == 1 {
		return prof[0].EleM
	}
	profTotal := prof[n-1].DistM
	if profTotal <= 0 {
		// Degenerate — fall back to first sample.
		return prof[0].EleM
	}
	d := fr * profTotal
	if d <= prof[0].DistM {
		return prof[0].EleM
	}
	if d >= prof[n-1].DistM {
		return prof[n-1].EleM
	}
	for i := 1; i < n; i++ {
		if prof[i].DistM >= d {
			d0 := prof[i-1].DistM
			d1 := prof[i].DistM
			e0 := prof[i-1].EleM
			e1 := prof[i].EleM
			if e0 == nil || e1 == nil {
				return nil
			}
			if d1 == d0 {
				return e0
			}
			t := (d - d0) / (d1 - d0)
			v := *e0 + t*(*e1-*e0)
			return &v
		}
	}
	return prof[n-1].EleM
}

// reverseLatLonEle reverses a LatLonEle slice in place (returns the same slice
// with reversed order). Used by routeSameEdge when src/dst-fractions imply
// traversal opposite to A→B edge orientation.
func reverseLatLonEle(s []LatLonEle) []LatLonEle {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
	return s
}
