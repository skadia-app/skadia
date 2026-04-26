package model

// SystemLayerCategory groups system layer keys for frontend UI rendering.
type SystemLayerCategory string

const (
	SystemLayerCategoryVector   SystemLayerCategory = "vector"
	SystemLayerCategoryCoverage SystemLayerCategory = "coverage"
)

// CoverageGroup composites a coverage sublayer into a parent toggle
// (matches frontend useCoverageOverlays.ts).
type CoverageGroup string

const (
	CoverageGroupNature       CoverageGroup = "nature"
	CoverageGroupNoise        CoverageGroup = "noise"
	CoverageGroupTrafficUrban CoverageGroup = "trafficUrban"
)

// SystemLayerDescriptor describes one of the 14 built-in layer keys.
// Source of truth for both backend and frontend
// (frontend fetches via /api/v1/system-layers/keys).
type SystemLayerDescriptor struct {
	Key            string              `json:"key"`
	Category       SystemLayerCategory `json:"category"`
	CompositeGroup *CoverageGroup      `json:"composite_group,omitempty"`
	DefaultLabel   string              `json:"default_label"`
}

func ptr[T any](v T) *T { return &v }

// SystemLayers is the canonical list of 14 built-in layer keys.
// CI-test (system_layers_consistency_test.go) verifies bidirectional equality
// with the DB seed in migration 000333.
var SystemLayers = []SystemLayerDescriptor{
	{Key: "summits", Category: SystemLayerCategoryVector, DefaultLabel: "Gipfel"},
	{Key: "waters", Category: SystemLayerCategoryVector, DefaultLabel: "Gewässer"},
	{Key: "forests", Category: SystemLayerCategoryVector, DefaultLabel: "Wälder"},
	{Key: "visited_pois", Category: SystemLayerCategoryVector, DefaultLabel: "Besuchte Einkehr"},
	{Key: "amenities_tiles", Category: SystemLayerCategoryVector, DefaultLabel: "Einkehr-Tiles"},
	{Key: "segments_lines", Category: SystemLayerCategoryVector, DefaultLabel: "Segmente Linien"},
	{Key: "coverage_landuse", Category: SystemLayerCategoryCoverage, CompositeGroup: ptr(CoverageGroupNature), DefaultLabel: "Landnutzung"},
	{Key: "coverage_forest", Category: SystemLayerCategoryCoverage, CompositeGroup: ptr(CoverageGroupNature), DefaultLabel: "Wald-Coverage"},
	{Key: "coverage_water", Category: SystemLayerCategoryCoverage, CompositeGroup: ptr(CoverageGroupNature), DefaultLabel: "Wasser-Coverage"},
	{Key: "coverage_river_proximity", Category: SystemLayerCategoryCoverage, CompositeGroup: ptr(CoverageGroupNature), DefaultLabel: "Flussnähe"},
	{Key: "coverage_protected", Category: SystemLayerCategoryCoverage, CompositeGroup: ptr(CoverageGroupNature), DefaultLabel: "Schutzgebiete"},
	{Key: "coverage_noise", Category: SystemLayerCategoryCoverage, CompositeGroup: ptr(CoverageGroupNoise), DefaultLabel: "Lärm"},
	{Key: "coverage_traffic", Category: SystemLayerCategoryCoverage, CompositeGroup: ptr(CoverageGroupTrafficUrban), DefaultLabel: "Verkehr"},
	{Key: "coverage_urbanization", Category: SystemLayerCategoryCoverage, CompositeGroup: ptr(CoverageGroupTrafficUrban), DefaultLabel: "Urbanisierung"},
}

// SystemLayerKeySet returns a set for O(1) membership checks.
func SystemLayerKeySet() map[string]struct{} {
	set := make(map[string]struct{}, len(SystemLayers))
	for _, l := range SystemLayers {
		set[l.Key] = struct{}{}
	}
	return set
}
