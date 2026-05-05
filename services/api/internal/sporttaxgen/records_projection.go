package sporttaxgen

import (
	"fmt"
	"io"
	"sort"

	"gopkg.in/yaml.v2"
)

type RecordsProjection struct {
	CategorySets     []RecordCategorySet     `yaml:"category_sets"`
	CategoryProfiles []RecordCategoryProfile `yaml:"category_profiles"`
	RecordGroups     []RecordGroup           `yaml:"record_groups"`
	MainCategories   []RecordMainCategory    `yaml:"main_categories"`
}

type RecordCategorySet struct {
	Key        string           `yaml:"key"`
	Categories []RecordCategory `yaml:"categories"`
}

type RecordCategory struct {
	Key        string    `yaml:"key"`
	Type       string    `yaml:"type"`
	DistanceM  float64   `yaml:"distance_m"`
	ElevationM float64   `yaml:"elevation_m"`
	DurationS  int       `yaml:"duration_s"`
	Labels     LocaleMap `yaml:"labels"`
}

type RecordCategoryProfile struct {
	Key          string   `yaml:"key"`
	CategorySets []string `yaml:"category_sets"`
}

type RecordGroup struct {
	Key             string            `yaml:"key"`
	Labels          LocaleMap         `yaml:"labels"`
	MainCategory    string            `yaml:"main_category"`
	Color           string            `yaml:"color"`
	CategoryProfile string            `yaml:"category_profile"`
	Badge           string            `yaml:"badge"`
	Match           []RecordMatchRule `yaml:"match"`
}

type RecordMatchRule struct {
	Priority        int             `yaml:"priority"`
	SportType       string          `yaml:"sport_type"`
	SubTypes        []string        `yaml:"sub_types"`
	ExcludeSubTypes []string        `yaml:"exclude_sub_types"`
	Modifiers       RecordModifiers `yaml:"modifiers"`
}

type RecordModifiers struct {
	IsIndoor   *bool `yaml:"is_indoor"`
	IsVirtual  *bool `yaml:"is_virtual"`
	IsElectric *bool `yaml:"is_electric"`
}

type RecordMainCategory struct {
	Key          string            `yaml:"key"`
	Labels       LocaleMap         `yaml:"labels"`
	Color        string            `yaml:"color"`
	RecordGroups []string          `yaml:"record_groups"`
	SubFilters   []RecordSubFilter `yaml:"sub_filters"`
	PowerCurve   *RecordPowerCurve `yaml:"power_curve"`
}

type RecordSubFilter struct {
	Label        string   `yaml:"label"`
	RecordGroups []string `yaml:"record_groups"`
}

type RecordPowerCurve struct {
	Label        string   `yaml:"label"`
	RecordGroups []string `yaml:"record_groups"`
}

type ExpandedRecordsProjection struct {
	Groups         []ExpandedRecordGroup
	MainCategories []ExpandedRecordMainCategory
	MatchRules     []ExpandedRecordMatchRule
}

type ExpandedRecordGroup struct {
	Key          string
	Label        LocaleMap
	MainCategory string
	Color        string
	Badge        string
	Categories   []RecordCategory
}

type ExpandedRecordMainCategory struct {
	Key          string
	Label        LocaleMap
	Color        string
	RecordGroups []string
	SubFilters   []RecordSubFilter
	PowerCurve   *RecordPowerCurve
}

type ExpandedRecordMatchRule struct {
	RecordGroup     string
	Priority        int
	Sequence        int
	SportType       string
	SubTypes        []string
	ExcludeSubTypes []string
	Modifiers       RecordModifiers
}

func LoadRecordsProjection(r io.Reader, taxonomy Taxonomy) (RecordsProjection, error) {
	var projection RecordsProjection
	dec := yaml.NewDecoder(r)
	dec.SetStrict(true)
	if err := dec.Decode(&projection); err != nil {
		return RecordsProjection{}, err
	}
	if err := validateRecordsProjection(projection, taxonomy); err != nil {
		return RecordsProjection{}, err
	}
	return projection, nil
}

func ExpandRecordsProjection(taxonomy Taxonomy, projection RecordsProjection) (ExpandedRecordsProjection, error) {
	if err := validateRecordsProjection(projection, taxonomy); err != nil {
		return ExpandedRecordsProjection{}, err
	}
	categorySets := make(map[string]RecordCategorySet, len(projection.CategorySets))
	for _, set := range projection.CategorySets {
		categorySets[set.Key] = set
	}
	categoryProfiles := make(map[string]RecordCategoryProfile, len(projection.CategoryProfiles))
	for _, profile := range projection.CategoryProfiles {
		categoryProfiles[profile.Key] = profile
	}

	out := ExpandedRecordsProjection{
		Groups:         make([]ExpandedRecordGroup, 0, len(projection.RecordGroups)),
		MainCategories: make([]ExpandedRecordMainCategory, 0, len(projection.MainCategories)),
	}
	sequence := 0
	for _, group := range projection.RecordGroups {
		expandedGroup := ExpandedRecordGroup{
			Key:          group.Key,
			Label:        cloneLocaleMap(group.Labels),
			MainCategory: group.MainCategory,
			Color:        group.Color,
			Badge:        group.Badge,
			Categories:   []RecordCategory{},
		}
		for _, setKey := range categoryProfiles[group.CategoryProfile].CategorySets {
			for _, category := range categorySets[setKey].Categories {
				expandedGroup.Categories = append(expandedGroup.Categories, cloneRecordCategory(category))
			}
		}
		out.Groups = append(out.Groups, expandedGroup)

		for _, rule := range group.Match {
			out.MatchRules = append(out.MatchRules, ExpandedRecordMatchRule{
				RecordGroup:     group.Key,
				Priority:        rule.Priority,
				Sequence:        sequence,
				SportType:       rule.SportType,
				SubTypes:        append([]string(nil), rule.SubTypes...),
				ExcludeSubTypes: append([]string(nil), rule.ExcludeSubTypes...),
				Modifiers:       cloneRecordModifiers(rule.Modifiers),
			})
			sequence++
		}
	}
	sort.SliceStable(out.MatchRules, func(i, j int) bool {
		if out.MatchRules[i].Priority != out.MatchRules[j].Priority {
			return out.MatchRules[i].Priority > out.MatchRules[j].Priority
		}
		return out.MatchRules[i].Sequence < out.MatchRules[j].Sequence
	})

	for _, category := range projection.MainCategories {
		out.MainCategories = append(out.MainCategories, ExpandedRecordMainCategory{
			Key:          category.Key,
			Label:        cloneLocaleMap(category.Labels),
			Color:        category.Color,
			RecordGroups: append([]string(nil), category.RecordGroups...),
			SubFilters:   cloneRecordSubFilters(category.SubFilters),
			PowerCurve:   cloneRecordPowerCurve(category.PowerCurve),
		})
	}
	return out, nil
}

func validateRecordsProjection(projection RecordsProjection, taxonomy Taxonomy) error {
	idx := newProjectionIndexes(taxonomy)
	categorySets, err := validateRecordCategorySets(projection.CategorySets)
	if err != nil {
		return err
	}
	categoryProfiles, err := validateRecordCategoryProfiles(projection.CategoryProfiles, categorySets)
	if err != nil {
		return err
	}
	recordGroups, err := validateRecordGroups(projection.RecordGroups, categoryProfiles, idx)
	if err != nil {
		return err
	}
	if err := validateRecordMainCategories(projection.MainCategories, recordGroups); err != nil {
		return err
	}
	if err := validateRecordGroupMainCategories(projection.RecordGroups, projection.MainCategories); err != nil {
		return err
	}
	if err := validateExpandedRecordCategoryKeys(projection.RecordGroups, categoryProfiles, categorySets); err != nil {
		return err
	}
	return nil
}

func validateRecordCategorySets(sets []RecordCategorySet) (map[string]RecordCategorySet, error) {
	if len(sets) == 0 {
		return nil, fmt.Errorf("missing category_sets")
	}
	byKey := map[string]RecordCategorySet{}
	for i, set := range sets {
		if set.Key == "" {
			return nil, fmt.Errorf("category_sets[%d]: missing key", i)
		}
		if _, ok := byKey[set.Key]; ok {
			return nil, fmt.Errorf("duplicate category_set key %q", set.Key)
		}
		if len(set.Categories) == 0 {
			return nil, fmt.Errorf("category_set %q: missing categories", set.Key)
		}
		for j, category := range set.Categories {
			if err := validateRecordCategory(category); err != nil {
				return nil, fmt.Errorf("category_set %q categories[%d]: %w", set.Key, j, err)
			}
		}
		byKey[set.Key] = set
	}
	return byKey, nil
}

func validateRecordCategory(category RecordCategory) error {
	if category.Key == "" {
		return fmt.Errorf("missing key")
	}
	if category.Labels["de"] == "" {
		return fmt.Errorf("category %q: missing labels.de", category.Key)
	}
	switch category.Type {
	case "time_lower":
		hasDistance := category.DistanceM > 0
		hasElevation := category.ElevationM > 0
		if hasDistance == hasElevation {
			return fmt.Errorf("category %q: time_lower requires exactly one positive distance_m or elevation_m", category.Key)
		}
		if category.DurationS != 0 {
			return fmt.Errorf("category %q: time_lower must not set duration_s", category.Key)
		}
	case "value_higher":
		if category.DurationS <= 0 {
			return fmt.Errorf("category %q: value_higher requires positive duration_s", category.Key)
		}
		if category.DistanceM != 0 || category.ElevationM != 0 {
			return fmt.Errorf("category %q: value_higher must not set distance_m or elevation_m", category.Key)
		}
	case "value_superlative":
		if category.DistanceM != 0 || category.ElevationM != 0 || category.DurationS != 0 {
			return fmt.Errorf("category %q: value_superlative must not set metric fields", category.Key)
		}
	default:
		return fmt.Errorf("category %q: invalid type %q", category.Key, category.Type)
	}
	return nil
}

func validateRecordCategoryProfiles(profiles []RecordCategoryProfile, categorySets map[string]RecordCategorySet) (map[string]RecordCategoryProfile, error) {
	if len(profiles) == 0 {
		return nil, fmt.Errorf("missing category_profiles")
	}
	byKey := map[string]RecordCategoryProfile{}
	for i, profile := range profiles {
		if profile.Key == "" {
			return nil, fmt.Errorf("category_profiles[%d]: missing key", i)
		}
		if _, ok := byKey[profile.Key]; ok {
			return nil, fmt.Errorf("duplicate category_profile key %q", profile.Key)
		}
		if len(profile.CategorySets) == 0 {
			return nil, fmt.Errorf("category_profile %q: missing category_sets", profile.Key)
		}
		for _, setKey := range profile.CategorySets {
			if _, ok := categorySets[setKey]; !ok {
				return nil, fmt.Errorf("category_profile %q: unknown category_set %q", profile.Key, setKey)
			}
		}
		byKey[profile.Key] = profile
	}
	return byKey, nil
}

func validateRecordGroups(groups []RecordGroup, profiles map[string]RecordCategoryProfile, idx projectionIndexes) (map[string]RecordGroup, error) {
	if len(groups) == 0 {
		return nil, fmt.Errorf("missing record_groups")
	}
	byKey := map[string]RecordGroup{}
	for i, group := range groups {
		if group.Key == "" {
			return nil, fmt.Errorf("record_groups[%d]: missing key", i)
		}
		if _, ok := byKey[group.Key]; ok {
			return nil, fmt.Errorf("duplicate record_group key %q", group.Key)
		}
		if group.Labels["de"] == "" {
			return nil, fmt.Errorf("record_group %q: missing labels.de", group.Key)
		}
		if group.MainCategory == "" {
			return nil, fmt.Errorf("record_group %q: missing main_category", group.Key)
		}
		if !validHex.MatchString(group.Color) {
			return nil, fmt.Errorf("record_group %q: invalid color %q", group.Key, group.Color)
		}
		if _, ok := profiles[group.CategoryProfile]; !ok {
			return nil, fmt.Errorf("record_group %q: unknown category_profile %q", group.Key, group.CategoryProfile)
		}
		if len(group.Match) == 0 {
			return nil, fmt.Errorf("record_group %q: missing match", group.Key)
		}
		for j, rule := range group.Match {
			if err := validateRecordMatchRule(rule, idx); err != nil {
				return nil, fmt.Errorf("record_group %q match[%d]: %w", group.Key, j, err)
			}
		}
		byKey[group.Key] = group
	}
	return byKey, nil
}

func validateRecordMatchRule(rule RecordMatchRule, idx projectionIndexes) error {
	if rule.SportType == "" {
		return fmt.Errorf("missing sport_type")
	}
	sport, ok := idx.sports[rule.SportType]
	if !ok {
		return fmt.Errorf("unknown sport_type %q", rule.SportType)
	}
	for _, subType := range rule.SubTypes {
		if subType == "" {
			continue
		}
		if !sportHasSubType(sport, subType) {
			return fmt.Errorf("sport %q has no sub_type %q", rule.SportType, subType)
		}
	}
	for _, subType := range rule.ExcludeSubTypes {
		if subType == "" {
			return fmt.Errorf("exclude_sub_types must not contain empty subtype sentinel")
		}
		if !sportHasSubType(sport, subType) {
			return fmt.Errorf("sport %q has no excluded sub_type %q", rule.SportType, subType)
		}
	}
	return nil
}

func validateRecordMainCategories(categories []RecordMainCategory, groups map[string]RecordGroup) error {
	if len(categories) == 0 {
		return fmt.Errorf("missing main_categories")
	}
	byKey := map[string]RecordMainCategory{}
	for i, category := range categories {
		if category.Key == "" {
			return fmt.Errorf("main_categories[%d]: missing key", i)
		}
		if _, ok := byKey[category.Key]; ok {
			return fmt.Errorf("duplicate main_category key %q", category.Key)
		}
		if category.Labels["de"] == "" {
			return fmt.Errorf("main_category %q: missing labels.de", category.Key)
		}
		if !validHex.MatchString(category.Color) {
			return fmt.Errorf("main_category %q: invalid color %q", category.Key, category.Color)
		}
		if len(category.RecordGroups) == 0 {
			return fmt.Errorf("main_category %q: missing record_groups", category.Key)
		}
		for _, groupKey := range category.RecordGroups {
			if _, ok := groups[groupKey]; !ok {
				return fmt.Errorf("main_category %q: unknown record_group %q", category.Key, groupKey)
			}
		}
		for j, filter := range category.SubFilters {
			if filter.Label == "" {
				return fmt.Errorf("main_category %q sub_filters[%d]: missing label", category.Key, j)
			}
			if len(filter.RecordGroups) == 0 {
				return fmt.Errorf("main_category %q sub_filters[%d]: missing record_groups", category.Key, j)
			}
			for _, groupKey := range filter.RecordGroups {
				if _, ok := groups[groupKey]; !ok {
					return fmt.Errorf("main_category %q sub_filters[%d]: unknown record_group %q", category.Key, j, groupKey)
				}
			}
		}
		if category.PowerCurve != nil {
			if category.PowerCurve.Label == "" {
				return fmt.Errorf("main_category %q power_curve: missing label", category.Key)
			}
			if len(category.PowerCurve.RecordGroups) == 0 {
				return fmt.Errorf("main_category %q power_curve: missing record_groups", category.Key)
			}
			for _, groupKey := range category.PowerCurve.RecordGroups {
				if _, ok := groups[groupKey]; !ok {
					return fmt.Errorf("main_category %q power_curve: unknown record_group %q", category.Key, groupKey)
				}
			}
		}
		byKey[category.Key] = category
	}
	return nil
}

func validateRecordGroupMainCategories(groups []RecordGroup, categories []RecordMainCategory) error {
	byKey := map[string]RecordMainCategory{}
	for _, category := range categories {
		byKey[category.Key] = category
	}
	for _, group := range groups {
		category, ok := byKey[group.MainCategory]
		if !ok {
			return fmt.Errorf("record_group %q: unknown main_category %q", group.Key, group.MainCategory)
		}
		if !containsString(category.RecordGroups, group.Key) {
			return fmt.Errorf("record_group %q: main_category %q does not include group", group.Key, group.MainCategory)
		}
	}
	return nil
}

func validateExpandedRecordCategoryKeys(groups []RecordGroup, profiles map[string]RecordCategoryProfile, sets map[string]RecordCategorySet) error {
	for _, group := range groups {
		seen := map[string]struct{}{}
		profile := profiles[group.CategoryProfile]
		for _, setKey := range profile.CategorySets {
			for _, category := range sets[setKey].Categories {
				if _, ok := seen[category.Key]; ok {
					return fmt.Errorf("record_group %q: duplicate expanded category key %q", group.Key, category.Key)
				}
				seen[category.Key] = struct{}{}
			}
		}
	}
	return nil
}

func cloneRecordCategory(category RecordCategory) RecordCategory {
	category.Labels = cloneLocaleMap(category.Labels)
	return category
}

func cloneRecordModifiers(modifiers RecordModifiers) RecordModifiers {
	return RecordModifiers{
		IsIndoor:   cloneBoolPtr(modifiers.IsIndoor),
		IsVirtual:  cloneBoolPtr(modifiers.IsVirtual),
		IsElectric: cloneBoolPtr(modifiers.IsElectric),
	}
}

func cloneBoolPtr(value *bool) *bool {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneRecordSubFilters(filters []RecordSubFilter) []RecordSubFilter {
	out := make([]RecordSubFilter, 0, len(filters))
	for _, filter := range filters {
		out = append(out, RecordSubFilter{
			Label:        filter.Label,
			RecordGroups: append([]string(nil), filter.RecordGroups...),
		})
	}
	return out
}

func cloneRecordPowerCurve(curve *RecordPowerCurve) *RecordPowerCurve {
	if curve == nil {
		return nil
	}
	return &RecordPowerCurve{
		Label:        curve.Label,
		RecordGroups: append([]string(nil), curve.RecordGroups...),
	}
}
