package coverage

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/paulmach/orb"

	graphpkg "github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// TileGridCache implements CoverageReader by building coverage grids on-demand
// and caching them per tile. Used by the graph builder to eliminate the
// DB roundtrip for enrichment values.
//
// NOT goroutine-safe by design — one tile-build runs sequentially per tile.
// Grids are built BEFORE the graph-build and freed AFTER.
type TileGridCache struct {
	pool     *pgxpool.Pool
	noiseCfg NoiseSettings
	grids    map[uint32]*enrichedTileGrids
}

type enrichedTileGrids struct {
	coverage *TileGrids
	// Traffic: 10km float64 grids resampled to 100m CoverageGrids (Spec C16)
	// Resampling keeps memory bounded (~4 × 1 MB uint8 statt ~4 × 144 MB float64)
	populate   *CoverageGrid // 100m, uint8, resampled from 10km float64
	industrial *CoverageGrid // 100m, uint8, resampled
	mwDensity  *CoverageGrid // 100m, uint8, resampled
	hwDensity  *CoverageGrid // 100m, uint8, resampled
}

// NewTileGridCache creates a cache that builds coverage grids on-demand.
func NewTileGridCache(pool *pgxpool.Pool, noiseCfg NoiseSettings) *TileGridCache {
	return &TileGridCache{
		pool:     pool,
		noiseCfg: noiseCfg,
		grids:    make(map[uint32]*enrichedTileGrids),
	}
}

// PreloadTile builds coverage grids for a tile. Idempotent.
func (c *TileGridCache) PreloadTile(ctx context.Context, tileID uint32) error {
	if _, ok := c.grids[tileID]; ok {
		return nil // already loaded
	}

	tg, trafficIn, err := buildGridsForTile(ctx, c.pool, tileID, c.noiseCfg)
	if err != nil {
		return fmt.Errorf("coverage tilecache: build grids for tile %d: %w", tileID, err)
	}

	// Apply forest/water override on urbanization (from existing builder.go, intentionally
	// included here even though Spec §8.4 omits it — Spec oversight, existing code is correct)
	ApplyForestWaterOverride(
		tg.Get(LayerUrbanization),
		tg.Get(LayerForest),
		tg.Get(LayerWater),
	)

	// Resample 10km float64 traffic grids to 100m CoverageGrids (Spec C16).
	// Keeps memory bounded: ~4 MB uint8 statt ~576 MB float64.
	bbox := graphpkg.TileBBoxExpanded(tileID, 0.1)
	bound := tileBBoxToBound(bbox)
	var pop100, ind100, mw100, hw100 *CoverageGrid
	if trafficIn != nil {
		pop100 = resampleFloatToGrid(trafficIn.populate, bound, 100)
		ind100 = resampleFloatToGrid(trafficIn.industrial, bound, 100)
		mw100 = resampleFloatToGrid(trafficIn.mwDensity, bound, 100)
		hw100 = resampleFloatToGrid(trafficIn.hwDensity, bound, 100)
		trafficIn.release() // free original 10km float64 grids
	}

	c.grids[tileID] = &enrichedTileGrids{
		coverage:   tg,
		populate:   pop100,
		industrial: ind100,
		mwDensity:  mw100,
		hwDensity:  hw100,
	}
	return nil
}

// SampleWay samples all coverage layers + traffic inputs along a way geometry.
func (c *TileGridCache) SampleWay(geom orb.LineString) CoverageResult {
	if len(geom) < 2 {
		return CoverageResult{}
	}

	mid := geom[len(geom)/2]
	tileID := graphpkg.TileIDFromCoord(mid[1], mid[0])

	tg, ok := c.grids[tileID]
	if !ok {
		return CoverageResult{}
	}

	cov := tg.coverage

	fSamples := cov.MustGet(LayerForest).SampleLine(geom, 25)
	pSamples := cov.MustGet(LayerProtected).SampleLine(geom, 50)
	lSamples := cov.MustGet(LayerLanduse).SampleLine(geom, 50)
	nSamples := cov.MustGet(LayerNoise).SampleLine(geom, 50)
	uSamples := cov.MustGet(LayerUrbanization).SampleLine(geom, 50)
	rSamples := cov.MustGet(LayerRiverProximity).SampleLine(geom, 25)
	wSamples := cov.MustGet(LayerWater).SampleLine(geom, 25)

	// Traffic: am Midpoint samplen (regionale Eigenschaft)
	midPt := orb.Point{mid[0], mid[1]}

	result := CoverageResult{
		ForestPct:      PctNonZero(fSamples),
		ProtectedArea:  WeightedAvg(pSamples),
		LanduseClass:   Mode(lSamples),
		NoiseScore:     min(100, MaxVal(nSamples)*100/255),
		Urbanization:   WeightedAvg(uSamples),
		RiverProximity: WeightedAvg(rSamples),
		Water:          anyNonZero(wSamples),
	}

	// Traffic raw inputs from resampled 100m CoverageGrids
	if tg.populate != nil {
		result.TrafficPopulate = float64(tg.populate.SamplePoint(midPt))
	}
	if tg.industrial != nil {
		result.TrafficIndustrial = float64(tg.industrial.SamplePoint(midPt))
	}
	if tg.mwDensity != nil {
		result.TrafficMWDensity = float64(tg.mwDensity.SamplePoint(midPt))
	}
	if tg.hwDensity != nil {
		result.TrafficHWDensity = float64(tg.hwDensity.SamplePoint(midPt))
	}

	return result
}

// Release frees all cached grids for a tile.
func (c *TileGridCache) Release(tileID uint32) {
	tg, ok := c.grids[tileID]
	if !ok {
		return
	}
	if tg.coverage != nil {
		tg.coverage.Release()
	}
	for _, g := range []*CoverageGrid{tg.populate, tg.industrial, tg.mwDensity, tg.hwDensity} {
		if g != nil {
			g.Release()
		}
	}
	delete(c.grids, tileID)
}

// ReleaseAll frees all cached grids.
func (c *TileGridCache) ReleaseAll() {
	for tileID := range c.grids {
		c.Release(tileID)
	}
}

// anyNonZero returns true if any sample is >0 (ignoring OutOfBounds).
func anyNonZero(samples []uint8) bool {
	for _, v := range samples {
		if v != OutOfBounds && v > 0 {
			return true
		}
	}
	return false
}
