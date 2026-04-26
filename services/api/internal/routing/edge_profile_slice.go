// edge_profile_slice.go — Fraction-based slicing of EdgeElevProf profiles.
package routing

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
