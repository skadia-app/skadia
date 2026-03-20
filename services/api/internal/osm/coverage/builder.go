package coverage

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/paulmach/orb"
	"github.com/paulmach/orb/encoding/wkb"
	"golang.org/x/sync/errgroup"

	graphpkg "github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// ---------------------------------------------------------------------------
// Noise configuration
// ---------------------------------------------------------------------------

// NoiseSourceConfig controls whether a noise category is enabled and its decay buffer.
type NoiseSourceConfig struct {
	Enabled bool    `json:"enabled"`
	BufferM float64 `json:"buffer_m"`
}

// NoiseSettings holds per-category noise configuration (persisted in users.noise_settings).
type NoiseSettings struct {
	Road       NoiseSourceConfig `json:"road"`
	Rail       NoiseSourceConfig `json:"rail"`
	Air        NoiseSourceConfig `json:"air"`
	Wind       NoiseSourceConfig `json:"wind"`
	Shooting   NoiseSourceConfig `json:"shooting"`
	Military   NoiseSourceConfig `json:"military"`
	Industrial NoiseSourceConfig `json:"industrial"`
}

// DefaultNoiseSettings returns the default noise configuration.
func DefaultNoiseSettings() NoiseSettings {
	return NoiseSettings{
		Road:       NoiseSourceConfig{true, 500},
		Rail:       NoiseSourceConfig{true, 300},
		Air:        NoiseSourceConfig{true, 3000},
		Wind:       NoiseSourceConfig{true, 800},
		Shooting:   NoiseSourceConfig{false, 400},
		Military:   NoiseSourceConfig{true, 500},
		Industrial: NoiseSourceConfig{true, 200},
	}
}

// noiseOffsets maps each noise category to its Weber-Fechner offset parameter.
var noiseOffsets = map[string]float64{
	"road": 15, "rail": 25, "air": 100, "wind": 50,
	"shooting": 30, "military": 40, "industrial": 20,
}

// ---------------------------------------------------------------------------
// TileGrids — all coverage grids for one 1°×1° tile (map-based registry)
// ---------------------------------------------------------------------------

// TileGrids bundles all coverage grids built for a single tile.
// Uses a map-based registry instead of named fields for extensibility.
type TileGrids struct {
	Layers map[string]*CoverageGrid
}

func NewTileGrids() *TileGrids {
	return &TileGrids{Layers: make(map[string]*CoverageGrid)}
}

// MustGet returns the grid for the given layer. Panics with clear message if not found.
func (tg *TileGrids) MustGet(name string) *CoverageGrid {
	g, ok := tg.Layers[name]
	if !ok || g == nil {
		panic(fmt.Sprintf("coverage: layer %q not found in TileGrids", name))
	}
	return g
}

// Get returns the grid for the given layer, or nil if not present.
func (tg *TileGrids) Get(name string) *CoverageGrid {
	return tg.Layers[name]
}

// Set adds or replaces a grid in the registry.
func (tg *TileGrids) Set(name string, g *CoverageGrid) {
	tg.Layers[name] = g
}

// Release returns all grid buffers to the pool.
func (tg *TileGrids) Release() {
	for _, g := range tg.Layers {
		if g != nil {
			g.Release()
		}
	}
}

// ---------------------------------------------------------------------------
// RunCoverageEnrichment — top-level entry point
// ---------------------------------------------------------------------------

// ProgressFunc is called after each tile completes: (done, total).
type ProgressFunc func(done, total int)

// RunCoverageEnrichment discovers active 1°×1° tiles and enriches all un-enriched
// osm_ways within each tile. bboxClause can restrict the area (e.g.
// " AND ST_Intersects(geom, ST_MakeEnvelope(..., 4326))") or be empty.
func RunCoverageEnrichment(ctx context.Context, pool *pgxpool.Pool, bboxClause string, workers int, progress ProgressFunc) error {
	// bboxClause from enrichment.go uses alias "w.geom" — coverage queries use bare "geom".
	coverageBBox := strings.Replace(bboxClause, "w.geom", "geom", 1)

	tiles, err := discoverActiveTiles(ctx, pool, coverageBBox)
	if err != nil {
		return fmt.Errorf("discover tiles: %w", err)
	}
	if len(tiles) == 0 {
		slog.Info("coverage: no active tiles found")
		return nil
	}
	slog.Info("coverage: discovered tiles", "count", len(tiles))

	ns, err := loadNoiseSettings(ctx, pool)
	if err != nil {
		slog.Warn("coverage: failed to load noise settings, using defaults", "err", err)
		ns = DefaultNoiseSettings()
	}

	if workers <= 0 {
		workers = 4
	}

	var done atomic.Int64
	total := len(tiles)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(workers)

	for _, tileID := range tiles {
		tid := tileID
		g.Go(func() error {
			if err := gctx.Err(); err != nil {
				return err
			}
			if err := processTile(gctx, pool, tid, ns, coverageBBox); err != nil {
				slog.Warn("coverage: tile failed, continuing", "tile", tid, "err", err)
				// Continue on tile failure — do not abort the whole run.
			}
			n := done.Add(1)
			if progress != nil {
				progress(int(n), total)
			}
			return nil
		})
	}

	return g.Wait()
}

// processTile builds grids for a tile and enriches its ways.
func processTile(ctx context.Context, pool *pgxpool.Pool, tileID uint32, ns NoiseSettings, bboxClause string) error {
	grids, trafficIn, err := buildGridsForTile(ctx, pool, tileID, ns)
	if err != nil {
		return fmt.Errorf("build grids tile %d: %w", tileID, err)
	}
	defer grids.Release()
	if trafficIn != nil {
		defer trafficIn.release()
	}

	if err := enrichWaysFromGrids(ctx, pool, tileID, grids, trafficIn, bboxClause); err != nil {
		return fmt.Errorf("enrich ways tile %d: %w", tileID, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// discoverActiveTiles
// ---------------------------------------------------------------------------

func discoverActiveTiles(ctx context.Context, pool *pgxpool.Pool, bboxClause string) ([]uint32, error) {
	q := `SELECT DISTINCT ((floor(ST_Y(ST_PointOnSurface(geom)))::int + 90) * 360
		+ (floor(ST_X(ST_PointOnSurface(geom)))::int + 180))::int
		FROM osm_ways WHERE highway IS NOT NULL` + bboxClause

	rows, err := pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tiles []uint32
	for rows.Next() {
		var tid int
		if err := rows.Scan(&tid); err != nil {
			return nil, err
		}
		tiles = append(tiles, uint32(tid))
	}
	return tiles, rows.Err()
}

// ---------------------------------------------------------------------------
// buildGridsForTile
// ---------------------------------------------------------------------------

func buildGridsForTile(ctx context.Context, pool *pgxpool.Pool, tileID uint32, ns NoiseSettings) (*TileGrids, *trafficInputs, error) {
	// 0.1° expansion so urbanization decay reaches into neighboring tiles.
	bbox := graphpkg.TileBBoxExpanded(tileID, 0.1)
	bound := tileBBoxToBound(bbox)
	env := tileBBoxToEnvelope(bbox)

	tg := NewTileGrids()

	// Medium grids (50 m) — sufficient for way-level aggregation, 4× less memory than 25m
	tg.Set(LayerForest, NewGrid(bound, 50))
	tg.Set(LayerLanduse, NewGrid(bound, 50))
	tg.Set(LayerWater, NewGrid(bound, 50))
	tg.Set(LayerRiverProximity, NewGrid(bound, 50))

	// Coarse grids (100 m)
	tg.Set(LayerProtected, NewGrid(bound, 100))
	tg.Set(LayerUrbanization, NewGrid(bound, 100))
	tg.Set(LayerNoise, NewGrid(bound, 100))

	// --- Forest ---
	if err := rasterizePolygonsFromDB(ctx, pool, tg.MustGet(LayerForest),
		fmt.Sprintf(`SELECT ST_AsBinary((ST_Dump(geom)).geom) FROM osm_forests WHERE geom && %s`, env),
		1, false); err != nil {
		return nil, nil, fmt.Errorf("forest: %w", err)
	}

	// --- Water ---
	if err := rasterizePolygonsFromDB(ctx, pool, tg.MustGet(LayerWater),
		fmt.Sprintf(`SELECT ST_AsBinary((ST_Dump(geom)).geom) FROM osm_waters WHERE geom && %s`, env),
		1, false); err != nil {
		return nil, nil, fmt.Errorf("water: %w", err)
	}

	// --- Landuse (multiple passes) ---
	landuseTypes := []struct {
		filter string
		val    uint8
	}{
		{"farmland", 2}, {"residential", 3}, {"industrial", 4},
		{"commercial", 5}, {"quarry", 6}, {"military", 7}, {"landfill", 8},
	}
	for _, lt := range landuseTypes {
		if err := rasterizePolygonsFromDB(ctx, pool, tg.MustGet(LayerLanduse),
			fmt.Sprintf(`SELECT ST_AsBinary((ST_Dump(geom)).geom) FROM osm_landuse WHERE landuse = '%s' AND geom && %s`, lt.filter, env),
			lt.val, false); err != nil {
			return nil, nil, fmt.Errorf("landuse %s: %w", lt.filter, err)
		}
	}

	// --- Protected areas ---
	for _, pc := range []struct {
		clause string
		val    uint8
	}{
		{"protect_class IN ('1','1a','1b')", 3},
		{"protect_class IN ('2','3','4')", 2},
		{"protect_class IS NOT NULL AND protect_class NOT IN ('1','1a','1b','2','3','4')", 1},
	} {
		if err := rasterizePolygonsFromDB(ctx, pool, tg.MustGet(LayerProtected),
			fmt.Sprintf(`SELECT ST_AsBinary((ST_Dump(geom)).geom) FROM osm_boundaries WHERE %s AND geom && %s`, pc.clause, env),
			pc.val, true); err != nil {
			return nil, nil, fmt.Errorf("protected: %w", err)
		}
	}

	// --- Urbanization ---
	if err := buildUrbanizationGrid(ctx, pool, tg, env); err != nil {
		return nil, nil, fmt.Errorf("urbanization: %w", err)
	}

	// --- River proximity ---
	if err := buildRiverProximityGrid(ctx, pool, tg.MustGet(LayerRiverProximity), env); err != nil {
		return nil, nil, fmt.Errorf("river proximity: %w", err)
	}

	// --- Noise ---
	if err := buildNoiseGrid(ctx, pool, tg.MustGet(LayerNoise), env, ns); err != nil {
		return nil, nil, fmt.Errorf("noise: %w", err)
	}

	// --- Traffic ---
	tg.Set(LayerTraffic, NewGrid(bound, 100))
	trafficIn, err := buildTrafficInputs(ctx, pool, tg, env)
	if err != nil {
		return nil, nil, fmt.Errorf("traffic: %w", err)
	}

	return tg, trafficIn, nil
}

// ---------------------------------------------------------------------------
// Urbanization
// ---------------------------------------------------------------------------

func buildUrbanizationGrid(ctx context.Context, pool *pgxpool.Pool, tg *TileGrids, env string) error {
	q := fmt.Sprintf(`SELECT ST_AsBinary(geom), subtype,
		COALESCE(CASE WHEN tags->>'population' ~ '^\d+$'
			THEN (tags->>'population')::bigint END, 0)
		FROM osm_pois WHERE category = 'settlement' AND geom && %s`, env)
	rows, err := pool.Query(ctx, q)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var wkbBytes []byte
		var subtype string
		var pop int64
		if err := rows.Scan(&wkbBytes, &subtype, &pop); err != nil {
			return err
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
		ApplyUrbanizationDecay(tg.MustGet(LayerUrbanization), pt, pop)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// Forest/water override — reduce urbanization where natural cover exists.
	ApplyForestWaterOverride(tg.MustGet(LayerUrbanization), tg.MustGet(LayerForest), tg.MustGet(LayerWater))
	return nil
}

// ---------------------------------------------------------------------------
// River proximity
// ---------------------------------------------------------------------------

func buildRiverProximityGrid(ctx context.Context, pool *pgxpool.Pool, g *CoverageGrid, env string) error {
	q := fmt.Sprintf(`SELECT ST_AsBinary(geom) FROM osm_waterways WHERE geom && %s`, env)
	return queryLineNoiseSources(ctx, pool, g, q, 165, 50)
}

// ---------------------------------------------------------------------------
// Noise grid
// ---------------------------------------------------------------------------

func buildNoiseGrid(ctx context.Context, pool *pgxpool.Pool, g *CoverageGrid, env string, ns NoiseSettings) error {
	// Road noise (line sources)
	if ns.Road.Enabled {
		q := fmt.Sprintf(`SELECT ST_AsBinary(geom) FROM osm_ways
			WHERE highway IN ('motorway','motorway_link','trunk','trunk_link','primary','primary_link','secondary','secondary_link')
			AND geom && %s`, env)
		if err := queryLineNoiseSources(ctx, pool, g, q, ns.Road.BufferM, noiseOffsets["road"]); err != nil {
			return fmt.Errorf("noise road: %w", err)
		}
	}

	// Rail noise (line sources)
	if ns.Rail.Enabled {
		q := fmt.Sprintf(`SELECT ST_AsBinary(geom) FROM osm_railways WHERE geom && %s`, env)
		if err := queryLineNoiseSources(ctx, pool, g, q, ns.Rail.BufferM, noiseOffsets["rail"]); err != nil {
			return fmt.Errorf("noise rail: %w", err)
		}
	}

	// Air noise (point sources — aerodromes, excluding gliding)
	if ns.Air.Enabled {
		q := fmt.Sprintf(`SELECT ST_AsBinary(geom) FROM osm_pois
			WHERE category = 'aerodrome' AND COALESCE(subtype, '') != 'gliding'
			AND geom && %s`, env)
		if err := queryPointNoiseSources(ctx, pool, g, q, ns.Air.BufferM, noiseOffsets["air"]); err != nil {
			return fmt.Errorf("noise air: %w", err)
		}
	}

	// Wind noise (point sources — wind turbines)
	if ns.Wind.Enabled {
		q := fmt.Sprintf(`SELECT ST_AsBinary(geom) FROM osm_pois
			WHERE category = 'wind_turbine'
			AND geom && %s`, env)
		if err := queryPointNoiseSources(ctx, pool, g, q, ns.Wind.BufferM, noiseOffsets["wind"]); err != nil {
			return fmt.Errorf("noise wind: %w", err)
		}
	}

	// Shooting noise (point sources)
	if ns.Shooting.Enabled {
		q := fmt.Sprintf(`SELECT ST_AsBinary(geom) FROM osm_pois
			WHERE category = 'shooting_range'
			AND geom && %s`, env)
		if err := queryPointNoiseSources(ctx, pool, g, q, ns.Shooting.BufferM, noiseOffsets["shooting"]); err != nil {
			return fmt.Errorf("noise shooting: %w", err)
		}
	}

	// Military noise (polygon → centroid)
	if ns.Military.Enabled {
		q := fmt.Sprintf(`SELECT ST_AsBinary(ST_Centroid(geom)) FROM osm_landuse
			WHERE landuse = 'military' AND geom && %s`, env)
		if err := queryPointNoiseSources(ctx, pool, g, q, ns.Military.BufferM, noiseOffsets["military"]); err != nil {
			return fmt.Errorf("noise military: %w", err)
		}
	}

	// Industrial noise (polygon → centroid, only if area > 10000 m²)
	if ns.Industrial.Enabled {
		q := fmt.Sprintf(`SELECT ST_AsBinary(ST_Centroid(geom)) FROM osm_landuse
			WHERE landuse = 'industrial' AND ST_Area(geom::geography) > 10000
			AND geom && %s`, env)
		if err := queryPointNoiseSources(ctx, pool, g, q, ns.Industrial.BufferM, noiseOffsets["industrial"]); err != nil {
			return fmt.Errorf("noise industrial: %w", err)
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// Noise source query helpers (avoid defer-in-loop)
// ---------------------------------------------------------------------------

// queryPointNoiseSources reads WKB points and applies decay to g.
func queryPointNoiseSources(ctx context.Context, pool *pgxpool.Pool, g *CoverageGrid, query string, bufferM, offsetM float64) error {
	rows, err := pool.Query(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var wkbBytes []byte
		if err := rows.Scan(&wkbBytes); err != nil {
			return err
		}
		geom, err := wkb.Unmarshal(wkbBytes)
		if err != nil {
			continue
		}
		pt, ok := geom.(orb.Point)
		if !ok {
			continue
		}
		ApplyNoiseDecay(g, pt, bufferM, offsetM)
	}
	return rows.Err()
}

// queryLineNoiseSources reads WKB line strings and applies line decay to g.
func queryLineNoiseSources(ctx context.Context, pool *pgxpool.Pool, g *CoverageGrid, query string, bufferM, offsetM float64) error {
	rows, err := pool.Query(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var wkbBytes []byte
		if err := rows.Scan(&wkbBytes); err != nil {
			return err
		}
		geom, err := wkb.Unmarshal(wkbBytes)
		if err != nil {
			continue
		}
		ls, ok := geom.(orb.LineString)
		if !ok {
			continue
		}
		ApplyNoiseDecayLine(g, ls, bufferM, offsetM)
	}
	return rows.Err()
}

// ---------------------------------------------------------------------------
// enrichWaysFromGrids — sample ways against grids and batch-write results
// ---------------------------------------------------------------------------

const batchSize = 5000

func enrichWaysFromGrids(ctx context.Context, pool *pgxpool.Pool, tileID uint32, tg *TileGrids, trafficIn *trafficInputs, bboxClause string) error {
	bbox := graphpkg.TileBBox(tileID)
	env := tileBBoxToEnvelope(bbox)

	q := fmt.Sprintf(`SELECT way_id, ST_AsBinary(geom), highway FROM osm_ways
		WHERE highway IS NOT NULL AND forest_pct IS NULL
		AND ST_Intersects(ST_PointOnSurface(geom), %s)%s`, env, bboxClause)

	rows, err := pool.Query(ctx, q)
	if err != nil {
		return err
	}
	defer rows.Close()

	type wayResult struct {
		wayID          int64
		forestPct      int
		protectedArea  int
		landuseClass   int
		noiseScore     int
		urbanization   int
		riverProximity int
		trafficClass   int
	}

	batch := make([]wayResult, 0, batchSize)

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		wayIDs := make([]int64, len(batch))
		forests := make([]int16, len(batch))
		protecteds := make([]int16, len(batch))
		landuses := make([]int16, len(batch))
		noises := make([]int16, len(batch))
		urbans := make([]int16, len(batch))
		rivers := make([]int16, len(batch))
		traffics := make([]int16, len(batch))
		for i, r := range batch {
			wayIDs[i] = r.wayID
			forests[i] = int16(r.forestPct)
			protecteds[i] = int16(r.protectedArea)
			landuses[i] = int16(r.landuseClass)
			noises[i] = int16(r.noiseScore)
			urbans[i] = int16(r.urbanization)
			rivers[i] = int16(r.riverProximity)
			traffics[i] = int16(r.trafficClass)
		}
		_, err := pool.Exec(ctx, `UPDATE osm_ways AS w SET
			forest_pct = v.forest_pct,
			protected_area = v.protected_area,
			landuse_class = v.landuse_class,
			noise_score = v.noise_score,
			urbanization = v.urbanization,
			river_proximity = v.river_proximity,
			traffic_class = v.traffic_class
			FROM (SELECT unnest($1::bigint[]) AS way_id,
				unnest($2::smallint[]) AS forest_pct,
				unnest($3::smallint[]) AS protected_area,
				unnest($4::smallint[]) AS landuse_class,
				unnest($5::smallint[]) AS noise_score,
				unnest($6::smallint[]) AS urbanization,
				unnest($7::smallint[]) AS river_proximity,
				unnest($8::smallint[]) AS traffic_class
			) v WHERE w.way_id = v.way_id`,
			wayIDs, forests, protecteds, landuses, noises, urbans, rivers, traffics)
		if err != nil {
			return fmt.Errorf("batch update: %w", err)
		}
		batch = batch[:0]
		return nil
	}

	for rows.Next() {
		var wayID int64
		var wkbBytes []byte
		var highway string
		if err := rows.Scan(&wayID, &wkbBytes, &highway); err != nil {
			return err
		}
		geom, err := wkb.Unmarshal(wkbBytes)
		if err != nil {
			continue
		}
		ls, ok := geom.(orb.LineString)
		if !ok {
			continue
		}

		forestSamples := tg.MustGet(LayerForest).SampleLine(ls, 50)
		protectedSamples := tg.MustGet(LayerProtected).SampleLine(ls, 100)
		landuseSamples := tg.MustGet(LayerLanduse).SampleLine(ls, 50)
		noiseSamples := tg.MustGet(LayerNoise).SampleLine(ls, 100)
		urbanSamples := tg.MustGet(LayerUrbanization).SampleLine(ls, 100)
		riverSamples := tg.MustGet(LayerRiverProximity).SampleLine(ls, 50)

		noiseMax := MaxVal(noiseSamples)

		// Traffic — BRouter-correct formula with highway_weight only on populate
		var hwWeight float64
		switch {
		case strings.HasPrefix(highway, "primary"):
			hwWeight = 1.0
		case strings.HasPrefix(highway, "secondary"):
			hwWeight = 0.6
		case strings.HasPrefix(highway, "tertiary"):
			hwWeight = 0.4
		}

		trafficClass := 0
		if hwWeight > 0 && trafficIn != nil {
			mid := orb.Point{(ls[0][0] + ls[len(ls)-1][0]) / 2, (ls[0][1] + ls[len(ls)-1][1]) / 2}
			pop := trafficIn.populate.sample(mid)
			ind := trafficIn.industrial.sample(mid)
			mw := trafficIn.mwDensity.sample(mid)
			hw := trafficIn.hwDensity.sample(mid)
			raw := TrafficRaw(pop*hwWeight, ind, mw, hw)
			trafficClass = int(ClassifyTraffic(raw))
		}

		forestPct := PctNonZero(forestSamples)
		landuseClass := Mode(landuseSamples)

		// landuse_class derivation (moved from EnrichLanduseClass)
		if forestPct >= 60 && landuseClass == 0 {
			landuseClass = 1
		}
		if landuseClass == 0 && (highway == "residential" || highway == "living_street") {
			landuseClass = 3
		}

		batch = append(batch, wayResult{
			wayID:          wayID,
			forestPct:      forestPct,
			protectedArea:  WeightedAvg(protectedSamples),
			landuseClass:   landuseClass,
			noiseScore:     noiseMax * 100 / 255,
			urbanization:   WeightedAvg(urbanSamples),
			riverProximity: WeightedAvg(riverSamples),
			trafficClass:   trafficClass,
		})

		if len(batch) >= batchSize {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return flush()
}

// ---------------------------------------------------------------------------
// Helper: load noise settings
// ---------------------------------------------------------------------------

func loadNoiseSettings(_ context.Context, _ *pgxpool.Pool) (NoiseSettings, error) {
	// Phase 1 enrichment is global (baked into osm_ways) — always use defaults.
	// Per-user noise_settings (Migration 000184) apply at routing time via
	// CoverageReader (Phase 2), where the router reads user-specific weights.
	return DefaultNoiseSettings(), nil
}

// ---------------------------------------------------------------------------
// Helper: coordinate conversion
// ---------------------------------------------------------------------------

// tileBBoxToBound converts a TileBBox [minLat, minLon, maxLat, maxLon] to orb.Bound.
// orb.Bound uses {lon, lat} (X=lon, Y=lat).
func tileBBoxToBound(bbox [4]float64) orb.Bound {
	return orb.Bound{
		Min: orb.Point{bbox[1], bbox[0]}, // lon, lat
		Max: orb.Point{bbox[3], bbox[2]}, // lon, lat
	}
}

// tileBBoxToEnvelope returns an ST_MakeEnvelope SQL string for a bbox.
func tileBBoxToEnvelope(bbox [4]float64) string {
	// ST_MakeEnvelope(xmin, ymin, xmax, ymax, srid)
	// xmin=minLon, ymin=minLat, xmax=maxLon, ymax=maxLat
	return fmt.Sprintf("ST_MakeEnvelope(%f, %f, %f, %f, 4326)",
		bbox[1], bbox[0], bbox[3], bbox[2])
}

// ---------------------------------------------------------------------------
// Helper: rasterize polygons from DB
// ---------------------------------------------------------------------------

// rasterizePolygonsFromDB streams WKB polygons from a query and rasterizes each one.
// If useMax is true, uses RasterizePolygonMax (max-write semantics).
func rasterizePolygonsFromDB(ctx context.Context, pool *pgxpool.Pool, g *CoverageGrid, query string, val uint8, useMax bool) error {
	rows, err := pool.Query(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var wkbBytes []byte
		if err := rows.Scan(&wkbBytes); err != nil {
			return err
		}
		geom, err := wkb.Unmarshal(wkbBytes)
		if err != nil {
			continue
		}
		poly, ok := geom.(orb.Polygon)
		if !ok {
			continue
		}
		if useMax {
			RasterizePolygonMax(g, poly, val)
		} else {
			RasterizePolygon(g, poly, val)
		}
	}
	return rows.Err()
}
