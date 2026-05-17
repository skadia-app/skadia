package model

import "github.com/skadia-app/skadia/services/api/internal/capabilities"

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
	Key                string              `json:"key"`
	Category           SystemLayerCategory `json:"category"`
	CompositeGroup     *CoverageGroup      `json:"composite_group,omitempty"`
	DefaultLabel       string              `json:"default_label"`
	RequiredCapability string              `json:"required_capability,omitempty"`
}

func ptr[T any](v T) *T { return &v }

// SystemLayers is the canonical list of 14 built-in layer keys.
// CI-test (system_layers_consistency_test.go) verifies bidirectional equality
// with the DB seed in migration 000333.
var SystemLayers = []SystemLayerDescriptor{
	{Key: "summits", Category: SystemLayerCategoryVector, DefaultLabel: "Gipfel", RequiredCapability: capabilities.KeySystemLayersVector},
	{Key: "waters", Category: SystemLayerCategoryVector, DefaultLabel: "Gewässer", RequiredCapability: capabilities.KeySystemLayersVector},
	{Key: "forests", Category: SystemLayerCategoryVector, DefaultLabel: "Wälder", RequiredCapability: capabilities.KeySystemLayersVector},
	{Key: "visited_pois", Category: SystemLayerCategoryVector, DefaultLabel: "Besuchte Einkehr", RequiredCapability: capabilities.KeySystemLayersVector},
	{Key: "amenities_tiles", Category: SystemLayerCategoryVector, DefaultLabel: "Einkehr-Tiles", RequiredCapability: capabilities.KeySystemLayersVector},
	{Key: "segments_lines", Category: SystemLayerCategoryVector, DefaultLabel: "Segmente Linien"},
	{Key: "coverage_landuse", Category: SystemLayerCategoryCoverage, CompositeGroup: ptr(CoverageGroupNature), DefaultLabel: "Landnutzung", RequiredCapability: capabilities.KeySystemLayersCoverage},
	{Key: "coverage_forest", Category: SystemLayerCategoryCoverage, CompositeGroup: ptr(CoverageGroupNature), DefaultLabel: "Wald-Coverage", RequiredCapability: capabilities.KeySystemLayersCoverage},
	{Key: "coverage_water", Category: SystemLayerCategoryCoverage, CompositeGroup: ptr(CoverageGroupNature), DefaultLabel: "Wasser-Coverage", RequiredCapability: capabilities.KeySystemLayersCoverage},
	{Key: "coverage_river_proximity", Category: SystemLayerCategoryCoverage, CompositeGroup: ptr(CoverageGroupNature), DefaultLabel: "Flussnähe", RequiredCapability: capabilities.KeySystemLayersCoverage},
	{Key: "coverage_protected", Category: SystemLayerCategoryCoverage, CompositeGroup: ptr(CoverageGroupNature), DefaultLabel: "Schutzgebiete", RequiredCapability: capabilities.KeySystemLayersCoverage},
	{Key: "coverage_noise", Category: SystemLayerCategoryCoverage, CompositeGroup: ptr(CoverageGroupNoise), DefaultLabel: "Lärm", RequiredCapability: capabilities.KeySystemLayersCoverage},
	{Key: "coverage_traffic", Category: SystemLayerCategoryCoverage, CompositeGroup: ptr(CoverageGroupTrafficUrban), DefaultLabel: "Verkehr", RequiredCapability: capabilities.KeySystemLayersCoverage},
	{Key: "coverage_urbanization", Category: SystemLayerCategoryCoverage, CompositeGroup: ptr(CoverageGroupTrafficUrban), DefaultLabel: "Urbanisierung", RequiredCapability: capabilities.KeySystemLayersCoverage},
}

// SystemLayerKeySet returns a set for O(1) membership checks.
func SystemLayerKeySet() map[string]struct{} {
	set := make(map[string]struct{}, len(SystemLayers))
	for _, l := range SystemLayers {
		set[l.Key] = struct{}{}
	}
	return set
}
