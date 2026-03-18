package coverage

import (
	"math"
	"sync"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geo"
)

const OutOfBounds uint8 = 0xFF
const earthRadiusM = 6_371_000.0

// gridPool recycles Data slices to reduce GC pressure (700 tiles = 26 GB allocs).
var gridPool = sync.Pool{
	New: func() any { return []uint8(nil) },
}

type CoverageGrid struct {
	Bound   orb.Bound
	Rows    int
	Cols    int
	ResM    float64
	LatStep float64
	LonStep float64
	Data    []uint8
}

func NewGrid(bbox orb.Bound, resolutionM float64) *CoverageGrid {
	centerLat := (bbox.Min[1] + bbox.Max[1]) / 2.0
	latSpanM := (bbox.Max[1] - bbox.Min[1]) * math.Pi / 180.0 * earthRadiusM
	lonSpanM := (bbox.Max[0] - bbox.Min[0]) * math.Pi / 180.0 * earthRadiusM * math.Cos(centerLat*math.Pi/180.0)

	rows := max(1, int(math.Ceil(latSpanM/resolutionM)))
	cols := max(1, int(math.Ceil(lonSpanM/resolutionM)))
	size := rows * cols

	// Try to reuse a pooled slice
	data := gridPool.Get().([]uint8)
	if cap(data) >= size {
		data = data[:size]
		clear(data)
	} else {
		data = make([]uint8, size)
	}

	return &CoverageGrid{
		Bound: bbox, Rows: rows, Cols: cols, ResM: resolutionM,
		LatStep: (bbox.Max[1] - bbox.Min[1]) / float64(rows),
		LonStep: (bbox.Max[0] - bbox.Min[0]) / float64(cols),
		Data:    data,
	}
}

// Release returns the Data slice to the pool. Call when the grid is no longer needed.
func (g *CoverageGrid) Release() {
	if g.Data != nil {
		gridPool.Put(g.Data)
		g.Data = nil
	}
}

func (g *CoverageGrid) Set(row, col int, val uint8) {
	if row >= 0 && row < g.Rows && col >= 0 && col < g.Cols {
		g.Data[row*g.Cols+col] = val
	}
}

func (g *CoverageGrid) Get(row, col int) uint8 {
	if row >= 0 && row < g.Rows && col >= 0 && col < g.Cols {
		return g.Data[row*g.Cols+col]
	}
	return 0
}

func (g *CoverageGrid) PointToCell(p orb.Point) (row, col int) {
	lon, lat := p[0], p[1]
	if lat < g.Bound.Min[1] || lat >= g.Bound.Max[1] || lon < g.Bound.Min[0] || lon >= g.Bound.Max[0] {
		return -1, -1
	}
	row = min(g.Rows-1, int((lat-g.Bound.Min[1])/g.LatStep))
	col = min(g.Cols-1, int((lon-g.Bound.Min[0])/g.LonStep))
	return row, col
}

func (g *CoverageGrid) CellCenter(row, col int) orb.Point {
	return orb.Point{
		g.Bound.Min[0] + (float64(col)+0.5)*g.LonStep,
		g.Bound.Min[1] + (float64(row)+0.5)*g.LatStep,
	}
}

func (g *CoverageGrid) SamplePoint(p orb.Point) uint8 {
	row, col := g.PointToCell(p)
	if row < 0 {
		return OutOfBounds
	}
	return g.Get(row, col)
}

// SampleLine samples a LineString at ~intervalM spacing. O(n + m) amortized.
// Pre-computes cumulative distances, then walks segIdx forward (never backward).
func (g *CoverageGrid) SampleLine(ls orb.LineString, intervalM float64) []uint8 {
	if len(ls) < 2 {
		return nil
	}

	// Pre-compute cumulative distances: O(n)
	cumDist := make([]float64, len(ls))
	for i := 1; i < len(ls); i++ {
		cumDist[i] = cumDist[i-1] + geo.DistanceHaversine(ls[i-1], ls[i])
	}
	totalM := cumDist[len(ls)-1]
	if totalM < 1.0 {
		mid := orb.Point{(ls[0][0] + ls[len(ls)-1][0]) / 2, (ls[0][1] + ls[len(ls)-1][1]) / 2}
		return []uint8{g.SamplePoint(mid)}
	}

	nSamples := max(2, min(1000, int(math.Ceil(totalM/intervalM))))
	step := totalM / float64(nSamples)
	results := make([]uint8, 0, nSamples)

	segIdx := 0
	for i := range nSamples {
		targetDist := (float64(i) + 0.5) * step
		// Walk segIdx forward (amortized O(1) per sample)
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
		results = append(results, g.SamplePoint(pt))
	}
	return results
}

// --- Aggregation Helpers (ignore OutOfBounds sentinel) ---

func PctNonZero(samples []uint8) int {
	valid, nonZero := 0, 0
	for _, v := range samples {
		if v == OutOfBounds {
			continue
		}
		valid++
		if v > 0 {
			nonZero++
		}
	}
	if valid == 0 {
		return 0
	}
	return nonZero * 100 / valid
}

func WeightedAvg(samples []uint8) int {
	valid, sum := 0, 0
	for _, v := range samples {
		if v == OutOfBounds {
			continue
		}
		valid++
		sum += int(v)
	}
	if valid == 0 {
		return 0
	}
	return sum / valid
}

func MaxVal(samples []uint8) int {
	best := 0
	for _, v := range samples {
		if v == OutOfBounds {
			continue
		}
		if int(v) > best {
			best = int(v)
		}
	}
	return best
}

func Mode(samples []uint8) int {
	var counts [9]int
	for _, v := range samples {
		if v == OutOfBounds || v > 8 {
			continue
		}
		counts[v]++
	}
	best, bestCount := 0, 0
	for v := 1; v < 9; v++ {
		if counts[v] > bestCount {
			best = v
			bestCount = counts[v]
		}
	}
	return best
}
