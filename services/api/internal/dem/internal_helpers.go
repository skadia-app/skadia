package dem

import (
	"errors"
	"math"
	"os"
	"path/filepath"
)

// ErrNoCoverage is returned when no HGT file covers the requested coordinate
// across all configured DEM directories.
var ErrNoCoverage = errors.New("dem: no coverage for coordinate")

// cellNameFor returns the canonical HGT cell name (without .hgt extension) for
// a WGS84 coordinate by delegating to HGTCell.FileName.
func cellNameFor(lat, lon float64) string {
	cell := HGTCell{
		Lat: int(math.Floor(lat)),
		Lon: int(math.Floor(lon)),
	}
	name := cell.FileName()
	// Strip the ".hgt" suffix — callers who need the full filename append it.
	return name[:len(name)-4]
}

// openGridFor searches hgtDirs in priority order for an HGT file covering
// the 1°×1° cell identified by cellName (without .hgt extension) and opens
// the first match. Returns (grid, nil) on success, (nil, ErrNoCoverage) when
// no file exists across all dirs, or (nil, err) for any other open error.
func openGridFor(hgtDirs []string, cellName string) (*HGTGrid, error) {
	filename := cellName + ".hgt"
	for _, dir := range hgtDirs {
		path := filepath.Join(dir, filename)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		g, err := OpenHGTGrid(path)
		if err != nil {
			return nil, err
		}
		return g, nil
	}
	return nil, ErrNoCoverage
}
