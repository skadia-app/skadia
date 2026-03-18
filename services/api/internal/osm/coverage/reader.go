package coverage

import "github.com/paulmach/orb"

// CoverageResult holds all enrichment values from a coverage sampling.
// Extensible: new fields can be added without breaking existing consumers.
type CoverageResult struct {
	ForestPct      int // 0-100
	ProtectedArea  int // 0-3
	LanduseClass   int // 0-8
	NoiseScore     int // 0-100 (combined from all enabled categories)
	Urbanization   int // 0-255
	RiverProximity int // 0-255
	Water          bool
}

// CoverageReader provides coverage grid access for the graph builder (Phase 2).
type CoverageReader interface {
	SampleWay(geom orb.LineString) CoverageResult
	PreloadTile(tileID uint32) error
}
