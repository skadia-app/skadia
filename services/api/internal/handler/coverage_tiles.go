package handler

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/osm/coverage"
	graphpkg "github.com/skadia-app/skadia/services/api/internal/routing/graph"

	"github.com/paulmach/orb"
)

func (h *Handler) CoverageTile(w http.ResponseWriter, r *http.Request) {
	// Chi wildcard routes pass the entire remainder as a single parameter.
	// We split the path to extract layer/z/x/y components.
	// Format: /api/v1/coverage/tiles/{layer}/{z}/{x}/{y}.png
	parts := strings.Split(r.URL.Path, "/")
	tilesIdx := -1
	for i, p := range parts {
		if p == "tiles" {
			tilesIdx = i
			break
		}
	}
	if tilesIdx < 0 || tilesIdx+4 >= len(parts) {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid path",
			"expected /tiles/{layer}/{z}/{x}/{y}.png", r.URL.Path)
		return
	}

	layer := parts[tilesIdx+1]
	zStr := parts[tilesIdx+2]
	xStr := parts[tilesIdx+3]
	yPng := parts[tilesIdx+4]

	if !strings.HasSuffix(yPng, ".png") {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid path",
			"tile path must end with .png", r.URL.Path)
		return
	}

	yStr := strings.TrimSuffix(yPng, ".png")

	if strings.HasPrefix(layer, "_") {
		httputil.WriteError(w, http.StatusBadRequest, "Internal layer",
			"layers with _ prefix are internal and not exposed via tile API", r.URL.Path)
		return
	}

	if _, ok := coverage.Palettes[layer]; !ok {
		httputil.WriteError(w, http.StatusBadRequest, "Unknown layer",
			fmt.Sprintf("unknown layer: %s", layer), r.URL.Path)
		return
	}

	z, err := strconv.Atoi(zStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid z", "z must be integer", r.URL.Path)
		return
	}
	x, err := strconv.Atoi(xStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid x", "x must be integer", r.URL.Path)
		return
	}
	y, err := strconv.Atoi(yStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid y", "y must be integer", r.URL.Path)
		return
	}

	if h.CoverageReader == nil {
		writePNG(w, coverage.TransparentPNG())
		return
	}

	if h.PNGCache != nil {
		ts := h.CoverageReader.BuildTimestamp()
		h.PNGCache.InvalidateIfStale(ts)

		cacheKey := coverage.PNGCacheKey{Layer: layer, Z: z, X: x, Y: y}
		if cached, ok := h.PNGCache.Get(cacheKey); ok {
			writePNG(w, cached)
			return
		}
	}

	bbox := slippyTileBBox(z, x, y)
	bound := orb.Bound{
		Min: orb.Point{bbox[1], bbox[0]},
		Max: orb.Point{bbox[3], bbox[2]},
	}

	srcLayer := coverage.SourceLayerName(layer)
	palette := coverage.Palettes[layer]

	tileIDs := coverageTileIDs(bbox)

	var grids []*coverage.MappedGrid
	for _, tid := range tileIDs {
		mg, err := h.CoverageReader.LoadLayer(srcLayer, tid)
		if err != nil {
			continue
		}
		grids = append(grids, mg)
	}

	if len(grids) == 0 {
		pngData := coverage.TransparentPNG()
		if h.PNGCache != nil {
			h.PNGCache.Put(coverage.PNGCacheKey{Layer: layer, Z: z, X: x, Y: y}, pngData)
		}
		writePNG(w, pngData)
		return
	}

	img := coverage.RenderSingleLayerTile(grids, palette, 256, 256, bound)
	pngData, err := coverage.EncodePNG(img)
	if err != nil {
		http.Error(w, "render failed", http.StatusInternalServerError)
		return
	}

	if h.PNGCache != nil {
		h.PNGCache.Put(coverage.PNGCacheKey{Layer: layer, Z: z, X: x, Y: y}, pngData)
	}

	writePNG(w, pngData)
}

func writePNG(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(data)
}

func slippyTileBBox(z, x, y int) [4]float64 {
	n := math.Pow(2, float64(z))
	minLon := float64(x)/n*360 - 180
	maxLon := float64(x+1)/n*360 - 180
	maxLat := math.Atan(math.Sinh(math.Pi*(1-2*float64(y)/n))) * 180 / math.Pi
	minLat := math.Atan(math.Sinh(math.Pi*(1-2*float64(y+1)/n))) * 180 / math.Pi
	return [4]float64{minLat, minLon, maxLat, maxLon}
}

func coverageTileIDs(bbox [4]float64) []uint32 {
	minLat := int(math.Floor(bbox[0]))
	minLon := int(math.Floor(bbox[1]))
	// Exclusive upper bounds: ensure at least the tile containing minLat/minLon is included.
	maxLat := max(minLat+1, int(math.Floor(bbox[2])))
	maxLon := max(minLon+1, int(math.Floor(bbox[3])))

	var ids []uint32
	for lat := minLat; lat < maxLat; lat++ {
		for lon := minLon; lon < maxLon; lon++ {
			ids = append(ids, graphpkg.TileIDFromCoord(float64(lat)+0.5, float64(lon)+0.5))
		}
	}
	return ids
}
