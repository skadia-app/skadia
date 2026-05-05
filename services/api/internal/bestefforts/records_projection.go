package bestefforts

import "github.com/skadia-app/skadia/services/api/internal/sport"

type RecordGroup struct {
	Key          string
	LabelDE      string
	MainCategory string
	Color        string
	Badge        string
	Categories   []Category
}

type RecordMainCategory struct {
	Key          string
	LabelDE      string
	Color        string
	RecordGroups []string
	SubFilters   []RecordSubFilter
	PowerCurve   *RecordPowerCurve
}

type RecordSubFilter struct {
	Label        string
	RecordGroups []string
}

type RecordPowerCurve struct {
	Label        string
	RecordGroups []string
}

type RecordMatchRule struct {
	RecordGroup     string
	Priority        int
	Sequence        int
	SportType       string
	SubTypes        []string
	ExcludeSubTypes []string
	IsIndoor        *bool
	IsVirtual       *bool
	IsElectric      *bool
}

func RecordGroups() []RecordGroup {
	return cloneRecordGroups(generatedRecordGroups)
}

func RecordGroupByKey(key string) (RecordGroup, bool) {
	for _, group := range generatedRecordGroups {
		if group.Key == key {
			return cloneRecordGroup(group), true
		}
	}
	return RecordGroup{}, false
}

func RecordMainCategories() []RecordMainCategory {
	return cloneRecordMainCategories(generatedRecordMainCategories)
}

func RecordGroupForActivity(sportType, subType string, isIndoor, isVirtual, isElectric bool) (string, bool) {
	return recordGroupForActivityWithModifierPresence(sportType, subType, isIndoor, isVirtual, isElectric, false, false, false)
}

func recordGroupForActivityWithModifierPresence(sportType, subType string, isIndoor, isVirtual, isElectric bool, explicitIndoor, explicitVirtual, explicitElectric bool) (string, bool) {
	isIndoor, isVirtual, isElectric = resolveRecordModifierDefaults(sportType, subType, isIndoor, isVirtual, isElectric, explicitIndoor, explicitVirtual, explicitElectric)
	for _, rule := range generatedRecordMatchRules {
		if recordRuleMatches(rule, sportType, subType, isIndoor, isVirtual, isElectric) {
			return rule.RecordGroup, true
		}
	}
	return "", false
}

func resolveRecordModifierDefaults(sportType, subType string, isIndoor, isVirtual, isElectric bool, explicitIndoor, explicitVirtual, explicitElectric bool) (bool, bool, bool) {
	defaults := sport.ModifierDefaultsForTuple(sportType, subType)
	if !explicitIndoor && defaults.IsIndoor != nil {
		isIndoor = *defaults.IsIndoor
	}
	return isIndoor, isVirtual, isElectric
}

func recordRuleMatches(rule RecordMatchRule, sportType, subType string, isIndoor, isVirtual, isElectric bool) bool {
	if rule.SportType != sportType {
		return false
	}
	if len(rule.SubTypes) > 0 && !recordSubTypeListMatches(rule.SubTypes, subType) {
		return false
	}
	if recordSubTypeListMatches(rule.ExcludeSubTypes, subType) {
		return false
	}
	if rule.IsIndoor != nil && *rule.IsIndoor != isIndoor {
		return false
	}
	if rule.IsVirtual != nil && *rule.IsVirtual != isVirtual {
		return false
	}
	if rule.IsElectric != nil && *rule.IsElectric != isElectric {
		return false
	}
	return true
}

func recordSubTypeListMatches(values []string, subType string) bool {
	subType = sport.CanonicalSubTypeID(subType)
	for _, value := range values {
		if sport.CanonicalSubTypeID(value) == subType {
			return true
		}
	}
	return false
}

func cloneRecordGroups(in []RecordGroup) []RecordGroup {
	out := make([]RecordGroup, len(in))
	for i := range in {
		out[i] = cloneRecordGroup(in[i])
	}
	return out
}

func cloneRecordGroup(in RecordGroup) RecordGroup {
	out := in
	out.Categories = append([]Category(nil), in.Categories...)
	return out
}

func cloneRecordMainCategories(in []RecordMainCategory) []RecordMainCategory {
	out := make([]RecordMainCategory, len(in))
	for i := range in {
		out[i] = cloneRecordMainCategory(in[i])
	}
	return out
}

func cloneRecordMainCategory(in RecordMainCategory) RecordMainCategory {
	out := in
	out.RecordGroups = append([]string(nil), in.RecordGroups...)
	out.SubFilters = make([]RecordSubFilter, len(in.SubFilters))
	for i := range in.SubFilters {
		out.SubFilters[i] = in.SubFilters[i]
		out.SubFilters[i].RecordGroups = append([]string(nil), in.SubFilters[i].RecordGroups...)
	}
	if in.PowerCurve != nil {
		out.PowerCurve = &RecordPowerCurve{
			Label:        in.PowerCurve.Label,
			RecordGroups: append([]string(nil), in.PowerCurve.RecordGroups...),
		}
	}
	return out
}
