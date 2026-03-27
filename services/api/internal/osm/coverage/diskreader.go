package coverage

import (
	"container/list"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"

	graphpkg "github.com/skadia-app/skadia/services/api/internal/routing/graph"
	"github.com/paulmach/orb"
)

// MappedGrid holds an mmap'd FCOV grid with exported accessors for the tile server.
type MappedGrid struct {
	header  FCOVHeader
	data    []byte
	cleanup func()
	rows    int
	cols    int
	resM    float64
	bound   orb.Bound
	latStep float64
	lonStep float64
}

func (m *MappedGrid) Bound() orb.Bound { return m.bound }
func (m *MappedGrid) Rows() int        { return m.rows }
func (m *MappedGrid) Cols() int        { return m.cols }
func (m *MappedGrid) ResM() float64    { return m.resM }
func (m *MappedGrid) Data() []byte     { return m.data }

type layerTileKey struct {
	layer  string
	tileID uint32
}

type lruEntry struct {
	key  layerTileKey
	grid *MappedGrid
	elem *list.Element
}

type DiskCoverageReader struct {
	baseDir     string
	maxMapped   int
	mu          sync.RWMutex
	mappings    map[layerTileKey]*lruEntry
	lruList     *list.List
	buildTS     string
	buildTSOnce sync.Once
}

func NewDiskCoverageReader(baseDir string, maxMappings int) (*DiskCoverageReader, error) {
	if maxMappings <= 0 {
		maxMappings = 256
	}
	return &DiskCoverageReader{
		baseDir:   baseDir,
		maxMapped: maxMappings,
		mappings:  make(map[layerTileKey]*lruEntry),
		lruList:   list.New(),
	}, nil
}

func (r *DiskCoverageReader) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.mappings {
		e.grid.cleanup()
	}
	r.mappings = make(map[layerTileKey]*lruEntry)
	r.lruList.Init()
}

// tileFilePath returns the filesystem path for a layer/tile, reusing the shared
// tileFilename function from diskwriter.go to keep naming consistent.
func (r *DiskCoverageReader) tileFilePath(layer string, tileID uint32) string {
	bbox := graphpkg.TileBBox(tileID)
	core := orb.Bound{
		Min: orb.Point{bbox[1], bbox[0]},
		Max: orb.Point{bbox[3], bbox[2]},
	}
	return filepath.Join(r.baseDir, layer, tileFilename(core))
}

// loadGrid mmap's a single layer/tile combination. Caller must hold write lock.
func (r *DiskCoverageReader) loadGrid(layer string, tileID uint32) (*MappedGrid, error) {
	key := layerTileKey{layer, tileID}
	if e, ok := r.mappings[key]; ok {
		r.lruList.MoveToBack(e.elem)
		return e.grid, nil
	}

	// Evict oldest entries until under limit.
	for len(r.mappings) >= r.maxMapped && r.lruList.Len() > 0 {
		oldest := r.lruList.Front()
		entry := oldest.Value.(*lruEntry)
		entry.grid.cleanup()
		r.lruList.Remove(oldest)
		delete(r.mappings, entry.key)
	}

	path := r.tileFilePath(layer, tileID)
	header, data, cleanup, err := MmapFCOV(path)
	if err != nil {
		return nil, err
	}

	m := &MappedGrid{
		header:  header,
		data:    data,
		cleanup: cleanup,
		rows:    int(header.Rows),
		cols:    int(header.Cols),
		resM:    float64(header.ResM),
		bound: orb.Bound{
			Min: orb.Point{float64(header.MinLon), float64(header.MinLat)},
			Max: orb.Point{float64(header.MaxLon), float64(header.MaxLat)},
		},
		latStep: (float64(header.MaxLat) - float64(header.MinLat)) / float64(header.Rows),
		lonStep: (float64(header.MaxLon) - float64(header.MinLon)) / float64(header.Cols),
	}

	entry := &lruEntry{key: key, grid: m}
	entry.elem = r.lruList.PushBack(entry)
	r.mappings[key] = entry
	return m, nil
}

// getGrid retrieves or loads a grid. The fast path (cache hit) promotes under
// write lock and re-checks existence to handle concurrent eviction (Fix #5).
func (r *DiskCoverageReader) getGrid(layer string, tileID uint32) (*MappedGrid, error) {
	key := layerTileKey{layer, tileID}

	r.mu.RLock()
	_, ok := r.mappings[key]
	r.mu.RUnlock()
	if ok {
		// Promote to most-recently-used under write lock.
		// Re-check existence: entry may have been evicted between RUnlock and Lock.
		r.mu.Lock()
		if e2, stillExists := r.mappings[key]; stillExists {
			r.lruList.MoveToBack(e2.elem)
			r.mu.Unlock()
			return e2.grid, nil
		}
		// Was evicted — reload under the write lock we already hold.
		m, err := r.loadGrid(layer, tileID)
		r.mu.Unlock()
		return m, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loadGrid(layer, tileID)
}

func (m *MappedGrid) samplePoint(lat, lon float64) uint8 {
	if lat < float64(m.header.MinLat) || lat >= float64(m.header.MaxLat) ||
		lon < float64(m.header.MinLon) || lon >= float64(m.header.MaxLon) {
		return OutOfBounds
	}
	row := int((lat - float64(m.header.MinLat)) / m.latStep)
	col := int((lon - float64(m.header.MinLon)) / m.lonStep)
	if row < 0 || row >= m.rows || col < 0 || col >= m.cols {
		return OutOfBounds
	}
	return m.data[row*m.cols+col]
}

var apiLayerToInternal = map[string]string{
	"populate":   "_traffic_populate",
	"industrial": "_traffic_industrial",
	"mw_density": "_traffic_mw_density",
	"hw_density": "_traffic_hw_density",
}

func (r *DiskCoverageReader) SamplePointByLayer(ctx context.Context, layer string, lat, lon float64) (uint8, error) {
	if mapped, ok := apiLayerToInternal[layer]; ok {
		layer = mapped
	}
	tileID := graphpkg.TileIDFromCoord(lat, lon)
	m, err := r.getGrid(layer, tileID)
	if err != nil {
		return 0, err
	}
	v := m.samplePoint(lat, lon)
	if v == OutOfBounds {
		return 0, nil
	}
	return v, nil
}

func (r *DiskCoverageReader) HasTile(layer string, tileID uint32) bool {
	path := r.tileFilePath(layer, tileID)
	_, err := os.Stat(path)
	return err == nil
}

func (r *DiskCoverageReader) LoadLayer(layer string, tileID uint32) (*MappedGrid, error) {
	return r.getGrid(layer, tileID)
}

func (r *DiskCoverageReader) PreloadTile(ctx context.Context, tileID uint32) error {
	layers := slices.Concat(allVisLayers, internalTrafficLayers)
	var loaded, missing int
	for _, layer := range layers {
		if _, err := r.getGrid(layer, tileID); err != nil {
			missing++
		} else {
			loaded++
		}
	}
	if loaded > 0 && missing > 0 {
		slog.Debug("preload: partial tile", "tileID", tileID, "loaded", loaded, "missing", missing)
	}
	return nil
}

func (m *MappedGrid) sampleLine(geom orb.LineString, intervalM float64) []uint8 {
	// Wrap mmap'd data in a temporary CoverageGrid to reuse SampleLine algorithm.
	// SAFETY: This temporary CoverageGrid wraps mmap'd data owned by the
	// DiskCoverageReader LRU cache. Never call Release() on it.
	g := &CoverageGrid{
		Bound:   m.bound,
		Rows:    m.rows,
		Cols:    m.cols,
		ResM:    m.resM,
		LatStep: m.latStep,
		LonStep: m.lonStep,
		Data:    m.data,
	}
	result := g.SampleLine(geom, intervalM)
	// Nil out Data to prevent accidental Release() from returning mmap'd
	// memory to the pool.
	g.Data = nil
	return result
}

func (r *DiskCoverageReader) SampleWay(geom orb.LineString) CoverageResult {
	if len(geom) < 2 {
		return CoverageResult{}
	}

	mid := geom[len(geom)/2]
	tileID := graphpkg.TileIDFromCoord(mid[1], mid[0])

	sample := func(layer string, interval float64) []uint8 {
		m, err := r.getGrid(layer, tileID)
		if err != nil {
			return nil
		}
		return m.sampleLine(geom, interval)
	}

	result := CoverageResult{
		ForestPct:      PctNonZero(sample(LayerForest, 25)),
		ProtectedArea:  WeightedAvg(sample(LayerProtected, 50)),
		LanduseClass:   Mode(sample(LayerLanduse, 50)),
		NoiseScore:     min(100, MaxVal(sample(LayerNoise, 50))*100/255),
		Urbanization:   WeightedAvg(sample(LayerUrbanization, 50)),
		RiverProximity: WeightedAvg(sample(LayerRiverProximity, 25)),
		Water:          anyNonZero(sample(LayerWater, 25)),
	}

	sampleMid := func(layer string) float64 {
		m, err := r.getGrid(layer, tileID)
		if err != nil {
			return 0
		}
		v := m.samplePoint(mid[1], mid[0])
		if v == OutOfBounds {
			return 0
		}
		return float64(v)
	}

	result.TrafficPopulate = sampleMid("_traffic_populate")
	result.TrafficIndustrial = sampleMid("_traffic_industrial")
	result.TrafficMWDensity = sampleMid("_traffic_mw_density")
	result.TrafficHWDensity = sampleMid("_traffic_hw_density")

	return result
}

// BuildTimestamp returns the build timestamp as a Unix epoch string.
// The result is cached after the first successful read (Fix #7).
func (r *DiskCoverageReader) BuildTimestamp() string {
	r.buildTSOnce.Do(func() {
		ts, err := ReadBuildTimestamp(r.baseDir)
		if err != nil {
			return
		}
		r.buildTS = fmt.Sprintf("%d", ts.Unix())
	})
	return r.buildTS
}
