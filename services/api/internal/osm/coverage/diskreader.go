package coverage

import (
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

type DiskCoverageReader struct {
	baseDir     string
	maxMapped   int
	mu          sync.RWMutex
	mappings    map[layerTileKey]*MappedGrid
	accessOrder []layerTileKey
}

func NewDiskCoverageReader(baseDir string, maxMappings int) (*DiskCoverageReader, error) {
	if maxMappings <= 0 {
		maxMappings = 256
	}
	return &DiskCoverageReader{
		baseDir:   baseDir,
		maxMapped: maxMappings,
		mappings:  make(map[layerTileKey]*MappedGrid),
	}, nil
}

func (r *DiskCoverageReader) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, m := range r.mappings {
		m.cleanup()
	}
	r.mappings = make(map[layerTileKey]*MappedGrid)
	r.accessOrder = nil
}

func (r *DiskCoverageReader) tileFilePath(layer string, tileID uint32) string {
	bbox := graphpkg.TileBBox(tileID)
	lat := int(bbox[0])
	lon := int(bbox[1])
	return filepath.Join(r.baseDir, layer, fmt.Sprintf("%d_%d.bin", lat, lon))
}

// loadGrid mmap's a single layer/tile combination. Caller must hold write lock.
func (r *DiskCoverageReader) loadGrid(layer string, tileID uint32) (*MappedGrid, error) {
	key := layerTileKey{layer, tileID}
	if m, ok := r.mappings[key]; ok {
		return m, nil
	}

	for len(r.mappings) >= r.maxMapped && len(r.accessOrder) > 0 {
		evictKey := r.accessOrder[0]
		r.accessOrder = r.accessOrder[1:]
		if m, ok := r.mappings[evictKey]; ok {
			m.cleanup()
			delete(r.mappings, evictKey)
		}
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
	r.mappings[key] = m
	r.accessOrder = append(r.accessOrder, key)
	return m, nil
}

func (r *DiskCoverageReader) getGrid(layer string, tileID uint32) (*MappedGrid, error) {
	key := layerTileKey{layer, tileID}

	r.mu.RLock()
	m, ok := r.mappings[key]
	r.mu.RUnlock()
	if ok {
		r.mu.Lock()
		for i, k := range r.accessOrder {
			if k == key {
				r.accessOrder = append(r.accessOrder[:i], r.accessOrder[i+1:]...)
				r.accessOrder = append(r.accessOrder, key)
				break
			}
		}
		r.mu.Unlock()
		return m, nil
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

func (r *DiskCoverageReader) BuildTimestamp() string {
	ts, err := ReadBuildTimestamp(r.baseDir)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d", ts.Unix())
}
