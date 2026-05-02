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

// ActivityFilterProjectionPreset is the generated option tree used by activity
// and map filters.
type ActivityFilterProjectionPreset struct {
	Key     string                          `json:"key"`
	LabelDE string                          `json:"label_de"`
	Groups  []ActivityFilterProjectionGroup `json:"groups"`
}

// ActivityFilterProjectionGroup groups related activity-filter leaves.
type ActivityFilterProjectionGroup struct {
	Key     string                         `json:"key"`
	LabelDE string                         `json:"label_de"`
	Leaves  []ActivityFilterProjectionLeaf `json:"leaves"`
}

// ActivityFilterProjectionLeaf describes a concrete sport tuple displayed in a
// filter group.
type ActivityFilterProjectionLeaf struct {
	SportType string `json:"sport_type"`
	SubType   string `json:"sub_type,omitempty"`
	LabelDE   string `json:"label_de"`
}

type SegmentProjectionDomain struct {
	Key     string `json:"key"`
	LabelDE string `json:"label_de"`
	Status  string `json:"status"`
}

type SegmentProjectionSegmentType struct {
	Key               string   `json:"key"`
	LabelDE           string   `json:"label_de"`
	ActiveDomains     []string `json:"active_domains"`
	CompatibilityRole string   `json:"compatibility_role"`
}

type SegmentProjectionEntry struct {
	SegmentType string `json:"segment_type"`
	Domain      string `json:"domain"`
	SportType   string `json:"sport_type"`
	SubType     string `json:"sub_type,omitempty"`
	SportGroup  string `json:"sport_group"`
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

// ActivityFilterProjectionPresets returns generated activity-filter projection
// presets in display order.
func ActivityFilterProjectionPresets() []ActivityFilterProjectionPreset {
	return cloneActivityFilterProjectionPresets(generatedActivityFilterProjectionPresets)
}

// ActivityFilterProjectionPresetByKey returns one generated activity-filter
// projection preset by key.
func ActivityFilterProjectionPresetByKey(key string) (ActivityFilterProjectionPreset, bool) {
	for _, preset := range generatedActivityFilterProjectionPresets {
		if preset.Key == key {
			return cloneActivityFilterProjectionPreset(preset), true
		}
	}
	return ActivityFilterProjectionPreset{}, false
}

func SegmentProjectionEntriesForType(segmentType string) []SegmentProjectionEntry {
	entries := make([]SegmentProjectionEntry, 0)
	activeDomains := segmentProjectionActiveDomains(segmentType)
	for _, entry := range generatedSegmentProjectionEntries {
		if entry.SegmentType != segmentType {
			continue
		}
		if _, ok := activeDomains[entry.Domain]; !ok {
			continue
		}
		entries = append(entries, entry)
	}
	return entries
}

func SegmentProjectionGroupForTuple(sportType, subType string) (string, bool) {
	if subType != "" && subType != "generic" {
		for _, entry := range generatedSegmentProjectionEntries {
			if entry.SportType == sportType && entry.SubType == subType && segmentProjectionEntryIsActive(entry) {
				return entry.SportGroup, true
			}
		}
	}
	for _, entry := range generatedSegmentProjectionEntries {
		if entry.SportType == sportType && entry.SubType == "" && segmentProjectionEntryIsActive(entry) {
			return entry.SportGroup, true
		}
	}
	return "", false
}

func SegmentProjectionGroupForPredicates(preds []Predicate) string {
	if len(preds) == 0 {
		return "generic"
	}
	group := ""
	for _, pred := range preds {
		subType := ""
		if pred.SubType != nil {
			subType = *pred.SubType
		}
		next, ok := SegmentProjectionGroupForTuple(pred.Sport, subType)
		if !ok {
			return "generic"
		}
		if group == "" {
			group = next
			continue
		}
		if group != next {
			return "generic"
		}
	}
	if group == "" {
		return "generic"
	}
	return group
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

func cloneActivityFilterProjectionPresets(in []ActivityFilterProjectionPreset) []ActivityFilterProjectionPreset {
	out := make([]ActivityFilterProjectionPreset, len(in))
	for i := range in {
		out[i] = cloneActivityFilterProjectionPreset(in[i])
	}
	return out
}

func cloneActivityFilterProjectionPreset(in ActivityFilterProjectionPreset) ActivityFilterProjectionPreset {
	out := in
	out.Groups = make([]ActivityFilterProjectionGroup, len(in.Groups))
	for i := range in.Groups {
		out.Groups[i] = in.Groups[i]
		out.Groups[i].Leaves = append([]ActivityFilterProjectionLeaf(nil), in.Groups[i].Leaves...)
	}
	return out
}

func segmentProjectionActiveDomains(segmentType string) map[string]struct{} {
	active := map[string]struct{}{}
	for _, candidate := range generatedSegmentProjectionSegmentTypes {
		if candidate.Key != segmentType {
			continue
		}
		for _, domain := range candidate.ActiveDomains {
			active[domain] = struct{}{}
		}
		break
	}
	return active
}

func segmentProjectionEntryIsActive(entry SegmentProjectionEntry) bool {
	_, ok := segmentProjectionActiveDomains(entry.SegmentType)[entry.Domain]
	return ok
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
