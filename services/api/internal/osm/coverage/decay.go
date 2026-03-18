package coverage

import (
	"math"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geo"
)

// ApplyUrbanizationDecay adds population-based decay gradient.
// BRouter formula: 10000 * pop * cos(lat) / (town_radius + 500 + dist)²
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
			dist := geo.DistanceHaversine(settlement, g.CellCenter(row, col))
			denom := townRadius + 500 + dist
			contribution := numerator / (denom * denom)
			idx := row*g.Cols + col
			g.Data[idx] = uint8(min(255, float64(g.Data[idx])+contribution))
		}
	}
}

// ApplyForestWaterOverride reduces (not zeroes) urbanization where forest/water present.
// Stadtpark in Berlin → urban × 0.3, not urban = 0. (Opus-Kritik Fix)
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

// ApplyNoiseDecay adds Weber-Fechner decay from a point source to the noise grid.
// Uses quadratic combination: sqrt(existing² + new²) — physically correct energy addition.
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
			dist := geo.DistanceHaversine(source, g.CellCenter(row, col))
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

// ApplyNoiseDecayLine rasterizes a line source (road, railway) with decay.
func ApplyNoiseDecayLine(g *CoverageGrid, ls orb.LineString, bufferM, offsetM float64) {
	totalM := geo.LengthHaversine(ls)
	if totalM < 1.0 {
		return
	}
	sampleIntervalM := max(g.ResM, 50.0)
	nPoints := max(2, int(math.Ceil(totalM/sampleIntervalM)))
	cumDist := make([]float64, len(ls))
	for i := 1; i < len(ls); i++ {
		cumDist[i] = cumDist[i-1] + geo.DistanceHaversine(ls[i-1], ls[i])
	}
	step := totalM / float64(nPoints)
	segIdx := 0
	for i := range nPoints {
		targetDist := (float64(i) + 0.5) * step
		for segIdx < len(cumDist)-2 && cumDist[segIdx+1] < targetDist {
			segIdx++
		}
		segLen := cumDist[segIdx+1] - cumDist[segIdx]
		frac := 0.0
		if segLen > 0 {
			frac = (targetDist - cumDist[segIdx]) / segLen
		}
		pt := orb.Point{
			ls[segIdx][0] + frac*(ls[segIdx+1][0]-ls[segIdx][0]),
			ls[segIdx][1] + frac*(ls[segIdx+1][1]-ls[segIdx][1]),
		}
		ApplyNoiseDecay(g, pt, bufferM, offsetM)
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
