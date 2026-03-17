// Package dem provides elevation data access via mmap'd HGT files.
package dem

import "log/slog"

// NoData is the sentinel value for missing elevation data.
// Distinct from sea level (0 decimeters).
const NoData int32 = -32768

// Reader provides DEM elevation lookups for tile builds.
type Reader interface {
	// ElevationAt returns elevation in decimeters.
	// Returns NoData if outside coverage or unresolvable void.
	ElevationAt(lat, lon float64) int32

	// ElevationBatch returns elevations for int32×1e7 encoded coordinates.
	ElevationBatch(lats, lons []int32) []int32

	// Close releases mmap resources. MUST be called (defer reader.Close()).
	Close()
}

// NoOpReader returns NoData for all lookups.
// Used when no DEM files are available.
type NoOpReader struct{}

func (r *NoOpReader) ElevationAt(lat, lon float64) int32 { return NoData }

func (r *NoOpReader) ElevationBatch(lats, lons []int32) []int32 {
	out := make([]int32, len(lats))
	for i := range out {
		out[i] = NoData
	}
	return out
}

func (r *NoOpReader) Close() {}

// NewNoOpReader creates a NoOpReader and logs a warning.
func NewNoOpReader(tileID uint32, searchedDirs []string) *NoOpReader {
	slog.Warn("no DEM data available — all elevations will be 0",
		"tile_id", tileID,
		"searched_dirs", searchedDirs,
	)
	return &NoOpReader{}
}
