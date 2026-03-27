package coverage

import (
	"bytes"
	"container/list"
	"image"
	"image/png"
	"sync"

	"github.com/paulmach/orb"
)

// RenderSingleLayerTile renders a coverage layer into an NRGBA image.
// Row-level pre-computation (Fix #18): lat and row offset are hoisted out of
// the inner loop; lon is computed with a pre-calculated step to avoid repeated
// division. Direct Pix access avoids per-pixel bounds checking from SetNRGBA.
func RenderSingleLayerTile(grids []*MappedGrid, palette PaletteFunc, width, height int, bbox orb.Bound) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	if len(grids) == 0 {
		return img
	}

	latRange := bbox.Max[1] - bbox.Min[1]
	lonRange := bbox.Max[0] - bbox.Min[0]
	lonStep := lonRange / float64(width)
	lonBase := bbox.Min[0] + lonStep*0.5

	for py := range height {
		lat := bbox.Max[1] - (float64(py)+0.5)/float64(height)*latRange
		rowOff := py * width * 4 // NRGBA stride
		for px := range width {
			lon := lonBase + float64(px)*lonStep
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
				off := rowOff + px*4
				img.Pix[off] = c.R
				img.Pix[off+1] = c.G
				img.Pix[off+2] = c.B
				img.Pix[off+3] = c.A
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

// --- PNG LRU Cache (Fix #17: O(1) via container/list) ---

type PNGCacheKey struct {
	Layer string
	Z     int
	X     int
	Y     int
}

type pngLRUEntry struct {
	key  PNGCacheKey
	data []byte
	elem *list.Element
}

type PNGCache struct {
	mu             sync.Mutex
	maxEntries     int
	buildTimestamp string
	entries        map[PNGCacheKey]*pngLRUEntry
	lruList        *list.List
}

func NewPNGCache(maxEntries int, buildTimestamp string) *PNGCache {
	if maxEntries <= 0 {
		maxEntries = 10_000
	}
	return &PNGCache{
		maxEntries:     maxEntries,
		buildTimestamp: buildTimestamp,
		entries:        make(map[PNGCacheKey]*pngLRUEntry, maxEntries),
		lruList:        list.New(),
	}
}

func (c *PNGCache) Get(key PNGCacheKey) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	c.lruList.MoveToBack(e.elem)
	return e.data, true
}

func (c *PNGCache) Put(key PNGCacheKey, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.entries[key]; ok {
		return
	}
	for len(c.entries) >= c.maxEntries && c.lruList.Len() > 0 {
		oldest := c.lruList.Front()
		entry := oldest.Value.(*pngLRUEntry)
		c.lruList.Remove(oldest)
		delete(c.entries, entry.key)
	}
	e := &pngLRUEntry{key: key, data: data}
	e.elem = c.lruList.PushBack(e)
	c.entries[key] = e
}

func (c *PNGCache) InvalidateIfStale(newTimestamp string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.buildTimestamp == newTimestamp {
		return
	}
	c.buildTimestamp = newTimestamp
	c.entries = make(map[PNGCacheKey]*pngLRUEntry, c.maxEntries)
	c.lruList.Init()
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
