package coverage

import (
	"math"

	"github.com/paulmach/orb"
)

// equirectDist returns approximate distance in meters using equirectangular projection.
// ~5× faster than Haversine, <0.3% error within 100km at mid-latitudes.
// Used in tight decay loops where millions of distance calculations dominate runtime.
func equirectDist(p1, p2 orb.Point, cosLat float64) float64 {
	dx := (p2[0] - p1[0]) * cosLat
	dy := p2[1] - p1[1]
	return math.Sqrt(dx*dx+dy*dy) * math.Pi / 180.0 * earthRadiusM
}

// ApplyUrbanizationDecay adds population-based decay gradient.
// BRouter formula: 10000 * pop * cos(lat) / (town_radius + 500 + dist)²
// Uses max() aggregation: overlapping cities take the higher value, not additive.
// This prevents saturation (255) in metropolitan areas with nearby cities.
func ApplyUrbanizationDecay(g *CoverageGrid, settlement orb.Point, population int64) {
	if population <= 0 {
		return
	}

	pop := float64(population)
	lat := settlement[1]
	townRadius := 8.0 * math.Sqrt(pop)
	cosLat := math.Cos(lat * math.Pi / 180.0)
	numerator := 10000.0 * pop * cosLat

	effectiveRadiusM := max(1000, min(100000, math.Sqrt(numerator/0.5)-townRadius-500))

	rowRadius := int(math.Ceil(effectiveRadiusM / (g.LatStep * math.Pi / 180.0 * earthRadiusM)))
	colRadius := int(math.Ceil(effectiveRadiusM / (g.LonStep * math.Pi / 180.0 * earthRadiusM * cosLat)))

	centerRow, centerCol := g.PointToCell(settlement)
	if centerRow < 0 {
		centerRow = int((lat - g.Bound.Min[1]) / g.LatStep)
		centerCol = int((settlement[0] - g.Bound.Min[0]) / g.LonStep)
	}

	for row := max(0, centerRow-rowRadius); row <= min(g.Rows-1, centerRow+rowRadius); row++ {
		for col := max(0, centerCol-colRadius); col <= min(g.Cols-1, centerCol+colRadius); col++ {
			dist := equirectDist(settlement, g.CellCenter(row, col), cosLat)
			denom := townRadius + 500 + dist
			contribution := numerator / (denom * denom)
			val := uint8(min(255, contribution))
			idx := row*g.Cols + col
			// max() aggregation: nearby cities don't saturate, largest contribution wins
			if val > g.Data[idx] {
				g.Data[idx] = val
			}
		}
	}
}

// ApplyForestWaterOverride reduces (not zeroes) urbanization where forest/water present.
// Stadtpark in Berlin → urban × 0.3, not urban = 0.
func ApplyForestWaterOverride(urban, forest, water *CoverageGrid) {
	if urban == nil {
		return
	}
	for i := range urban.Data {
		if urban.Data[i] == 0 {
			continue
		}
		center := urban.CellCenter(i/urban.Cols, i%urban.Cols)
		var isForest, isWater bool
		if forest != nil {
			v := forest.SamplePoint(center)
			isForest = v > 0 && v != OutOfBounds
		}
		if water != nil {
			v := water.SamplePoint(center)
			isWater = v > 0 && v != OutOfBounds
		}
		if isForest || isWater {
			urban.Data[i] = uint8(float64(urban.Data[i]) * 0.3)
		}
	}
}

// ApplyNoiseDecay adds inverse-distance decay from a point source to the noise grid.
// Decay formula: 255 * offset / (dist + offset) — inverse linear (1/r for large r).
// Uses quadratic combination: sqrt(existing² + new²) — energy addition for uncorrelated sources.
func ApplyNoiseDecay(g *CoverageGrid, source orb.Point, bufferM, offsetM float64) {
	lat := source[1]
	cosLat := math.Cos(lat * math.Pi / 180.0)
	rowRadius := int(math.Ceil(bufferM / (g.LatStep * math.Pi / 180.0 * earthRadiusM)))
	colRadius := int(math.Ceil(bufferM / (g.LonStep * math.Pi / 180.0 * earthRadiusM * cosLat)))

	centerRow, centerCol := g.PointToCell(source)
	if centerRow < 0 {
		centerRow = int((lat - g.Bound.Min[1]) / g.LatStep)
		centerCol = int((source[0] - g.Bound.Min[0]) / g.LonStep)
	}

	for row := max(0, centerRow-rowRadius); row <= min(g.Rows-1, centerRow+rowRadius); row++ {
		for col := max(0, centerCol-colRadius); col <= min(g.Cols-1, centerCol+colRadius); col++ {
			dist := equirectDist(source, g.CellCenter(row, col), cosLat)
			if dist > bufferM {
				continue
			}
			decay := 255.0 * offsetM / (dist + offsetM)
			existing := float64(g.Data[row*g.Cols+col])
			combined := math.Sqrt(existing*existing + decay*decay)
			g.Data[row*g.Cols+col] = uint8(min(255, combined))
		}
	}
}

// ApplyNoiseDecayLine applies noise decay along a line source (road, railway).
// Instead of sampling points and applying circular decay per point (O(n×k²)),
// this rasterizes the line into the grid first (marking cells the line passes through),
// then applies distance-based decay outward. This reduces redundant cell writes
// when adjacent sample points have overlapping decay circles.
func ApplyNoiseDecayLine(g *CoverageGrid, ls orb.LineString, bufferM, offsetM float64) {
	if len(ls) < 2 {
		return
	}

	// Step 1: Rasterize line into a temporary seed grid (which cells does the line cross?)
	// Use Bresenham-like walking along segments, marking cells at grid resolution.
	cosLat := math.Cos(((g.Bound.Min[1] + g.Bound.Max[1]) / 2) * math.Pi / 180.0)
	bufferRows := int(math.Ceil(bufferM / (g.LatStep * math.Pi / 180.0 * earthRadiusM)))
	bufferCols := int(math.Ceil(bufferM / (g.LonStep * math.Pi / 180.0 * earthRadiusM * cosLat)))

	// Collect seed cells (cells the line passes through)
	type cellPos struct{ row, col int }
	seen := make(map[cellPos]bool)
	var seeds []cellPos

	// Walk along each segment, sampling at grid resolution intervals
	for si := 0; si < len(ls)-1; si++ {
		p0, p1 := ls[si], ls[si+1]
		segLen := equirectDist(p0, p1, cosLat)
		if segLen < 0.1 {
			continue
		}
		// Sample at half-grid resolution to avoid gaps
		nSteps := max(1, int(math.Ceil(segLen/(g.ResM*0.5))))
		for s := 0; s <= nSteps; s++ {
			frac := float64(s) / float64(nSteps)
			pt := orb.Point{
				p0[0] + frac*(p1[0]-p0[0]),
				p0[1] + frac*(p1[1]-p0[1]),
			}
			r, c := g.PointToCell(pt)
			if r < 0 {
				continue
			}
			cp := cellPos{r, c}
			if !seen[cp] {
				seen[cp] = true
				seeds = append(seeds, cp)
			}
		}
	}

	if len(seeds) == 0 {
		return
	}

	// Step 2: For each cell in the buffer zone around all seeds, compute distance
	// to nearest seed and apply decay. This is O(seeds × buffer_area) but avoids
	// the O(n_points × buffer_area) of the old approach where n_points >> seeds.
	for _, seed := range seeds {
		seedCenter := g.CellCenter(seed.row, seed.col)
		for row := max(0, seed.row-bufferRows); row <= min(g.Rows-1, seed.row+bufferRows); row++ {
			for col := max(0, seed.col-bufferCols); col <= min(g.Cols-1, seed.col+bufferCols); col++ {
				dist := equirectDist(seedCenter, g.CellCenter(row, col), cosLat)
				if dist > bufferM {
					continue
				}
				decay := 255.0 * offsetM / (dist + offsetM)
				idx := row*g.Cols + col
				existing := float64(g.Data[idx])
				combined := math.Sqrt(existing*existing + decay*decay)
				g.Data[idx] = uint8(min(255, combined))
			}
		}
	}
}

// PopulationFallback returns estimated population when OSM tag is missing.
func PopulationFallback(placeType string) int64 {
	switch placeType {
	case "city":
		return 100000
	case "town":
		return 10000
	case "village":
		return 500
	case "hamlet":
		return 50
	case "isolated_dwelling":
		return 5
	default:
		return 0
	}
}

// PopulationPlausible checks BRouter-style sanity (village > 90k = broken data).
func PopulationPlausible(placeType string, pop int64) bool {
	switch placeType {
	case "village":
		return pop <= 90000
	case "town":
		return pop <= 1500000
	case "city":
		return pop <= 40000000
	default:
		return true
	}
}
