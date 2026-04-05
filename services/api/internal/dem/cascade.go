package dem

import (
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
)

// HGTCell represents a 1°×1° DEM cell.
type HGTCell struct {
	Lat int // south edge latitude (floor)
	Lon int // west edge longitude (floor)
}

// FileName returns the canonical HGT filename for this cell.
func (c HGTCell) FileName() string {
	latPrefix := 'N'
	latAbs := c.Lat
	if c.Lat < 0 {
		latPrefix = 'S'
		latAbs = -c.Lat
	}
	lonPrefix := 'E'
	lonAbs := c.Lon
	if c.Lon < 0 {
		lonPrefix = 'W'
		lonAbs = -c.Lon
	}
	return fmt.Sprintf("%c%02d%c%03d.hgt", latPrefix, latAbs, lonPrefix, lonAbs)
}

// HGTCellsForBBox returns all 1°×1° HGT cells that a bounding box overlaps.
// bbox = [minLat, minLon, maxLat, maxLon].
func HGTCellsForBBox(bbox [4]float64) []HGTCell {
	minLat := int(math.Floor(bbox[0]))
	minLon := int(math.Floor(bbox[1]))
	maxLat := int(math.Floor(bbox[2]))
	maxLon := int(math.Floor(bbox[3]))

	var cells []HGTCell
	for lat := minLat; lat <= maxLat; lat++ {
		for lon := minLon; lon <= maxLon; lon++ {
			cells = append(cells, HGTCell{Lat: lat, Lon: lon})
		}
	}
	return cells
}

// MultiHGTReader dispatches lookups to 1+ HGTGrids by coordinate.
type MultiHGTReader struct {
	grids []*HGTGrid
}

func (m *MultiHGTReader) ElevationAt(lat, lon float64) int32 {
	for _, g := range m.grids {
		if lat >= g.north-1.0 && lat <= g.north && lon >= g.west && lon <= g.west+1.0 {
			return g.ElevationAt(lat, lon)
		}
	}
	return NoData
}

func (m *MultiHGTReader) ElevationBatch(lats, lons []int32) []int32 {
	out := make([]int32, len(lats))
	for i := range lats {
		out[i] = m.ElevationAt(float64(lats[i])/1e7, float64(lons[i])/1e7)
	}
	return out
}

func (m *MultiHGTReader) Close() {
	for _, g := range m.grids {
		g.Close()
	}
}

// NewReaderForTile creates a DEM reader for a routing tile by finding the
// highest-resolution HGT files available. hgtDirs are searched in priority
// order (e.g., ["data/dem/sonny-05", "data/dem/sonny-1", "data/dem/glo30"]).
func NewReaderForTile(hgtDirs []string, tileID uint32) (Reader, error) {
	bbox := tileBBoxExpanded(tileID)
	cells := HGTCellsForBBox(bbox)

	var grids []*HGTGrid
	for _, cell := range cells {
		var found bool
		for _, dir := range hgtDirs {
			path := filepath.Join(dir, cell.FileName())
			if _, err := os.Stat(path); err != nil {
				continue
			}
			g, err := OpenHGTGrid(path)
			if err != nil {
				slog.Warn("failed to open HGT file, skipping",
					"path", path, "error", err)
				continue
			}
			grids = append(grids, g)
			found = true
			break // highest priority for this cell found
		}
		if !found {
			slog.Debug("no HGT file for cell", "cell", cell.FileName())
		}
	}

	if len(grids) == 0 {
		return NewNoOpReader(tileID, hgtDirs), nil
	}
	if len(grids) < len(cells) {
		slog.Warn("partial DEM coverage for tile",
			"tile_id", tileID,
			"cells_needed", len(cells),
			"cells_found", len(grids))
	}
	return &MultiHGTReader{grids: grids}, nil
}

// tileBBoxExpanded returns the expanded tile bounding box.
// Duplicates graph.TileBBoxExpanded to avoid circular import.

// borderBuffer MUST match graph.BorderBuffer (0.02).
// Duplicated here to avoid circular import (dem → graph → dem).
const borderBuffer = 0.02

func tileBBoxExpanded(tileID uint32) [4]float64 {
	latFloor := int(tileID/360) - 90
	lonFloor := int(tileID%360) - 180
	return [4]float64{
		float64(latFloor) - borderBuffer,
		float64(lonFloor) - borderBuffer,
		float64(latFloor+1) + borderBuffer,
		float64(lonFloor+1) + borderBuffer,
	}
}
