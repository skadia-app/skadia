package sporttaxgen

import (
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v2"
)

type ActivityFilterProjections struct {
	Presets []ActivityFilterPreset `yaml:"presets"`
}

type ActivityFilterPreset struct {
	Key    string                `yaml:"key"`
	Labels LocaleMap             `yaml:"labels"`
	Groups []ActivityFilterGroup `yaml:"groups"`
}

type ActivityFilterGroup struct {
	Key      string                `yaml:"key"`
	Labels   LocaleMap             `yaml:"labels"`
	LabelRef string                `yaml:"label_ref"`
	Entries  []ActivityFilterEntry `yaml:"entries"`
}

type ActivityFilterEntry struct {
	CategoryKey string    `yaml:"category_key"`
	SportType   string    `yaml:"sport_type"`
	SubType     string    `yaml:"sub_type"`
	SportOrder  []string  `yaml:"sport_order"`
	Labels      LocaleMap `yaml:"labels"`
	LabelMode   string    `yaml:"label_mode"`
}

type ExpandedActivityFilterPreset struct {
	Key    string
	Label  LocaleMap
	Groups []ExpandedActivityFilterGroup
}

type ExpandedActivityFilterGroup struct {
	Key    string
	Label  LocaleMap
	Leaves []ExpandedActivityFilterLeaf
}

type ExpandedActivityFilterLeaf struct {
	SportType string
	SubType   string
	Label     LocaleMap
}

type projectionIndexes struct {
	categories map[string]Category
	sports     map[string]Sport
	byCategory map[string][]Sport
}

type pendingProjectionLeaf struct {
	SportType         string
	SubType           string
	ParentLabelDE     string
	CompactLabelDE    string
	WithParentLabelDE string
	ExplicitLabelDE   string
	ForceParentLabel  bool
}

var promotedProjectionParents = map[string]struct{}{
	"training":          {},
	"fitness_equipment": {},
	"racket":            {},
}

var parentContextSubTypes = map[string]struct{}{
	"warm_up":  {},
	"exercise": {},
	"match":    {},
}

func LoadActivityFilterProjections(r io.Reader, taxonomy Taxonomy) (ActivityFilterProjections, error) {
	var projections ActivityFilterProjections
	dec := yaml.NewDecoder(r)
	dec.SetStrict(true)
	if err := dec.Decode(&projections); err != nil {
		return ActivityFilterProjections{}, err
	}
	if err := validateActivityFilterProjections(projections, taxonomy); err != nil {
		return ActivityFilterProjections{}, err
	}
	return projections, nil
}

func ExpandActivityFilterPresets(taxonomy Taxonomy, projections ActivityFilterProjections) ([]ExpandedActivityFilterPreset, error) {
	idx := newProjectionIndexes(taxonomy)
	expanded := make([]ExpandedActivityFilterPreset, 0, len(projections.Presets))
	for _, preset := range projections.Presets {
		outPreset := ExpandedActivityFilterPreset{Key: preset.Key, Label: cloneLocaleMap(preset.Labels)}
		presetTuples := map[string]struct{}{}
		for _, group := range preset.Groups {
			groupLabel, err := resolveGroupLabel(group, idx)
			if err != nil {
				return nil, fmt.Errorf("preset %q group %q: %w", preset.Key, group.Key, err)
			}
			pending := []pendingProjectionLeaf{}
			for _, entry := range group.Entries {
				leaves, err := expandProjectionEntry(entry, idx)
				if err != nil {
					return nil, fmt.Errorf("preset %q group %q: %w", preset.Key, group.Key, err)
				}
				pending = append(pending, leaves...)
			}
			outGroup := ExpandedActivityFilterGroup{Key: group.Key, Label: groupLabel}
			for _, leaf := range disambiguateProjectionLeaves(pending) {
				key := projectionTupleKey(leaf.SportType, leaf.SubType)
				if _, ok := presetTuples[key]; ok {
					return nil, fmt.Errorf("preset %q: duplicate tuple %q", preset.Key, key)
				}
				presetTuples[key] = struct{}{}
				outGroup.Leaves = append(outGroup.Leaves, leaf)
			}
			if err := validateProjectionGroupLabels(preset.Key, outGroup); err != nil {
				return nil, err
			}
			outPreset.Groups = append(outPreset.Groups, outGroup)
		}
		expanded = append(expanded, outPreset)
	}
	return expanded, nil
}

func validateActivityFilterProjections(projections ActivityFilterProjections, taxonomy Taxonomy) error {
	idx := newProjectionIndexes(taxonomy)
	if len(projections.Presets) == 0 {
		return fmt.Errorf("missing presets")
	}
	presetKeys := map[string]struct{}{}
	for i, preset := range projections.Presets {
		if preset.Key == "" {
			return fmt.Errorf("presets[%d]: missing key", i)
		}
		if _, ok := presetKeys[preset.Key]; ok {
			return fmt.Errorf("duplicate preset key %q", preset.Key)
		}
		presetKeys[preset.Key] = struct{}{}
		if preset.Labels["de"] == "" {
			return fmt.Errorf("preset %q: missing labels.de", preset.Key)
		}
		if len(preset.Groups) == 0 {
			return fmt.Errorf("preset %q: missing groups", preset.Key)
		}
		groupKeys := map[string]struct{}{}
		for j, group := range preset.Groups {
			if group.Key == "" {
				return fmt.Errorf("preset %q groups[%d]: missing key", preset.Key, j)
			}
			if _, ok := groupKeys[group.Key]; ok {
				return fmt.Errorf("preset %q: duplicate group key %q", preset.Key, group.Key)
			}
			groupKeys[group.Key] = struct{}{}
			if (group.Labels["de"] == "") == (group.LabelRef == "") {
				return fmt.Errorf("preset %q group %q: exactly one of labels.de or label_ref is required", preset.Key, group.Key)
			}
			if group.LabelRef != "" {
				if _, err := resolveGroupLabel(group, idx); err != nil {
					return fmt.Errorf("preset %q group %q: %w", preset.Key, group.Key, err)
				}
			}
			if len(group.Entries) == 0 {
				return fmt.Errorf("preset %q group %q: missing entries", preset.Key, group.Key)
			}
			for k, entry := range group.Entries {
				if err := validateProjectionEntry(entry, idx); err != nil {
					return fmt.Errorf("preset %q group %q entries[%d]: %w", preset.Key, group.Key, k, err)
				}
			}
		}
	}
	for _, required := range []string{"activity_all", "map_panel_gps"} {
		if _, ok := presetKeys[required]; !ok {
			return fmt.Errorf("missing required preset %q", required)
		}
	}
	return nil
}

func validateProjectionEntry(entry ActivityFilterEntry, idx projectionIndexes) error {
	hasCategory := entry.CategoryKey != ""
	hasSport := entry.SportType != ""
	if hasCategory == hasSport {
		return fmt.Errorf("exactly one of category_key or sport_type is required")
	}
	if entry.SubType != "" && !hasSport {
		return fmt.Errorf("sub_type requires sport_type")
	}
	if entry.SportOrder != nil && !hasCategory {
		return fmt.Errorf("sport_order requires category_key")
	}
	if entry.LabelMode != "" && entry.LabelMode != "compact" && entry.LabelMode != "with_parent" {
		return fmt.Errorf("invalid label_mode %q", entry.LabelMode)
	}
	if entry.Labels != nil && entry.Labels["de"] == "" {
		return fmt.Errorf("labels requires de")
	}
	if hasCategory {
		sports, ok := idx.byCategory[entry.CategoryKey]
		if !ok || len(sports) == 0 {
			return fmt.Errorf("unknown category_key %q", entry.CategoryKey)
		}
		seenOrder := map[string]struct{}{}
		for _, sportType := range entry.SportOrder {
			sport, ok := idx.sports[sportType]
			if !ok {
				return fmt.Errorf("sport_order references unknown sport_type %q", sportType)
			}
			if sport.CategoryKey != entry.CategoryKey {
				return fmt.Errorf("sport_order sport_type %q is not in category %q", sportType, entry.CategoryKey)
			}
			if _, ok := seenOrder[sportType]; ok {
				return fmt.Errorf("sport_order duplicates sport_type %q", sportType)
			}
			seenOrder[sportType] = struct{}{}
		}
		return nil
	}
	sport, ok := idx.sports[entry.SportType]
	if !ok {
		return fmt.Errorf("unknown sport_type %q", entry.SportType)
	}
	if entry.SubType != "" && !sportHasSubType(sport, entry.SubType) {
		return fmt.Errorf("sport %q has no sub_type %q", entry.SportType, entry.SubType)
	}
	return nil
}

func expandProjectionEntry(entry ActivityFilterEntry, idx projectionIndexes) ([]pendingProjectionLeaf, error) {
	if entry.CategoryKey != "" {
		sports := orderedSportsForEntry(entry, idx.byCategory[entry.CategoryKey])
		leaves := []pendingProjectionLeaf{}
		for _, sport := range sports {
			leaves = append(leaves, leavesForSport(sport, entry, true)...)
		}
		return leaves, nil
	}
	sport := idx.sports[entry.SportType]
	if entry.SubType != "" {
		subType := mustFindSubType(sport, entry.SubType)
		return []pendingProjectionLeaf{leafForSubType(sport, subType, entry)}, nil
	}
	return leavesForSport(sport, entry, false), nil
}

func orderedSportsForEntry(entry ActivityFilterEntry, sports []Sport) []Sport {
	if len(entry.SportOrder) == 0 {
		out := append([]Sport(nil), sports...)
		return out
	}
	byType := make(map[string]Sport, len(sports))
	for _, sport := range sports {
		byType[sport.SportType] = sport
	}
	out := make([]Sport, 0, len(sports))
	seen := map[string]struct{}{}
	for _, sportType := range entry.SportOrder {
		out = append(out, byType[sportType])
		seen[sportType] = struct{}{}
	}
	for _, sport := range sports {
		if _, ok := seen[sport.SportType]; !ok {
			out = append(out, sport)
		}
	}
	return out
}

func leavesForSport(sport Sport, entry ActivityFilterEntry, throughCategory bool) []pendingProjectionLeaf {
	leaves := []pendingProjectionLeaf{}
	parentIsPromoted := isPromotedProjectionParent(sport.SportType)
	parentIsBroadFallback := sport.Lifecycle == "broad_fallback"
	includeParent := false
	if throughCategory {
		includeParent = !parentIsPromoted && !parentIsBroadFallback && sport.Selectable != nil && *sport.Selectable
	} else {
		includeParent = (sport.Selectable != nil && *sport.Selectable) || parentIsBroadFallback
	}
	if includeParent {
		leaves = append(leaves, pendingProjectionLeaf{
			SportType:       sport.SportType,
			ParentLabelDE:   sport.Labels["de"],
			CompactLabelDE:  sport.Labels["de"],
			ExplicitLabelDE: entry.Labels["de"],
		})
	}
	for _, subType := range sport.SubTypes {
		leaves = append(leaves, leafForSubType(sport, subType, entry))
	}
	return leaves
}

func leafForSubType(sport Sport, subType SubType, entry ActivityFilterEntry) pendingProjectionLeaf {
	forceParent := entry.LabelMode == "with_parent"
	if _, ok := parentContextSubTypes[subType.SubType]; ok {
		forceParent = true
	}
	compact := subType.Labels["de"]
	return pendingProjectionLeaf{
		SportType:         sport.SportType,
		SubType:           subType.SubType,
		ParentLabelDE:     sport.Labels["de"],
		CompactLabelDE:    compact,
		WithParentLabelDE: sport.Labels["de"] + ": " + compact,
		ExplicitLabelDE:   entry.Labels["de"],
		ForceParentLabel:  forceParent,
	}
}

func disambiguateProjectionLeaves(pending []pendingProjectionLeaf) []ExpandedActivityFilterLeaf {
	out := make([]ExpandedActivityFilterLeaf, 0, len(pending))
	for _, leaf := range pending {
		out = append(out, ExpandedActivityFilterLeaf{
			SportType: leaf.SportType,
			SubType:   leaf.SubType,
			Label:     LocaleMap{"de": projectionLeafLabel(leaf)},
		})
	}
	return out
}

func projectionLeafLabel(leaf pendingProjectionLeaf) string {
	if leaf.ExplicitLabelDE != "" {
		return leaf.ExplicitLabelDE
	}
	if leaf.SubType != "" && leaf.ForceParentLabel {
		return leaf.WithParentLabelDE
	}
	return leaf.CompactLabelDE
}

func validateProjectionGroupLabels(presetKey string, group ExpandedActivityFilterGroup) error {
	labels := map[string]string{}
	for _, leaf := range group.Leaves {
		label := leaf.Label["de"]
		if existing, ok := labels[label]; ok {
			return fmt.Errorf("preset %q group %q: duplicate leaf label %q for %s and %s", presetKey, group.Key, label, existing, projectionTupleKey(leaf.SportType, leaf.SubType))
		}
		labels[label] = projectionTupleKey(leaf.SportType, leaf.SubType)
	}
	return nil
}

func resolveGroupLabel(group ActivityFilterGroup, idx projectionIndexes) (LocaleMap, error) {
	if group.Labels["de"] != "" {
		return cloneLocaleMap(group.Labels), nil
	}
	if !strings.HasPrefix(group.LabelRef, "category:") {
		return nil, fmt.Errorf("unsupported label_ref %q", group.LabelRef)
	}
	categoryKey := strings.TrimPrefix(group.LabelRef, "category:")
	category, ok := idx.categories[categoryKey]
	if !ok {
		return nil, fmt.Errorf("label_ref references unknown category %q", categoryKey)
	}
	return cloneLocaleMap(category.Labels), nil
}

func newProjectionIndexes(taxonomy Taxonomy) projectionIndexes {
	idx := projectionIndexes{
		categories: make(map[string]Category, len(taxonomy.Categories)),
		sports:     make(map[string]Sport, len(taxonomy.Sports)),
		byCategory: map[string][]Sport{},
	}
	for _, category := range taxonomy.Categories {
		idx.categories[category.Key] = category
	}
	for _, sport := range taxonomy.Sports {
		idx.sports[sport.SportType] = sport
		idx.byCategory[sport.CategoryKey] = append(idx.byCategory[sport.CategoryKey], sport)
	}
	return idx
}

func cloneLocaleMap(in LocaleMap) LocaleMap {
	out := make(LocaleMap, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func isPromotedProjectionParent(sportType string) bool {
	_, ok := promotedProjectionParents[sportType]
	return ok
}

func sportHasSubType(sport Sport, subType string) bool {
	for _, candidate := range sport.SubTypes {
		if candidate.SubType == subType {
			return true
		}
	}
	return false
}

func mustFindSubType(sport Sport, subType string) SubType {
	for _, candidate := range sport.SubTypes {
		if candidate.SubType == subType {
			return candidate
		}
	}
	panic("validated subtype missing")
}

func projectionTupleKey(sportType, subType string) string {
	if subType == "" {
		return sportType
	}
	return sportType + ":" + subType
}
