package sport

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"sync"
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

// ModifierDefaults describes canonical tuple-level modifier defaults keyed by
// generated modifier key.
type ModifierDefaults map[string]bool

type GearDefaultSportOption struct {
	SportType string `json:"sport_type"`
	LabelDE   string `json:"label_de"`
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

type ActivityTraitBucket struct {
	Key                string   `json:"key"`
	TraitPolicyKey     string   `json:"trait_policy_key"`
	TraitPolicyVersion int      `json:"trait_policy_version"`
	SourceHash         string   `json:"source_hash"`
	SportType          string   `json:"sport_type"`
	SubType            string   `json:"sub_type"`
	IsIndoor           bool     `json:"is_indoor"`
	IsVirtual          bool     `json:"is_virtual"`
	IsElectric         bool     `json:"is_electric"`
	MovementFamilies   []string `json:"movement_families"`
	PropulsionModes    []string `json:"propulsion_modes"`
	EquipmentFamilies  []string `json:"equipment_families"`
	ActivityForm       string   `json:"activity_form"`
	EnvironmentState   string   `json:"environment_state"`
}

type ActivityTraitBucketInput struct {
	SportType  string
	SubType    string
	IsIndoor   bool
	IsVirtual  bool
	IsElectric bool
}

type activityTraitBucketTuple struct {
	SportType         string
	SubType           string
	MovementFamilies  []string
	PropulsionModes   []string
	EquipmentFamilies []string
	ActivityForm      string
	EnvironmentState  string
}

type CourseEligibilityInput struct {
	TargetKind           string
	TargetID             string
	TargetVersionID      string
	SegmentType          string
	SportGroup           string
	AllowedPredicates    []Predicate
	AllowedPredicatesSet bool
	Bidirectional        bool
	Visibility           string
	ElevationKind        string
	AdapterKey           string
	AdapterVersion       int
}

type CourseEligibilityProjectionRow struct {
	ActivityTraitBucketKey string
	CompatibilityPreset    string
	ActivityGroup          string
	MatchAllowed           bool
	CompletionAllowed      bool
	RankAllowed            bool
	LeaderboardTemplate    map[string]any
	SourceHash             string
	SourceHashPayload      map[string]any
}

type RuleTupleOption struct {
	SportType string `json:"sport_type"`
	SubType   string `json:"sub_type,omitempty"`
	LabelDE   string `json:"label_de"`
}

var (
	segmentProjectionIndexOnce      sync.Once
	segmentProjectionEntriesByType  map[string][]SegmentProjectionEntry
	segmentProjectionGroupsByTuple  map[string]string
	segmentProjectionActiveByType   map[string]map[string]struct{}
	segmentProjectionActiveByDomain map[string]struct{}
	activityTraitBucketIndexOnce    sync.Once
	activityTraitBucketsByKey       map[string]ActivityTraitBucket
)

type RuleModifierProjection struct {
	Key                 string `json:"key"`
	LabelDE             string `json:"label_de"`
	ConditionApplicable bool   `json:"condition_applicable"`
	ActionApplicable    bool   `json:"action_applicable"`
	RenderExisting      bool   `json:"render_existing"`
}

type RuleTraitProjection struct {
	Key     string            `json:"key"`
	Field   string            `json:"field"`
	LabelDE string            `json:"label_de"`
	Options []RuleTraitOption `json:"options"`
}

type RuleTraitOption struct {
	Key     string `json:"key"`
	LabelDE string `json:"label_de"`
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
	SubTypeOrder     []string              `json:"-"`
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
	if !ok {
		return SportDef{}, false
	}
	return cloneSportDef(d), true
}

// IsSelectableTuple reports whether a canonical sport tuple is available for
// new user-authored activity choices.
func IsSelectableTuple(sportType, subType string) bool {
	d, ok := registry[CanonicalSportTypeID(sportType)]
	if !ok {
		return false
	}
	subType = CanonicalSubTypeID(subType)
	if subType == "" {
		return d.Selectable
	}
	st, ok := d.SubTypes[subType]
	return ok && st.Selectable
}

// SportLabel returns the compact German display label for a sport tuple.
func SportLabel(sportType, subType string) string {
	d, ok := registry[sportType]
	if !ok {
		return titleCase(sportType)
	}
	subType = CanonicalSubTypeID(subType)
	if subType != "" {
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
	subType = CanonicalSubTypeID(subType)
	if subType == "" {
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
	subType = CanonicalSubTypeID(subType)
	if subType != "" {
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
	subType = CanonicalSubTypeID(subType)
	if subType != "" {
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

// AllSports returns all registered sport definitions in taxonomy display order.
func AllSports() []SportDef {
	result := make([]SportDef, len(generatedSportDefs))
	for i, def := range generatedSportDefs {
		result[i] = cloneSportDef(def)
	}
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
	ensureSegmentProjectionIndex()
	return append([]SegmentProjectionEntry(nil), segmentProjectionEntriesByType[segmentType]...)
}

func SegmentProjectionGroupForTuple(sportType, subType string) (string, bool) {
	ensureSegmentProjectionIndex()
	subType = CanonicalSubTypeID(subType)
	if subType != "" {
		if group, ok := segmentProjectionGroupsByTuple[projectionTupleKey(sportType, subType)]; ok {
			return group, true
		}
	}
	if group, ok := segmentProjectionGroupsByTuple[projectionTupleKey(sportType, "")]; ok {
		return group, true
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

func ActivityTraitBuckets() []ActivityTraitBucket {
	out := make([]ActivityTraitBucket, len(generatedActivityTraitBuckets))
	for i, bucket := range generatedActivityTraitBuckets {
		out[i] = cloneActivityTraitBucket(bucket)
	}
	return out
}

func ActivityTraitBucketByKey(key string) (ActivityTraitBucket, bool) {
	ensureActivityTraitBucketIndex()
	bucket, ok := activityTraitBucketsByKey[key]
	if !ok {
		return ActivityTraitBucket{}, false
	}
	return cloneActivityTraitBucket(bucket), true
}

func BuildActivityTraitBucketKey(input ActivityTraitBucketInput) string {
	sportType := CanonicalSportTypeID(input.SportType)
	subType := bucketSubType(input.SubType)
	var sb strings.Builder
	sb.Grow(len(sportType) + len(subType) + 48)
	sb.WriteString("atb:v1:sport=")
	sb.WriteString(sportType)
	sb.WriteString(":sub=")
	sb.WriteString(subType)
	sb.WriteString(":indoor=")
	sb.WriteString(boolKey(input.IsIndoor))
	sb.WriteString(":virtual=")
	sb.WriteString(boolKey(input.IsVirtual))
	sb.WriteString(":electric=")
	sb.WriteString(boolKey(input.IsElectric))
	return sb.String()
}

func ActivityTraitBucketKeyForActivity(sportType, subType string, state ActivityModifierState) (string, bool) {
	tuple, _, err := NormalizeSportTuple(sportType, subType)
	if err != nil {
		return "", false
	}
	return activityTraitBucketKeyForTuple(tuple.SportType, tuple.SubType, state)
}

func ActivityTraitBucketKeyForActivityLegacyCandidate(sportType, subType string, state ActivityModifierState) (string, bool) {
	tuple, _, err := NormalizeSportTuple(sportType, subType)
	if err == nil {
		if key, ok := activityTraitBucketKeyForTuple(tuple.SportType, tuple.SubType, state); ok {
			return key, true
		}
	}

	canonicalSport := CanonicalSportTypeID(sportType)
	if canonicalSport == UnknownSportType {
		return activityTraitBucketKeyForTuple(GenericSportType, "", state)
	}
	if _, ok := registry[canonicalSport]; ok {
		return activityTraitBucketKeyForTuple(canonicalSport, "", state)
	}
	return activityTraitBucketKeyForTuple(GenericSportType, "", state)
}

func CourseEligibilityRowsForSegment(input CourseEligibilityInput) []CourseEligibilityProjectionRow {
	if input.TargetKind != "" && input.TargetKind != "segment" {
		return nil
	}
	if !input.AllowedPredicatesSet && (input.SegmentType == "wintersport" || input.SegmentType == "wassersport") {
		return nil
	}
	if input.AllowedPredicatesSet && len(input.AllowedPredicates) == 0 {
		return nil
	}

	buckets := ActivityTraitBuckets()
	rows := make([]CourseEligibilityProjectionRow, 0, len(buckets))
	for _, bucket := range buckets {
		group := activityGroupForBucket(bucket)
		if !courseEligibilityBucketAllowed(input, bucket, group) {
			continue
		}
		row := CourseEligibilityProjectionRow{
			ActivityTraitBucketKey: bucket.Key,
			CompatibilityPreset:    input.SegmentType,
			ActivityGroup:          group,
			MatchAllowed:           true,
			CompletionAllowed:      true,
			RankAllowed:            input.SegmentType != "lift",
			LeaderboardTemplate: map[string]any{
				"activity_trait_bucket_key": bucket.Key,
				"assist_class":              "legacy",
				"direction":                 "legacy",
				"matching_contract_key":     "course_gps_v1",
				"matching_contract_version": 1,
				"ranking_scope":             "legacy_segment_wide",
			},
		}
		row.SourceHashPayload = courseEligibilitySourceHashPayload(input, bucket, group)
		row.SourceHash = sourceHash(row.SourceHashPayload)
		rows = append(rows, row)
	}
	return rows
}

func RuleTupleOptions() []RuleTupleOption {
	return append([]RuleTupleOption(nil), generatedRuleTupleOptions...)
}

func RuleModifierProjectionForTuple(sportType, subType, key string) (RuleModifierProjection, bool) {
	for _, modifier := range generatedRuleModifierProjections[projectionTupleKey(sportType, subType)] {
		if modifier.Key == key {
			return modifier, true
		}
	}
	return RuleModifierProjection{}, false
}

func RuleTraitProjections() []RuleTraitProjection {
	out := make([]RuleTraitProjection, len(generatedRuleTraitProjections))
	for i, projection := range generatedRuleTraitProjections {
		out[i] = projection
		out[i].Options = append([]RuleTraitOption(nil), projection.Options...)
	}
	return out
}

func RuleTraitValues(field string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, projection := range generatedRuleTraitProjections {
		if projection.Field != field {
			continue
		}
		for _, option := range projection.Options {
			out[option.Key] = struct{}{}
		}
	}
	return out
}

func EffectiveTraitsForTuple(sportType, subType string) (SportTraits, bool) {
	def, ok := registry[sportType]
	if !ok {
		return SportTraits{}, false
	}
	subType = CanonicalSubTypeID(subType)
	if subType != "" {
		if st, stOK := def.SubTypes[subType]; stOK {
			return cloneSportTraits(st.Traits), true
		}
	}
	return cloneSportTraits(def.Traits), true
}

// SportsInCategory returns all sport definitions belonging to a category in
// taxonomy display order.
func SportsInCategory(categoryKey string) []SportDef {
	var result []SportDef
	for _, d := range generatedSportDefs {
		if d.CategoryKey == categoryKey {
			result = append(result, cloneSportDef(d))
		}
	}
	return result
}

// OrderedSubTypes returns a sport's sub-types in taxonomy display order.
func OrderedSubTypes(def SportDef) []SubTypeDef {
	if len(def.SubTypes) == 0 {
		return nil
	}
	result := make([]SubTypeDef, 0, len(def.SubTypes))
	seen := make(map[string]struct{}, len(def.SubTypes))
	for _, key := range def.SubTypeOrder {
		if st, ok := def.SubTypes[key]; ok {
			result = append(result, st)
			seen[key] = struct{}{}
		}
	}
	if len(result) == len(def.SubTypes) {
		return result
	}
	fallbackKeys := make([]string, 0, len(def.SubTypes)-len(result))
	for key := range def.SubTypes {
		if _, ok := seen[key]; !ok {
			fallbackKeys = append(fallbackKeys, key)
		}
	}
	sort.Strings(fallbackKeys)
	for _, key := range fallbackKeys {
		result = append(result, def.SubTypes[key])
	}
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

func cloneSportDef(def SportDef) SportDef {
	def.Traits = cloneSportTraits(def.Traits)
	def.ModifierDefaults = cloneModifierDefaults(def.ModifierDefaults)
	def.SubTypeOrder = append([]string(nil), def.SubTypeOrder...)
	if def.SubTypes != nil {
		subTypes := make(map[string]SubTypeDef, len(def.SubTypes))
		for key, subType := range def.SubTypes {
			subTypes[key] = cloneSubTypeDef(subType)
		}
		def.SubTypes = subTypes
	}
	return def
}

func cloneSubTypeDef(def SubTypeDef) SubTypeDef {
	def.Traits = cloneSportTraits(def.Traits)
	def.ModifierDefaults = cloneModifierDefaults(def.ModifierDefaults)
	if def.MovingThreshold != nil {
		value := *def.MovingThreshold
		def.MovingThreshold = &value
	}
	return def
}

func cloneSportTraits(traits SportTraits) SportTraits {
	traits.Environments = cloneStringSlice(traits.Environments)
	traits.MovementFamilies = cloneStringSlice(traits.MovementFamilies)
	traits.EquipmentFamilies = cloneStringSlice(traits.EquipmentFamilies)
	traits.PropulsionModes = cloneStringSlice(traits.PropulsionModes)
	return traits
}

func cloneStringSlice(values []string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func cloneActivityTraitBucket(bucket ActivityTraitBucket) ActivityTraitBucket {
	bucket.MovementFamilies = cloneStringSlice(bucket.MovementFamilies)
	bucket.PropulsionModes = cloneStringSlice(bucket.PropulsionModes)
	bucket.EquipmentFamilies = cloneStringSlice(bucket.EquipmentFamilies)
	return bucket
}

// ModifierDefaultsForTuple returns effective canonical modifier defaults for a
// sport tuple. Unknown sports return zero defaults; unknown subtypes inherit the parent.
func ModifierDefaultsForTuple(sportType, subType string) ModifierDefaults {
	d, ok := registry[sportType]
	if !ok {
		return ModifierDefaults{}
	}
	defaults := cloneModifierDefaults(d.ModifierDefaults)
	subType = CanonicalSubTypeID(subType)
	if subType != "" {
		st, stOK := d.SubTypes[subType]
		if !stOK {
			return defaults
		}
		defaults = mergeModifierDefaults(defaults, st.ModifierDefaults)
	}
	return defaults
}

func mergeModifierDefaults(parent, child ModifierDefaults) ModifierDefaults {
	merged := cloneModifierDefaults(parent)
	for key, value := range child {
		merged[key] = value
	}
	return merged
}

func cloneModifierDefaults(defaults ModifierDefaults) ModifierDefaults {
	if defaults == nil {
		return ModifierDefaults{}
	}
	cloned := make(ModifierDefaults, len(defaults))
	for key, value := range defaults {
		cloned[key] = value
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

func projectionTupleKey(sportType, subType string) string {
	if subType == "" {
		return sportType
	}
	return sportType + ":" + subType
}

func bucketSubType(subType string) string {
	subType = CanonicalSubTypeID(subType)
	if subType == "" {
		return "_"
	}
	return subType
}

func boolKey(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

func buildGeneratedActivityTraitBuckets(tuples []activityTraitBucketTuple) []ActivityTraitBucket {
	buckets := make([]ActivityTraitBucket, 0, len(tuples)*8)
	for _, tuple := range tuples {
		for _, indoor := range []bool{false, true} {
			for _, virtual := range []bool{false, true} {
				for _, electric := range []bool{false, true} {
					bucket := ActivityTraitBucket{
						Key: BuildActivityTraitBucketKey(ActivityTraitBucketInput{
							SportType:  tuple.SportType,
							SubType:    tuple.SubType,
							IsIndoor:   indoor,
							IsVirtual:  virtual,
							IsElectric: electric,
						}),
						TraitPolicyKey:     "activity_traits",
						TraitPolicyVersion: 1,
						SportType:          tuple.SportType,
						SubType:            bucketSubType(tuple.SubType),
						IsIndoor:           indoor,
						IsVirtual:          virtual,
						IsElectric:         electric,
						MovementFamilies:   cloneStringSlice(tuple.MovementFamilies),
						PropulsionModes:    cloneStringSlice(tuple.PropulsionModes),
						EquipmentFamilies:  cloneStringSlice(tuple.EquipmentFamilies),
						ActivityForm:       tuple.ActivityForm,
						EnvironmentState:   tuple.EnvironmentState,
					}
					bucket.SourceHash = sourceHash(map[string]any{
						"activity_form":        bucket.ActivityForm,
						"environment_state":    bucket.EnvironmentState,
						"equipment_families":   bucket.EquipmentFamilies,
						"is_electric":          bucket.IsElectric,
						"is_indoor":            bucket.IsIndoor,
						"is_virtual":           bucket.IsVirtual,
						"movement_families":    bucket.MovementFamilies,
						"propulsion_modes":     bucket.PropulsionModes,
						"sport_type":           bucket.SportType,
						"sub_type":             bucket.SubType,
						"trait_policy_key":     bucket.TraitPolicyKey,
						"trait_policy_version": bucket.TraitPolicyVersion,
					})
					buckets = append(buckets, bucket)
				}
			}
		}
	}
	return buckets
}

func activityTraitBucketKeyForTuple(sportType, subType string, state ActivityModifierState) (string, bool) {
	key := BuildActivityTraitBucketKey(ActivityTraitBucketInput{
		SportType:  sportType,
		SubType:    subType,
		IsIndoor:   state.IsIndoor,
		IsVirtual:  state.IsVirtual,
		IsElectric: state.IsElectric,
	})
	if _, ok := ActivityTraitBucketByKey(key); !ok {
		return "", false
	}
	return key, true
}

func activityGroupForBucket(bucket ActivityTraitBucket) string {
	subType := bucket.SubType
	if subType == "_" {
		subType = ""
	}
	if bucket.SportType == GenericSportType {
		return "generic"
	}
	if group, ok := SegmentProjectionGroupForTuple(bucket.SportType, subType); ok {
		return group
	}
	return "generic"
}

func courseEligibilityBucketAllowed(input CourseEligibilityInput, bucket ActivityTraitBucket, group string) bool {
	if input.AllowedPredicatesSet {
		subType := bucket.SubType
		if subType == "_" {
			subType = ""
		}
		return PredicatesMatch(input.AllowedPredicates, bucket.SportType, &subType)
	}
	if input.SportGroup == "generic" {
		return true
	}
	return group == input.SportGroup
}

func courseEligibilitySourceHashPayload(input CourseEligibilityInput, bucket ActivityTraitBucket, activityGroup string) map[string]any {
	return map[string]any{
		"activity_group":            activityGroup,
		"activity_trait_bucket_key": bucket.Key,
		"activity_trait_source":     bucket.SourceHash,
		"adapter_key":               input.AdapterKey,
		"adapter_version":           input.AdapterVersion,
		"allowed_predicates":        input.AllowedPredicates,
		"allowed_predicates_set":    input.AllowedPredicatesSet,
		"bidirectional":             input.Bidirectional,
		"elevation_kind":            input.ElevationKind,
		"matching_contract_key":     "course_gps_v1",
		"matching_contract_version": 1,
		"policy_key":                "course_eligibility",
		"policy_version":            1,
		"segment_type":              input.SegmentType,
		"sport_group":               input.SportGroup,
		"target_id":                 input.TargetID,
		"target_kind":               "segment",
		"target_version_id":         input.TargetVersionID,
		"visibility":                input.Visibility,
	}
}

func sourceHash(payload map[string]any) string {
	encoded, _ := json.Marshal(payload)
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func segmentProjectionActiveDomains(segmentType string) map[string]struct{} {
	ensureSegmentProjectionIndex()
	return segmentProjectionActiveByType[segmentType]
}

func segmentProjectionEntryIsActive(entry SegmentProjectionEntry) bool {
	ensureSegmentProjectionIndex()
	_, ok := segmentProjectionActiveByDomain[entry.SegmentType+":"+entry.Domain]
	return ok
}

func ensureSegmentProjectionIndex() {
	segmentProjectionIndexOnce.Do(func() {
		segmentProjectionEntriesByType = map[string][]SegmentProjectionEntry{}
		segmentProjectionGroupsByTuple = map[string]string{}
		segmentProjectionActiveByType = map[string]map[string]struct{}{}
		segmentProjectionActiveByDomain = map[string]struct{}{}

		for _, segmentType := range generatedSegmentProjectionSegmentTypes {
			active := map[string]struct{}{}
			for _, domain := range segmentType.ActiveDomains {
				active[domain] = struct{}{}
				segmentProjectionActiveByDomain[segmentType.Key+":"+domain] = struct{}{}
			}
			segmentProjectionActiveByType[segmentType.Key] = active
		}

		for _, entry := range generatedSegmentProjectionEntries {
			if !segmentProjectionEntryIsActiveIndexed(entry) {
				continue
			}
			segmentProjectionEntriesByType[entry.SegmentType] = append(segmentProjectionEntriesByType[entry.SegmentType], entry)
			key := projectionTupleKey(entry.SportType, entry.SubType)
			if _, exists := segmentProjectionGroupsByTuple[key]; !exists {
				segmentProjectionGroupsByTuple[key] = entry.SportGroup
			}
		}
	})
}

func ensureActivityTraitBucketIndex() {
	activityTraitBucketIndexOnce.Do(func() {
		activityTraitBucketsByKey = make(map[string]ActivityTraitBucket, len(generatedActivityTraitBuckets))
		for _, bucket := range generatedActivityTraitBuckets {
			activityTraitBucketsByKey[bucket.Key] = bucket
		}
	})
}

func segmentProjectionEntryIsActiveIndexed(entry SegmentProjectionEntry) bool {
	_, ok := segmentProjectionActiveByDomain[entry.SegmentType+":"+entry.Domain]
	return ok
}

// IsIndoor returns true when generated canonical modifier defaults mark the
// given sport/sub-type combination as indoor.
func IsIndoor(sportType, subType string) bool {
	defaults := ModifierDefaultsForTuple(sportType, subType)
	return defaults["is_indoor"]
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
