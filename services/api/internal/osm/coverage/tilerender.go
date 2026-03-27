package coverage

import (
	"bytes"
	"image"
	"image/png"
	"sync"

	"github.com/paulmach/orb"
)

func RenderSingleLayerTile(grids []*MappedGrid, palette PaletteFunc, width, height int, bbox orb.Bound) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	if len(grids) == 0 {
		return img
	}

	latRange := bbox.Max[1] - bbox.Min[1]
	lonRange := bbox.Max[0] - bbox.Min[0]

	for py := range height {
		lat := bbox.Max[1] - (float64(py)+0.5)/float64(height)*latRange
		for px := range width {
			lon := bbox.Min[0] + (float64(px)+0.5)/float64(width)*lonRange
			var val uint8 = OutOfBounds
			for _, mg := range grids {
				v := mg.samplePoint(lat, lon)
				if v != OutOfBounds {
					val = v
					break
				}
			}
			if val == OutOfBounds {
				continue
			}
			c := palette(val)
			if c.A > 0 {
				img.SetNRGBA(px, py, c)
			}
		}
	}
	return img
}

func EncodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

var transparentPNG []byte
var transparentPNGOnce sync.Once

func TransparentPNG() []byte {
	transparentPNGOnce.Do(func() {
		img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
		transparentPNG, _ = EncodePNG(img)
	})
	return transparentPNG
}

// --- PNG LRU Cache ---

type PNGCacheKey struct {
	Layer string
	Z     int
	X     int
	Y     int
}

type pngCacheEntry struct {
	key  PNGCacheKey
	data []byte
}

type PNGCache struct {
	mu             sync.Mutex
	maxEntries     int
	buildTimestamp string
	entries        map[PNGCacheKey]*pngCacheEntry
	order          []PNGCacheKey
}

func NewPNGCache(maxEntries int, buildTimestamp string) *PNGCache {
	if maxEntries <= 0 {
		maxEntries = 10_000
	}
	return &PNGCache{
		maxEntries:     maxEntries,
		buildTimestamp: buildTimestamp,
		entries:        make(map[PNGCacheKey]*pngCacheEntry, maxEntries),
	}
}

func (c *PNGCache) Get(key PNGCacheKey) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	for i, k := range c.order {
		if k == key {
			c.order = append(c.order[:i], c.order[i+1:]...)
			c.order = append(c.order, key)
			break
		}
	}
	return e.data, true
}

func (c *PNGCache) Put(key PNGCacheKey, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.entries[key]; ok {
		return
	}
	for len(c.entries) >= c.maxEntries && len(c.order) > 0 {
		evictKey := c.order[0]
		c.order = c.order[1:]
		delete(c.entries, evictKey)
	}
	c.entries[key] = &pngCacheEntry{key: key, data: data}
	c.order = append(c.order, key)
}

func (c *PNGCache) InvalidateIfStale(newTimestamp string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.buildTimestamp == newTimestamp {
		return
	}
	c.buildTimestamp = newTimestamp
	c.entries = make(map[PNGCacheKey]*pngCacheEntry, c.maxEntries)
	c.order = nil
}

var sourceLayerForAPI = map[string]string{
	"forest":          "forest",
	"water":           "water",
	"river_proximity": "river_proximity",
	"landuse":         "landuse",
	"protected":       "protected",
	"urbanization":    "urbanization",
	"traffic":         "traffic",
	"noise":           "noise",
	"quiet":           "noise",
}

func SourceLayerName(apiLayer string) string {
	if s, ok := sourceLayerForAPI[apiLayer]; ok {
		return s
	}
	return apiLayer
}
