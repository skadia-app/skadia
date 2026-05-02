package sport

import (
	"sort"
	"strings"
)

// Category groups related sport types into a UI-friendly bucket.
type Category struct {
	Key     string `json:"key"`
	LabelDE string `json:"label_de"`
	Color   string `json:"color"`
}

// SportTraits describes stable taxonomy dimensions for a sport tuple.
type SportTraits struct {
	Environments      []string `json:"environments"`
	MovementFamilies  []string `json:"movement_families"`
	EquipmentFamilies []string `json:"equipment_families"`
	ActivityForm      string   `json:"activity_form"`
	PropulsionModes   []string `json:"propulsion_modes"`
}

// ModifierDefaults describes canonical tuple-level modifier defaults.
type ModifierDefaults struct {
	IsIndoor *bool `json:"is_indoor,omitempty"`
}

// SubTypeDef describes a sub-sport variation within a parent sport.
type SubTypeDef struct {
	SubType          string           `json:"sub_type"`
	LabelDE          string           `json:"label_de"`
	DetailDE         string           `json:"detail_de"`
	AutoTitleDE      string           `json:"auto_title_de"`
	MovingThreshold  *float64         `json:"moving_threshold,omitempty"`
	Selectable       bool             `json:"selectable"`
	Traits           SportTraits      `json:"traits"`
	ModifierDefaults ModifierDefaults `json:"modifier_defaults,omitempty"`
}

// SportDef holds every piece of metadata for a single sport type.
type SportDef struct {
	SportType        string                `json:"sport_type"`
	CategoryKey      string                `json:"category_key"`
	LabelDE          string                `json:"label_de"`
	AutoTitleDE      string                `json:"auto_title_de"`
	SegmentGroup     string                `json:"segment_group"`
	TypicallyHasGPS  bool                  `json:"typically_has_gps"`
	MovingThreshold  float64               `json:"moving_threshold"`
	IsIndoor         bool                  `json:"is_indoor"`
	Lifecycle        string                `json:"lifecycle"`
	Selectable       bool                  `json:"selectable"`
	Traits           SportTraits           `json:"traits"`
	ModifierDefaults ModifierDefaults      `json:"modifier_defaults,omitempty"`
	SubTypes         map[string]SubTypeDef `json:"sub_types,omitempty"`
}

// ---- Categories ----

var categories = generatedCategories

var categoryByKey map[string]Category

func init() {
	categoryByKey = make(map[string]Category, len(categories))
	for _, c := range categories {
		categoryByKey[c.Key] = c
	}

	registry = make(map[string]SportDef, len(generatedSportDefs))
	for _, d := range generatedSportDefs {
		registry[d.SportType] = d
	}
}

// ---- Sport definitions ----

// registry maps sport_type string → SportDef.
var registry map[string]SportDef

// ---- Lookup functions ----

// GetSportDef returns the full definition for a sport type.
// The second return value is false when the sport type is unknown.
func GetSportDef(sportType string) (SportDef, bool) {
	d, ok := registry[sportType]
	return d, ok
}

// SportLabel returns the compact German display label for a sport tuple.
func SportLabel(sportType, subType string) string {
	d, ok := registry[sportType]
	if !ok {
		return titleCase(sportType)
	}
	if subType != "" && subType != "generic" {
		if st, stOK := d.SubTypes[subType]; stOK {
			return st.LabelDE
		}
	}
	return d.LabelDE
}

// SportDetailLabel returns a detailed German label including sub-type info.
// Examples: "Radfahren (MTB)", "Krafttraining", "Kajak (Wildwasser)".
func SportDetailLabel(sportType, subType string) string {
	d, ok := registry[sportType]
	if !ok {
		return titleCase(sportType)
	}
	if subType == "" || subType == "generic" {
		return d.LabelDE
	}
	if st, stOK := d.SubTypes[subType]; stOK {
		if subTypeLabelCanStandAlone(d) {
			return st.DetailDE
		}
		return d.LabelDE + " (" + st.LabelDE + ")"
	}
	return d.LabelDE
}

// AutoTitle returns the German auto-title string used for auto-naming activities.
// Sub-type-promoted sports use the sub-type's title. Fallback is "Training".
func AutoTitle(sportType, subType string) string {
	d, ok := registry[sportType]
	if !ok {
		return "Training"
	}
	if subType != "" && subType != "generic" {
		if st, stOK := d.SubTypes[subType]; stOK && st.AutoTitleDE != "" {
			return st.AutoTitleDE
		}
	}
	return d.AutoTitleDE
}

// SegmentGroup returns the segment matching group for a sport type.
// Possible values: bike, foot, swim, ski, paddle, generic.
func SegmentGroup(sportType string) string {
	d, ok := registry[sportType]
	if !ok {
		return "generic"
	}
	return d.SegmentGroup
}

// MovingThreshold returns the minimum speed (m/s) that counts as "moving"
// for the given sport type.
func MovingThreshold(sportType string) float64 {
	return MovingThresholdForTuple(sportType, "")
}

// MovingThresholdForTuple returns the minimum speed (m/s) that counts as
// moving for the given sport tuple.
func MovingThresholdForTuple(sportType, subType string) float64 {
	d, ok := registry[sportType]
	if !ok {
		return 0.0
	}
	if subType != "" && subType != "generic" {
		if st, stOK := d.SubTypes[subType]; stOK && st.MovingThreshold != nil {
			return *st.MovingThreshold
		}
	}
	return d.MovingThreshold
}

// CategoryForSport returns the category for a given sport type.
// Falls back to the "other" category for unknown sport types.
func CategoryForSport(sportType string) Category {
	d, ok := registry[sportType]
	if !ok {
		return categoryByKey["other"]
	}
	c, cOK := categoryByKey[d.CategoryKey]
	if !cOK {
		return categoryByKey["other"]
	}
	return c
}

// AllCategories returns all categories in display order.
func AllCategories() []Category {
	result := make([]Category, len(categories))
	copy(result, categories)
	return result
}

// AllSports returns all registered sport definitions, sorted alphabetically by SportType.
func AllSports() []SportDef {
	result := make([]SportDef, 0, len(registry))
	for _, d := range registry {
		result = append(result, d)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].SportType < result[j].SportType
	})
	return result
}

// SportsInCategory returns all sport definitions belonging to a category,
// sorted alphabetically by SportType.
func SportsInCategory(categoryKey string) []SportDef {
	var result []SportDef
	for _, d := range registry {
		if d.CategoryKey == categoryKey {
			result = append(result, d)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].SportType < result[j].SportType
	})
	return result
}

// ---- Helpers ----

// titleCase converts a snake_case string to a human-readable title.
func titleCase(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

// ModifierDefaultsForTuple returns effective canonical modifier defaults for a
// sport tuple. Unknown sports and subtypes return zero defaults.
func ModifierDefaultsForTuple(sportType, subType string) ModifierDefaults {
	d, ok := registry[sportType]
	if !ok {
		return ModifierDefaults{}
	}
	defaults := cloneModifierDefaults(d.ModifierDefaults)
	if subType != "" && subType != "generic" {
		st, stOK := d.SubTypes[subType]
		if !stOK {
			return ModifierDefaults{}
		}
		defaults = mergeModifierDefaults(defaults, st.ModifierDefaults)
	}
	return defaults
}

func mergeModifierDefaults(parent, child ModifierDefaults) ModifierDefaults {
	merged := cloneModifierDefaults(parent)
	if child.IsIndoor != nil {
		merged.IsIndoor = ptrBool(*child.IsIndoor)
	}
	return merged
}

func cloneModifierDefaults(defaults ModifierDefaults) ModifierDefaults {
	cloned := ModifierDefaults{}
	if defaults.IsIndoor != nil {
		cloned.IsIndoor = ptrBool(*defaults.IsIndoor)
	}
	return cloned
}

// IsIndoor returns true when generated canonical modifier defaults mark the
// given sport/sub-type combination as indoor.
func IsIndoor(sportType, subType string) bool {
	defaults := ModifierDefaultsForTuple(sportType, subType)
	return defaults.IsIndoor != nil && *defaults.IsIndoor
}

func subTypeLabelCanStandAlone(d SportDef) bool {
	switch d.SportType {
	case "training", "fitness_equipment", "racket":
		return true
	default:
		return false
	}
}

func ptrFloat64(v float64) *float64 {
	return &v
}

func ptrBool(v bool) *bool {
	return &v
}
