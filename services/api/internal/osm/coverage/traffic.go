package coverage

import (
	"context"
	"fmt"
	"log/slog"
	"math"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/paulmach/orb"
	"github.com/paulmach/orb/encoding/wkb"

	"github.com/skadia-app/skadia/services/api/internal/routing/trafficformula"
)

// ClassifyTraffic maps raw traffic score to BRouter 6-class system.
// Returns 0 for not classified (raw <= 0), 1-6 for traffic classes.
// Delegates to trafficformula.Classify to avoid import cycle with graph/.
func ClassifyTraffic(raw float64) uint8 {
	return trafficformula.Classify(raw)
}

// TrafficRaw computes BRouter's division formula.
// peak_sum_ele = 0 (DEM not available, TODO Stufe 3b).
// Delegates to trafficformula.Raw to avoid import cycle with graph/.
func TrafficRaw(populate, industrial, motorwayDensity, highwayDensity float64) float64 {
	return trafficformula.Raw(populate, industrial, motorwayDensity, highwayDensity)
}

// floatGrid is a temporary float64 grid for intermediate calculations.
// Unlike CoverageGrid (uint8), it avoids premature clamping and supports negative values.
// Used for populate_factor, industrial_factor, motorway/highway density — freed after traffic build.
type floatGrid struct {
	Bound            orb.Bound
	Rows, Cols       int
	LatStep, LonStep float64
	Data             []float64
}

func newFloatGrid(bound orb.Bound, resolutionM float64) *floatGrid {
	centerLat := (bound.Min[1] + bound.Max[1]) / 2.0
	latSpanM := (bound.Max[1] - bound.Min[1]) * math.Pi / 180.0 * earthRadiusM
	lonSpanM := (bound.Max[0] - bound.Min[0]) * math.Pi / 180.0 * earthRadiusM * math.Cos(centerLat*math.Pi/180.0)
	rows := max(1, int(math.Ceil(latSpanM/resolutionM)))
	cols := max(1, int(math.Ceil(lonSpanM/resolutionM)))
	return &floatGrid{
		Bound: bound, Rows: rows, Cols: cols,
		LatStep: (bound.Max[1] - bound.Min[1]) / float64(rows),
		LonStep: (bound.Max[0] - bound.Min[0]) / float64(cols),
		Data:    make([]float64, rows*cols),
	}
}

func (g *floatGrid) cellCenter(row, col int) orb.Point {
	return orb.Point{
		g.Bound.Min[0] + (float64(col)+0.5)*g.LonStep,
		g.Bound.Min[1] + (float64(row)+0.5)*g.LatStep,
	}
}

func (g *floatGrid) pointToCell(p orb.Point) (int, int) {
	lon, lat := p[0], p[1]
	if lat < g.Bound.Min[1] || lat >= g.Bound.Max[1] || lon < g.Bound.Min[0] || lon >= g.Bound.Max[0] {
		return -1, -1
	}
	row := min(g.Rows-1, int((lat-g.Bound.Min[1])/g.LatStep))
	col := min(g.Cols-1, int((lon-g.Bound.Min[0])/g.LonStep))
	return row, col
}

func (g *floatGrid) sample(p orb.Point) float64 {
	row, col := g.pointToCell(p)
	if row < 0 {
		return 0
	}
	return g.Data[row*g.Cols+col]
}

// trafficInputs holds the 4 float64 intermediate grids that survive until Way-Sampling.
// enrichWaysFromGrids uses them to compute BRouter-correct per-Way traffic_class with
// highway_weight applied only to populate (not industrial).
type trafficInputs struct {
	populate, industrial *floatGrid
	mwDensity, hwDensity *floatGrid
}

func (ti *trafficInputs) release() {
	ti.populate = nil
	ti.industrial = nil
	ti.mwDensity = nil
	ti.hwDensity = nil
}

// buildTrafficInputs builds the 4 float64 intermediate grids AND fills LayerTraffic
// with hwWeight=1.0 (for Phase 2 CoverageReader). Returns the inputs for Way-Sampling.
func buildTrafficInputs(ctx context.Context, pool *pgxpool.Pool, tg *TileGrids, env string) (*trafficInputs, error) {
	bound := tg.MustGet(LayerUrbanization).Bound

	// --- 1. Populate factor (additive float64, 0.5° expansion for large cities) ---
	populateGrid := newFloatGrid(bound, 100)
	expandedEnv := fmt.Sprintf("ST_Expand(%s, 0.5)", env)

	settRows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT ST_AsBinary(geom), subtype,
			COALESCE(CASE WHEN tags->>'population' ~ '^\d+$'
				THEN (tags->>'population')::bigint END, 0)
		FROM osm_pois WHERE category = 'settlement' AND geom && %s`, expandedEnv))
	if err != nil {
		return nil, fmt.Errorf("populate query: %w", err)
	}
	defer settRows.Close()
	for settRows.Next() {
		var wkbBytes []byte
		var subtype string
		var pop int64
		if err := settRows.Scan(&wkbBytes, &subtype, &pop); err != nil {
			continue
		}
		geom, err := wkb.Unmarshal(wkbBytes)
		if err != nil {
			continue
		}
		pt, ok := geom.(orb.Point)
		if !ok {
			continue
		}
		if pop <= 0 || !PopulationPlausible(subtype, pop) {
			pop = PopulationFallback(subtype)
		}
		if pop <= 0 {
			continue
		}
		applyPopulateDecayFloat(populateGrid, pt, pop)
	}
	if err := settRows.Err(); err != nil {
		return nil, fmt.Errorf("populate rows: %w", err)
	}

	// --- 2. Industrial factor (area-based decay, float64) + Aerodromes ---
	industrialGrid := newFloatGrid(bound, 100)

	indRows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT ST_AsBinary(ST_Centroid(geom)), ST_Area(geom::geography)
		FROM osm_landuse
		WHERE landuse IN ('industrial', 'retail')
		  AND ST_Area(geom::geography) > 20000
		  AND geom && %s`, env))
	if err != nil {
		return nil, fmt.Errorf("industrial query: %w", err)
	}
	defer indRows.Close()
	for indRows.Next() {
		var wkbBytes []byte
		var area float64
		if err := indRows.Scan(&wkbBytes, &area); err != nil {
			continue
		}
		geom, err := wkb.Unmarshal(wkbBytes)
		if err != nil {
			continue
		}
		pt, ok := geom.(orb.Point)
		if !ok {
			continue
		}
		applyIndustrialDecayFloat(industrialGrid, pt, area)
	}
	if err := indRows.Err(); err != nil {
		return nil, fmt.Errorf("industrial rows: %w", err)
	}

	// --- 3. Motorway density (10km, float64) ---
	mwGrid := newFloatGrid(bound, 10000)
	if err := buildDensityGridFloat(ctx, pool, mwGrid, env,
		"('motorway','motorway_link','trunk','trunk_link')"); err != nil {
		return nil, fmt.Errorf("motorway density: %w", err)
	}

	// --- 4. Highway density (10km, float64) ---
	hwGrid := newFloatGrid(bound, 10000)
	if err := buildDensityGridFloat(ctx, pool, hwGrid, env,
		"('primary','primary_link','secondary','secondary_link','tertiary','tertiary_link')"); err != nil {
		return nil, fmt.Errorf("highway density: %w", err)
	}

	// Aerodromes (large airports generate significant road traffic — BRouter RE-05)
	aeroRows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT ST_AsBinary(geom), COALESCE(ST_Area(geom::geography), 500000)
		FROM osm_pois WHERE category = 'aerodrome' AND COALESCE(subtype,'') != 'gliding'
		AND geom && %s`, env))
	if err != nil {
		slog.Warn("coverage: aerodrome query failed (non-fatal)", "err", err)
	} else {
		defer aeroRows.Close()
		for aeroRows.Next() {
			var wkbBytes []byte
			var area float64
			if err := aeroRows.Scan(&wkbBytes, &area); err != nil {
				continue
			}
			geom, err := wkb.Unmarshal(wkbBytes)
			if err != nil {
				continue
			}
			pt, ok := geom.(orb.Point)
			if !ok {
				continue
			}
			if area < 20000 {
				area = 500000
			}
			applyIndustrialDecayFloat(industrialGrid, pt, area)
		}
		if err := aeroRows.Err(); err != nil {
			slog.Warn("coverage: aerodrome rows iteration error (non-fatal)", "err", err)
		}
	}

	// --- 5. Auffahrten-Override ---
	if err := applyMotorwayJunctionOverride(ctx, pool, mwGrid, env); err != nil {
		slog.Warn("coverage: motorway junction override failed (non-fatal)", "err", err)
	}

	// --- 6. Fill LayerTraffic with hwWeight=1.0 for Phase 2 CoverageReader ---
	trafficGrid := tg.MustGet(LayerTraffic)
	for row := range trafficGrid.Rows {
		for col := range trafficGrid.Cols {
			center := trafficGrid.CellCenter(row, col)
			pop := populateGrid.sample(center)
			ind := industrialGrid.sample(center)
			mw := mwGrid.sample(center)
			hw := hwGrid.sample(center)
			raw := TrafficRaw(pop*1.0, ind, mw, hw)
			trafficGrid.Data[row*trafficGrid.Cols+col] = ClassifyTraffic(raw)
		}
	}

	slog.Info("coverage: traffic inputs built")
	return &trafficInputs{
		populate:   populateGrid,
		industrial: industrialGrid,
		mwDensity:  mwGrid,
		hwDensity:  hwGrid,
	}, nil
}

// applyPopulateDecayFloat is like ApplyUrbanizationDecay but for float64 grid (additive, no clamping).
func applyPopulateDecayFloat(g *floatGrid, settlement orb.Point, population int64) {
	pop := float64(population)
	lat := settlement[1]
	townRadius := 8.0 * math.Sqrt(pop)
	cosLat := math.Cos(lat * math.Pi / 180.0)
	numerator := 10000.0 * pop * cosLat
	effectiveRadiusM := max(1000.0, min(100000.0, math.Sqrt(numerator/0.5)-townRadius-500))
	rowRadius := int(math.Ceil(effectiveRadiusM / (g.LatStep * math.Pi / 180.0 * earthRadiusM)))
	colRadius := int(math.Ceil(effectiveRadiusM / (g.LonStep * math.Pi / 180.0 * earthRadiusM * cosLat)))
	centerRow, centerCol := g.pointToCell(settlement)
	if centerRow < 0 {
		centerRow = int((lat - g.Bound.Min[1]) / g.LatStep)
		centerCol = int((settlement[0] - g.Bound.Min[0]) / g.LonStep)
	}
	for row := max(0, centerRow-rowRadius); row <= min(g.Rows-1, centerRow+rowRadius); row++ {
		for col := max(0, centerCol-colRadius); col <= min(g.Cols-1, centerCol+colRadius); col++ {
			dist := equirectDist(settlement, g.cellCenter(row, col), cosLat)
			denom := townRadius + 500 + dist
			g.Data[row*g.Cols+col] += numerator / (denom * denom)
		}
	}
}

// applyIndustrialDecayFloat: area / (sqrt(area) + 500 + dist)², float64 grid.
func applyIndustrialDecayFloat(g *floatGrid, center orb.Point, areaM2 float64) {
	sqrtArea := math.Sqrt(areaM2)
	cosLat := math.Cos(center[1] * math.Pi / 180.0)
	bufferM := 12860.0
	rowRadius := int(math.Ceil(bufferM / (g.LatStep * math.Pi / 180.0 * earthRadiusM)))
	colRadius := int(math.Ceil(bufferM / (g.LonStep * math.Pi / 180.0 * earthRadiusM * cosLat)))
	centerRow, centerCol := g.pointToCell(center)
	if centerRow < 0 {
		centerRow = int((center[1] - g.Bound.Min[1]) / g.LatStep)
		centerCol = int((center[0] - g.Bound.Min[0]) / g.LonStep)
	}
	for row := max(0, centerRow-rowRadius); row <= min(g.Rows-1, centerRow+rowRadius); row++ {
		for col := max(0, centerCol-colRadius); col <= min(g.Cols-1, centerCol+colRadius); col++ {
			dist := equirectDist(center, g.cellCenter(row, col), cosLat)
			if dist > bufferM {
				continue
			}
			denom := sqrtArea + 500 + dist
			g.Data[row*g.Cols+col] += areaM2 / (denom * denom)
		}
	}
}

// buildDensityGridFloat accumulates road lengths per 10km cell, normalized by /6430.
func buildDensityGridFloat(ctx context.Context, pool *pgxpool.Pool, g *floatGrid, env, highwayTypes string) error {
	rows, err := pool.Query(ctx, fmt.Sprintf(
		"SELECT ST_AsBinary(geom) FROM osm_ways WHERE highway IN %s AND geom && %s",
		highwayTypes, env))
	if err != nil {
		return err
	}
	defer rows.Close()
	cosLat := math.Cos(((g.Bound.Min[1] + g.Bound.Max[1]) / 2) * math.Pi / 180.0)
	for rows.Next() {
		var wkbBytes []byte
		if err := rows.Scan(&wkbBytes); err != nil {
			continue
		}
		geom, err := wkb.Unmarshal(wkbBytes)
		if err != nil {
			continue
		}
		ls, ok := geom.(orb.LineString)
		if !ok {
			continue
		}
		for si := 0; si < len(ls)-1; si++ {
			p0, p1 := ls[si], ls[si+1]
			segLen := equirectDist(p0, p1, cosLat)
			mid := orb.Point{(p0[0] + p1[0]) / 2, (p0[1] + p1[1]) / 2}
			row, col := g.pointToCell(mid)
			if row < 0 {
				continue
			}
			g.Data[row*g.Cols+col] += segLen / 6430.0
		}
	}
	return rows.Err()
}

// resampleFloatToGrid resamples a float64 grid (typically 10km) to a uint8
// CoverageGrid at the given resolution. Each target cell gets the value of
// its parent cell in the source grid (nearest-neighbor, ratio-based lookup).
// Values are clamped to [0, 255]. Spec C16: keeps memory bounded.
func resampleFloatToGrid(src *floatGrid, bound orb.Bound, targetResM float64) *CoverageGrid {
	if src == nil {
		return nil
	}
	dst := NewGrid(bound, targetResM)
	for row := range dst.Rows {
		lat := dst.Bound.Min[1] + (float64(row)+0.5)*dst.LatStep
		srcRow := int((lat - src.Bound.Min[1]) / src.LatStep)
		if srcRow < 0 || srcRow >= src.Rows {
			continue
		}
		for col := range dst.Cols {
			lon := dst.Bound.Min[0] + (float64(col)+0.5)*dst.LonStep
			srcCol := int((lon - src.Bound.Min[0]) / src.LonStep)
			if srcCol < 0 || srcCol >= src.Cols {
				continue
			}
			val := src.Data[srcRow*src.Cols+srcCol]
			dst.Data[row*dst.Cols+col] = uint8(min(255, max(0, val)))
		}
	}
	return dst
}

// applyMotorwayJunctionOverride: BRouter sets motorway_factor = -15 near junctions.
func applyMotorwayJunctionOverride(ctx context.Context, pool *pgxpool.Pool, mwGrid *floatGrid, env string) error {
	rows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT DISTINCT ST_AsBinary(pt) FROM (
			SELECT ST_StartPoint(ml.geom) AS pt FROM osm_ways ml
			WHERE ml.highway = 'motorway_link' AND ml.geom && %s
			  AND EXISTS (
				SELECT 1 FROM osm_ways hw
				WHERE hw.highway IN ('primary','secondary','tertiary',
				                     'primary_link','secondary_link','tertiary_link')
				  AND ST_DWithin(ST_StartPoint(ml.geom), hw.geom, 0.0005)
			  )
			UNION
			SELECT ST_EndPoint(ml.geom) AS pt FROM osm_ways ml
			WHERE ml.highway = 'motorway_link' AND ml.geom && %s
			  AND EXISTS (
				SELECT 1 FROM osm_ways hw
				WHERE hw.highway IN ('primary','secondary','tertiary',
				                     'primary_link','secondary_link','tertiary_link')
				  AND ST_DWithin(ST_EndPoint(ml.geom), hw.geom, 0.0005)
			  )
		) sub`, env, env))
	if err != nil {
		return err
	}
	defer rows.Close()
	cosLat := math.Cos(((mwGrid.Bound.Min[1] + mwGrid.Bound.Max[1]) / 2) * math.Pi / 180.0)
	for rows.Next() {
		var wkbBytes []byte
		if err := rows.Scan(&wkbBytes); err != nil {
			continue
		}
		geom, err := wkb.Unmarshal(wkbBytes)
		if err != nil {
			continue
		}
		pt, ok := geom.(orb.Point)
		if !ok {
			continue
		}
		jRow, jCol := mwGrid.pointToCell(pt)
		if jRow < 0 {
			continue
		}
		rowR2000 := int(math.Ceil(2000 / (mwGrid.LatStep * math.Pi / 180.0 * earthRadiusM)))
		colR2000 := int(math.Ceil(2000 / (mwGrid.LonStep * math.Pi / 180.0 * earthRadiusM * cosLat)))
		for row := max(0, jRow-rowR2000); row <= min(mwGrid.Rows-1, jRow+rowR2000); row++ {
			for col := max(0, jCol-colR2000); col <= min(mwGrid.Cols-1, jCol+colR2000); col++ {
				dist := equirectDist(pt, mwGrid.cellCenter(row, col), cosLat)
				idx := row*mwGrid.Cols + col
				if dist <= 1000 {
					mwGrid.Data[idx] = -15
				} else if dist <= 2000 {
					mwGrid.Data[idx] = 0
				}
			}
		}
	}
	return rows.Err()
}
