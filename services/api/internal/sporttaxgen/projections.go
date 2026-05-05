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

type SegmentProjection struct {
	Domains      []SegmentProjectionDomain      `yaml:"domains"`
	SegmentTypes []SegmentProjectionSegmentType `yaml:"segment_types"`
	Eligible     []SegmentProjectionEligible    `yaml:"eligible"`
}

type SegmentProjectionDomain struct {
	Key    string    `yaml:"key"`
	Labels LocaleMap `yaml:"labels"`
	Status string    `yaml:"status"`
}

type SegmentProjectionSegmentType struct {
	Key               string    `yaml:"key"`
	Labels            LocaleMap `yaml:"labels"`
	ActiveDomains     []string  `yaml:"active_domains"`
	CompatibilityRole string    `yaml:"compatibility_role"`
}

type SegmentProjectionEligible struct {
	SportType       string   `yaml:"sport_type"`
	SubTypes        []string `yaml:"sub_types"`
	SegmentTypes    []string `yaml:"segment_types"`
	Domains         []string `yaml:"domains"`
	SportGroup      string   `yaml:"sport_group"`
	EligibilityRole string   `yaml:"eligibility_role"`
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

type ExpandedSegmentProjection struct {
	Domains      []ExpandedSegmentProjectionDomain
	SegmentTypes []ExpandedSegmentProjectionSegmentType
	Entries      []ExpandedSegmentProjectionEntry
}

type ExpandedSegmentProjectionDomain struct {
	Key    string
	Label  LocaleMap
	Status string
}

type ExpandedSegmentProjectionSegmentType struct {
	Key               string
	Label             LocaleMap
	ActiveDomains     []string
	CompatibilityRole string
}

type ExpandedSegmentProjectionEntry struct {
	SegmentType string
	Domain      string
	SportType   string
	SubType     string
	SportGroup  string
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

func LoadSegmentProjection(r io.Reader, taxonomy Taxonomy) (SegmentProjection, error) {
	var projection SegmentProjection
	dec := yaml.NewDecoder(r)
	dec.SetStrict(true)
	if err := dec.Decode(&projection); err != nil {
		return SegmentProjection{}, err
	}
	if err := validateSegmentProjection(projection, taxonomy); err != nil {
		return SegmentProjection{}, err
	}
	return projection, nil
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

func ExpandSegmentProjection(taxonomy Taxonomy, projection SegmentProjection) (ExpandedSegmentProjection, error) {
	if err := validateSegmentProjection(projection, taxonomy); err != nil {
		return ExpandedSegmentProjection{}, err
	}
	out := ExpandedSegmentProjection{
		Domains:      make([]ExpandedSegmentProjectionDomain, 0, len(projection.Domains)),
		SegmentTypes: make([]ExpandedSegmentProjectionSegmentType, 0, len(projection.SegmentTypes)),
	}
	for _, domain := range projection.Domains {
		out.Domains = append(out.Domains, ExpandedSegmentProjectionDomain{
			Key:    domain.Key,
			Label:  cloneLocaleMap(domain.Labels),
			Status: domain.Status,
		})
	}
	for _, segmentType := range projection.SegmentTypes {
		out.SegmentTypes = append(out.SegmentTypes, ExpandedSegmentProjectionSegmentType{
			Key:               segmentType.Key,
			Label:             cloneLocaleMap(segmentType.Labels),
			ActiveDomains:     append([]string(nil), segmentType.ActiveDomains...),
			CompatibilityRole: segmentType.CompatibilityRole,
		})
	}
	out.Entries = expandSegmentProjectionEntries(projection)
	return out, nil
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

func validateSegmentProjection(projection SegmentProjection, taxonomy Taxonomy) error {
	idx := newProjectionIndexes(taxonomy)
	if len(projection.Domains) == 0 {
		return fmt.Errorf("missing domains")
	}
	if len(projection.SegmentTypes) == 0 {
		return fmt.Errorf("missing segment_types")
	}
	domains := map[string]SegmentProjectionDomain{}
	for i, domain := range projection.Domains {
		if domain.Key == "" {
			return fmt.Errorf("domains[%d]: missing key", i)
		}
		if _, ok := domains[domain.Key]; ok {
			return fmt.Errorf("duplicate domain key %q", domain.Key)
		}
		if domain.Labels["de"] == "" {
			return fmt.Errorf("domain %q: missing labels.de", domain.Key)
		}
		if domain.Status != "active" && domain.Status != "reserved" {
			return fmt.Errorf("domain %q: invalid status %q", domain.Key, domain.Status)
		}
		domains[domain.Key] = domain
	}
	segmentTypes := map[string]SegmentProjectionSegmentType{}
	for i, segmentType := range projection.SegmentTypes {
		if segmentType.Key == "" {
			return fmt.Errorf("segment_types[%d]: missing key", i)
		}
		if _, ok := segmentTypes[segmentType.Key]; ok {
			return fmt.Errorf("duplicate segment_type key %q", segmentType.Key)
		}
		if segmentType.Labels["de"] == "" {
			return fmt.Errorf("segment_type %q: missing labels.de", segmentType.Key)
		}
		if segmentType.CompatibilityRole != "legacy_segment_type" && segmentType.CompatibilityRole != "legacy_lift" {
			return fmt.Errorf("segment_type %q: invalid compatibility_role %q", segmentType.Key, segmentType.CompatibilityRole)
		}
		if len(segmentType.ActiveDomains) == 0 && segmentType.Key != "lift" {
			return fmt.Errorf("segment_type %q: missing active_domains", segmentType.Key)
		}
		if len(segmentType.ActiveDomains) > 0 && segmentType.Key == "lift" {
			return fmt.Errorf("segment_type %q: lift must not have active_domains", segmentType.Key)
		}
		for _, domainKey := range segmentType.ActiveDomains {
			domain, ok := domains[domainKey]
			if !ok {
				return fmt.Errorf("segment_type %q: active_domains references unknown domain %q", segmentType.Key, domainKey)
			}
			if domain.Status != "active" {
				return fmt.Errorf("segment_type %q: active_domains references non-active domain %q", segmentType.Key, domainKey)
			}
		}
		segmentTypes[segmentType.Key] = segmentType
	}
	if len(projection.Eligible) == 0 {
		return fmt.Errorf("missing eligible")
	}
	seenRows := map[string]struct{}{}
	activeTupleGroups := map[string]string{}
	for i, eligible := range projection.Eligible {
		if eligible.SportType == "" {
			return fmt.Errorf("eligible[%d]: missing sport_type", i)
		}
		sport, ok := idx.sports[eligible.SportType]
		if !ok {
			return fmt.Errorf("eligible[%d]: unknown sport_type %q", i, eligible.SportType)
		}
		if eligible.EligibilityRole != "" && eligible.EligibilityRole != "legacy_fallback_coverage" {
			return fmt.Errorf("eligible[%d]: invalid eligibility_role %q", i, eligible.EligibilityRole)
		}
		if eligible.EligibilityRole == "legacy_fallback_coverage" && !isAllowedSegmentFallbackSport(sport, eligible.SportGroup, eligible.EligibilityRole) {
			return fmt.Errorf("eligible[%d]: eligibility_role legacy_fallback_coverage requires matching broad fallback sport_group", i)
		}
		if (sport.Selectable == nil || !*sport.Selectable) && !isAllowedSegmentFallbackSport(sport, eligible.SportGroup, eligible.EligibilityRole) {
			return fmt.Errorf("eligible[%d]: sport_type %q is not selectable", i, eligible.SportType)
		}
		if sport.Traits.ActivityForm == "system" {
			return fmt.Errorf("eligible[%d]: sport_type %q has system activity_form", i, eligible.SportType)
		}
		if len(eligible.SegmentTypes) == 0 {
			return fmt.Errorf("eligible[%d]: missing segment_types", i)
		}
		if len(eligible.Domains) == 0 {
			return fmt.Errorf("eligible[%d]: missing domains", i)
		}
		if !isValidSegmentGroup(eligible.SportGroup) {
			return fmt.Errorf("eligible[%d]: invalid sport_group %q", i, eligible.SportGroup)
		}
		for _, subType := range eligible.SubTypes {
			if !sportHasSubType(sport, subType) {
				return fmt.Errorf("eligible[%d]: sport %q has no sub_type %q", i, eligible.SportType, subType)
			}
		}
		for _, segmentTypeKey := range eligible.SegmentTypes {
			segmentType, ok := segmentTypes[segmentTypeKey]
			if !ok {
				return fmt.Errorf("eligible[%d]: unknown segment_type %q", i, segmentTypeKey)
			}
			for _, domainKey := range eligible.Domains {
				domain, ok := domains[domainKey]
				if !ok {
					return fmt.Errorf("eligible[%d]: unknown domain %q", i, domainKey)
				}
				activeForSegmentType := domain.Status == "active" && containsString(segmentType.ActiveDomains, domainKey)
				if domain.Status == "active" && !activeForSegmentType {
					return fmt.Errorf("eligible[%d]: domain %q is not active for segment_type %q", i, domainKey, segmentTypeKey)
				}
				for _, subType := range segmentProjectionSubTypes(eligible) {
					rowKey := strings.Join([]string{segmentTypeKey, domainKey, eligible.SportType, subType}, "\x00")
					if _, ok := seenRows[rowKey]; ok {
						return fmt.Errorf("eligible[%d]: duplicate tuple %q for segment_type %q and domain %q", i, projectionTupleKey(eligible.SportType, subType), segmentTypeKey, domainKey)
					}
					seenRows[rowKey] = struct{}{}
					if !activeForSegmentType {
						continue
					}
					tupleKey := projectionTupleKey(eligible.SportType, subType)
					if existing, ok := activeTupleGroups[tupleKey]; ok && existing != eligible.SportGroup {
						return fmt.Errorf("eligible[%d]: tuple %q uses sport_group %q and %q", i, tupleKey, existing, eligible.SportGroup)
					}
					activeTupleGroups[tupleKey] = eligible.SportGroup
				}
			}
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

func expandSegmentProjectionEntries(projection SegmentProjection) []ExpandedSegmentProjectionEntry {
	entries := []ExpandedSegmentProjectionEntry{}
	for _, eligible := range projection.Eligible {
		for _, segmentType := range eligible.SegmentTypes {
			for _, domain := range eligible.Domains {
				for _, subType := range segmentProjectionSubTypes(eligible) {
					entries = append(entries, ExpandedSegmentProjectionEntry{
						SegmentType: segmentType,
						Domain:      domain,
						SportType:   eligible.SportType,
						SubType:     subType,
						SportGroup:  eligible.SportGroup,
					})
				}
			}
		}
	}
	return entries
}

func segmentProjectionSubTypes(entry SegmentProjectionEligible) []string {
	if len(entry.SubTypes) == 0 {
		return []string{""}
	}
	return entry.SubTypes
}

func isAllowedSegmentFallbackSport(sport Sport, sportGroup, eligibilityRole string) bool {
	return eligibilityRole == "legacy_fallback_coverage" &&
		sport.Lifecycle == "broad_fallback" &&
		sport.Traits.ActivityForm == "fallback" &&
		sport.SegmentGroup != "generic" &&
		sport.SegmentGroup == sportGroup
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
		includeParent = !parentIsPromoted && !parentIsBroadFallback && ((sport.Selectable != nil && *sport.Selectable) || sport.Traits.ActivityForm == "system")
	} else {
		includeParent = (sport.Selectable != nil && *sport.Selectable) || parentIsBroadFallback || sport.Traits.ActivityForm == "transition"
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
