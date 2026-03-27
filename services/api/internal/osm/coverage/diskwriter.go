package coverage

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/paulmach/orb"
)

// CropGridToCore extracts the 1°×1° core from an expanded grid.
func CropGridToCore(g *CoverageGrid, core orb.Bound) *CoverageGrid {
	dst := NewGrid(core, g.ResM)
	for row := range dst.Rows {
		lat := core.Min[1] + (float64(row)+0.5)*dst.LatStep
		for col := range dst.Cols {
			lon := core.Min[0] + (float64(col)+0.5)*dst.LonStep
			val := g.SamplePoint(orb.Point{lon, lat})
			if val != OutOfBounds {
				dst.Data[row*dst.Cols+col] = val
			}
		}
	}
	return dst
}

// fcovHeaderFromGrid creates an FCOV header from a CoverageGrid.
func fcovHeaderFromGrid(g *CoverageGrid) FCOVHeader {
	return FCOVHeader{
		Magic:   fcovMagic,
		Version: fcovVersion,
		ResM:    uint16(math.Round(g.ResM)),
		Rows:    uint32(g.Rows),
		Cols:    uint32(g.Cols),
		MinLat:  float32(g.Bound.Min[1]),
		MinLon:  float32(g.Bound.Min[0]),
		MaxLat:  float32(g.Bound.Max[1]),
		MaxLon:  float32(g.Bound.Max[0]),
	}
}

// tileFilename returns "47_8.bin" for a core bound starting at lat=47, lon=8.
func tileFilename(core orb.Bound) string {
	lat := int(math.Floor(core.Min[1]))
	lon := int(math.Floor(core.Min[0]))
	return fmt.Sprintf("%d_%d.bin", lat, lon)
}

// allVisLayers are the 8 visualization layers written to disk.
var allVisLayers = []string{
	LayerForest, LayerWater, LayerRiverProximity, LayerLanduse,
	LayerProtected, LayerUrbanization, LayerTraffic, LayerNoise,
}

// internalTrafficLayers are the 4 internal traffic-input layers.
var internalTrafficLayers = []string{
	"_traffic_populate", "_traffic_industrial", "_traffic_mw_density", "_traffic_hw_density",
}

// shouldWriteLayer returns true if the layer should be written given the filter.
func shouldWriteLayer(layerName, filterLayer string) bool {
	if filterLayer == "" {
		return true
	}
	if filterLayer == "traffic" {
		return layerName == LayerTraffic || strings.HasPrefix(layerName, "_traffic_")
	}
	return layerName == filterLayer
}

// WriteTileGrids serialises all layers from TileGrids (and optional trafficInputs)
// into FCOV files under baseDir. Grids are cropped from expanded to core bounds.
// If filterLayer is non-empty, only that layer (+ _traffic_* for "traffic") is written.
func WriteTileGrids(tg *TileGrids, ti *trafficInputs, baseDir string, core orb.Bound, filterLayer string) error {
	fname := tileFilename(core)

	for _, layer := range allVisLayers {
		if !shouldWriteLayer(layer, filterLayer) {
			continue
		}
		g := tg.Get(layer)
		if g == nil {
			continue
		}
		cropped := CropGridToCore(g, core)
		path := filepath.Join(baseDir, layer, fname)
		if err := WriteFCOV(path, fcovHeaderFromGrid(cropped), cropped.Data); err != nil {
			return fmt.Errorf("write layer %s: %w", layer, err)
		}
	}

	// Write internal traffic-input grids (quantised from float64 to uint8)
	if ti != nil {
		type inputPair struct {
			name string
			grid *floatGrid
		}
		pairs := []inputPair{
			{"_traffic_populate", ti.populate},
			{"_traffic_industrial", ti.industrial},
			{"_traffic_mw_density", ti.mwDensity},
			{"_traffic_hw_density", ti.hwDensity},
		}
		for _, p := range pairs {
			if !shouldWriteLayer(p.name, filterLayer) {
				continue
			}
			if p.grid == nil {
				continue
			}
			resampled := resampleFloatToGrid(p.grid, core, 100)
			path := filepath.Join(baseDir, p.name, fname)
			if err := WriteFCOV(path, fcovHeaderFromGrid(resampled), resampled.Data); err != nil {
				return fmt.Errorf("write layer %s: %w", p.name, err)
			}
		}
	}

	return nil
}

// WriteBuildTimestamp writes the current time to .build-timestamp.
func WriteBuildTimestamp(baseDir string) error {
	path := filepath.Join(baseDir, ".build-timestamp")
	return os.WriteFile(path, []byte(time.Now().UTC().Format(time.RFC3339Nano)), 0644)
}

// ReadBuildTimestamp returns the last build timestamp.
func ReadBuildTimestamp(baseDir string) (time.Time, error) {
	data, err := os.ReadFile(filepath.Join(baseDir, ".build-timestamp"))
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339Nano, string(data))
}
